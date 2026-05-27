package step

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	containertypes "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

func TestDockerContainerRuntimeRegistryAuthFileReloadsPerPull(t *testing.T) {
	t.Parallel()

	authFile := filepath.Join(t.TempDir(), "docker-auth.json")
	writeAuthFile(t, authFile, "first-user", "first-token")

	rt := newDockerContainerRuntimeWithClient(&fakeDockerClient{}, DockerContainerRuntimeOptions{
		RegistryAuthConfigFile: authFile,
	})

	first := decodeRegistryAuthForImage(t, rt, "ghcr.io/acme/image:latest")
	if first["username"] != "first-user" {
		t.Fatalf("first username = %v, want first-user", first["username"])
	}

	writeAuthFile(t, authFile, "second-user", "second-token")
	second := decodeRegistryAuthForImage(t, rt, "ghcr.io/acme/image:latest")
	if second["username"] != "second-user" {
		t.Fatalf("second username = %v, want second-user", second["username"])
	}
}

func TestDockerContainerRuntimeCreate_RefreshesAuthAndRetriesOnceOnUnauthorized(t *testing.T) {
	t.Parallel()

	authFile := filepath.Join(t.TempDir(), "docker-auth.json")
	writeAuthFile(t, authFile, "octocat", "secret")

	fake := &fakeDockerClient{
		createResult: client.ContainerCreateResult{ID: "container-after-refresh"},
		pullErrs: []error{
			errors.New("unauthorized: authentication required"),
			nil,
		},
		containerListResults: []client.ContainerListResult{
			{Items: []containertypes.Summary{{ID: "helper123", Names: []string{"/ploy-node-auth-helper"}}}},
		},
		execCreateResult:  client.ExecCreateResult{ID: "exec123"},
		execAttachOutput:  "refreshed\n",
		execInspectResult: client.ExecInspectResult{ExitCode: 0},
	}
	rt := newDockerContainerRuntimeWithClient(fake, DockerContainerRuntimeOptions{
		PullImage:                    true,
		RegistryAuthConfigFile:       authFile,
		RegistryAuthRefreshContainer: "ploy-node-auth-helper",
	})

	handle, err := rt.Create(context.Background(), ContainerSpec{Image: "ghcr.io/acme/private:latest"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if handle != "container-after-refresh" {
		t.Fatalf("handle = %q, want container-after-refresh", handle)
	}
	if fake.pullCalls != 2 {
		t.Fatalf("ImagePull calls = %d, want 2", fake.pullCalls)
	}
	if !fake.execCreateCalled {
		t.Fatal("expected auth refresh exec to be created")
	}
	if fake.execContainer != "helper123" {
		t.Fatalf("exec container = %q, want helper123", fake.execContainer)
	}
	wantCmd := []string{registryAuthRefreshCommand, "refresh-for-pull", "ghcr.io/acme/private:latest"}
	if len(fake.execCreateOpts.Cmd) != len(wantCmd) {
		t.Fatalf("refresh command = %#v, want %#v", fake.execCreateOpts.Cmd, wantCmd)
	}
	for i := range wantCmd {
		if fake.execCreateOpts.Cmd[i] != wantCmd[i] {
			t.Fatalf("refresh command = %#v, want %#v", fake.execCreateOpts.Cmd, wantCmd)
		}
	}
}

func writeAuthFile(t *testing.T, path, username, password string) {
	t.Helper()
	data := map[string]any{
		"auths": map[string]any{
			"ghcr.io": map[string]any{
				"username": username,
				"password": password,
			},
		},
	}
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal auth file: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write auth file: %v", err)
	}
}

func decodeRegistryAuthForImage(t *testing.T, rt *DockerContainerRuntime, imageRef string) map[string]any {
	t.Helper()
	encoded, err := rt.registryAuthForImage(imageRef)
	if err != nil {
		t.Fatalf("registryAuthForImage() error = %v", err)
	}
	decoded, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode registry auth: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(decoded, &payload); err != nil {
		t.Fatalf("unmarshal registry auth: %v", err)
	}
	return payload
}
