module github.com/kubernetes-sigs/cluster-api-provider-kubemark

go 1.16

require (
	github.com/Masterminds/semver v1.5.0 // indirect
	github.com/go-logr/logr v0.4.0
	k8s.io/api v0.21.3
	k8s.io/apimachinery v0.22.0-alpha.0.0.20210417144234-8daf28983e6e
	k8s.io/client-go v0.21.3
	k8s.io/klog/v2 v2.9.0
	k8s.io/utils v0.0.0-20210722164352-7f3ee0f31471
	sigs.k8s.io/cluster-api v0.4.2
	sigs.k8s.io/controller-runtime v0.9.6
)
