```
   ________           __               ___    ____  ____
  / ____/ /_  _______/ /____  _____   /   |  / __ \/  _/
 / /   / / / / / ___/ __/ _ \/ ___/  / /| | / /_/ // /
/ /___/ / /_/ (__  ) /_/  __/ /     / ___ |/ ____// /
\____/_/\__,_/____/\__/\___/_/     /_/  |_/_/   /___/

    ____                  _     __
   / __ \_________ _   __(_)___/ /__  ______
  / /_/ / ___/ __ \ | / / / __  / _ \/ ___(_)
 / ____/ /  / /_/ / |/ / / /_/ /  __/ /  _
/_/   /_/   \____/|___/_/\__,_/\___/_/  (_)

    __ __      __                              __
   / //_/_  __/ /_  ___  ____ ___  ____ ______/ /__
  / ,< / / / / __ \/ _ \/ __ `__ \/ __ `/ ___/ //_/
 / /| / /_/ / /_/ /  __/ / / / / / /_/ / /  / ,<
/_/ |_\__,_/_.___/\___/_/ /_/ /_/\__,_/_/  /_/|_|

```

## What is the Cluster API Provider Kubemark

Cluster API Provider Kubemark (CAPK) is a provider for [Cluster
API][cluster_api] (CAPI) that allows users to deploy fake, [Kubemark][kubemark_docs]-backed machines to their
clusters. This is useful in a variety of scenarios, such load-testing and
simulation testing.

It is slightly similar to [CAPD][capd], the Docker
provider, in that it does not deploy actual infrastructure resources (VMs or
bare-metal machines). It differs significantly in that Kubemark nodes only
pretend to run pods scheduled to them. For more information on Kubemark, the
[Kubemark developer guide][kubemark_docs] has more details.

## Supported Versions

| Dependency  | Version |
|:------------|:-------:|
| kubernetes  |  v1.32  |
| cluster-api |  v1.10  |

## Getting started

**Prerequisites**
* Ubuntu Server 22.04
* clusterctl v1.1.4

At this point the Kubemark provider is extremely alpha. To deploy the Kubemark
provider, you can add the latest release to your clusterctl config file, by
default located at `~/.cluster-api/clusterctl.yaml`.

```yaml
providers:
- name: "kubemark"
  url: "https://github.com/kubernetes-sigs/cluster-api-provider-kubemark/releases/v0.6.0/infrastructure-components.yaml"
  type: "InfrastructureProvider"
```

For demonstration purposes, we'll use the [CAPD][capd] provider. Other
providers will also work, but CAPD is supported with a custom
[template](templates/cluster-template-capd.yaml) that makes deployment super
simple.

Initialize this provider into your cluster by running:

```bash
clusterctl init --infrastructure kubemark,docker
```

Once initialized, you'll need to deploy your workload cluster using the `capd`
flavor to get a hybrid CAPD/CAPK cluster:

```bash
export SERVICE_CIDR=["172.17.0.0/16"]
export POD_CIDR=["192.168.122.0/24"]
clusterctl generate cluster wow --infrastructure kubemark --flavor capd --kubernetes-version 1.32.8 --control-plane-machine-count=1 --worker-machine-count=4 | kubectl apply -f-
```

*Note: these CIDR values are specific to Ubuntu Server 22.04*

You should see your cluster come up and quickly become available with 4 Kubemark machines connected to your CAPD control plane.

To bring all the cluster nodes into a ready state you will need to deploy a CNI
solution into the kubemark cluster. Please see the [Cluster API Book](https://cluster-api.sigs.k8s.io/user/quick-start.html?highlight=cni#deploy-a-cni-solution)
for more information.

For other providers, you can either create a custom hybrid cluster template, or deploy the control plane and worker machines separately, specifiying the same cluster name:

```bash
clusterctl generate cluster wow --infrastructure aws      --kubernetes-version 1.32.2 --control-plane-machine-count=1 | kubectl apply -f-
clusterctl generate cluster wow --infrastructure kubemark --kubernetes-version 1.32.2 --worker-machine-count=4        | kubectl apply -f-
```

## Using tilt
To deploy the Kubemark provider, the recommended way at this time is using
[Tilt][tilt]. Clone this repo and use the [CAPI tilt guide][capi_tilt] to get
Tilt setup. Add `kubemark` to the list of providers in your
`tilt-settings.json` file and you should be off to the races.

## Hollow Node Resources
When using Kubernetes version 1.22 and greater, you may specify the resource
capacities advertised by the Kubemark Nodes.

By default, all nodes will report `1` CPU, and `4G` memory. These are [Kubernetes quantity][k8s_quantity_docs]
values and can be modified by adding fields to the KubemarkMachineTemplates you are using.
For example, to create a Node that advertises `32` CPU and `128G` memory, create a manifest as follows:

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha4
kind: KubemarkMachineTemplate
metadata:
  labels:
    cluster.x-k8s.io/cluster-name: mycluster
  name: kubemark-big-machine
  namespace: default
spec:
  template:
    spec:
      kubemarkOptions:
        extendedResources:
          cpu: "32"
          memory: "128G"
```

<!-- References -->

[capd]: https://github.com/kubernetes-sigs/cluster-api/tree/master/test/infrastructure/docker
[kubemark_docs]: https://github.com/kubernetes/community/blob/master/contributors/devel/sig-scalability/kubemark-guide.md
[cluster_api]: https://github.com/kubernetes-sigs/cluster-api
[tilt]: https://tilt.dev
[capi_tilt]: https://cluster-api.sigs.k8s.io/developer/tilt.html
[k8s_quantity_docs]: https://kubernetes.io/docs/reference/kubernetes-api/common-definitions/quantity/

## Running Integration (E2E) Test

### Prerequisites

To run this pipeline docker, kind, python must be installed.

You can run the E2E test with the following steps:

```bash
export KIND_CLUSTER_IMAGE=docker.io/kindest/node:v1.32.8
export CAPI_PATH=<path to cluster-api repository>
export ROOT_DIR=<path to cluster-api-provider-kubemark repository>
cd $(ROOT_DIR)
make test-e2e
```
