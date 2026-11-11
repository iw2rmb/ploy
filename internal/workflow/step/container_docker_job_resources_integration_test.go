//go:build dockerintegration

package step

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

// Run on Linux with the test binary and TMPDIR mounted at identical host paths.
// PLOY_DOCKER_TEST_IMAGE must already exist on this disposable Docker host.
func TestDockerJobResourcesLive(t *testing.T) {
	image := os.Getenv("PLOY_DOCKER_TEST_IMAGE")
	if image == "" {
		t.Fatal("PLOY_DOCKER_TEST_IMAGE is required")
	}
	upstream := strings.TrimPrefix(os.Getenv("DOCKER_HOST"), "unix://")
	if upstream == "" {
		upstream = "/var/run/docker.sock"
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	host, err := client.New(client.WithHost("unix://" + upstream))
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	for _, ending := range []string{"success", "cancelled"} {
		t.Run(ending, func(t *testing.T) {
			// Nested clients must create owned siblings which disappear at job teardown.
			owner := DockerJobOwner{RunID: types.NewRunID(), JobID: types.NewJobID()}
			root := t.TempDir()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			proxy, err := startDockerSocketProxy(ctx, []ContainerMount{{Source: upstream, Target: "/var/run/docker.sock"}, {Source: executable, Target: "/smoke/step.test"}}, upstream, root, owner)
			if err != nil {
				t.Fatal(err)
			}
			defer proxy.Close()
			jobs, err := client.New(client.WithHost("unix://" + proxy.socketPath))
			if err != nil {
				t.Fatal(err)
			}
			defer jobs.Close()
			name := "ploy-owner-" + owner.JobID.String()
			vol, err := jobs.VolumeCreate(ctx, client.VolumeCreateOptions{Name: name})
			if err != nil {
				t.Fatal(err)
			}
			net, err := jobs.NetworkCreate(ctx, name, client.NetworkCreateOptions{})
			if err != nil {
				t.Fatal(err)
			}
			created, err := jobs.ContainerCreate(ctx, client.ContainerCreateOptions{
				Config:           &container.Config{Image: image, Entrypoint: []string{"/smoke/step.test"}, Cmd: []string{"-test.run=^TestDockerJobResourceLiveHelper$"}, Env: []string{"PLOY_DOCKER_TEST_IMAGE=" + image, "PLOY_DOCKER_SMOKE_MODE=parent"}},
				HostConfig:       &container.HostConfig{Binds: []string{"/smoke/step.test:/smoke/step.test:ro", "/var/run/docker.sock:/var/run/docker.sock", vol.Volume.Name + ":/smoke-volume"}},
				NetworkingConfig: &network.NetworkingConfig{EndpointsConfig: map[string]*network.EndpointSettings{name: {NetworkID: net.ID}}},
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := jobs.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(20 * time.Second)
			for {
				resources, err := listDockerJobResources(ctx, host)
				if err != nil {
					t.Fatal(err)
				}
				count := 0
				for _, r := range resources {
					if owner.owns(r.labels) && r.kind == "container" {
						count++
					}
				}
				if count == 2 {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("nested client did not create owned container; found %d", count)
				}
				time.Sleep(50 * time.Millisecond)
			}
			if ending == "cancelled" {
				cancel()
			}
			if err := proxy.Close(); err != nil {
				t.Fatal(err)
			}
			resources, err := listDockerJobResources(context.Background(), host)
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range resources {
				if owner.owns(r.labels) {
					t.Fatalf("resource survived teardown: %s %s", r.kind, r.id)
				}
			}
			if _, err := os.Stat(filepath.Dir(proxy.socketPath)); !os.IsNotExist(err) {
				t.Fatalf("proxy directory remains: %v", err)
			}
		})
	}
}

func TestDockerJobResourceLiveHelper(t *testing.T) {
	mode := os.Getenv("PLOY_DOCKER_SMOKE_MODE")
	if mode == "" {
		t.Skip("child process only")
	}
	if mode == "parent" {
		docker, err := client.New(client.WithHost("unix:///var/run/docker.sock"))
		if err != nil {
			t.Fatal(err)
		}
		defer docker.Close()
		child, err := docker.ContainerCreate(context.Background(), client.ContainerCreateOptions{
			Config:     &container.Config{Image: os.Getenv("PLOY_DOCKER_TEST_IMAGE"), Entrypoint: []string{"/smoke/step.test"}, Cmd: []string{"-test.run=^TestDockerJobResourceLiveHelper$"}, Env: []string{"PLOY_DOCKER_SMOKE_MODE=leaf"}},
			HostConfig: &container.HostConfig{Binds: []string{"/smoke/step.test:/smoke/step.test:ro", "/var/run/docker.sock:/var/run/docker.sock"}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := docker.ContainerStart(context.Background(), child.ID, client.ContainerStartOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(time.Hour)
}
