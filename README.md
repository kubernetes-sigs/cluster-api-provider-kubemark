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

## Getting started
At this point the Kubemark provider is extremely alpha. To deploy the Kubemark
provider, the recommended way at this time is using [Tilt][tilt]. Clone this
repo and use the [CAPI tilt guide][capi_tilt] to get Tilt setup. Add `kubemark`
to the list of providers in your `tilt-settings.json` file and you should be
off to the races.

<!-- References -->

[capd]: https://github.com/kubernetes-sigs/cluster-api/tree/master/test/infrastructure/docker
[kubemark_docs]: https://github.com/kubernetes/community/blob/master/contributors/devel/sig-scalability/kubemark-guide.md
[cluster_api]: https://github.com/kubernetes-sigs/cluster-api
[tilt]: https://tilt.dev
[capi_tilt]: https://master.cluster-api.sigs.k8s.io/developer/tilt.html
