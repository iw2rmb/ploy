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
// containerRuntime creation tests
// -----------------------------------------------------------------------------

// TestContainerRuntimeCreate verifies container creation with moby client.
func TestContainerRuntimeCreate(t *testing.T) {
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
		wantPulls   int
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
			createRes:  client.ContainerCreateResult{ID: "pulled-container"},
			pullImage:  true,
			inspectErr: cerrdefs.ErrNotFound,
			wantErr:    false,
		},
		{
			name: "success_pull_even_when_image_present",
			spec: ContainerSpec{
				Image: "gate-gradle:latest",
			},
			createRes: client.ContainerCreateResult{ID: "local-image-container"},
			pullImage: true,
			wantErr:   false,
		},
		{
			name: "error_image_pull_fails",
			spec: ContainerSpec{
				Image: "private/image:latest",
			},
			pullImage:   true,
			inspectErr:  cerrdefs.ErrNotFound,
			pullErr:     errors.New("authentication required"),
			wantPulls:   1,
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
			rt := newContainerRuntimeWithClient(fake, ContainerRuntimeOptions{
				PullImage: tc.pullImage,
			})

			handle, err := rt.Create(context.Background(), tc.spec)

			if tc.wantErr {
				requireErrContains(t, err, tc.errContains)
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(handle) != tc.createRes.ID {
				t.Errorf("got ID %q, want %q", string(handle), tc.createRes.ID)
			}
			// HostConfig.AutoRemove must stay false so logs can be retrieved after exit.
			if fake.createOpts.HostConfig == nil || fake.createOpts.HostConfig.AutoRemove {
				t.Error("HostConfig.AutoRemove should be false for log retrieval")
			}
			// Every mount we configure should reach moby as a bind mount.
			if len(tc.spec.Mounts) > 0 {
				gotMounts := fake.createOpts.HostConfig.Mounts
				if len(gotMounts) != len(tc.spec.Mounts) {
					t.Fatalf("mounts: got %d, want %d", len(gotMounts), len(tc.spec.Mounts))
				}
				for i, m := range gotMounts {
					if string(m.Type) != "bind" {
						t.Errorf("Mount[%d].Type=%q, want %q", i, m.Type, "bind")
					}
				}
			}
			// Verify image pull was called whenever configured.
			expectPull := tc.pullImage
			if expectPull && !fake.pullCalled {
				t.Error("image pull should have been called")
			}
			if !expectPull && fake.pullCalled {
				t.Error("image pull should NOT have been called")
			}
			if tc.wantPulls > 0 && fake.pullCalls != tc.wantPulls {
				t.Fatalf("image pull calls = %d, want %d", fake.pullCalls, tc.wantPulls)
			}
		})
	}
}

func TestContainerRuntimeCreate_ImagePullUsesRegistryAuth(t *testing.T) {
	t.Parallel()

	authJSON := `{"auths":{"ghcr.io":{"username":"octocat","password":"secret"}}}`
	fake := &fakeDockerClient{
		createResult:    client.ContainerCreateResult{ID: "pulled-container"},
		imageInspectErr: cerrdefs.ErrNotFound,
	}
	rt := newContainerRuntimeWithClient(fake, ContainerRuntimeOptions{
		PullImage:              true,
		RegistryAuthConfigJSON: authJSON,
	})

	_, err := rt.Create(context.Background(), ContainerSpec{
		Image: "ghcr.io/iw2rmb/ploy/amata-codex-java-17-maven:latest",
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

func TestContainerRuntimeCreate_InvalidRegistryAuthConfig(t *testing.T) {
	t.Parallel()

	fake := &fakeDockerClient{
		createResult:    client.ContainerCreateResult{ID: "pulled-container"},
		imageInspectErr: cerrdefs.ErrNotFound,
	}
	rt := newContainerRuntimeWithClient(fake, ContainerRuntimeOptions{
		PullImage:              true,
		RegistryAuthConfigJSON: `{"auths":`,
	})

	_, err := rt.Create(context.Background(), ContainerSpec{
		Image: "ghcr.io/iw2rmb/ploy/amata-codex-java-17-maven:latest",
	})
	requireErrContains(t, err, "parse registry auth config")
	if fake.pullCalled {
		t.Fatal("did not expect image pull when auth config is invalid")
	}
}

func TestContainerRuntimeCreate_DockerHubRegistryAuthAlias(t *testing.T) {
	t.Parallel()

	encodedUserPass := base64.StdEncoding.EncodeToString([]byte("hub-user:hub-token"))
	authJSON := `{"auths":{"https://index.docker.io/v1/":{"auth":"` + encodedUserPass + `"}}}`
	fake := &fakeDockerClient{
		createResult:    client.ContainerCreateResult{ID: "pulled-container"},
		imageInspectErr: cerrdefs.ErrNotFound,
	}
	rt := newContainerRuntimeWithClient(fake, ContainerRuntimeOptions{
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

// TestContainerRuntimeEnvPassthrough verifies that ContainerSpec.Env is
// correctly converted to Docker's Env []string format and passed to the container.
// This test confirms the flattenEnv function works correctly and that env vars
// injected by the control plane reach the moby API.
func TestContainerRuntimeEnvPassthrough(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		env         map[string]string
		wantEnvKeys []string // expected keys (values checked separately)
	}{
		{
			name: "global_env_vars",
			env: map[string]string{
				"EXAMPLE_SECRET":  "value",
				"CODEX_AUTH_JSON": `{"token":"secret123"}`,
				"OPENAI_API_KEY":  "sk-test-key",
			},
			wantEnvKeys: []string{"EXAMPLE_SECRET", "CODEX_AUTH_JSON", "OPENAI_API_KEY"},
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
			rt := newContainerRuntimeWithClient(fake, ContainerRuntimeOptions{})

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

// TestContainerRuntimeNetworkMode verifies network option is applied.
func TestContainerRuntimeNetworkMode(t *testing.T) {
	t.Parallel()
	fake := &fakeDockerClient{
		createResult: client.ContainerCreateResult{ID: "net-container"},
	}
	rt := newContainerRuntimeWithClient(fake, ContainerRuntimeOptions{
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
