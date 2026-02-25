package step

import (
	"context"
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
				Image: "ploy-gate-gradle:jdk11",
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

// TestDockerContainerRuntimeEnvPassthrough verifies that ContainerSpec.Env is
// correctly converted to Docker's Env []string format and passed to the container.
// This test confirms the flattenEnv function works correctly and that env vars
// injected by the control plane (e.g., CA_CERTS_PEM_BUNDLE) reach the moby API.
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
				"CA_CERTS_PEM_BUNDLE": "-----BEGIN CERTIFICATE-----\ntest\n-----END CERTIFICATE-----",
				"CODEX_AUTH_JSON":     `{"token":"secret123"}`,
				"OPENAI_API_KEY":      "sk-test-key",
			},
			wantEnvKeys: []string{"CA_CERTS_PEM_BUNDLE", "CODEX_AUTH_JSON", "OPENAI_API_KEY"},
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
