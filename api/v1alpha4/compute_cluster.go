/*
Copyright 2020 The Kubernetes Authors.

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

package v1alpha4

import corev1 "k8s.io/api/core/v1"

// ComputeClusterSpec defines spec for a compute cluster.
type ComputeClusterSpec struct {
	// SecretRef is the reference to an existing secret with a `kubeconfig` value
	// providing the kubeconfig for accessing to the ComputeCluster.
	// NOTE: The secret needs to be created for the controller to find it.
	// TODO: validation (mandatory if ComputeClusterSpec is defined; cannot be changed if the KubemarkCluster is ready)
	KubeConfigSecretRef corev1.LocalObjectReference `json:"secretRef,omitempty"`

	// Namespace defines namespace where to host pods running kubemark in the ComputeCluster. If empty
	// the namespace of the kubeconfig will be used as a default.
	// Note: it is not supported using the same backing namespace for two clusters with the same name.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}
