# Release Process

The Kubernetes Template Project is released on an as-needed basis. The process is as follows:

1. An issue is proposing a new release with a changelog since the last release
1. All [OWNERS](OWNERS) must LGTM this release
1. Before tagging a new release, ensure that the `metadata.yaml` and `clusterctl-settings.json` files are up to date
1. An OWNER runs `git tag -s $VERSION` and inserts the changelog and pushes the tag with `git push $VERSION`
1. The release issue is closed
1. An announcement email is sent to `kubernetes-dev@googlegroups.com` with the subject `[ANNOUNCE] kubernetes-template-project $VERSION is released`
