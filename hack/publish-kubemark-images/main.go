/*
Copyright 2020 The Kubernetes Authors.

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
	"github.com/spf13/pflag"
)

type PublishImages struct {
	ImageName       string
	KubeDir         string
	StartingVersion string
	Push            bool
}

func bindFlags(p *PublishImages) error {
	flags := pflag.NewFlagSet("publish-images", pflag.ExitOnError)
	flags.StringVar(&p.ImageName, "image-name", "kubemark", "name for the image to be tagged")
	flags.StringVar(&p.KubeDir, "kube-dir", "", "path to your kubernetes checkout")
	flags.StringVar(&p.StartingVersion, "starting-version", "1.17.0", "version (inclusive) from which to start building images")
	flags.BoolVar(&p.Push, "push", false, "whether the script should push the images after building")
	return flags.Parse(os.Args)
}

func main() {
	p := &PublishImages{}
	if err := bindFlags(p); err != nil {
		log.Fatalf("failed to parse flags: %v", err)
	}
	if p.KubeDir == "" {
		log.Fatalf("--kube-dir must be set")
	}

	constraint, err := semver.NewConstraint(fmt.Sprintf(">= %s", p.StartingVersion))
	if err != nil {
		log.Fatalf("error parsing starting version semver %s: %v", p.StartingVersion, err)
	}

	cmd := exec.Command("git", "-C", p.KubeDir, "fetch", "--tags")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatalf("error fetching git tags from kubernetes repository: %v", err)
	}

	// use the kube-proxy image for a convenient way to get k8s version tags
	repo, err := name.NewRepository("registry.k8s.io/kube-proxy")
	if err != nil {
		log.Fatalf("error parsing registry.k8s.io/kube-proxy: %v", err)
	}

	// build a list of version tags from the known kube-proxy images
	tags, err := remote.List(repo)
	if err != nil {
		log.Fatalf("error fetching image tags from registry.k8s.io/kube-proxy: %v", err)
	}

	for _, t := range tags {
		v, err := semver.NewVersion(t)
		if err != nil {
			log.Printf("skipping image build for unrecognized semver %s", t)
			continue
		}

		if constraint.Check(v) {
			exists, err := p.imageExists(v)
			if err != nil {
				log.Fatalf("error checking if image exists for %v: %v", v, err)
			}
			if exists {
				log.Printf("skipping image build for version %s, image already exists", v.Original())
			} else {
				if err := p.dockerBuild(v); err != nil {
					log.Fatalf("error building image for %v: %v", v, err)
				}
			}
			if err := p.dockerPush(v); err != nil {
				log.Fatalf("error pushing image: %v", err)
			}
		}
	}
}

func (p *PublishImages) dockerBuild(v *semver.Version) error {
	cmd := exec.Command("git", "-C", p.KubeDir, "fetch", "--tags")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}

	cmd = exec.Command("git", "-C", p.KubeDir, "checkout", v.Original())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}

	cmd = exec.Command("make", "-C", p.KubeDir, "clean", "all", "WHAT=cmd/kubemark", "CGO_ENABLED=0")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}

	dockerfilePath := path.Join(p.KubeDir, "cluster/images/kubemark")
	symlinkPath := path.Join(dockerfilePath, "kubemark")
	if err := os.Link(path.Join(p.KubeDir, "_output/bin/kubemark"), symlinkPath); err != nil {
		return err
	}
	defer func() {
		os.Remove(symlinkPath)
	}()
	cmd = exec.Command("docker", "build", "--pull",
		fmt.Sprintf("--tag=%s:%s", p.ImageName, v.Original()),
		dockerfilePath,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (p *PublishImages) dockerPush(v *semver.Version) error {
	if !p.Push {
		return nil
	}
	cmd := exec.Command("docker", "push", fmt.Sprintf("%s:%s", p.ImageName, v.Original()))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (p *PublishImages) imageExists(v *semver.Version) (bool, error) {
	cmd := exec.Command("docker", "image", "inspect",
		fmt.Sprintf("%s:%s", p.ImageName, v.Original()))
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			if exitError.ExitCode() == 1 || exitError.ExitCode() == 125 {
				return false, nil
			}
		}
		return false, err
	}
	return true, nil
}
