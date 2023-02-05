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
	"bytes"
	"context"
	b64 "encoding/base64"
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
	"runtime"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	clusterctlclient "sigs.k8s.io/cluster-api/cmd/clusterctl/client"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
	"time"
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

		// Get a ClusterProxy so we can interact with the workload cluster
		workloadClusterProxy := input.BootstrapClusterProxy.GetWorkloadCluster(ctx, clusterResources.Cluster.Namespace, clusterResources.Cluster.Name)

		By("Installing the autoscaler in the workload cluster")
		ApplyAutoscalerToWorkloadCluster(ctx, ApplyAutoscalerToWorkloadClusterInput{
			E2EConfig:              input.E2EConfig,
			artifactFolder:         artifactFolder,
			infrastructureProvider: infrastructureProvider,
			ManagementClusterProxy: input.BootstrapClusterProxy,
			WorkloadClusterProxy:   workloadClusterProxy,
			Cluster:                clusterResources.Cluster,
		})

		By("Creating workload that force the system to scale up")
		AddScaleUpDeploymentAndWait(ctx, AddScaleUpDeploymentAndWaitInput{
			Replicas:     5,
			ClusterProxy: workloadClusterProxy,
		}, input.E2EConfig.GetIntervals(bootstrapClusterProxy.GetName(), "wait-autoscaler")...)

		By("PASSED!")
	})

	AfterEach(func() {
		// Dumps all the resources in the spec namespace, then cleanups the cluster object and the spec namespace itself.
		dumpSpecResourcesAndCleanup(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder, namespace, cancelWatches, clusterResources.Cluster, input.E2EConfig.GetIntervals, input.SkipCleanup)
	})
})

type ApplyAutoscalerToWorkloadClusterInput struct {
	E2EConfig            *clusterctl.E2EConfig
	ClusterctlConfigPath string

	// info about where autoscaler yaml are
	// Note:
	//  - the file creating the service account to be used by the autoscaler when connecting to the management cluster
	//    - must be named "autoscaler-to-workload-management.yaml"
	//    - must deploy objects in the $CLUSTER_NAMESPACE
	//    - must create a service account with name "cluster-$CLUSTER_NAME" and the RBAC rules required to work.
	//    - must create a secret with name "cluster-$CLUSTER_NAME-token" and type "kubernetes.io/service-account-token".
	//  - the file creating the autoscaler deployment in the workload cluster
	//    - must be named "autoscaler-to-workload-workload.yaml"
	//    - must deploy objects in the cluster-autoscaler-system namespace
	//    - must create a deployment named "cluster-autoscaler"
	//    - must run the autoscaler with --cloud-provider=clusterapi,
	//      --node-group-auto-discovery=clusterapi:namespace=${CLUSTER_NAMESPACE},clusterName=${CLUSTER_NAME}
	//      and --cloud-config pointing to a kubeconfig to connect to the management cluster
	//      using the token above.
	//    - could use following vars to build the management cluster kubeconfig:
	//      $MANAGEMENT_CLUSTER_TOKEN, $MANAGEMENT_CLUSTER_CA, $MANAGEMENT_CLUSTER_ADDRESS
	artifactFolder         string
	infrastructureProvider string

	ManagementClusterProxy framework.ClusterProxy
	Cluster                *clusterv1.Cluster
	WorkloadClusterProxy   framework.ClusterProxy
}

func ApplyAutoscalerToWorkloadCluster(ctx context.Context, input ApplyAutoscalerToWorkloadClusterInput) {
	infrastructureProviderVersions := input.E2EConfig.GetProviderVersions(input.infrastructureProvider)

	Logf("Creating the service account to be used by the autoscaler when connecting to the management cluster")

	managementYamlPath := filepath.Join(input.artifactFolder, "repository", fmt.Sprintf("infrastructure-%s", input.infrastructureProvider), infrastructureProviderVersions[0], "autoscaler-to-workload-management.yaml")
	managementYamlTemplate, err := os.ReadFile(managementYamlPath)
	Expect(err).ToNot(HaveOccurred())

	managementYaml, err := ProcessYAML(&ProcessYAMLInput{
		Template:             managementYamlTemplate,
		ClusterctlConfigPath: input.ClusterctlConfigPath,
		Env: map[string]string{
			"CLUSTER_NAMESPACE": input.Cluster.Namespace,
			"CLUSTER_NAME":      input.Cluster.Name,
		},
	})
	Expect(err).ToNot(HaveOccurred())
	Expect(input.ManagementClusterProxy.Apply(ctx, managementYaml)).To(Succeed())

	tokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: input.Cluster.Namespace,
			Name:      fmt.Sprintf("cluster-%s-token", input.Cluster.Name),
		},
	}
	Eventually(func() bool {
		err := input.ManagementClusterProxy.GetClient().Get(ctx, client.ObjectKeyFromObject(tokenSecret), tokenSecret)
		if err != nil {
			return false
		}
		if _, ok := tokenSecret.Data["token"]; !ok {
			return false
		}
		if _, ok := tokenSecret.Data["ca.crt"]; !ok {
			return false
		}
		return true
	}, 5*time.Second, 100*time.Millisecond).Should(BeTrue())

	Logf("Creating the autoscaler deployment in the workload cluster")

	workloadYamlPath := filepath.Join(input.artifactFolder, "repository", fmt.Sprintf("infrastructure-%s", input.infrastructureProvider), infrastructureProviderVersions[0], "autoscaler-to-workload-workload.yaml")
	workloadYamlTemplate, err := os.ReadFile(workloadYamlPath)
	Expect(err).ToNot(HaveOccurred())

	serverAddr := input.ManagementClusterProxy.GetRESTConfig().Host
	// On CAPD, if not running on Linux, we need to use Docker's proxy to connect back to the host
	// to the CAPD cluster. Moby on Linux doesn't use the host.docker.internal DNS name.
	if runtime.GOOS != "linux" {
		serverAddr = strings.ReplaceAll(serverAddr, "127.0.0.1", "host.docker.internal")
	}

	workloadYaml, err := ProcessYAML(&ProcessYAMLInput{
		Template:             workloadYamlTemplate,
		ClusterctlConfigPath: input.ClusterctlConfigPath,
		Env: map[string]string{
			"CLUSTER_NAMESPACE":          input.Cluster.Namespace,
			"CLUSTER_NAME":               input.Cluster.Name,
			"MANAGEMENT_CLUSTER_TOKEN":   string(tokenSecret.Data["token"]),
			"MANAGEMENT_CLUSTER_CA":      b64.StdEncoding.EncodeToString(tokenSecret.Data["ca.crt"]),
			"MANAGEMENT_CLUSTER_ADDRESS": serverAddr,
		},
	})
	Expect(err).ToNot(HaveOccurred())
	Expect(input.WorkloadClusterProxy.Apply(ctx, workloadYaml)).To(Succeed())

	Logf("Wait for the autoscaler deployment and collect logs")
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-autoscaler",
			Namespace: "cluster-autoscaler-system",
		},
	}

	if err := input.WorkloadClusterProxy.GetClient().Get(ctx, client.ObjectKeyFromObject(deployment), deployment); apierrors.IsNotFound(err) {
		framework.WaitForDeploymentsAvailable(ctx, framework.WaitForDeploymentsAvailableInput{
			Getter:     input.WorkloadClusterProxy.GetClient(),
			Deployment: deployment,
		}, input.E2EConfig.GetIntervals(bootstrapClusterProxy.GetName(), "wait-controllers")...)

		// Start streaming logs from all controller providers
		framework.WatchDeploymentLogs(ctx, framework.WatchDeploymentLogsInput{
			GetLister:  input.WorkloadClusterProxy.GetClient(),
			ClientSet:  input.WorkloadClusterProxy.GetClientSet(),
			Deployment: deployment,
			LogPath:    filepath.Join(artifactFolder, "clusters", input.WorkloadClusterProxy.GetName(), "logs", deployment.GetNamespace()),
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

// TODO: move into the CAPI E2E framewowrk
type ProcessYAMLInput struct {
	Template             []byte
	ClusterctlConfigPath string
	Env                  map[string]string
}

func ProcessYAML(input *ProcessYAMLInput) ([]byte, error) {
	for n, v := range input.Env {
		os.Setenv(n, v)
	}

	c, err := clusterctlclient.New(input.ClusterctlConfigPath)
	if err != nil {
		return nil, err
	}
	options := clusterctlclient.ProcessYAMLOptions{
		ReaderSource: &clusterctlclient.ReaderSourceOptions{
			Reader: bytes.NewReader(input.Template),
		},
	}

	printer, err := c.ProcessYAML(options)
	if err != nil {
		return nil, err
	}

	out, err := printer.Yaml()
	if err != nil {
		return nil, err
	}

	return out, nil
}
