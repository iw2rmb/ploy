package step

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"iter"
	"strings"

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
	waitOptions    client.ContainerWaitOptions

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

	// ImageInspect behavior (used by pull policy tests)
	imageInspectErr    error
	imageInspectCalled bool
	imageInspectRef    string
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
	f.waitOptions = options
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

// ImageInspect simulates inspecting an image reference.
func (f *fakeDockerClient) ImageInspect(ctx context.Context, imageID string, inspectOpts ...client.ImageInspectOption) (client.ImageInspectResult, error) {
	f.imageInspectCalled = true
	f.imageInspectRef = imageID
	return client.ImageInspectResult{}, f.imageInspectErr
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
