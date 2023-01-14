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
	gocontext "context"
	"strings"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// KubemarkCluster defines a cluster hosting kubemark pods.
type KubemarkCluster interface {
	GenerateKubemarkClusterClient(kubemarkClusterSecretRef *corev1.ObjectReference, ownerNamespace string, context gocontext.Context) (client.Client, string, error)
}

// NewKubemarkCluster creates new KubemarkCluster instance.
func NewKubemarkCluster(client client.Client) KubemarkCluster {
	return &kubemarkCluster{
		Client: client,
	}
}

type kubemarkCluster struct {
	client.Client
}

// GenerateKubemarkClusterClient creates a client for kubemark cluster.
func (w *kubemarkCluster) GenerateKubemarkClusterClient(kubemarkClusterSecretRef *corev1.ObjectReference, ownerNamespace string, context gocontext.Context) (client.Client, string, error) {
	if kubemarkClusterSecretRef == nil {
		return w.Client, ownerNamespace, nil
	}

	kubemarkClusterKubeconfigSecret := &corev1.Secret{}
	kubemarkClusterKubeconfigSecretKey := client.ObjectKey{Namespace: kubemarkClusterSecretRef.Namespace, Name: kubemarkClusterSecretRef.Name}
	if err := w.Client.Get(context, kubemarkClusterKubeconfigSecretKey, kubemarkClusterKubeconfigSecret); err != nil {
		return nil, "", errors.Wrapf(err, "failed to fetch kubemark cluster kubeconfig secret %s/%s", kubemarkClusterSecretRef.Namespace, kubemarkClusterSecretRef.Name)
	}

	kubeConfig, ok := kubemarkClusterKubeconfigSecret.Data["kubeconfig"]
	if !ok {
		return nil, "", errors.New("failed to retrieve kubemark cluster kubeconfig from secret: 'kubeconfig' key is missing")
	}

	namespace := "default"
	namespaceBytes, ok := kubemarkClusterKubeconfigSecret.Data["namespace"]
	if ok {
		namespace = string(namespaceBytes)
		namespace = strings.TrimSpace(namespace)
	}

	// generate REST config
	restConfig, err := clientcmd.RESTConfigFromKubeConfig(kubeConfig)
	if err != nil {
		return nil, "", errors.Wrap(err, "failed to create REST config")
	}

	// create the client
	kubemarkClusterClient, err := client.New(restConfig, client.Options{Scheme: w.Client.Scheme()})
	if err != nil {
		return nil, "", errors.Wrap(err, "failed to create kubemark cluster client")
	}

	return kubemarkClusterClient, namespace, nil
}
