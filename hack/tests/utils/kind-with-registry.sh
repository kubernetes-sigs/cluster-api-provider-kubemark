#!/bin/sh

# Copyright 2021 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Adapted from:
# https://github.com/kubernetes-sigs/kind/commits/master/site/static/examples/kind-with-registry.sh

set -o errexit
# desired cluster name; default is "kind"
KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-kind}"
KIND_CLUSTER_OPTS="--name ${KIND_CLUSTER_NAME}"
if [ -n "${KIND_CLUSTER_IMAGE}" ]; then
  KIND_CLUSTER_OPTS="${KIND_CLUSTER_OPTS} --image ${KIND_CLUSTER_IMAGE}"
else
  KIND_CLUSTER_OPTS="${KIND_CLUSTER_OPTS} --image docker.io/kindest/node:v1.26.6"
fi
kind_version=$(kind version)
kind_network='kind'
reg_name='kind-registry'
reg_port='5000'
# create registry container unless it already exists
running="$(docker inspect -f '{{.State.Running}}' "${reg_name}" 2>/dev/null || true)"
if [ "${running}" != 'true' ]; then
  docker run \
    -d --restart=always -p "${reg_port}:5000" --name "${reg_name}" --net=kind \
    registry:2
fi
reg_host="${reg_name}"
echo "Registry Host: ${reg_host}"
reg_ip="$(docker inspect -f '{{.NetworkSettings.Networks.kind.IPAddress}}' "${reg_name}")"
echo "Registry IP: ${reg_ip}"
# create a cluster with the local registry enabled in containerd
cat <<EOF | kind create cluster ${KIND_CLUSTER_OPTS} --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
    extraMounts:
      - hostPath: /var/lib/docker
        containerPath: /var/lib/docker
      - hostPath: /var/run/docker.sock
        containerPath: /var/run/docker.sock
containerdConfigPatches:
- |-
  [plugins."io.containerd.grpc.v1.cri".registry.mirrors."localhost:${reg_port}"]
    endpoint = ["http://${reg_ip}:5000"]
EOF
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: local-registry-hosting
  namespace: kube-public
data:
  localRegistryHosting.v1: |
    host: "localhost:${reg_port}"
    help: "https://kind.sigs.k8s.io/docs/user/local-registry/"
EOF
