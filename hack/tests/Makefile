TEST_DIR := $(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))
ROOT_DIR := $(TEST_DIR)/../..
CERT_MANAGER_IMAGE_REPOSITORY ?= quay.io/jetstack
CERT_MANAGER_CAINJECTOR ?= cert-manager-cainjector
CERT_MANAGER_CONTROLLER ?= cert-manager-controller
CERT_MANAGER_WEBHOOK ?= cert-manager-webhook
CERT_MANAGER_VERSION ?= v1.1.0
KUBE_RBAC_PROXY_VERSION ?= v0.5.0
KUBE_RBAC_IMAGE_REPOSITORY ?= quay.io/coreos
KUBE_RBAC_PROXY ?= kube-rbac-proxy
DEV_REGISTRY ?= localhost:5000

.start-kind-cluster:
	@kind delete cluster; sleep 20; docker network create kind; $(TEST_DIR)/utils/kind-with-registry.sh

.install-cert-manager:
	@docker pull $(CERT_MANAGER_IMAGE_REPOSITORY)/$(CERT_MANAGER_CAINJECTOR):$(CERT_MANAGER_VERSION)
	@docker tag $(CERT_MANAGER_IMAGE_REPOSITORY)/$(CERT_MANAGER_CAINJECTOR):$(CERT_MANAGER_VERSION) $(DEV_REGISTRY)/$(CERT_MANAGER_CAINJECTOR):$(CERT_MANAGER_VERSION)
	@docker push $(DEV_REGISTRY)/$(CERT_MANAGER_CAINJECTOR):$(CERT_MANAGER_VERSION)

	@docker pull $(CERT_MANAGER_IMAGE_REPOSITORY)/$(CERT_MANAGER_CONTROLLER):$(CERT_MANAGER_VERSION)
	@docker tag $(CERT_MANAGER_IMAGE_REPOSITORY)/$(CERT_MANAGER_CONTROLLER):$(CERT_MANAGER_VERSION) $(DEV_REGISTRY)/$(CERT_MANAGER_CONTROLLER):$(CERT_MANAGER_VERSION)
	@docker push $(DEV_REGISTRY)/$(CERT_MANAGER_CONTROLLER):$(CERT_MANAGER_VERSION)

	@docker pull $(CERT_MANAGER_IMAGE_REPOSITORY)/$(CERT_MANAGER_WEBHOOK):$(CERT_MANAGER_VERSION)
	@docker tag $(CERT_MANAGER_IMAGE_REPOSITORY)/$(CERT_MANAGER_WEBHOOK):$(CERT_MANAGER_VERSION) $(DEV_REGISTRY)/$(CERT_MANAGER_WEBHOOK):$(CERT_MANAGER_VERSION)
	@docker push $(DEV_REGISTRY)/$(CERT_MANAGER_WEBHOOK):$(CERT_MANAGER_VERSION)

	@docker pull $(KUBE_RBAC_IMAGE_REPOSITORY)/$(KUBE_RBAC_PROXY):$(KUBE_RBAC_PROXY_VERSION)
	@docker tag $(KUBE_RBAC_IMAGE_REPOSITORY)/$(KUBE_RBAC_PROXY):$(KUBE_RBAC_PROXY_VERSION) $(DEV_REGISTRY)/$(KUBE_RBAC_PROXY):$(KUBE_RBAC_PROXY_VERSION)
	@docker push $(DEV_REGISTRY)/$(KUBE_RBAC_PROXY):$(KUBE_RBAC_PROXY_VERSION)

## --------------------------------------
## Build CAPI
## --------------------------------------

# Ask the user for the CAPI path, if not provided
.ensure-capi-path:
ifndef CAPI_PATH
	$(eval export CAPI_PATH = $(shell read -p $$'Please specify the CAPI_PATH: ' capi_path; echo $$capi_path))
endif

## --------------------------------------
## Configure clusterctl
## --------------------------------------

define CLUSTERCTL_SETTINGS_PROVIDERS
{
    "providers": ["cluster-api", "bootstrap-kubeadm", "control-plane-kubeadm", "infrastructure-docker"]
}
endef

.generate-clusterctl-config: export CLUSTERCTL_SETTINGS:=$(CLUSTERCTL_SETTINGS_PROVIDERS)
.generate-clusterctl-config: .ensure-capi-path
	@rm -f $(CAPI_PATH)/clusterctl-settings.json
	@echo "$${CLUSTERCTL_SETTINGS}" >> $(abspath ${CAPI_PATH})/clusterctl-settings.json

## --------------------------------------
## Build clusterctl
## --------------------------------------

.capi-build-clusterctl: .ensure-capi-path
	@cd $(CAPI_PATH) && make clusterctl

## --------------------------------------
## Build images and manifests
## --------------------------------------

.docker-build-capi: .ensure-capi-path
	@make -C $(CAPI_PATH) docker-build REGISTRY=$(DEV_REGISTRY) && \
		make -C $(CAPI_PATH) docker-push REGISTRY=$(DEV_REGISTRY)
	@make -C ${CAPI_PATH}/test/infrastructure/docker docker-build REGISTRY=$(DEV_REGISTRY) && \
  		make -C ${CAPI_PATH}/test/infrastructure/docker docker-push REGISTRY=$(DEV_REGISTRY)

.docker-build-capk:
	@make -C $(ROOT_DIR) release-manifests
	@make -C $(ROOT_DIR) docker-build REGISTRY=$(DEV_REGISTRY) && \
		make -C $(ROOT_DIR) docker-push REGISTRY=$(DEV_REGISTRY)

.docker-build: .docker-build-capi .docker-build-capk

.generate-capi-manifests: .ensure-capi-path
	@make -C $(CAPI_PATH) generate-manifests REGISTRY=$(DEV_REGISTRY) PULL_POLICY=Always
	@make -C ${CAPI_PATH}/test/infrastructure/docker generate-manifests REGISTRY=$(DEV_REGISTRY) PULL_POLICY=Always

.generate-capk-manifests:
	@make -C $(ROOT_DIR) generate-manifests REGISTRY=$(DEV_REGISTRY) PULL_POLICY=Always

.generate-manifests: .generate-capi-manifests .generate-capk-manifests

CLUSTERCTL_CONFIG_PATH := $(HOME)/.cluster-api
.create-local-repository: .ensure-capi-path
	@rm -fr $(CLUSTERCTL_CONFIG_PATH)
	@cd $(CAPI_PATH) && PATH=$${PATH}:$(abspath $(CAPI_PATH)) && ./cmd/clusterctl/hack/create-local-repository.py

## --------------------------------------
## Setup local repository
## --------------------------------------

define LOCAL_REPOSITORY_PATCH
cert-manager:
  url: "$(ROOT_DIR)/hack/tests/resources/cert-manager/v1.1.0/cert-manager.yaml"
images:
  all:
    repository: "$(DEV_REGISTRY)"
endef

.config-local-repository: export LOCAL_REPOSITORY_PATCH:=$(LOCAL_REPOSITORY_PATCH)
.config-local-repository: $(CLUSTERCTL_CONFIG_PATH)
	@echo "$${LOCAL_REPOSITORY_PATCH}" >> $(CLUSTERCTL_CONFIG_PATH)/dev-repository/config.yaml

## --------------------------------------
## Deploy CAPI
## --------------------------------------

CLUSTERCTL_BIN := $(abspath $(CAPI_PATH)/bin/)
$(CLUSTERCTL_BIN): .ensure-capi-path
	@make -C $(CAPI_PATH) clusterctl

.deploy-cluster-api:
	@$(CAPI_PATH)/bin/clusterctl init \
	 --config ~/.cluster-api/dev-repository/config.yaml \
	 --core cluster-api:v0.4.99 \
	 --bootstrap kubeadm:v0.4.99 \
	 --control-plane kubeadm:v0.4.99 \
	 --infrastructure docker:v0.4.99

	@kubectl kustomize $(abspath $(ROOT_DIR))/config | \
	  kubectl apply -f -
	 
	@kubectl --kubeconfig $(HOME)/.kube/config wait -n capi-system --for=condition=Available=true deployment/capi-controller-manager --timeout=4m
	@kubectl --kubeconfig $(HOME)/.kube/config wait -n capi-kubeadm-bootstrap-system --for=condition=Available=true deployment/capi-kubeadm-bootstrap-controller-manager --timeout=4m
	@kubectl --kubeconfig $(HOME)/.kube/config wait -n capi-kubeadm-control-plane-system --for=condition=Available=true deployment/capi-kubeadm-control-plane-controller-manager --timeout=4m
	@kubectl --kubeconfig $(HOME)/.kube/config wait -n capd-system --for=condition=Available=true deployment/capd-controller-manager --timeout=4m
	@kubectl --kubeconfig $(HOME)/.kube/config wait -n capk-system --for=condition=Available=true deployment/capk-controller-manager --timeout=4m

## --------------------------------------
## Tenant cluster creation
## --------------------------------------

.create-tenant-cluster-control-plane:
	@echo "\nCreating tenant cluster control plane..."
	@kubectl apply -f $(TEST_DIR)/resources/docker-control-plane.yaml
	@kubectl wait cluster km-cp --for=condition=Ready=true --timeout=3m
	@echo "\nCreated tenant cluster control plane:"
	@$(CAPI_PATH)/bin/clusterctl describe cluster km-cp

.create-tenant-cluster-hollow-nodes: 
	@echo "\nCreating tenant cluster hollow nodes..."
	@kubectl apply -f $(TEST_DIR)/resources/kubemark-machine-deployment.yaml
	@sleep 30
	@kubectl wait pods -l app=hollow-node --for condition=Ready --timeout=1m
	@echo "Created tenant cluster hollow nodes."
	
.generate-tenant-cluster-kubeconfig:
	@echo "\nGenerating tenant cluster kubeconfig..."
	@$(CAPI_PATH)/bin/clusterctl get kubeconfig km-cp > /tmp/km.kubeconfig
	@sed -i -e "s/server:.*/server: https:\/\/$$(docker port km-cp-lb 6443/tcp | sed "s/0.0.0.0/localhost/")/g" /tmp/km.kubeconfig
	@sed -i -e "s/certificate-authority-data:.*/insecure-skip-tls-verify: true/g" /tmp/km.kubeconfig
	@echo "\nGenerated tenant cluster kubeconfig:"
	@cat /tmp/km.kubeconfig

.tenant-cluster-info:
	@echo "\nTenant cluster is ready!"
	@echo "\nYou can access tenant cluster with the above kubeconfig. (It also can be found in '/tmp/km.kubeconfig')"
	@echo "\nFor CNI, you may apply calico with 'kubectl --kubeconfig /tmp/km.kubeconfig apply -f https://docs.projectcalico.org/v3.20/manifests/calico.yaml'"

.PHONY: test-e2e
test-e2e:	.start-kind-cluster \
	 	.install-cert-manager \
		.capi-build-clusterctl \
		.generate-clusterctl-config \
		.generate-manifests \
		.docker-build \
		.create-local-repository \
		.config-local-repository \
		.deploy-cluster-api \
		.create-tenant-cluster-control-plane \
		.create-tenant-cluster-hollow-nodes \
		.generate-tenant-cluster-kubeconfig \
		.tenant-cluster-info
