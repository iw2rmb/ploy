package step

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/client"
)

// -----------------------------------------------------------------------------
// DockerContainerRuntime creation tests
// -----------------------------------------------------------------------------

// TestDockerContainerRuntimeCreate verifies container creation with moby client.
func TestDockerContainerRuntimeCreate(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name        string
		spec        ContainerSpec
		createRes   client.ContainerCreateResult
		createErr   error
		createPanic any
		pullImage   bool
		inspectErr  error
		pullErr     error
		wantErr     bool
		errContains string
	}{
		{
			name: "success_basic",
			spec: ContainerSpec{
				Image:      "alpine:latest",
				Command:    []string{"echo", "hello"},
				WorkingDir: "/app",
				Env:        map[string]string{"FOO": "bar"},
			},
			createRes: client.ContainerCreateResult{ID: "container123"},
			wantErr:   false,
		},
		{
			name: "success_with_mounts",
			spec: ContainerSpec{
				Image: "alpine:latest",
				Mounts: []ContainerMount{
					{Source: "/host/path", Target: "/container/path", ReadOnly: true},
				},
			},
			createRes: client.ContainerCreateResult{ID: "container456"},
			wantErr:   false,
		},
		{
			name: "success_with_resource_limits",
			spec: ContainerSpec{
				Image:            "alpine:latest",
				LimitNanoCPUs:    1000000000, // 1 CPU
				LimitMemoryBytes: 1073741824, // 1GB
			},
			createRes: client.ContainerCreateResult{ID: "container789"},
			wantErr:   false,
		},
		{
			name: "success_with_storage_opt",
			spec: ContainerSpec{
				Image:          "alpine:latest",
				StorageSizeOpt: "10G",
			},
			createRes: client.ContainerCreateResult{ID: "container-storage"},
			wantErr:   false,
		},
		{
			name: "error_empty_image",
			spec: ContainerSpec{
				Image: "",
			},
			wantErr:     true,
			errContains: "container image required",
		},
		{
			name: "error_whitespace_image",
			spec: ContainerSpec{
				Image: "   ",
			},
			wantErr:     true,
			errContains: "container image required",
		},
		{
			name: "error_create_fails",
			spec: ContainerSpec{
				Image: "alpine:latest",
			},
			createErr:   errors.New("daemon connection refused"),
			wantErr:     true,
			errContains: "create container",
		},
		{
			name: "error_create_panics",
			spec: ContainerSpec{
				Image: "alpine:latest",
			},
			createPanic: "json encoder panic",
			wantErr:     true,
			errContains: "create container panic",
		},
		{
			name: "success_with_image_pull",
			spec: ContainerSpec{
				Image: "alpine:latest",
			},
			createRes: client.ContainerCreateResult{ID: "pulled-container"},
			pullImage: true,
			// Simulate a missing image so PullImage triggers a registry pull.
			inspectErr: cerrdefs.ErrNotFound,
			wantErr:    false,
		},
		{
			name: "success_skip_pull_when_image_present",
			spec: ContainerSpec{
				Image: "gate-gradle:jdk11",
			},
			createRes: client.ContainerCreateResult{ID: "local-image-container"},
			pullImage: true,
			// Image is present locally, but a registry pull would fail (private/local tag).
			// Runtime should skip pull when inspect succeeds.
			pullErr: errors.New("pull access denied"),
			wantErr: false,
		},
		{
			name: "error_image_pull_fails",
			spec: ContainerSpec{
				Image: "private/image:latest",
			},
			pullImage:   true,
			inspectErr:  cerrdefs.ErrNotFound,
			pullErr:     errors.New("authentication required"),
			wantErr:     true,
			errContains: "pull image",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fake := &fakeDockerClient{
				createResult:    tc.createRes,
				createErr:       tc.createErr,
				createPanic:     tc.createPanic,
				imageInspectErr: tc.inspectErr,
				pullErr:         tc.pullErr,
			}
			rt := newDockerContainerRuntimeWithClient(fake, DockerContainerRuntimeOptions{
				PullImage: tc.pullImage,
			})

			handle, err := rt.Create(context.Background(), tc.spec)

			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tc.errContains != "" && !strings.Contains(err.Error(), tc.errContains) {
					t.Errorf("error %q should contain %q", err.Error(), tc.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(handle) != tc.createRes.ID {
				t.Errorf("got ID %q, want %q", string(handle), tc.createRes.ID)
			}
			// Verify image pull was called only when configured and the image is missing.
			expectPull := tc.pullImage && cerrdefs.IsNotFound(tc.inspectErr)
			if expectPull && !fake.pullCalled {
				t.Error("image pull should have been called")
			}
			if !expectPull && fake.pullCalled {
				t.Error("image pull should NOT have been called")
			}
		})
	}
}

func TestDockerContainerRuntimeCreate_ImagePullUsesRegistryAuth(t *testing.T) {
	t.Parallel()

	authJSON := `{"auths":{"ghcr.io":{"username":"octocat","password":"secret"}}}`
	fake := &fakeDockerClient{
		createResult:    client.ContainerCreateResult{ID: "pulled-container"},
		imageInspectErr: cerrdefs.ErrNotFound,
	}
	rt := newDockerContainerRuntimeWithClient(fake, DockerContainerRuntimeOptions{
		PullImage:              true,
		RegistryAuthConfigJSON: authJSON,
	})

	_, err := rt.Create(context.Background(), ContainerSpec{
		Image: "ghcr.io/iw2rmb/ploy/codex:latest",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !fake.pullCalled {
		t.Fatal("expected image pull to be called")
	}
	if strings.TrimSpace(fake.pullOpts.RegistryAuth) == "" {
		t.Fatal("expected image pull RegistryAuth to be populated")
	}

	decodedJSON, err := base64.URLEncoding.DecodeString(fake.pullOpts.RegistryAuth)
	if err != nil {
		t.Fatalf("decode registry auth: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(decodedJSON, &payload); err != nil {
		t.Fatalf("unmarshal registry auth payload: %v", err)
	}
	if got := payload["username"]; got != "octocat" {
		t.Fatalf("registry auth username = %v, want octocat", got)
	}
	if got := payload["password"]; got != "secret" {
		t.Fatalf("registry auth password = %v, want secret", got)
	}
	if got := payload["serveraddress"]; got != "ghcr.io" {
		t.Fatalf("registry auth serveraddress = %v, want ghcr.io", got)
	}
}

func TestDockerContainerRuntimeCreate_InvalidRegistryAuthConfig(t *testing.T) {
	t.Parallel()

	fake := &fakeDockerClient{
		createResult:    client.ContainerCreateResult{ID: "pulled-container"},
		imageInspectErr: cerrdefs.ErrNotFound,
	}
	rt := newDockerContainerRuntimeWithClient(fake, DockerContainerRuntimeOptions{
		PullImage:              true,
		RegistryAuthConfigJSON: `{"auths":`,
	})

	_, err := rt.Create(context.Background(), ContainerSpec{
		Image: "ghcr.io/iw2rmb/ploy/codex:latest",
	})
	if err == nil {
		t.Fatal("expected error for invalid registry auth config")
	}
	if !strings.Contains(err.Error(), "parse registry auth config") {
		t.Fatalf("error = %q, expected parse registry auth config", err.Error())
	}
	if fake.pullCalled {
		t.Fatal("did not expect image pull when auth config is invalid")
	}
}

func TestDockerContainerRuntimeCreate_DockerHubRegistryAuthAlias(t *testing.T) {
	t.Parallel()

	encodedUserPass := base64.StdEncoding.EncodeToString([]byte("hub-user:hub-token"))
	authJSON := `{"auths":{"https://index.docker.io/v1/":{"auth":"` + encodedUserPass + `"}}}`
	fake := &fakeDockerClient{
		createResult:    client.ContainerCreateResult{ID: "pulled-container"},
		imageInspectErr: cerrdefs.ErrNotFound,
	}
	rt := newDockerContainerRuntimeWithClient(fake, DockerContainerRuntimeOptions{
		PullImage:              true,
		RegistryAuthConfigJSON: authJSON,
	})

	_, err := rt.Create(context.Background(), ContainerSpec{
		Image: "alpine:3.20",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !fake.pullCalled {
		t.Fatal("expected image pull to be called")
	}
	if strings.TrimSpace(fake.pullOpts.RegistryAuth) == "" {
		t.Fatal("expected image pull RegistryAuth to be populated")
	}

	decodedJSON, err := base64.URLEncoding.DecodeString(fake.pullOpts.RegistryAuth)
	if err != nil {
		t.Fatalf("decode registry auth: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(decodedJSON, &payload); err != nil {
		t.Fatalf("unmarshal registry auth payload: %v", err)
	}
	if got := payload["username"]; got != "hub-user" {
		t.Fatalf("registry auth username = %v, want hub-user", got)
	}
	if got := payload["password"]; got != "hub-token" {
		t.Fatalf("registry auth password = %v, want hub-token", got)
	}
}

// TestDockerContainerRuntimeEnvPassthrough verifies that ContainerSpec.Env is
// correctly converted to Docker's Env []string format and passed to the container.
// This test confirms the flattenEnv function works correctly and that env vars
// injected by the control plane (e.g., PLOY_CA_CERTS) reach the moby API.
func TestDockerContainerRuntimeEnvPassthrough(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		env         map[string]string
		wantEnvKeys []string // expected keys (values checked separately)
	}{
		{
			name: "global_env_vars",
			env: map[string]string{
				"PLOY_CA_CERTS":  "-----BEGIN CERTIFICATE-----\ntest\n-----END CERTIFICATE-----",
				"CODEX_AUTH_JSON": `{"token":"secret123"}`,
				"OPENAI_API_KEY":  "sk-test-key",
			},
			wantEnvKeys: []string{"PLOY_CA_CERTS", "CODEX_AUTH_JSON", "OPENAI_API_KEY"},
		},
		{
			name: "mixed_env_vars",
			env: map[string]string{
				"CUSTOM_VAR":    "custom-value",
				"PATH_OVERRIDE": "/custom/bin:/usr/bin",
			},
			wantEnvKeys: []string{"CUSTOM_VAR", "PATH_OVERRIDE"},
		},
		{
			name:        "nil_env",
			env:         nil,
			wantEnvKeys: nil,
		},
		{
			name:        "empty_env",
			env:         map[string]string{},
			wantEnvKeys: nil,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fake := &fakeDockerClient{
				createResult: client.ContainerCreateResult{ID: "env-test-" + tc.name},
			}
			rt := newDockerContainerRuntimeWithClient(fake, DockerContainerRuntimeOptions{})

			spec := ContainerSpec{
				Image: "alpine:latest",
				Env:   tc.env,
			}

			_, err := rt.Create(context.Background(), spec)
			if err != nil {
				t.Fatalf("Create failed: %v", err)
			}

			if !fake.createCalled {
				t.Fatal("ContainerCreate was not called")
			}

			// Verify env was passed to moby Config.
			if fake.createOpts.Config == nil {
				t.Fatal("Config should not be nil")
			}

			gotEnv := fake.createOpts.Config.Env
			if tc.wantEnvKeys == nil {
				if len(gotEnv) != 0 {
					t.Errorf("expected no env vars, got %v", gotEnv)
				}
				return
			}

			// Check each expected key appears in the "KEY=value" format.
			for _, key := range tc.wantEnvKeys {
				found := false
				for _, envStr := range gotEnv {
					if strings.HasPrefix(envStr, key+"=") {
						found = true
						// Verify value matches.
						expectedVal := tc.env[key]
						expectedEntry := key + "=" + expectedVal
						if envStr != expectedEntry {
							t.Errorf("env entry for %s: got %q, want %q", key, envStr, expectedEntry)
						}
						break
					}
				}
				if !found {
					t.Errorf("expected env key %q to be present in %v", key, gotEnv)
				}
			}
		})
	}
}

// TestDockerContainerRuntimeNetworkMode verifies network option is applied.
func TestDockerContainerRuntimeNetworkMode(t *testing.T) {
	t.Parallel()
	fake := &fakeDockerClient{
		createResult: client.ContainerCreateResult{ID: "net-container"},
	}
	rt := newDockerContainerRuntimeWithClient(fake, DockerContainerRuntimeOptions{
		Network: "custom-network",
	})

	_, err := rt.Create(context.Background(), ContainerSpec{Image: "alpine:latest"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify network mode was set in host config.
	if fake.createOpts.HostConfig == nil {
		t.Fatal("HostConfig should not be nil")
	}
	if got := string(fake.createOpts.HostConfig.NetworkMode); got != "custom-network" {
		t.Errorf("NetworkMode = %q, want %q", got, "custom-network")
	}
}
