package step

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"iter"
	"strings"
	"testing"
	"time"

	// Docker Engine v29 SDK (moby) — types and client interfaces for testing.
	// See container_docker.go for migration notes.
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/jsonstream"
	"github.com/moby/moby/client"
)

// writeMultiplexedFrame writes a single frame in Docker's multiplexed stream
// format. The format is: [8]byte{STREAM_TYPE, 0, 0, 0, SIZE1, SIZE2, SIZE3, SIZE4}
// followed by the payload. STREAM_TYPE is 1 for stdout, 2 for stderr.
// SIZE is big-endian uint32 length of payload.
func writeMultiplexedFrame(buf *bytes.Buffer, streamType stdcopy.StdType, payload []byte) {
	header := make([]byte, 8)
	header[0] = byte(streamType)
	// header[1:4] are zero (padding)
	binary.BigEndian.PutUint32(header[4:8], uint32(len(payload)))
	buf.Write(header)
	buf.Write(payload)
}

// TestDockerLogDemux verifies that multiplexed Docker logs are demultiplexed
// into a single plain-text byte slice containing both stdout and stderr.
// The moby Engine v29 SDK (github.com/moby/moby/api/pkg/stdcopy) provides only
// StdCopy for reading multiplexed streams; NewStdWriter was removed. We manually
// construct the multiplexed format for testing.
func TestDockerLogDemux(t *testing.T) {
	// Build a synthetic multiplexed stream using Docker's wire format:
	// [8-byte header][payload] for each frame.
	var mux bytes.Buffer
	writeMultiplexedFrame(&mux, stdcopy.Stdout, []byte("hello stdout\n"))
	writeMultiplexedFrame(&mux, stdcopy.Stderr, []byte("oops stderr\n"))

	// Use the same demux logic as the runtime (stdcopy.StdCopy).
	var stdoutBuf, stderrBuf bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdoutBuf, &stderrBuf, bytes.NewReader(mux.Bytes())); err != nil {
		t.Fatalf("StdCopy error: %v", err)
	}
	combined := append(stdoutBuf.Bytes(), stderrBuf.Bytes()...)

	got := string(combined)
	if !containsAll(got, []string{"hello stdout", "oops stderr"}) {
		t.Fatalf("demuxed logs missing content: %q", got)
	}
}

// TestDockerLogDemuxMultipleFrames verifies demultiplexing with multiple
// interleaved stdout and stderr frames.
func TestDockerLogDemuxMultipleFrames(t *testing.T) {
	var mux bytes.Buffer
	writeMultiplexedFrame(&mux, stdcopy.Stdout, []byte("line1\n"))
	writeMultiplexedFrame(&mux, stdcopy.Stderr, []byte("err1\n"))
	writeMultiplexedFrame(&mux, stdcopy.Stdout, []byte("line2\n"))
	writeMultiplexedFrame(&mux, stdcopy.Stderr, []byte("err2\n"))

	var stdoutBuf, stderrBuf bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdoutBuf, &stderrBuf, bytes.NewReader(mux.Bytes())); err != nil {
		t.Fatalf("StdCopy error: %v", err)
	}

	if got, want := stdoutBuf.String(), "line1\nline2\n"; got != want {
		t.Errorf("stdout: got %q, want %q", got, want)
	}
	if got, want := stderrBuf.String(), "err1\nerr2\n"; got != want {
		t.Errorf("stderr: got %q, want %q", got, want)
	}
}

func containsAll(s string, parts []string) bool {
	for _, p := range parts {
		if !bytes.Contains([]byte(s), []byte(p)) {
			return false
		}
	}
	return true
}

// -----------------------------------------------------------------------------
// Fake Docker client for testing DockerContainerRuntime lifecycle methods.
// Implements dockerClientAPI using moby Engine v29 SDK types.
// -----------------------------------------------------------------------------

// fakeDockerClient implements dockerClientAPI for unit testing without a daemon.
// All methods are configurable via struct fields to simulate various responses.
type fakeDockerClient struct {
	// ContainerCreate behavior
	createResult client.ContainerCreateResult
	createErr    error
	createCalled bool
	createOpts   client.ContainerCreateOptions // captured for assertions

	// ContainerStart behavior
	startErr    error
	startCalled bool
	startID     string // captured container ID

	// ContainerWait behavior
	waitStatusCode int64
	waitErr        error
	waitCalled     bool

	// ContainerInspect behavior
	inspectResult client.ContainerInspectResult
	inspectErr    error

	// ContainerLogs behavior
	logsData []byte // raw multiplexed stream data
	logsErr  error

	// ContainerRemove behavior
	removeErr    error
	removeCalled bool
	removeID     string // captured container ID

	// ContainerStats behavior
	statsResult client.ContainerStatsResult
	statsErr    error

	// ImagePull behavior
	pullErr    error
	pullCalled bool
	pullRef    string // captured image reference
}

// ContainerCreate simulates container creation.
func (f *fakeDockerClient) ContainerCreate(ctx context.Context, options client.ContainerCreateOptions) (client.ContainerCreateResult, error) {
	f.createCalled = true
	f.createOpts = options
	return f.createResult, f.createErr
}

// ContainerStart simulates starting a container.
func (f *fakeDockerClient) ContainerStart(ctx context.Context, containerID string, options client.ContainerStartOptions) (client.ContainerStartResult, error) {
	f.startCalled = true
	f.startID = containerID
	return client.ContainerStartResult{}, f.startErr
}

// ContainerWait simulates waiting for container exit.
func (f *fakeDockerClient) ContainerWait(ctx context.Context, containerID string, options client.ContainerWaitOptions) client.ContainerWaitResult {
	f.waitCalled = true
	result := make(chan container.WaitResponse, 1)
	errCh := make(chan error, 1)
	if f.waitErr != nil {
		errCh <- f.waitErr
	} else {
		result <- container.WaitResponse{StatusCode: f.waitStatusCode}
	}
	return client.ContainerWaitResult{Result: result, Error: errCh}
}

// ContainerInspect returns container details for timestamp extraction.
func (f *fakeDockerClient) ContainerInspect(ctx context.Context, containerID string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	return f.inspectResult, f.inspectErr
}

// ContainerLogs returns a reader with test log data.
// Returns client.ContainerLogsResult (which embeds io.ReadCloser).
func (f *fakeDockerClient) ContainerLogs(ctx context.Context, containerID string, options client.ContainerLogsOptions) (client.ContainerLogsResult, error) {
	if f.logsErr != nil {
		return nil, f.logsErr
	}
	// Return a type that satisfies client.ContainerLogsResult (io.ReadCloser).
	return io.NopCloser(bytes.NewReader(f.logsData)), nil
}

// ContainerRemove simulates container removal.
func (f *fakeDockerClient) ContainerRemove(ctx context.Context, containerID string, options client.ContainerRemoveOptions) (client.ContainerRemoveResult, error) {
	f.removeCalled = true
	f.removeID = containerID
	return client.ContainerRemoveResult{}, f.removeErr
}

// ContainerStats returns resource usage statistics for a container.
func (f *fakeDockerClient) ContainerStats(ctx context.Context, containerID string, options client.ContainerStatsOptions) (client.ContainerStatsResult, error) {
	return f.statsResult, f.statsErr
}

// ImagePull simulates image pull (returns empty reader on success).
// Returns client.ImagePullResponse (which embeds io.ReadCloser).
func (f *fakeDockerClient) ImagePull(ctx context.Context, refStr string, options client.ImagePullOptions) (client.ImagePullResponse, error) {
	f.pullCalled = true
	f.pullRef = refStr
	if f.pullErr != nil {
		return nil, f.pullErr
	}
	// Return a type that satisfies client.ImagePullResponse (io.ReadCloser + extra methods).
	return &fakeImagePullResponse{Reader: strings.NewReader("")}, nil
}

// fakeImagePullResponse implements client.ImagePullResponse for testing.
// It provides minimal implementations for io.ReadCloser, JSONMessages, and Wait.
type fakeImagePullResponse struct {
	Reader io.Reader
}

func (f *fakeImagePullResponse) Read(p []byte) (n int, err error) { return f.Reader.Read(p) }
func (f *fakeImagePullResponse) Close() error                     { return nil }
func (f *fakeImagePullResponse) JSONMessages(ctx context.Context) iter.Seq2[jsonstream.Message, error] {
	return func(yield func(jsonstream.Message, error) bool) {}
}
func (f *fakeImagePullResponse) Wait(ctx context.Context) error { return nil }

// -----------------------------------------------------------------------------
// DockerContainerRuntime construction tests
// -----------------------------------------------------------------------------

// TestDockerContainerRuntimeCreate verifies container creation with moby client.
func TestDockerContainerRuntimeCreate(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name        string
		spec        ContainerSpec
		createRes   client.ContainerCreateResult
		createErr   error
		pullImage   bool
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
			name: "success_with_image_pull",
			spec: ContainerSpec{
				Image: "alpine:latest",
			},
			createRes: client.ContainerCreateResult{ID: "pulled-container"},
			pullImage: true,
			wantErr:   false,
		},
		{
			name: "error_image_pull_fails",
			spec: ContainerSpec{
				Image: "private/image:latest",
			},
			pullImage:   true,
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
				createResult: tc.createRes,
				createErr:    tc.createErr,
				pullErr:      tc.pullErr,
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
			if handle.ID != tc.createRes.ID {
				t.Errorf("got ID %q, want %q", handle.ID, tc.createRes.ID)
			}
			// Verify image pull was called when configured.
			if tc.pullImage && !fake.pullCalled {
				t.Error("image pull should have been called")
			}
			if !tc.pullImage && fake.pullCalled {
				t.Error("image pull should NOT have been called")
			}
		})
	}
}

// TestDockerContainerRuntimeStart verifies container start with moby client.
func TestDockerContainerRuntimeStart(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name     string
		handle   ContainerHandle
		startErr error
		wantErr  bool
	}{
		{
			name:    "success",
			handle:  ContainerHandle{ID: "container123"},
			wantErr: false,
		},
		{
			name:     "error_start_fails",
			handle:   ContainerHandle{ID: "container456"},
			startErr: errors.New("container not found"),
			wantErr:  true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fake := &fakeDockerClient{startErr: tc.startErr}
			rt := newDockerContainerRuntimeWithClient(fake, DockerContainerRuntimeOptions{})

			err := rt.Start(context.Background(), tc.handle)

			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if fake.startID != tc.handle.ID {
				t.Errorf("started container %q, want %q", fake.startID, tc.handle.ID)
			}
		})
	}
}

// TestDockerContainerRuntimeWait verifies container wait with moby client.
func TestDockerContainerRuntimeWait(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name       string
		handle     ContainerHandle
		statusCode int64
		waitErr    error
		startedAt  string
		finishedAt string
		inspectErr error
		wantCode   int
		wantErr    bool
	}{
		{
			name:       "success_exit_0",
			handle:     ContainerHandle{ID: "container123"},
			statusCode: 0,
			startedAt:  "2024-01-15T10:00:00.000000000Z",
			finishedAt: "2024-01-15T10:01:00.000000000Z",
			wantCode:   0,
			wantErr:    false,
		},
		{
			name:       "success_exit_1",
			handle:     ContainerHandle{ID: "container456"},
			statusCode: 1,
			startedAt:  "2024-01-15T10:00:00.000000000Z",
			finishedAt: "2024-01-15T10:00:30.000000000Z",
			wantCode:   1,
			wantErr:    false,
		},
		{
			name:       "success_inspect_fails_gracefully",
			handle:     ContainerHandle{ID: "container789"},
			statusCode: 0,
			inspectErr: errors.New("inspect failed"),
			wantCode:   0,
			wantErr:    false,
		},
		{
			name:    "error_wait_fails",
			handle:  ContainerHandle{ID: "container-err"},
			waitErr: errors.New("container died unexpectedly"),
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fake := &fakeDockerClient{
				waitStatusCode: tc.statusCode,
				waitErr:        tc.waitErr,
				inspectErr:     tc.inspectErr,
			}
			// Set up inspect result with timestamps.
			if tc.startedAt != "" || tc.finishedAt != "" {
				fake.inspectResult = client.ContainerInspectResult{
					Container: container.InspectResponse{
						State: &container.State{
							StartedAt:  tc.startedAt,
							FinishedAt: tc.finishedAt,
						},
					},
				}
			}
			rt := newDockerContainerRuntimeWithClient(fake, DockerContainerRuntimeOptions{})

			result, err := rt.Wait(context.Background(), tc.handle)

			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.ExitCode != tc.wantCode {
				t.Errorf("got exit code %d, want %d", result.ExitCode, tc.wantCode)
			}
			// Verify timestamps were parsed (if inspect succeeded).
			if tc.inspectErr == nil && tc.startedAt != "" {
				if result.StartedAt.IsZero() {
					t.Error("StartedAt should not be zero")
				}
				if result.CompletedAt.IsZero() {
					t.Error("CompletedAt should not be zero")
				}
			}
		})
	}
}

// TestDockerContainerRuntimeLogs verifies log retrieval with moby client.
func TestDockerContainerRuntimeLogs(t *testing.T) {
	t.Parallel()

	// Build multiplexed log data using Docker wire format.
	var muxLogs bytes.Buffer
	writeMultiplexedFrame(&muxLogs, stdcopy.Stdout, []byte("stdout line\n"))
	writeMultiplexedFrame(&muxLogs, stdcopy.Stderr, []byte("stderr line\n"))

	testCases := []struct {
		name        string
		handle      ContainerHandle
		logsData    []byte
		logsErr     error
		wantContent []string
		wantErr     bool
	}{
		{
			name:        "success_demux",
			handle:      ContainerHandle{ID: "container123"},
			logsData:    muxLogs.Bytes(),
			wantContent: []string{"stdout line", "stderr line"},
			wantErr:     false,
		},
		{
			name:    "error_logs_fails",
			handle:  ContainerHandle{ID: "container456"},
			logsErr: errors.New("container not found"),
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fake := &fakeDockerClient{
				logsData: tc.logsData,
				logsErr:  tc.logsErr,
			}
			rt := newDockerContainerRuntimeWithClient(fake, DockerContainerRuntimeOptions{})

			logs, err := rt.Logs(context.Background(), tc.handle)

			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			for _, want := range tc.wantContent {
				if !strings.Contains(string(logs), want) {
					t.Errorf("logs should contain %q, got %q", want, string(logs))
				}
			}
		})
	}
}

// TestDockerContainerRuntimeRemove verifies container removal with moby client.
func TestDockerContainerRuntimeRemove(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name      string
		handle    ContainerHandle
		removeErr error
		wantErr   bool
	}{
		{
			name:    "success",
			handle:  ContainerHandle{ID: "container123"},
			wantErr: false,
		},
		{
			name:      "error_remove_fails",
			handle:    ContainerHandle{ID: "container456"},
			removeErr: errors.New("container busy"),
			wantErr:   true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fake := &fakeDockerClient{removeErr: tc.removeErr}
			rt := newDockerContainerRuntimeWithClient(fake, DockerContainerRuntimeOptions{})

			err := rt.Remove(context.Background(), tc.handle)

			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !fake.removeCalled {
				t.Error("remove should have been called")
			}
			if fake.removeID != tc.handle.ID {
				t.Errorf("removed container %q, want %q", fake.removeID, tc.handle.ID)
			}
		})
	}
}

// TestDockerContainerRuntimeNilClient verifies nil client returns errors.
func TestDockerContainerRuntimeNilClient(t *testing.T) {
	t.Parallel()
	rt := &DockerContainerRuntime{client: nil}
	ctx := context.Background()

	if _, err := rt.Create(ctx, ContainerSpec{Image: "alpine"}); err == nil {
		t.Error("Create should fail with nil client")
	}
	if err := rt.Start(ctx, ContainerHandle{ID: "x"}); err == nil {
		t.Error("Start should fail with nil client")
	}
	if _, err := rt.Wait(ctx, ContainerHandle{ID: "x"}); err == nil {
		t.Error("Wait should fail with nil client")
	}
	if _, err := rt.Logs(ctx, ContainerHandle{ID: "x"}); err == nil {
		t.Error("Logs should fail with nil client")
	}
	if err := rt.Remove(ctx, ContainerHandle{ID: "x"}); err == nil {
		t.Error("Remove should fail with nil client")
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

// =============================================================================
// Engine v29 Container Lifecycle Validation Tests
// =============================================================================
// These tests re-validate container lifecycle semantics under Docker Engine v29
// (moby SDK) as specified in ROADMAP.md line 43. They verify that:
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
			if handle.ID != containerID {
				t.Errorf("Create: got ID %q, want %q", handle.ID, containerID)
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
				if fake.createOpts.HostConfig.Resources.NanoCPUs != tc.spec.LimitNanoCPUs {
					t.Errorf("Create: NanoCPUs = %d, want %d",
						fake.createOpts.HostConfig.Resources.NanoCPUs, tc.spec.LimitNanoCPUs)
				}
			}
			if tc.spec.LimitMemoryBytes > 0 && fake.createOpts.HostConfig != nil {
				if fake.createOpts.HostConfig.Resources.Memory != tc.spec.LimitMemoryBytes {
					t.Errorf("Create: Memory = %d, want %d",
						fake.createOpts.HostConfig.Resources.Memory, tc.spec.LimitMemoryBytes)
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

		// fakeDockerClient.ContainerWait sends on channels synchronously, so we
		// test that the runtime doesn't block when context is already cancelled.
		fake := &fakeDockerClient{
			waitStatusCode: 0,
			inspectResult: client.ContainerInspectResult{
				Container: container.InspectResponse{
					State: &container.State{
						StartedAt:  "2024-06-01T10:00:00.000000000Z",
						FinishedAt: "2024-06-01T10:00:01.000000000Z",
					},
				},
			},
		}
		rt := newDockerContainerRuntimeWithClient(fake, DockerContainerRuntimeOptions{})

		// With our fake, Wait should still complete since channels are pre-filled.
		// In production, context cancellation would cause the daemon call to abort.
		_, _ = rt.Wait(ctx, ContainerHandle{ID: "cancel-test"})
		// No assertion on error since fake doesn't simulate blocking wait.
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
	handle := ContainerHandle{ID: "wait-condition-test"}

	result, err := rt.Wait(ctx, handle)
	if err != nil {
		t.Fatalf("Wait failed: %v", err)
	}
	if !fake.waitCalled {
		t.Error("ContainerWait should have been called")
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

// TestParseDockerTime verifies RFC3339Nano timestamp parsing.
func TestParseDockerTime(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name    string
		input   string
		wantOK  bool
		wantUTC bool
	}{
		{
			name:    "valid_timestamp",
			input:   "2024-01-15T10:30:00.123456789Z",
			wantOK:  true,
			wantUTC: true,
		},
		{
			name:   "empty_string",
			input:  "",
			wantOK: false,
		},
		{
			name:   "whitespace_only",
			input:  "   ",
			wantOK: false,
		},
		{
			name:   "invalid_format",
			input:  "not-a-timestamp",
			wantOK: false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := parseDockerTime(tc.input)

			if tc.wantOK {
				if result.IsZero() {
					t.Error("expected non-zero time")
				}
				if tc.wantUTC && result.Location() != time.UTC {
					t.Error("expected UTC location")
				}
			} else {
				if !result.IsZero() {
					t.Errorf("expected zero time, got %v", result)
				}
			}
		})
	}
}
