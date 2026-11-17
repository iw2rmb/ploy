package nodeagent

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

type imageUseDockerRecorder struct {
	*client.Client
	removals []client.ImageRemoveOptions
}

func (d *imageUseDockerRecorder) ImageRemove(ctx context.Context, image string, options client.ImageRemoveOptions) (client.ImageRemoveResult, error) {
	d.removals = append(d.removals, options)
	return d.Client.ImageRemove(ctx, image, options)
}

// Docker 29 resolves container identities and removes expired aliases while protecting recent use and references.
func TestCleanupImagesDockerLastUseAndAliases(t *testing.T) {
	if os.Getenv("PLOY_TEST_DOCKER_IMAGE_USAGE") != "1" {
		t.Skip("requires a disposable Docker 29 daemon labelled ploy.cleanup-image-test=true with registry:2 preloaded")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	docker, err := client.New(client.FromEnv)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = docker.Close() })
	info, err := docker.Info(ctx, client.InfoOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(info.Info.ServerVersion, "29.") || !slices.Contains(info.Info.Labels, "ploy.cleanup-image-test=true") {
		t.Fatal("refusing image cleanup outside the labelled disposable Docker 29 daemon")
	}
	t.Logf("Docker %s, storage driver %s, driver status %v, daemon %s", info.Info.ServerVersion, info.Info.Driver, info.Info.DriverStatus, info.Info.Name)
	command := func(args ...string) string {
		t.Helper()
		out, err := exec.CommandContext(ctx, "docker", append([]string{"--host", docker.DaemonHost()}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("docker %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	removeContainer := func(id string) {
		t.Cleanup(func() {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cleanupCancel()
			_, err := docker.ContainerRemove(cleanupCtx, id, client.ContainerRemoveOptions{Force: true, RemoveVolumes: true})
			if err != nil && !errdefs.IsNotFound(err) {
				t.Errorf("remove test container: %v", err)
			}
		})
	}
	name := fmt.Sprintf("ploy-image-usage-%d", time.Now().UnixNano())
	tags := []string{name + ":first", name + ":second"}
	archive := filepath.Join(t.TempDir(), "old-image.tar")
	architecture := info.Info.Architecture
	switch architecture {
	case "aarch64":
		architecture = "arm64"
	case "x86_64":
		architecture = "amd64"
	}
	writeImageUseArchive(t, archive, tags, architecture)
	command("load", "--input", archive)
	registry := command("run", "--pull=never", "-d", "--publish", "127.0.0.1::5000", "registry:2")
	removeContainer(registry)
	_, port, err := net.SplitHostPort(command("port", registry, "5000/tcp"))
	if err != nil {
		t.Fatal(err)
	}
	registryTag := "localhost:" + port + "/" + name + ":pushed"
	command("tag", tags[0], registryTag)
	command("push", registryTag)
	image, err := docker.ImageInspect(ctx, tags[0])
	if err != nil {
		t.Fatal(err)
	}
	created, err := time.Parse(time.RFC3339Nano, image.Created)
	if err != nil || created.Year() != 2000 || len(image.RepoTags) < 3 || len(image.RepoDigests) == 0 {
		t.Fatalf("old image with tags and digest aliases: created=%q tags=%v digests=%v error=%v", image.Created, image.RepoTags, image.RepoDigests, err)
	}
	listed, err := docker.ImageList(ctx, client.ImageListOptions{All: true})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range listed.Items {
		canonical, err := docker.ImageInspect(ctx, item.ID)
		if err != nil {
			t.Fatal(err)
		}
		found = found || canonical.ID == image.ID
	}
	if !found {
		t.Fatal("image listing did not resolve to the imported image identity")
	}
	id := command("create", tags[0], "/fixture")
	removeContainer(id)
	inspected, err := docker.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := docker.ImageInspect(ctx, inspected.Container.Image)
	if err != nil || canonical.ID != image.ID {
		t.Fatalf("container image %q resolved to %q, want %q: %v", inspected.Container.Image, canonical.ID, image.ID, err)
	}
	t.Logf("container.Image=%s resolves to canonical image %s; aliases=%v digests=%v", inspected.Container.Image, canonical.ID, image.RepoTags, image.RepoDigests)
	now := time.Now()
	r := &runController{imageLastUsed: map[string]time.Time{image.ID: now.Add(-25 * time.Hour)}}
	recorder := &imageUseDockerRecorder{Client: docker}
	if err := r.pruneCleanupImages(ctx, recorder, []container.InspectResponse{inspected.Container}, now, 24*time.Hour); err != nil {
		t.Fatal(err)
	}
	if len(recorder.removals) != 0 {
		t.Fatal("referenced image was removed")
	}
	command("rm", id)
	r.imageLastUsed[image.ID] = now.Add(-time.Hour)
	if err := r.pruneCleanupImages(ctx, recorder, nil, now, 24*time.Hour); err != nil {
		t.Fatal(err)
	}
	if len(recorder.removals) != 0 {
		t.Fatal("recently used old image was removed")
	}
	r.imageLastUsed[image.ID] = now.Add(-25 * time.Hour)
	if err := r.pruneCleanupImages(ctx, recorder, nil, now, 24*time.Hour); err != nil {
		t.Fatal(err)
	}
	if len(recorder.removals) < 3 {
		t.Fatalf("expected individual alias removal, got %d calls", len(recorder.removals))
	}
	for _, options := range recorder.removals {
		if options.Force || options.PruneChildren {
			t.Fatalf("unsafe image removal options: %+v", options)
		}
	}
	for _, ref := range append(append(image.RepoTags, image.RepoDigests...), image.ID) {
		if _, err := docker.ImageInspect(ctx, ref); !errdefs.IsNotFound(err) {
			t.Errorf("expired image alias %q remains: %v", ref, err)
		}
	}
}

func writeImageUseArchive(t *testing.T, path string, tags []string, architecture string) {
	t.Helper()
	layer := new(bytes.Buffer)
	writer := tar.NewWriter(layer)
	if err := writer.WriteHeader(&tar.Header{Name: "fixture", Mode: 0644, Size: 0}); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	config, err := json.Marshal(map[string]any{
		"created": "2000-01-01T00:00:00Z", "architecture": architecture, "os": "linux",
		"config":  map[string]any{"Cmd": []string{"/fixture"}},
		"rootfs":  map[string]any{"type": "layers", "diff_ids": []string{fmt.Sprintf("sha256:%x", sha256.Sum256(layer.Bytes()))}},
		"history": []map[string]string{{"created": "2000-01-01T00:00:00Z", "created_by": "image usage test"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := json.Marshal([]map[string]any{{"Config": "config.json", "RepoTags": tags, "Layers": []string{"layer.tar"}}})
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	archive := tar.NewWriter(file)
	for name, contents := range map[string][]byte{"config.json": config, "manifest.json": manifest, "layer.tar": layer.Bytes()} {
		if err := archive.WriteHeader(&tar.Header{Name: name, Mode: 0644, Size: int64(len(contents))}); err != nil {
			t.Fatal(err)
		}
		if _, err := archive.Write(contents); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
}
