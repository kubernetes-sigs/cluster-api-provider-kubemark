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
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	infrav1 "github.com/kubernetes-sigs/cluster-api-provider-kubemark/api/v1alpha4"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type KubemarkMachineTemplateReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=kubemarkmachinetemplates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=kubemarkmachinetemplates/status,verbs=get;update;patch

func (r *KubemarkMachineTemplateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Log.WithValues("kubemarkmachinetemplate", req.NamespacedName)
	updateRequired := false

	var machineTemplate infrav1.KubemarkMachineTemplate
	if err := r.Get(ctx, req.NamespacedName, &machineTemplate); err != nil {
		logger.Error(err, "unable to fetch KubemarkMachineTemplate")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	helper, err := patch.NewHelper(&machineTemplate, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to init patch helper: %w", err)
	}

	if machineTemplate.Spec.Template.Spec.KubemarkOptions.ExtendedResources != nil {
		extendedResources := getKubemarkExtendedResources(machineTemplate.Spec.Template.Spec.KubemarkOptions)
		capacity := corev1.ResourceList{}
		for k, v := range extendedResources {
			capacity[corev1.ResourceName(k)] = v
		}

		if !reflect.DeepEqual(machineTemplate.Status.Capacity, capacity) {
			machineTemplate.Status.Capacity = capacity
			updateRequired = true
		}
	}

	if updateRequired {
		if err := helper.Patch(ctx, &machineTemplate); err != nil {
			if !apierrors.IsNotFound(err) {
				logger.Error(err, "failed to patch machineTemplate")
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

func (r *KubemarkMachineTemplateReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.KubemarkMachineTemplate{}).
		Complete(r)
}
