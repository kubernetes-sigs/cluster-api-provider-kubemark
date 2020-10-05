/*


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
	"context"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/cluster-api/controllers/remote"
	"sigs.k8s.io/cluster-api/util"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infrav1 "github.com/benmoss/cluster-api-provider-kubemark/api/v1alpha3"
)

const kubemarkName = "hollow-node"

var (
	hostPathFile = v1.HostPathFile
	deployment   = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubemarkName,
			Labels:    map[string]string{"app": kubemarkName},
			Namespace: "kube-system",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: pointer.Int32Ptr(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": kubemarkName,
				},
			},
			Template: apiv1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": kubemarkName,
					},
				},
				Spec: apiv1.PodSpec{
					InitContainers: []apiv1.Container{
						{
							Name:    "kubeadm",
							Image:   "gcr.io/cf-london-servces-k8s/bmo/kubeadm@sha256:f27a915a9fe84906afd5d85c347c7867ed9154657d27bbe258c040e7785c1742",
							Command: []string{"kubeadm"},
							Args:    []string{"join", "--skip-phases=preflight"},
						},
					},
					Containers: []apiv1.Container{
						{
							Name:    kubemarkName,
							Image:   "gcr.io/cf-london-servces-k8s/bmo/kubemark@sha256:9f717e0f2fc1b00c72719f157c1a3846ab8180070c201b950cade504c12dec59",
							Command: []string{"/kubemark"},
							Args: []string{
								"--v=3",
								"--morph=kubelet",
								"--log-file=/var/log/kubelet.log",
								"--logtostderr=false",
							},
							VolumeMounts: []v1.VolumeMount{
								{
									MountPath: "/kubeconfig/kubeconfig",
									Name:      "kubeconfig",
								},
								{
									MountPath: "/var/lib/kubelet/pki",
									Name:      "kubeconfig-certs",
								},
							},
						},
					},
					Tolerations: []apiv1.Toleration{
						{
							Key:    "node-role.kubernetes.io/master",
							Effect: apiv1.TaintEffectNoSchedule,
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "kubeconfig",
							VolumeSource: v1.VolumeSource{
								HostPath: &v1.HostPathVolumeSource{
									Path: "/etc/kubernetes/kubelet.conf",
									Type: &hostPathFile,
								},
							},
						},
						{
							Name: "kubeconfig-certs",
							VolumeSource: v1.VolumeSource{
								HostPath: &v1.HostPathVolumeSource{
									Path: "/var/lib/kubelet/pki",
								},
							},
						},
					},
				},
			},
		},
	}
)

// KubemarkMachineReconciler reconciles a KubemarkMachine object
type KubemarkMachineReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=kubemarkmachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=kubemarkmachines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines;machines/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets;,verbs=get;list;watch

func (r *KubemarkMachineReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	logger := r.Log.WithValues("kubemarkmachine", req.NamespacedName)

	kubemarkMachine := &infrav1.KubemarkMachine{}
	err := r.Get(ctx, req.NamespacedName, kubemarkMachine)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Fetch the Machine.
	machine, err := util.GetOwnerMachine(ctx, r.Client, kubemarkMachine.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}
	if machine == nil {
		logger.Info("Machine Controller has not yet set OwnerRef")
		return ctrl.Result{}, nil
	}

	logger = logger.WithValues("machine", machine.Name)

	// Fetch the Cluster.
	cluster, err := util.GetClusterFromMetadata(ctx, r.Client, machine.ObjectMeta)
	if err != nil {
		logger.Info("Machine is missing cluster label or cluster does not exist")
		return ctrl.Result{}, nil
	}
	restConfig, err := remote.RESTConfig(ctx, r.Client, util.ObjectKey(cluster))
	if err != nil {
		return ctrl.Result{}, err
	}
	restConfig.Timeout = 30 * time.Second

	c, err := client.New(restConfig, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		return ctrl.Result{}, err
	}

	if !cluster.Status.InfrastructureReady {
		logger.Info("Cluster infrastructure is not ready yet")
		return ctrl.Result{}, nil
	}
	if machine.Spec.Bootstrap.DataSecretName == nil {
		logger.Info("Bootstrap data secret reference is not yet available")
		return ctrl.Result{}, nil
	}
	deployment.Name = kubemarkMachine.Name
	deployment.Spec.Template.Spec.InitContainers[0].Args += ""
	err = c.Create(context.TODO(), deployment)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *KubemarkMachineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.KubemarkMachine{}).
		Complete(r)
}
