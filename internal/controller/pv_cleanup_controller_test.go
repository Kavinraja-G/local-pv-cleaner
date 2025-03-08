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

	"github.com/kavinraja-g/local-pv-cleaner/test/utils"
	"github.com/stretchr/testify/require"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestPVCleanupController_listAllPVs(t *testing.T) {
	s := scheme.Scheme
	_ = corev1.AddToScheme(s)

	type args struct {
		DryRun            bool
		NodeSelectorKeys  []string
		StorageClassNames []string
	}

	// Define test-cases
	var tests = []struct {
		name    string
		objects []client.Object
		args    args
		wantOut []corev1.PersistentVolume
		wantErr bool
	}{
		{
			name: "List PVs",
			objects: []client.Object{
				&corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: "pv-1"}},
				&corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: "pv-2"}},
			},
			wantErr: false,
			wantOut: []corev1.PersistentVolume{
				{ObjectMeta: metav1.ObjectMeta{Name: "pv-1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "pv-2"}},
			},
		},
		{
			name:    "No PVs available",
			objects: []client.Object{},
			wantErr: false,
			wantOut: []corev1.PersistentVolume{},
		},
	}

	// Run tests
	for _, tt := range tests {
		ctx := context.Background()
		fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(tt.objects...).Build()

		t.Run(tt.name, func(t *testing.T) {
			r := &PVCleanupController{
				Client:            fakeClient,
				DryRun:            tt.args.DryRun,
				NodeSelectorKeys:  tt.args.NodeSelectorKeys,
				StorageClassNames: tt.args.StorageClassNames,
			}

			allPVs, err := r.listAllPVs(ctx)

			if tt.wantErr {
				assert.Error(t, err, "Expected an error but got none")
			} else {
				require.NoError(t, err, "Unexpected error occurred")
			}

			got := utils.NormalizePVs(allPVs)
			want := utils.NormalizePVs(tt.wantOut)

			assert.ElementsMatch(t, want, got,
				"Mismatch in retrieved PVs. Expected: %v, Got: %v", want, got)
		})
	}
}

func TestPVCleanupController_filterPVByStorageClass(t *testing.T) {
	s := scheme.Scheme
	_ = corev1.AddToScheme(s)

	type args struct {
		DryRun            bool
		NodeSelectorKeys  []string
		StorageClassNames []string
	}

	// Define test-cases
	var tests = []struct {
		name    string
		objects []client.Object
		args    args
		wantOut []corev1.PersistentVolume
		wantErr bool
	}{
		{
			name: "List filtered PVs with StorageClass",
			args: args{StorageClassNames: []string{"foo"}},
			objects: []client.Object{
				&corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: "pv-1"}, Spec: corev1.PersistentVolumeSpec{
					StorageClassName: "foo",
				}},
				&corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: "pv-2"}, Spec: corev1.PersistentVolumeSpec{
					StorageClassName: "bar",
				}},
			},
			wantErr: false,
			wantOut: []corev1.PersistentVolume{
				{ObjectMeta: metav1.ObjectMeta{Name: "pv-1"}, Spec: corev1.PersistentVolumeSpec{
					StorageClassName: "foo",
				}},
			},
		},
		{
			name:    "No filtered PVs available",
			args:    args{StorageClassNames: []string{"foo"}},
			objects: []client.Object{},
			wantErr: false,
			wantOut: []corev1.PersistentVolume{},
		},
	}

	// Run tests
	for _, tt := range tests {
		ctx := context.Background()
		fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(tt.objects...).Build()

		t.Run(tt.name, func(t *testing.T) {
			r := &PVCleanupController{
				Client:            fakeClient,
				DryRun:            tt.args.DryRun,
				NodeSelectorKeys:  tt.args.NodeSelectorKeys,
				StorageClassNames: tt.args.StorageClassNames,
			}

			allPVs, err := r.listAllPVs(ctx)
			filteredPVs := r.filterPVByStorageClass(allPVs)

			if tt.wantErr {
				assert.Error(t, err, "Expected an error but got none")
			} else {
				require.NoError(t, err, "Unexpected error occurred")
			}

			got := utils.NormalizePVs(filteredPVs)
			want := utils.NormalizePVs(tt.wantOut)

			assert.ElementsMatch(t, want, got,
				"Mismatch in filtered PVs. Expected: %v, Got: %v", want, got)
		})
	}
}

func TestPVCleanupController_cleanupOrphanedPVs(t *testing.T) {
	s := scheme.Scheme
	_ = corev1.AddToScheme(s)

	type args struct {
		DryRun            bool
		NodeSelectorKeys  []string
		StorageClassNames []string
	}

	// Define test-cases
	var tests = []struct {
		name            string
		objects         []client.Object
		args            args
		deletedNodeName string
		orphanedPVNames []string
		wantErr         bool
	}{
		{
			name: "Cleanup orphaned PVs",
			args: args{StorageClassNames: []string{"bar"}, NodeSelectorKeys: []string{"node-selector-key"}},
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
			wantErr:         false,
			deletedNodeName: "node-01",
			orphanedPVNames: []string{"pv-2"},
		}, {
			name: "No orphaned PVs",
			args: args{StorageClassNames: []string{"bar"}, NodeSelectorKeys: []string{"node-selector-key"}},
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
			wantErr:         false,
			deletedNodeName: "node-01",
			orphanedPVNames: []string{},
		},
	}

	// Run tests
	for _, tt := range tests {
		ctx := context.Background()
		fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(tt.objects...).Build()

		t.Run(tt.name, func(t *testing.T) {
			r := &PVCleanupController{
				Client:            fakeClient,
				DryRun:            tt.args.DryRun,
				NodeSelectorKeys:  tt.args.NodeSelectorKeys,
				StorageClassNames: tt.args.StorageClassNames,
			}

			err := r.cleanupOrphanedPVs(ctx, tt.deletedNodeName)

			if tt.wantErr {
				assert.Error(t, err, "Expected an error but got none")
			} else {
				require.NoError(t, err, "Unexpected error occurred")
			}

			// Assert deleted PVs
			for _, pvName := range tt.orphanedPVNames {
				deletedPV := &corev1.PersistentVolume{}
				err := fakeClient.Get(ctx, client.ObjectKey{Name: pvName}, deletedPV)
				assert.Error(t, err, "Expected PV %s to be deleted", pvName)
			}
		})
	}
}
