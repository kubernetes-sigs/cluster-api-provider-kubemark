/*
Copyright 2022 The Kubernetes Authors.

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

package controllers

import (
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"

	infrav1 "github.com/kubernetes-sigs/cluster-api-provider-kubemark/api/v1alpha4"
)

func TestGetKubemarkExtendedResources(t *testing.T) {
	tests := []struct {
		name      string
		resources infrav1.KubemarkProcessOptions
		expected  infrav1.KubemarkExtendedResourceList
	}{
		{
			name:      "default values",
			resources: infrav1.KubemarkProcessOptions{},
			expected: infrav1.KubemarkExtendedResourceList{
				infrav1.KubemarkExtendedResourceCPU:    resource.MustParse("1"),
				infrav1.KubemarkExtendedResourceMemory: resource.MustParse("4G"),
			},
		},
		{
			name: "default values with extra",
			resources: infrav1.KubemarkProcessOptions{
				ExtendedResources: infrav1.KubemarkExtendedResourceList{
					"nvidia.com/gpu": resource.MustParse("2"),
				},
			},
			expected: infrav1.KubemarkExtendedResourceList{
				infrav1.KubemarkExtendedResourceCPU:    resource.MustParse("1"),
				infrav1.KubemarkExtendedResourceMemory: resource.MustParse("4G"),
				"nvidia.com/gpu":                       resource.MustParse("2"),
			},
		},
		{
			name: "replace mem",
			resources: infrav1.KubemarkProcessOptions{
				ExtendedResources: infrav1.KubemarkExtendedResourceList{
					infrav1.KubemarkExtendedResourceMemory: resource.MustParse("200G"),
				},
			},
			expected: infrav1.KubemarkExtendedResourceList{
				infrav1.KubemarkExtendedResourceCPU:    resource.MustParse("1"),
				infrav1.KubemarkExtendedResourceMemory: resource.MustParse("200G"),
			},
		},
		{
			name: "replace cpu",
			resources: infrav1.KubemarkProcessOptions{
				ExtendedResources: infrav1.KubemarkExtendedResourceList{
					infrav1.KubemarkExtendedResourceCPU: resource.MustParse("100"),
				},
			},
			expected: infrav1.KubemarkExtendedResourceList{
				infrav1.KubemarkExtendedResourceCPU:    resource.MustParse("100"),
				infrav1.KubemarkExtendedResourceMemory: resource.MustParse("4G"),
			},
		},
		{
			name: "replace all",
			resources: infrav1.KubemarkProcessOptions{
				ExtendedResources: infrav1.KubemarkExtendedResourceList{
					infrav1.KubemarkExtendedResourceCPU:    resource.MustParse("100"),
					infrav1.KubemarkExtendedResourceMemory: resource.MustParse("200G"),
					"nvidia.com/gpu":                       resource.MustParse("2"),
				},
			},
			expected: infrav1.KubemarkExtendedResourceList{
				infrav1.KubemarkExtendedResourceCPU:    resource.MustParse("100"),
				infrav1.KubemarkExtendedResourceMemory: resource.MustParse("200G"),
				"nvidia.com/gpu":                       resource.MustParse("2"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			observed := getKubemarkExtendedResources(tt.resources)
			if !reflect.DeepEqual(observed, tt.expected) {
				t.Errorf("unexpected mismatch, observed %v, expected %v", observed, tt.expected)
			}
		})
	}
}
