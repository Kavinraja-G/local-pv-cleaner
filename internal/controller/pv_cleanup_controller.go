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
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/utils/strings/slices"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// PVCleanupController reconciles a PersistentVolume object
type PVCleanupController struct {
	client.Client
	Scheme            *runtime.Scheme
	DryRun            bool
	NodeSelectorKeys  []string
	StorageClassNames []string
	RequeueDuration   time.Duration
}

var (
	deletedPVsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "local_pv_cleaner_deleted_pvs_total",
			Help: "Total number of Orphaned PVs deleted",
		},
		[]string{"storage_class"},
	)
)

func init() {
	metrics.Registry.MustRegister(deletedPVsTotal)
}

// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=persistentvolumes,verbs=get;list;watch;delete

func (r *PVCleanupController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var pv corev1.PersistentVolume
	if err := r.Client.Get(ctx, req.NamespacedName, &pv); err != nil {
		logger.Error(err, "PersistentVolume not found", "pv", req.Name)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// skip if reclaim policy is Retain
	if pv.Spec.PersistentVolumeReclaimPolicy != corev1.PersistentVolumeReclaimRetain {
		logger.V(1).Info("Skipping PV with reclaim policy not Retain", "policy", pv.Spec.PersistentVolumeReclaimPolicy)
		return ctrl.Result{}, nil
	}

	// skip if storageClass is not in the user filters
	if len(r.StorageClassNames) > 0 && !slices.Contains(r.StorageClassNames, pv.Spec.StorageClassName) {
		logger.V(1).Info("Skipping PV due to unmatched StorageClass", "storageClass", pv.Spec.StorageClassName)
		return ctrl.Result{}, nil
	}

	nodeName := getNodeNameFromAffinity(pv.Spec.NodeAffinity, r.NodeSelectorKeys)
	if nodeName == "" {
		logger.V(1).Info("No matching NodeAffinity found, skipping")
		return ctrl.Result{}, nil
	}

	var node corev1.Node
	err := r.Client.Get(ctx, client.ObjectKey{Name: nodeName}, &node)
	if err != nil {
		// node doesn't exist, delete PV
		logger.V(1).Info("Node not found for PV, deleting PV", "pv", pv.Name, "node", nodeName)
		if delErr := r.deleteOrphanedPV(ctx, pv); delErr != nil {
			logger.Error(err, "Failed to delete orphaned PV", "pv", pv.Name, "node", nodeName)
			return ctrl.Result{}, delErr
		}

		return ctrl.Result{}, nil
	}

	// node exists, requeue after X minutes
	return ctrl.Result{RequeueAfter: r.RequeueDuration}, nil
}

// getNodeNameFromAffinity gets the nodeName based on the given nodeSelector keys
func getNodeNameFromAffinity(affinity *corev1.VolumeNodeAffinity, nodeSelectorKeys []string) string {
	if affinity == nil {
		return ""
	}

	for _, term := range affinity.Required.NodeSelectorTerms {
		for _, exp := range term.MatchExpressions {
			if slices.Contains(nodeSelectorKeys, exp.Key) && len(exp.Values) > 0 {
				return exp.Values[0]
			}
		}
	}

	return ""
}

// deleteOrphanedPV deletes the given PersistentVolume if the DryRun is not enabled
func (r *PVCleanupController) deleteOrphanedPV(ctx context.Context, pv corev1.PersistentVolume) error {
	logger := log.FromContext(ctx)

	if !r.DryRun {
		if err := r.Client.Delete(ctx, &pv); err != nil {
			logger.Error(err, "Failed to delete PV", "pv", pv.Name)
			return err
		}
		logger.Info("Deleted orphaned PV", "pv", pv.Name)
		deletedPVsTotal.WithLabelValues(pv.Spec.StorageClassName).Inc()
	} else {
		logger.Info("DryRun enabled, skipping deletion of PV", "pv", pv.Name)
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PVCleanupController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.PersistentVolume{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
		}).
		WithEventFilter(predicate.Funcs{
			CreateFunc:  func(e event.CreateEvent) bool { return true },
			UpdateFunc:  func(e event.UpdateEvent) bool { return false },
			DeleteFunc:  func(e event.DeleteEvent) bool { return false },
			GenericFunc: func(e event.GenericEvent) bool { return false },
		}).
		Named("local-pv-cleaner").
		Complete(r)
}
