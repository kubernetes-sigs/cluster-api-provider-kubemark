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
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	infrav1 "github.com/kubernetes-sigs/cluster-api-provider-kubemark/api/v1alpha4"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	kubemarkExtendedResourcesFlag = "--extended-resources="
)

func TestGetKubemarkExtendedResourcesFlag(t *testing.T) {
	tests := []struct {
		name          string
		resources     *infrav1.KubemarkProcessOptions
		expectedFlags string // the expected flags string does not need to be in a specific order
	}{
		{
			name:          "default values",
			resources:     nil,
			expectedFlags: "--extended-resources=cpu=1,memory=4G",
		},
		{
			name: "replace cpu",
			resources: &infrav1.KubemarkProcessOptions{
				ExtendedResources: map[infrav1.KubemarkExtendedResourceName]resource.Quantity{
					infrav1.KubemarkExtendedResourceCPU: resource.MustParse("2"),
				},
			},
			expectedFlags: "--extended-resources=cpu=2,memory=4G",
		},
		{
			name: "replace memory",
			resources: &infrav1.KubemarkProcessOptions{
				ExtendedResources: map[infrav1.KubemarkExtendedResourceName]resource.Quantity{
					infrav1.KubemarkExtendedResourceMemory: resource.MustParse("16G"),
				},
			},
			expectedFlags: "--extended-resources=cpu=1,memory=16G",
		},
		{
			name: "replace all",
			resources: &infrav1.KubemarkProcessOptions{
				ExtendedResources: map[infrav1.KubemarkExtendedResourceName]resource.Quantity{
					infrav1.KubemarkExtendedResourceCPU:    resource.MustParse("2"),
					infrav1.KubemarkExtendedResourceMemory: resource.MustParse("16G"),
				},
			},
			expectedFlags: "--extended-resources=cpu=2,memory=16G",
		},
		{
			name: "replace all, add gpu",
			resources: &infrav1.KubemarkProcessOptions{
				ExtendedResources: map[infrav1.KubemarkExtendedResourceName]resource.Quantity{
					infrav1.KubemarkExtendedResourceCPU:    resource.MustParse("2"),
					infrav1.KubemarkExtendedResourceMemory: resource.MustParse("16G"),
					"nvidia.com/gpu":                       resource.MustParse("1"),
				},
			},
			expectedFlags: "--extended-resources=cpu=2,memory=16G,nvidia.com/gpu=1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			observedFlags := getKubemarkExtendedResourcesFlag(tt.resources)
			observed, err := mapFromExtendedResourceFlags(observedFlags)
			if err != nil {
				t.Error("unable to process observed flag string", err)
			}
			expected, err := mapFromExtendedResourceFlags(tt.expectedFlags)
			if err != nil {
				t.Error("unable to process expected flag string", err)
			}
			if !reflect.DeepEqual(observed, expected) {
				t.Error("observed flags did not match expected", observedFlags, tt.expectedFlags)
			}
		})
	}
}

// This is a helper function for processing the extended resources command line flags.
// It accepts a string in the format of the flag and returns a map of resources and quantities.
func mapFromExtendedResourceFlags(flags string) (map[string]string, error) {
	if !strings.HasPrefix(flags, kubemarkExtendedResourcesFlag) {
		return nil, errors.New(fmt.Sprintf("extended resources flag does not contain proper prefix `%s`, `%s`", kubemarkExtendedResourcesFlag, flags))
	}

	ret := map[string]string{}
	// create an array of resources strings (eg "cpu=1")
	resources := strings.Split(flags[len(kubemarkExtendedResourcesFlag):], ",")
	for _, r := range resources {
		// split the resource string into its key and value
		rsplit := strings.Split(r, "=")
		if len(rsplit) != 2 {
			return nil, errors.New(fmt.Sprintf("unable to split resource pair `%s` in `%s`", r, flags))
		}
		ret[rsplit[0]] = rsplit[1]
	}

	return ret, nil
}
