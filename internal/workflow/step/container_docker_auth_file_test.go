package step

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/client"
)

// Create passes the auth file entry for the image's registry to the image pull.
func TestContainerRuntimeCreate_ImagePullRegistryAuth(t *testing.T) {
	t.Parallel()

	hubAuth := base64.StdEncoding.EncodeToString([]byte("hub-user:hub-token"))
	tests := []struct {
		name     string
		authJSON string
		image    string
		want     map[string]string
		wantErr  string
	}{
		{
			name:     "username and password for the image registry",
			authJSON: `{"auths":{"ghcr.io":{"username":"octocat","password":"secret"}}}`,
			image:    "ghcr.io/iw2rmb/ploy/amata-codex-java-17-maven:latest",
			want:     map[string]string{"username": "octocat", "password": "secret", "serveraddress": "ghcr.io"},
		},
		{
			name:     "Docker Hub URL key with base64 auth matches an unqualified image",
			authJSON: `{"auths":{"https://index.docker.io/v1/":{"auth":"` + hubAuth + `"}}}`,
			image:    "alpine:3.20",
			want:     map[string]string{"username": "hub-user", "password": "hub-token", "serveraddress": "docker.io"},
		},
		{
			name:     "invalid auth file fails before the pull",
			authJSON: `{"auths":`,
			image:    "ghcr.io/iw2rmb/ploy/amata-codex-java-17-maven:latest",
			wantErr:  "parse registry auth config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			authFile := filepath.Join(t.TempDir(), "docker-auth.json")
			if err := os.WriteFile(authFile, []byte(tt.authJSON), 0o600); err != nil {
				t.Fatalf("write auth file: %v", err)
			}
			fake := &fakeDockerClient{
				createResult:    client.ContainerCreateResult{ID: "pulled-container"},
				imageInspectErr: cerrdefs.ErrNotFound,
			}
			rt := newContainerRuntimeWithClient(fake, ContainerRuntimeOptions{
				PullImage:              true,
				RegistryAuthConfigFile: authFile,
			})

			_, err := rt.Create(context.Background(), ContainerSpec{Image: tt.image})
			if tt.wantErr != "" {
				requireErrContains(t, err, tt.wantErr)
				if fake.pullCalled {
					t.Fatal("image pull ran with an invalid auth file")
				}
				return
			}
			if err != nil {
				t.Fatalf("Create() error = %v", err)
			}
			got := decodeRegistryAuth(t, fake.pullOpts.RegistryAuth)
			for key, want := range tt.want {
				if got[key] != want {
					t.Errorf("registry auth %s = %v, want %s", key, got[key], want)
				}
			}
		})
	}
}

func TestContainerRuntimeRegistryAuthFileReloadsPerPull(t *testing.T) {
	t.Parallel()

	authFile := filepath.Join(t.TempDir(), "docker-auth.json")
	writeAuthFile(t, authFile, "first-user", "first-token")

	rt := newContainerRuntimeWithClient(&fakeDockerClient{}, ContainerRuntimeOptions{
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

func decodeRegistryAuthForImage(t *testing.T, rt *containerRuntime, imageRef string) map[string]any {
	t.Helper()
	encoded, err := rt.registryAuthForImage(imageRef)
	if err != nil {
		t.Fatalf("registryAuthForImage() error = %v", err)
	}
	return decodeRegistryAuth(t, encoded)
}

func decodeRegistryAuth(t *testing.T, encoded string) map[string]any {
	t.Helper()
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
