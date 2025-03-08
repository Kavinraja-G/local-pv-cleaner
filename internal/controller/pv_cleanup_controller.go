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

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

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
	Scheme            *runtime.Scheme
	DryRun            bool
	NodeSelectorKeys  []string
	StorageClassNames []string
	NodeLabelFilters  map[string]string
}

var (
	orphanedPVsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "local_pv_cleaner_orphaned_pvs_total",
			Help: "Total number of Orphaned PVs detected",
		},
		[]string{"storage_class"},
	)
	deletedPVsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "local_pv_cleaner_deleted_pvs_total",
			Help: "Total number of Orphaned PVs deleted",
		},
		[]string{"storage_class"},
	)
)

func init() {
	metrics.Registry.MustRegister(orphanedPVsTotal, deletedPVsTotal)
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

// cleanupOrphanedPVs list based on storageClass filters & deletes the PVs for which the hosts/nodes are no longer available
func (r *PVCleanupController) cleanupOrphanedPVs(ctx context.Context, deletedNodeName string) error {
	logger := log.FromContext(ctx)

	// List PVs based on StorageClassName
	allPVs, err := r.listAllPVs(ctx)
	if err != nil {
		return err
	}

	// Filter PVs by storage classes
	filteredPVs := r.filterPVByStorageClass(allPVs)

	// Iterate over PVs and delete orphaned ones which hosts are no longer available
	var deletedVolumes []string
	for _, pv := range filteredPVs {
		if pv.Spec.NodeAffinity != nil && pv.Spec.PersistentVolumeReclaimPolicy == corev1.PersistentVolumeReclaimRetain {
			for _, term := range pv.Spec.NodeAffinity.Required.NodeSelectorTerms {
				for _, exp := range term.MatchExpressions {
					if slices.Contains(r.NodeSelectorKeys, exp.Key) && len(exp.Values) > 0 {
						nodeName := exp.Values[0]

						if nodeName == deletedNodeName {
							logger.Info("Found orphaned volume", "pv", pv.Name, "node", nodeName)
							orphanedPVsTotal.WithLabelValues(pv.Spec.StorageClassName).Inc()
							if !r.DryRun {
								logger.Info("Deleting orphaned volume", "pv", pv.Name, "node", nodeName)
								if err := r.Client.Delete(ctx, &pv); err != nil {
									return err
								}
								deletedPVsTotal.WithLabelValues(pv.Spec.StorageClassName).Inc()
								deletedVolumes = append(deletedVolumes, pv.Name)
							}
						} else {
							logger.Info("PV is still attached to a node", "pv", pv.Name, "node", nodeName)
						}
					}
				}
			}
		}
	}

	logger.Info("Total deleted orphaned PVs", "total", len(deletedVolumes), "deleted_pvs", deletedVolumes)

	return nil
}

// listAllPVs lists the Persistent volumes in the cluster
func (r *PVCleanupController) listAllPVs(ctx context.Context) ([]corev1.PersistentVolume, error) {
	var allPVs []corev1.PersistentVolume
	pvList := &corev1.PersistentVolumeList{}

	opts := &client.ListOptions{Limit: PVListLimit}

	for {
		if err := r.Client.List(ctx, pvList, opts); err != nil {
			return nil, err
		}
		allPVs = append(allPVs, pvList.Items...)

		if len(pvList.Items) < PVListLimit || opts.Continue == "" {
			break
		}
		opts.Continue = pvList.Continue
	}

	return allPVs, nil
}

// filterPVByStorageClass filters the list of PVs based on storage class names
func (r *PVCleanupController) filterPVByStorageClass(pvList []corev1.PersistentVolume) []corev1.PersistentVolume {
	if len(r.StorageClassNames) == 0 {
		return pvList
	}

	var filteredPVs []corev1.PersistentVolume
	for _, pv := range pvList {
		if slices.Contains(r.StorageClassNames, pv.Spec.StorageClassName) {
			filteredPVs = append(filteredPVs, pv)
		}
	}

	return filteredPVs
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
			DeleteFunc: func(deleteEvent event.DeleteEvent) bool {
				nodeLabels := deleteEvent.Object.GetLabels()

				// Do not reconcile if the deleted node doesn't have the required label filters
				for key, expectedValue := range r.NodeLabelFilters {
					if val, exists := nodeLabels[key]; !exists || val != expectedValue {
						return false
					}
				}

				return true
			},
		}).
		Named("local-pv-cleaner").
		Complete(r)
}
