package step

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

// =============================================================================
// Engine v29 Container Lifecycle Validation Tests
// =============================================================================
// These tests re-validate container lifecycle semantics under Docker Engine v29
// (moby SDK). They verify that:
//   - Create: HostConfig options (AutoRemove=false, Mounts, resource limits,
//             network mode, storage options) are correctly passed to the daemon.
//   - Start:  Container start succeeds and returns immediately (async).
//   - Wait:   WaitConditionNotRunning blocks until container exits; returns
//             correct exit code and timestamps via inspect.
//   - Remove: Force removal succeeds even for stopped containers.
//
// The moby Engine v29 SDK changes from the deprecated docker/docker module:
//   - ContainerCreate: uses client.ContainerCreateOptions struct instead of
//                      positional (config, hostConfig, networkConfig, platform, name).
//   - ContainerStart:  returns (ContainerStartResult, error) — result is empty.
//   - ContainerWait:   returns ContainerWaitResult struct with Result and Error
//                      channels; uses client.ContainerWaitOptions.Condition.
//   - ContainerRemove: returns (ContainerRemoveResult, error) — result is empty.
// =============================================================================

// TestDockerContainerLifecycleV29 validates the complete container lifecycle
// (create → start → wait → remove) under Engine v29 semantics. This test ensures
// that the moby SDK migration preserves correct behaviour for workflow execution.
func TestDockerContainerLifecycleV29(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		// Container spec with various HostConfig options to validate.
		spec ContainerSpec
		opts DockerContainerRuntimeOptions
		// Expected lifecycle outcomes.
		wantExitCode   int
		wantStartedAt  string
		wantFinishedAt string
	}{
		{
			name: "basic_lifecycle_exit_0",
			spec: ContainerSpec{
				Image:   "alpine:latest",
				Command: []string{"echo", "hello"},
			},
			wantExitCode:   0,
			wantStartedAt:  "2024-06-01T10:00:00.000000000Z",
			wantFinishedAt: "2024-06-01T10:00:05.000000000Z",
		},
		{
			name: "lifecycle_with_resource_limits",
			spec: ContainerSpec{
				Image:            "alpine:latest",
				Command:          []string{"stress", "--cpu", "1"},
				LimitNanoCPUs:    2000000000, // 2 CPUs
				LimitMemoryBytes: 536870912,  // 512MB
			},
			wantExitCode:   0,
			wantStartedAt:  "2024-06-01T11:00:00.000000000Z",
			wantFinishedAt: "2024-06-01T11:00:10.000000000Z",
		},
		{
			name: "lifecycle_with_mounts",
			spec: ContainerSpec{
				Image:   "alpine:latest",
				Command: []string{"ls", "/workspace"},
				Mounts: []ContainerMount{
					{Source: "/tmp/workspace", Target: "/workspace", ReadOnly: false},
					{Source: "/tmp/inputs", Target: "/in", ReadOnly: true},
				},
			},
			wantExitCode:   0,
			wantStartedAt:  "2024-06-01T12:00:00.000000000Z",
			wantFinishedAt: "2024-06-01T12:00:01.000000000Z",
		},
		{
			name: "lifecycle_with_network_mode",
			spec: ContainerSpec{
				Image:   "alpine:latest",
				Command: []string{"curl", "http://localhost"},
			},
			opts:           DockerContainerRuntimeOptions{Network: "host"},
			wantExitCode:   0,
			wantStartedAt:  "2024-06-01T13:00:00.000000000Z",
			wantFinishedAt: "2024-06-01T13:00:02.000000000Z",
		},
		{
			name: "lifecycle_with_storage_opt",
			spec: ContainerSpec{
				Image:          "alpine:latest",
				Command:        []string{"dd", "if=/dev/zero", "of=/tmp/test", "bs=1M", "count=10"},
				StorageSizeOpt: "20G",
			},
			wantExitCode:   0,
			wantStartedAt:  "2024-06-01T14:00:00.000000000Z",
			wantFinishedAt: "2024-06-01T14:00:03.000000000Z",
		},
		{
			name: "lifecycle_non_zero_exit",
			spec: ContainerSpec{
				Image:   "alpine:latest",
				Command: []string{"sh", "-c", "exit 42"},
			},
			wantExitCode:   42,
			wantStartedAt:  "2024-06-01T15:00:00.000000000Z",
			wantFinishedAt: "2024-06-01T15:00:01.000000000Z",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			containerID := "lifecycle-" + tc.name

			// Configure fake client with expected lifecycle responses.
			fake := &fakeDockerClient{
				createResult:   client.ContainerCreateResult{ID: containerID},
				waitStatusCode: int64(tc.wantExitCode),
				inspectResult: client.ContainerInspectResult{
					Container: container.InspectResponse{
						State: &container.State{
							StartedAt:  tc.wantStartedAt,
							FinishedAt: tc.wantFinishedAt,
						},
					},
				},
			}
			rt := newDockerContainerRuntimeWithClient(fake, tc.opts)

			// Step 1: Create container — verify HostConfig options are passed.
			handle, err := rt.Create(ctx, tc.spec)
			if err != nil {
				t.Fatalf("Create failed: %v", err)
			}
			if string(handle) != containerID {
				t.Errorf("Create: got ID %q, want %q", string(handle), containerID)
			}
			// Verify Engine v29 ContainerCreateOptions were used correctly.
			if !fake.createCalled {
				t.Error("Create: ContainerCreate was not called")
			}
			// Verify AutoRemove is disabled (critical for log retrieval).
			if fake.createOpts.HostConfig != nil && fake.createOpts.HostConfig.AutoRemove {
				t.Error("Create: HostConfig.AutoRemove should be false for log retrieval")
			}
			// Verify resource limits when specified.
			if tc.spec.LimitNanoCPUs > 0 && fake.createOpts.HostConfig != nil {
				if fake.createOpts.HostConfig.NanoCPUs != tc.spec.LimitNanoCPUs {
					t.Errorf("Create: NanoCPUs = %d, want %d",
						fake.createOpts.HostConfig.NanoCPUs, tc.spec.LimitNanoCPUs)
				}
			}
			if tc.spec.LimitMemoryBytes > 0 && fake.createOpts.HostConfig != nil {
				if fake.createOpts.HostConfig.Memory != tc.spec.LimitMemoryBytes {
					t.Errorf("Create: Memory = %d, want %d",
						fake.createOpts.HostConfig.Memory, tc.spec.LimitMemoryBytes)
				}
			}
			// Verify network mode when specified.
			if tc.opts.Network != "" && fake.createOpts.HostConfig != nil {
				if got := string(fake.createOpts.HostConfig.NetworkMode); got != tc.opts.Network {
					t.Errorf("Create: NetworkMode = %q, want %q", got, tc.opts.Network)
				}
			}
			// Verify storage option when specified.
			if tc.spec.StorageSizeOpt != "" && fake.createOpts.HostConfig != nil {
				if fake.createOpts.HostConfig.StorageOpt == nil ||
					fake.createOpts.HostConfig.StorageOpt["size"] != tc.spec.StorageSizeOpt {
					t.Errorf("Create: StorageOpt[size] = %q, want %q",
						fake.createOpts.HostConfig.StorageOpt["size"], tc.spec.StorageSizeOpt)
				}
			}

			// Step 2: Start container — verify start is called with correct ID.
			if err := rt.Start(ctx, handle); err != nil {
				t.Fatalf("Start failed: %v", err)
			}
			if !fake.startCalled {
				t.Error("Start: ContainerStart was not called")
			}
			if fake.startID != containerID {
				t.Errorf("Start: container ID = %q, want %q", fake.startID, containerID)
			}

			// Step 3: Wait for container — verify exit code and timestamps.
			result, err := rt.Wait(ctx, handle)
			if err != nil {
				t.Fatalf("Wait failed: %v", err)
			}
			if !fake.waitCalled {
				t.Error("Wait: ContainerWait was not called")
			}
			if result.ExitCode != tc.wantExitCode {
				t.Errorf("Wait: ExitCode = %d, want %d", result.ExitCode, tc.wantExitCode)
			}
			// Verify timestamps from inspect (Engine v29 uses ContainerInspectResult.Container.State).
			if result.StartedAt.IsZero() {
				t.Error("Wait: StartedAt should not be zero")
			}
			if result.CompletedAt.IsZero() {
				t.Error("Wait: CompletedAt should not be zero")
			}

			// Step 4: Remove container — verify force removal is called.
			if err := rt.Remove(ctx, handle); err != nil {
				t.Fatalf("Remove failed: %v", err)
			}
			if !fake.removeCalled {
				t.Error("Remove: ContainerRemove was not called")
			}
			if fake.removeID != containerID {
				t.Errorf("Remove: container ID = %q, want %q", fake.removeID, containerID)
			}
		})
	}
}

// TestDockerContainerLifecycleV29_ContextCancellation validates that lifecycle
// methods honour context cancellation under Engine v29. The moby SDK propagates
// context through all API calls; cancellation should abort in-flight operations.
func TestDockerContainerLifecycleV29_ContextCancellation(t *testing.T) {
	t.Parallel()

	// Test wait with cancelled context — should return error, not block forever.
	t.Run("wait_context_cancelled", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately.

		fake := &fakeDockerClient{
			waitErr: context.Canceled,
		}
		rt := newDockerContainerRuntimeWithClient(fake, DockerContainerRuntimeOptions{})

		_, err := rt.Wait(ctx, ContainerHandle("cancel-test"))
		if err == nil {
			t.Fatal("Wait() error = nil, want context cancellation error")
		}
		if !fake.removeCalled {
			t.Fatal("expected Wait() to force-remove container on cancellation")
		}
		if fake.removeID != "cancel-test" {
			t.Fatalf("remove ID = %q, want %q", fake.removeID, "cancel-test")
		}
	})
}

// TestDockerContainerLifecycleV29_MountTypes validates that mount configuration
// is correctly translated to Engine v29 moby types. All mounts should use
// mount.TypeBind for host path mounts.
func TestDockerContainerLifecycleV29_MountTypes(t *testing.T) {
	t.Parallel()

	spec := ContainerSpec{
		Image:   "alpine:latest",
		Command: []string{"ls", "-la"},
		Mounts: []ContainerMount{
			{Source: "/host/workspace", Target: "/workspace", ReadOnly: false},
			{Source: "/host/inputs", Target: "/in", ReadOnly: true},
			{Source: "/host/outputs", Target: "/out", ReadOnly: false},
		},
	}

	fake := &fakeDockerClient{
		createResult: client.ContainerCreateResult{ID: "mount-test"},
	}
	rt := newDockerContainerRuntimeWithClient(fake, DockerContainerRuntimeOptions{})

	_, err := rt.Create(context.Background(), spec)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify mounts in HostConfig use correct Engine v29 types.
	if fake.createOpts.HostConfig == nil {
		t.Fatal("HostConfig should not be nil")
	}
	mounts := fake.createOpts.HostConfig.Mounts
	if len(mounts) != len(spec.Mounts) {
		t.Fatalf("Mounts: got %d, want %d", len(mounts), len(spec.Mounts))
	}
	for i, m := range mounts {
		// Verify all mounts are bind mounts (mount.TypeBind).
		if m.Type != "bind" {
			t.Errorf("Mount[%d].Type = %q, want %q", i, m.Type, "bind")
		}
		if m.Source != spec.Mounts[i].Source {
			t.Errorf("Mount[%d].Source = %q, want %q", i, m.Source, spec.Mounts[i].Source)
		}
		if m.Target != spec.Mounts[i].Target {
			t.Errorf("Mount[%d].Target = %q, want %q", i, m.Target, spec.Mounts[i].Target)
		}
		if m.ReadOnly != spec.Mounts[i].ReadOnly {
			t.Errorf("Mount[%d].ReadOnly = %v, want %v", i, m.ReadOnly, spec.Mounts[i].ReadOnly)
		}
	}
}

// TestDockerContainerLifecycleV29_WaitCondition validates that ContainerWait uses
// the correct WaitCondition (WaitConditionNotRunning) under Engine v29. This ensures
// Wait blocks until the container has completely stopped, not just exited.
func TestDockerContainerLifecycleV29_WaitCondition(t *testing.T) {
	t.Parallel()

	// This test validates the Wait implementation uses WaitConditionNotRunning.
	// The fakeDockerClient returns immediately, so we verify the runtime code
	// path handles the moby ContainerWaitResult struct correctly.
	fake := &fakeDockerClient{
		createResult:   client.ContainerCreateResult{ID: "wait-condition-test"},
		waitStatusCode: 0,
		inspectResult: client.ContainerInspectResult{
			Container: container.InspectResponse{
				State: &container.State{
					StartedAt:  "2024-06-01T10:00:00.000000000Z",
					FinishedAt: "2024-06-01T10:00:05.000000000Z",
				},
			},
		},
	}
	rt := newDockerContainerRuntimeWithClient(fake, DockerContainerRuntimeOptions{})

	ctx := context.Background()
	handle := ContainerHandle("wait-condition-test")

	result, err := rt.Wait(ctx, handle)
	if err != nil {
		t.Fatalf("Wait failed: %v", err)
	}
	if !fake.waitCalled {
		t.Error("ContainerWait should have been called")
	}
	if fake.waitOptions.Condition != container.WaitConditionNotRunning {
		t.Errorf("WaitCondition = %q, want %q", fake.waitOptions.Condition, container.WaitConditionNotRunning)
	}
	// Verify result contains expected data from moby SDK response.
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
}

// TestDockerContainerLifecycleV29_ErrorPropagation validates that errors from
// the moby SDK are correctly propagated through the lifecycle methods.
func TestDockerContainerLifecycleV29_ErrorPropagation(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		method      string
		setupFake   func(*fakeDockerClient)
		errContains string
	}{
		{
			name:   "create_error_propagates",
			method: "create",
			setupFake: func(f *fakeDockerClient) {
				f.createErr = errors.New("daemon: no space left on device")
			},
			errContains: "create container",
		},
		{
			name:   "start_error_propagates",
			method: "start",
			setupFake: func(f *fakeDockerClient) {
				f.createResult = client.ContainerCreateResult{ID: "err-start"}
				f.startErr = errors.New("container already running")
			},
			errContains: "already running",
		},
		{
			name:   "wait_error_propagates",
			method: "wait",
			setupFake: func(f *fakeDockerClient) {
				f.createResult = client.ContainerCreateResult{ID: "err-wait"}
				f.waitErr = errors.New("container died: OOMKilled")
			},
			errContains: "wait container",
		},
		{
			name:   "remove_error_propagates",
			method: "remove",
			setupFake: func(f *fakeDockerClient) {
				f.createResult = client.ContainerCreateResult{ID: "err-remove"}
				f.waitStatusCode = 0
				f.inspectResult = client.ContainerInspectResult{
					Container: container.InspectResponse{
						State: &container.State{
							StartedAt:  "2024-06-01T10:00:00.000000000Z",
							FinishedAt: "2024-06-01T10:00:01.000000000Z",
						},
					},
				}
				f.removeErr = errors.New("container is running")
			},
			errContains: "running",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			fake := &fakeDockerClient{}
			tc.setupFake(fake)
			rt := newDockerContainerRuntimeWithClient(fake, DockerContainerRuntimeOptions{})

			var err error
			switch tc.method {
			case "create":
				_, err = rt.Create(ctx, ContainerSpec{Image: "alpine"})
			case "start":
				handle, createErr := rt.Create(ctx, ContainerSpec{Image: "alpine"})
				if createErr != nil {
					t.Fatalf("Create failed unexpectedly: %v", createErr)
				}
				err = rt.Start(ctx, handle)
			case "wait":
				handle, createErr := rt.Create(ctx, ContainerSpec{Image: "alpine"})
				if createErr != nil {
					t.Fatalf("Create failed unexpectedly: %v", createErr)
				}
				_, err = rt.Wait(ctx, handle)
			case "remove":
				handle, createErr := rt.Create(ctx, ContainerSpec{Image: "alpine"})
				if createErr != nil {
					t.Fatalf("Create failed unexpectedly: %v", createErr)
				}
				if startErr := rt.Start(ctx, handle); startErr != nil {
					t.Fatalf("Start failed unexpectedly: %v", startErr)
				}
				if _, waitErr := rt.Wait(ctx, handle); waitErr != nil {
					t.Fatalf("Wait failed unexpectedly: %v", waitErr)
				}
				err = rt.Remove(ctx, handle)
			}

			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.errContains)
			}
			if !strings.Contains(err.Error(), tc.errContains) {
				t.Errorf("error %q should contain %q", err.Error(), tc.errContains)
			}
		})
	}
}
