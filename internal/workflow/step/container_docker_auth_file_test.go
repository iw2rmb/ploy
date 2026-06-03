package step

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

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
