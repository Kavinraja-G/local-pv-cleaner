package controllers

import (
	"context"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
)

var nodeSelectorKey = "topology.topolvm.io/node"

func TestPVCleanupController_cleanupOrphanedPVs(t *testing.T) {
	s := scheme.Scheme
	_ = corev1.AddToScheme(s)

	// Define reusable test PVs and Nodes
	orphanedPV := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "orphaned-pv"},
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimRetain,
			NodeAffinity: &corev1.VolumeNodeAffinity{
				Required: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{
						{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:    nodeSelectorKey,
									Values: []string{"non-existing-node"},
								},
							},
						},
					},
				},
			},
		},
	}

	validNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "existing-node"},
	}

	validPV := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "valid-pv"},
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimRetain,
			NodeAffinity: &corev1.VolumeNodeAffinity{
				Required: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{
						{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:    nodeSelectorKey,
									Values: []string{"existing-node"},
								},
							},
						},
					},
				},
			},
		},
	}

	type args struct {
		DryRun           bool
		NodeSelectorKeys []string
	}

	// Define test-cases
	var tests = []struct {
		name            string
		objects         []client.Object
		args            args
		deletedNodeName string
		wantDeleted     []string
		wantRemain      []string
		wantErr         bool
	}{
		{
			name:            "Orphaned PV should be deleted",
			objects:         []client.Object{orphanedPV},
			wantDeleted:     []string{"orphaned-pv"},
			wantRemain:      []string{},
			deletedNodeName: "non-existing-node",
			wantErr:         false,
			args: args{
				DryRun:           false,
				NodeSelectorKeys: []string{nodeSelectorKey},
			},
		},
		{
			name:            "Valid PV should remain",
			objects:         []client.Object{validNode, validPV},
			wantDeleted:     []string{},
			wantRemain:      []string{"valid-pv"},
			deletedNodeName: "some-other-node",
			wantErr:         false,
			args: args{
				DryRun:           false,
				NodeSelectorKeys: []string{nodeSelectorKey},
			},
		},
		{
			name:            "No PVs exist",
			objects:         []client.Object{},
			wantDeleted:     []string{},
			wantRemain:      []string{},
			deletedNodeName: "",
			wantErr:         false,
			args: args{
				DryRun:           false,
				NodeSelectorKeys: []string{nodeSelectorKey},
			},
		},
	}

	// Run tests
	for _, tt := range tests {
		ctx := context.Background()
		fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(tt.objects...).Build()

		t.Run(tt.name, func(t *testing.T) {
			r := &PVCleanupController{
				Client:           fakeClient,
				DryRun:           tt.args.DryRun,
				NodeSelectorKeys: tt.args.NodeSelectorKeys,
			}

			// TODO: Need to write better tests as we are changing the core logic now
			if err := r.cleanupOrphanedPVs(ctx, tt.deletedNodeName); (err != nil) != tt.wantErr {
				t.Errorf("cleanupOrphanedPVs() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Assert deleted PVs
			for _, pvName := range tt.wantDeleted {
				deletedPV := &corev1.PersistentVolume{}
				err := fakeClient.Get(ctx, client.ObjectKey{Name: pvName}, deletedPV)
				assert.Error(t, err, "Expected PV %s to be deleted", pvName)
			}

			// Assert remaining PVs
			for _, pvName := range tt.wantRemain {
				remainingPV := &corev1.PersistentVolume{}
				err := fakeClient.Get(ctx, client.ObjectKey{Name: pvName}, remainingPV)
				assert.NoError(t, err, "Expected PV %s to remain", pvName)
			}
		})
	}
}
