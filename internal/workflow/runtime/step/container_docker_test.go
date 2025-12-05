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
