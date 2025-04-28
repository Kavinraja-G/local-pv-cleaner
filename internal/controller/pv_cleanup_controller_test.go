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
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	crFake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestPVCleanupController_getNodeNameFromAffinity(t *testing.T) {
	var tests = []struct {
		name             string
		affinity         *corev1.VolumeNodeAffinity
		nodeSelectorKeys []string
		expectedNodeName string
	}{
		{
			name:             "Nil Affinity",
			affinity:         nil,
			nodeSelectorKeys: []string{"key1"},
			expectedNodeName: "",
		},
		{
			name: "No matching NodeSelectorKey",
			affinity: &corev1.VolumeNodeAffinity{
				Required: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{
						{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:    "different-key",
									Values: []string{"node-01"},
								},
							},
						},
					},
				},
			},
			nodeSelectorKeys: []string{"key1"},
			expectedNodeName: "",
		},
		{
			name: "Matching NodeSelectorKey",
			affinity: &corev1.VolumeNodeAffinity{
				Required: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{
						{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:    "key1",
									Values: []string{"node-01"},
								},
							},
						},
					},
				},
			},
			nodeSelectorKeys: []string{"key1"},
			expectedNodeName: "node-01",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodeName := getNodeNameFromAffinity(tt.affinity, tt.nodeSelectorKeys)
			assert.Equal(t, tt.expectedNodeName, nodeName)
		})
	}
}

func TestPVCleanupController_deleteOrphanedPVs(t *testing.T) {
	s := scheme.Scheme
	_ = corev1.AddToScheme(s)

	type args struct {
		DryRun bool
	}

	// Define test-cases
	var tests = []struct {
		name       string
		objects    []client.Object
		args       args
		orphanedPV []corev1.PersistentVolume
		wantErr    bool
	}{
		{
			name: "Cleanup orphaned PVs",
			objects: []client.Object{
				&corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: "pv-1"}, Spec: corev1.PersistentVolumeSpec{
					StorageClassName: "foo",
				}},
				&corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: "pv-2"}, Spec: corev1.PersistentVolumeSpec{
					StorageClassName:              "bar",
					PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimRetain,
					NodeAffinity: &corev1.VolumeNodeAffinity{Required: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key:    "node-selector-key",
										Values: []string{"node-01"},
									},
								},
							},
						},
					}},
				}},
			},
			wantErr: false,
			orphanedPV: []corev1.PersistentVolume{
				{ObjectMeta: metav1.ObjectMeta{Name: "pv-1"}},
			},
		}, {
			name: "Orphaned PV not found",
			objects: []client.Object{
				&corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: "pv-1"}, Spec: corev1.PersistentVolumeSpec{
					StorageClassName:              "bar",
					PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimRetain,
				}},
				&corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: "pv-2"}, Spec: corev1.PersistentVolumeSpec{
					StorageClassName:              "bar",
					PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimRetain,
					NodeAffinity: &corev1.VolumeNodeAffinity{Required: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key:    "node-selector-key",
										Values: []string{"node-02"},
									},
								},
							},
						},
					}},
				}},
			},
			wantErr: true,
			orphanedPV: []corev1.PersistentVolume{
				{ObjectMeta: metav1.ObjectMeta{Name: "non-exist-pv"}},
			},
		},
	}

	// Run tests
	for _, tt := range tests {
		ctx := context.Background()
		fakeClient := crFake.NewClientBuilder().WithScheme(s).WithObjects(tt.objects...).Build()

		t.Run(tt.name, func(t *testing.T) {
			r := &PVCleanupController{
				Client: fakeClient,
				DryRun: tt.args.DryRun,
			}

			err := r.deleteOrphanedPV(ctx, tt.orphanedPV[0])

			if tt.wantErr {
				assert.Error(t, err, "Expected an error but got none")
			} else {
				require.NoError(t, err, "Unexpected error occurred")
			}

			// Assert deleted PVs
			for _, pv := range tt.orphanedPV {
				deletedPV := &corev1.PersistentVolume{}
				err := fakeClient.Get(ctx, client.ObjectKey{Name: pv.Name}, deletedPV)
				assert.Error(t, err, "Expected PV %s to be deleted", pv.Name)
			}
		})
	}
}
