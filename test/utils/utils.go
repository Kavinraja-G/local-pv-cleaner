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

package utils

import (
	corev1 "k8s.io/api/core/v1"
)

// NormalizePVs normalizes by extracting only Name & StorageClass
func NormalizePVs(pvs []corev1.PersistentVolume) []map[string]string {
	result := make([]map[string]string, len(pvs))

	for _, pv := range pvs {
		result = append(result, map[string]string{
			"name":          pv.Name,
			"storageClass":  pv.Spec.StorageClassName,
			"reclaimPolicy": string(pv.Spec.PersistentVolumeReclaimPolicy),
		})
	}

	return result
}
