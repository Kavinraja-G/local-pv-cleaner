package controllers

import (
	"context"
	"slices"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *PVCleanupController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := klog.FromContext(ctx)
	logger.Info("Reconciling for orphaned PVs...")

	// Check if the request is for a Node
	var node corev1.Node
	if err := r.Client.Get(ctx, req.NamespacedName, &node); err != nil {
		logger.Info("Node not found, checking for orphaned PVs...")
		if err := r.cleanupOrphanedPVs(ctx); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *PVCleanupController) PeriodicPVCleanup(ctx context.Context) error {
	logger := klog.FromContext(ctx)
	ticker := time.NewTicker(r.PeriodicCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("Stopping periodic cleanup")
			return nil
		case <-ticker.C:
			logger.Info("Running periodic orphaned PV cleanup")
			if err := r.cleanupOrphanedPVs(ctx); err != nil {
				return err
			}
		}
	}
}

func (r *PVCleanupController) cleanupOrphanedPVs(ctx context.Context) error {
	logger := klog.FromContext(ctx)

	// List all PVs
	var pvList corev1.PersistentVolumeList
	if err := r.Client.List(ctx, &pvList); err != nil {
		logger.Error(err, "Failed to list PVs")
		return err
	}

	// List all Nodes
	var nodeList corev1.NodeList
	if err := r.Client.List(ctx, &nodeList); err != nil {
		logger.Error(err, "Failed to list nodes")
		return err
	}

	existingNodes := make(map[string]bool)
	for _, node := range nodeList.Items {
		existingNodes[node.Name] = true
	}

	var deletedVolumes int = 0
	// Iterate over PVs and delete orphaned ones which hosts are no longer available
	for _, pv := range pvList.Items {
		if pv.Spec.NodeAffinity != nil && pv.Spec.PersistentVolumeReclaimPolicy == corev1.PersistentVolumeReclaimRetain {
			for _, term := range pv.Spec.NodeAffinity.Required.NodeSelectorTerms {
				for _, exp := range term.MatchExpressions {
					if slices.Contains(r.NodeSelectorKeys, exp.Key) && len(exp.Values) > 0 {
						nodeName := exp.Values[0]
						if !existingNodes[nodeName] {
							logger.Info("Found orphaned volume:", "pv", pv.Name, "node", nodeName)
							if !r.DryRun {
								logger.Info("Deleting orphaned volume:", "pv", pv.Name, "node", nodeName)
								err := r.Client.Delete(ctx, &pv)
								if err != nil {
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
