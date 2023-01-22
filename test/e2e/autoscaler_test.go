//go:build e2e
// +build e2e

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

package e2e

import (
	"context"
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"os"
	"path/filepath"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TODO: Move this test in CAPI so all the providers can test the autoscaler, then we can cleanup this file

// AutoScalerSpecInput is the input for AutoscalerSpec.
type AutoScalerSpecInput struct {
	E2EConfig             *clusterctl.E2EConfig
	ClusterctlConfigPath  string
	BootstrapClusterProxy framework.ClusterProxy
	ArtifactFolder        string
	SkipCleanup           bool
	ControlPlaneWaiters   clusterctl.ControlPlaneWaiters

	// Flavor, if specified is the template flavor used to create the cluster for testing.
	// If not specified, and the e2econfig variable IPFamily is IPV6, then "ipv6" is used,
	// otherwise the default flavor is used.
	Flavor *string
}

var _ = Describe("When using the autoscaler with Cluster API [PR-Blocking]", func() {

	var (
		specName         = "autoscaler"
		input            AutoScalerSpecInput
		namespace        *corev1.Namespace
		cancelWatches    context.CancelFunc
		clusterResources *clusterctl.ApplyClusterTemplateAndWaitResult
	)

	BeforeEach(func() {
		Expect(ctx).NotTo(BeNil(), "ctx is required for %s spec", specName)
		input = AutoScalerSpecInput{
			E2EConfig:             e2eConfig,
			ClusterctlConfigPath:  clusterctlConfigPath,
			BootstrapClusterProxy: bootstrapClusterProxy,
			ArtifactFolder:        artifactFolder,
			SkipCleanup:           skipCleanup,

			Flavor: pointer.String("autoscaler"),
		}

		Expect(input.E2EConfig).ToNot(BeNil(), "Invalid argument. input.E2EConfig can't be nil when calling %s spec", specName)
		Expect(input.ClusterctlConfigPath).To(BeAnExistingFile(), "Invalid argument. input.ClusterctlConfigPath must be an existing file when calling %s spec", specName)
		Expect(input.BootstrapClusterProxy).ToNot(BeNil(), "Invalid argument. input.BootstrapClusterProxy can't be nil when calling %s spec", specName)
		Expect(os.MkdirAll(input.ArtifactFolder, 0750)).To(Succeed(), "Invalid argument. input.ArtifactFolder can't be created for %s spec", specName)

		Expect(input.E2EConfig.Variables).To(HaveKey(KubernetesVersion))

		// Setup a Namespace where to host objects for this spec and create a watcher for the namespace events.
		namespace, cancelWatches = setupSpecNamespace(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder)
		clusterResources = new(clusterctl.ApplyClusterTemplateAndWaitResult)
	})

	It("Should create a workload cluster", func() {
		By("Creating a workload cluster")

		flavor := clusterctl.DefaultFlavor
		if input.Flavor != nil {
			flavor = *input.Flavor
		}

		infrastructureProvider := "kubemark" // TODO: Make this configurable in CAPI

		clusterctl.ApplyClusterTemplateAndWait(ctx, clusterctl.ApplyClusterTemplateAndWaitInput{
			ClusterProxy: input.BootstrapClusterProxy,
			ConfigCluster: clusterctl.ConfigClusterInput{
				LogFolder:                filepath.Join(input.ArtifactFolder, "clusters", input.BootstrapClusterProxy.GetName()),
				ClusterctlConfigPath:     input.ClusterctlConfigPath,
				KubeconfigPath:           input.BootstrapClusterProxy.GetKubeconfigPath(),
				InfrastructureProvider:   infrastructureProvider,
				Flavor:                   flavor,
				Namespace:                namespace.Name,
				ClusterName:              fmt.Sprintf("%s-%s", specName, util.RandomString(6)),
				KubernetesVersion:        input.E2EConfig.GetVariable(KubernetesVersion),
				ControlPlaneMachineCount: pointer.Int64(1),
				WorkerMachineCount:       pointer.Int64(1), // TODO: Make this configurable in CAPI
			},
			ControlPlaneWaiters:          input.ControlPlaneWaiters,
			WaitForClusterIntervals:      input.E2EConfig.GetIntervals(specName, "wait-cluster"),
			WaitForControlPlaneIntervals: input.E2EConfig.GetIntervals(specName, "wait-control-plane"),
			WaitForMachineDeployments:    input.E2EConfig.GetIntervals(specName, "wait-worker-nodes"),
		}, clusterResources)

		By("Installing the autoscaler")
		versions := input.E2EConfig.GetProviderVersions(infrastructureProvider)
		autoScalerYamlPath := filepath.Join(artifactFolder, "repository", fmt.Sprintf("infrastructure-%s", infrastructureProvider), versions[0], "autoscaler.yaml") // TODO: Make this configurable in CAPI,

		ApplyAutoscalerYaml(ctx, ApplyAutoscalerYamlInput{
			E2EConfig:    input.E2EConfig,
			ClusterProxy: input.BootstrapClusterProxy,
			path:         autoScalerYamlPath,
		})

		By("Creating workload that force the system to scale up")
		AddScaleUpDeploymentAndWait(ctx, AddScaleUpDeploymentAndWaitInput{
			Replicas:     5,
			ClusterProxy: input.BootstrapClusterProxy.GetWorkloadCluster(ctx, clusterResources.Cluster.Namespace, clusterResources.Cluster.Name),
		}, input.E2EConfig.GetIntervals(bootstrapClusterProxy.GetName(), "wait-autoscaler")...)

		By("PASSED!")
	})

	AfterEach(func() {
		// Dumps all the resources in the spec namespace, then cleanups the cluster object and the spec namespace itself.
		dumpSpecResourcesAndCleanup(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder, namespace, cancelWatches, clusterResources.Cluster, input.E2EConfig.GetIntervals, input.SkipCleanup)
	})
})

type ApplyAutoscalerYamlInput struct {
	E2EConfig    *clusterctl.E2EConfig
	path         string
	ClusterProxy framework.ClusterProxy
}

func ApplyAutoscalerYaml(ctx context.Context, input ApplyAutoscalerYamlInput) {
	Logf("Applying the cluster autoscaler deployment to the cluster")

	autoscalerYaml, err := os.ReadFile(input.path)
	Expect(err).ToNot(HaveOccurred())
	Expect(input.ClusterProxy.Apply(ctx, autoscalerYaml)).To(Succeed())

	Logf("Wait for the autoscaler deployment and collect logs")
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-autoscaler",        // TODO: get deployment name in input (or infer it from the yaml...)
			Namespace: "cluster-autoscaler-system", // TODO: get deployment namespace in input (or infer it from the yaml...)
		},
	}

	if err := input.ClusterProxy.GetClient().Get(ctx, client.ObjectKeyFromObject(deployment), deployment); apierrors.IsNotFound(err) {
		framework.WaitForDeploymentsAvailable(ctx, framework.WaitForDeploymentsAvailableInput{
			Getter:     input.ClusterProxy.GetClient(),
			Deployment: deployment,
		}, input.E2EConfig.GetIntervals(bootstrapClusterProxy.GetName(), "wait-controllers")...)

		// Start streaming logs from all controller providers
		framework.WatchDeploymentLogs(ctx, framework.WatchDeploymentLogsInput{
			GetLister:  input.ClusterProxy.GetClient(),
			ClientSet:  input.ClusterProxy.GetClientSet(),
			Deployment: deployment,
			LogPath:    filepath.Join(artifactFolder, "clusters", bootstrapClusterProxy.GetName(), "logs", deployment.GetNamespace()), // TODO: get log folder in input
		})
	}
}

type AddScaleUpDeploymentAndWaitInput struct {
	Replicas     int32
	ClusterProxy framework.ClusterProxy
}

func AddScaleUpDeploymentAndWait(ctx context.Context, input AddScaleUpDeploymentAndWaitInput, intervals ...interface{}) {
	Logf("Create a scale up deployment requiring 2G memory for each replica")

	scalelUpDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "scale-up",
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				"app": "scale-up",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: pointer.Int32(input.Replicas),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "scale-up",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "scale-up",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "busybox",
							Image: "busybox",
							Resources: corev1.ResourceRequirements{
								Requests: map[corev1.ResourceName]resource.Quantity{
									corev1.ResourceMemory: resource.MustParse("2G"), // TODO: consider if to make this configurable
									corev1.ResourceCPU:    resource.MustParse("100m"),
								},
							},
							Command: []string{"/bin/sh", "-c", "echo \"up\" & sleep infinity"},
						},
					},
				},
			},
		},
	}

	Logf("Scale up deployment created")
	Expect(input.ClusterProxy.GetClient().Create(ctx, scalelUpDeployment)).To(Succeed())

	Logf("Wait for the scale up deployment to become ready (this implies machines to be created)")
	framework.WaitForDeploymentsAvailable(ctx, framework.WaitForDeploymentsAvailableInput{
		Getter:     input.ClusterProxy.GetClient(),
		Deployment: scalelUpDeployment,
	}, intervals...)
}
