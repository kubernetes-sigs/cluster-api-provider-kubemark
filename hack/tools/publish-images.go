package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"

	"github.com/Masterminds/semver"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

const (
	supportedVersions = ">= 1.17.0"
)

func dockerBuild(v *semver.Version, kubernetesDir string) error {
	cmd := exec.Command("git", "-C", kubernetesDir, "checkout", v.Original())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}

	cmd = exec.Command("make", "-C", kubernetesDir, "clean", "all", "WHAT=cmd/kubemark")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}

	dockerfilePath := path.Join(kubernetesDir, "cluster/images/kubemark")
	symlinkPath := path.Join(dockerfilePath, "kubemark")
	if err := os.Link(path.Join(kubernetesDir, "_output/bin/kubemark"), symlinkPath); err != nil {
		return err
	}
	defer func() {
		os.Remove(symlinkPath)
	}()
	cmd = exec.Command("docker", "build", "--pull",
		fmt.Sprintf("--tag=gcr.io/cf-london-servces-k8s/bmo/kubemark:%s", v.Original()),
		dockerfilePath,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func dockerPush() error {
	cmd := exec.Command("docker", "push", "gcr.io/cf-london-servces-k8s/bmo/kubemark")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func imageExists(v *semver.Version) (bool, error) {
	cmd := exec.Command("docker", "image", "inspect",
		fmt.Sprintf("gcr.io/cf-london-servces-k8s/bmo/kubemark:%s", v.Original()))
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			if exitError.ExitCode() == 1 {
				return false, nil
			}
		}
		return false, err
	}
	return true, nil
}

func main() {
	if len(os.Args) < 2 {
		log.Println("usage: publish-images <path to kubernetes directory>")
		os.Exit(1)
	}
	kubernetesDir := os.Args[1]
	constraint, err := semver.NewConstraint(supportedVersions)
	if err != nil {
		log.Fatalf("err parsing %s: %v", supportedVersions, err)
	}

	cmd := exec.Command("git", "-C", kubernetesDir, "fetch", "--tags")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatalf("err fetching remote tags: %v", err)
	}
	repo, err := name.NewRepository("k8s.gcr.io/kube-proxy")
	if err != nil {
		log.Fatalf("err parsing %v", err)
	}
	tags, err := remote.List(repo)
	if err != nil {
		log.Fatalf("err fetching tags %v", err)
	}
	for _, t := range tags {
		v, err := semver.NewVersion(t)
		if err != nil {
			log.Printf("err parsing %s, %v", t, err)
		}
		if constraint.Check(v) {
			exists, err := imageExists(v)
			if err != nil {
				log.Fatalf("err checking if image exists for %v: %v", v, err)
			}
			if exists {
				log.Printf("skipping version %s, image already exists", v.Original())
				continue
			}
			if err := dockerBuild(v, kubernetesDir); err != nil {
				log.Fatalf("err building docker image for %v: %v", v, err)
			}
		}
	}
	if err := dockerPush(); err != nil {
		log.Fatalf("err pushing docker image: %v", err)
	}
}
