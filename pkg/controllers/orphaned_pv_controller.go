package controllers

import (
	"context"
	"slices"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
)

// Reconcile function watches for node events and triggers the cleanupOrphanedPVs
func (r *PVCleanupController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := klog.FromContext(ctx)
	logger.Info("Reconciling for orphaned PVs...")

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

// cleanupOrphanedPVs finds and deletes the PVs to which the hosts/nodes are no longer available
func (r *PVCleanupController) cleanupOrphanedPVs(ctx context.Context, deletedNodeName string) error {
	logger := klog.FromContext(ctx)

	// List PVs
	var pvList corev1.PersistentVolumeList
	if err := r.Client.List(ctx, &pvList); err != nil {
		logger.Error(err, "Failed to list PVs")
		return err
	}

	// Iterate over PVs and delete orphaned ones which hosts are no longer available
	var deletedVolumes = 0
	for _, pv := range pvList.Items {
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
