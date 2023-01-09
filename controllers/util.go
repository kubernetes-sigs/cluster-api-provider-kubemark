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
	"k8s.io/apimachinery/pkg/api/resource"

	infrav1 "github.com/kubernetes-sigs/cluster-api-provider-kubemark/api/v1alpha4"
)

const (
	// DefaultCPUResource is the default cpu capacity when none is specified (k8s 1.22+).
	DefaultCPUResource = "1"
	// DefaultMemResource is the default memory capacity when none is specified (k8s 1.22+).
	DefaultMemResource = "4G"
)

// getKubemarkExtendedResources returns a KubemarkExtendedResourceList from the
// KubemarkProcessOptions provided. This function ensures that the cpu and memory
// fields will be set with defaults of `1` and `4G`, respectively.
func getKubemarkExtendedResources(options infrav1.KubemarkProcessOptions) infrav1.KubemarkExtendedResourceList {
	resourcemap := map[infrav1.KubemarkExtendedResourceName]resource.Quantity{
		infrav1.KubemarkExtendedResourceCPU:    resource.MustParse(DefaultCPUResource),
		infrav1.KubemarkExtendedResourceMemory: resource.MustParse(DefaultMemResource),
	}

	if options.ExtendedResources != nil {
		for k, v := range options.ExtendedResources {
			resourcemap[k] = v
		}
	}

	return resourcemap
}
