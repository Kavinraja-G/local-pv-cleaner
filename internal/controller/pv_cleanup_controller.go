/*
Copyright 2025 Kavinraja-G.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"

	"k8s.io/utils/strings/slices"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// PVCleanupController reconciles a Node object
type PVCleanupController struct {
	client.Client
	DryRun            bool
	NodeSelectorKeys  []string
	StorageClassNames []string
	Scheme            *runtime.Scheme
}

// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=persistentvolumes,verbs=get;list;watch;delete

func (r *PVCleanupController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Check if the request is for a Node
	var node corev1.Node
	if err := r.Client.Get(ctx, req.NamespacedName, &node); err != nil {
		logger.Info("Node not found, checking for orphaned PVs...")
		if err := r.cleanupOrphanedPVs(ctx, node.Name); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *PVCleanupController) listPVsByStorageClass(ctx context.Context) ([]corev1.PersistentVolume, error) {
	var pvList corev1.PersistentVolumeList

	// Fetch all PVs
	if err := r.Client.List(ctx, &pvList); err != nil {
		return nil, err
	}

	// Filter PVs by storageClassName
	var filteredPVs []corev1.PersistentVolume
	for _, pv := range pvList.Items {
		if slices.Contains(r.StorageClassNames, pv.Spec.StorageClassName) {
			filteredPVs = append(filteredPVs, pv)
		}
	}

	return filteredPVs, nil
}

// cleanupOrphanedPVs finds and deletes the PVs to which the hosts/nodes are no longer available
func (r *PVCleanupController) cleanupOrphanedPVs(ctx context.Context, deletedNodeName string) error {
	logger := log.FromContext(ctx)

	// List PVs based on StorageClassName
	filteredPVs, err := r.listPVsByStorageClass(ctx)
	if err != nil {
		return err
	}

	// Iterate over PVs and delete orphaned ones which hosts are no longer available
	var deletedVolumes = 0
	for _, pv := range filteredPVs {
		if pv.Spec.NodeAffinity != nil && pv.Spec.PersistentVolumeReclaimPolicy == corev1.PersistentVolumeReclaimRetain {
			for _, term := range pv.Spec.NodeAffinity.Required.NodeSelectorTerms {
				for _, exp := range term.MatchExpressions {
					if slices.Contains(r.NodeSelectorKeys, exp.Key) && len(exp.Values) > 0 {
						nodeName := exp.Values[0]

						if nodeName == deletedNodeName {
							logger.Info("Found orphaned volume:", "pv", pv.Name, "node", nodeName)
							if !r.DryRun {
								logger.Info("Deleting orphaned volume:", "pv", pv.Name, "node", nodeName)
								if err := r.Client.Delete(ctx, &pv); err != nil {
									return err
								}
								deletedVolumes++
							}
						} else {
							logger.Info("PV is already attached to a node", "pv", pv.Name, "node", nodeName)
						}
					}
				}
			}
		}
	}

	logger.Info("Total Deleted orphaned PVs", "total", deletedVolumes)

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PVCleanupController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Node{}).
		WithEventFilter(predicate.Funcs{
			GenericFunc: func(genericEvent event.GenericEvent) bool {
				return false
			},
			UpdateFunc: func(updateEvent event.UpdateEvent) bool {
				return false
			},
			CreateFunc: func(createEvent event.CreateEvent) bool {
				return false
			},
		}).
		Named("local-pv-cleaner").
		Complete(r)
}
