package step

import (
	"context"
	"errors"
	"strings"
	"testing"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/jsonstream"
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

func TestContainerRuntimeCreate_ImagePullStreamErrorStopsBeforeCreate(t *testing.T) {
	t.Parallel()

	fake := &fakeDockerClient{
		createResult: client.ContainerCreateResult{ID: "stale-image-container"},
		pullMessages: []jsonstream.Message{
			{Status: "Pulling image"},
			{Error: &jsonstream.Error{Message: "manifest unknown"}},
		},
	}
	rt := newContainerRuntimeWithClient(fake, ContainerRuntimeOptions{
		PullImage: true,
	})

	_, err := rt.Create(context.Background(), ContainerSpec{
		Image: "registry.example/ploy/amata:latest",
	})

	requireErrContains(t, err, "pull image")
	requireErrContains(t, err, "manifest unknown")
	if fake.createCalled {
		t.Fatal("container creation must not continue after a streamed pull error")
	}
}

func TestContainerRuntimeCreate_DockerSocketMountGetsRegistryAuthConfig(t *testing.T) {
	t.Parallel()

	const (
		dockerSocketTarget = "/var/run/docker.sock"
		authConfigFile     = "/etc/ploy/docker-auth-config/config.json"
		authConfigDir      = "/etc/ploy/docker-auth-config"
		authConfigTarget   = "/root/.docker"
	)

	testCases := []struct {
		name                   string
		registryAuthConfigFile string
		mounts                 []ContainerMount
		wantAuthMount          bool
		wantAuthSource         string
		wantAuthMountCount     int
	}{
		{
			name:                   "socket mount gets auth config",
			registryAuthConfigFile: authConfigFile,
			mounts: []ContainerMount{
				{Source: dockerSocketTarget, Target: dockerSocketTarget, ReadOnly: false},
			},
			wantAuthMount:      true,
			wantAuthSource:     authConfigDir,
			wantAuthMountCount: 1,
		},
		{
			name:                   "no socket mount does not get auth config",
			registryAuthConfigFile: authConfigFile,
			mounts: []ContainerMount{
				{Source: "/workspace", Target: "/workspace", ReadOnly: false},
			},
			wantAuthMount:      false,
			wantAuthMountCount: 0,
		},
		{
			name: "socket mount without configured auth config does not get auth config",
			mounts: []ContainerMount{
				{Source: dockerSocketTarget, Target: dockerSocketTarget, ReadOnly: false},
			},
			wantAuthMount:      false,
			wantAuthMountCount: 0,
		},
		{
			name:                   "explicit auth dir mount is preserved",
			registryAuthConfigFile: authConfigFile,
			mounts: []ContainerMount{
				{Source: dockerSocketTarget, Target: dockerSocketTarget, ReadOnly: false},
				{Source: "/custom/docker", Target: authConfigTarget, ReadOnly: true},
			},
			wantAuthMount:      true,
			wantAuthSource:     "/custom/docker",
			wantAuthMountCount: 1,
		},
		{
			name:                   "explicit auth config file mount is preserved",
			registryAuthConfigFile: authConfigFile,
			mounts: []ContainerMount{
				{Source: dockerSocketTarget, Target: dockerSocketTarget, ReadOnly: false},
				{Source: "/custom/config.json", Target: "/root/.docker/config.json", ReadOnly: true},
			},
			wantAuthMount:      false,
			wantAuthMountCount: 0,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fake := &fakeDockerClient{
				createResult: client.ContainerCreateResult{ID: "docker-auth-mount"},
			}
			rt := newContainerRuntimeWithClient(fake, ContainerRuntimeOptions{
				RegistryAuthConfigFile: tc.registryAuthConfigFile,
			})
			rt.startDockerProxy = func(context.Context, []ContainerMount, string, string, DockerJobOwner) (*dockerSocketProxy, error) {
				return &dockerSocketProxy{socketPath: "/job/docker.sock", close: func() error { return nil }}, nil
			}
			tc.mounts = append(append([]ContainerMount(nil), tc.mounts...), ContainerMount{Source: t.TempDir(), Target: "/tmp"})
			t.Cleanup(func() { _ = rt.Close() })

			_, err := rt.Create(context.Background(), ContainerSpec{
				Image: "alpine:latest", Labels: testDockerJobOwner.labels(),
				Mounts: tc.mounts,
			})
			if err != nil {
				t.Fatalf("Create() error = %v", err)
			}

			var authMounts []string
			var gotReadOnly bool
			for _, m := range fake.createOpts.HostConfig.Mounts {
				if m.Target == authConfigTarget {
					authMounts = append(authMounts, m.Source)
					gotReadOnly = m.ReadOnly
				}
			}
			if len(authMounts) != tc.wantAuthMountCount {
				t.Fatalf("auth mount count = %d, want %d; mounts=%+v", len(authMounts), tc.wantAuthMountCount, fake.createOpts.HostConfig.Mounts)
			}
			if !tc.wantAuthMount {
				return
			}
			if authMounts[0] != tc.wantAuthSource {
				t.Fatalf("auth mount source = %q, want %q", authMounts[0], tc.wantAuthSource)
			}
			if !gotReadOnly {
				t.Fatal("auth config mount should be read-only")
			}
		})
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
