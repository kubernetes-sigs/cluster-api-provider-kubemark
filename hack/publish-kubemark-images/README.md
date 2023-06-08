# publish-kubemark-images

This is a tool for automating the process of building kubemark images and
pushing them to container registries. The minimum requirements for usage are
the Go language build toolchain, a local copy of the
[Kubernetes source code repository][k8s], and the [Docker][docker] or
[Podman][podman] container engine.

Build the tool by running `make` from within this directory.

Run the tool as follows:

```
publish-kubemark-images --kube-dir $HOME/opt/kubernetes --starting-version 1.25.0
```

This command will build every tagged version of Kubernetes from `1.25.0` through
the current release.

[k8s]: https://github.com/kubernetes/kubernetes
[docker]: https://github.com/docker/cli
[podman]: https://podman.io
