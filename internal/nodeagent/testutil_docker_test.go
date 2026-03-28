package nodeagent

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"sync"

	containertypes "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/moby/moby/api/pkg/stdcopy"
)

// ---------------------------------------------------------------------------
// Call recorder
// ---------------------------------------------------------------------------

// callRecorder is a thread-safe helper for tracking named call events in tests.
type callRecorder struct {
	mu    sync.Mutex
	calls []string
}

func (r *callRecorder) Record(name string) {
	r.mu.Lock()
	r.calls = append(r.calls, name)
	r.mu.Unlock()
}

func (r *callRecorder) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
}

func (r *callRecorder) All() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.calls))
	copy(out, r.calls)
	return out
}

// ---------------------------------------------------------------------------
// Docker test doubles
// ---------------------------------------------------------------------------

// inspectWithState builds a ContainerInspectResult with the given state fields.
func inspectWithState(running bool, status containertypes.ContainerState, finishedAt string) client.ContainerInspectResult {
	return client.ContainerInspectResult{
		Container: containertypes.InspectResponse{
			State: &containertypes.State{
				Running:    running,
				Status:     status,
				FinishedAt: finishedAt,
			},
		},
	}
}

// fakeDockerClient is a composable test double that satisfies both
// crashReconcileDockerClient and claimCleanupDockerClient interfaces.
type fakeDockerClient struct {
	listResult client.ContainerListResult
	listErr    error
	listCalls  int

	inspectByID    map[string]client.ContainerInspectResult
	inspectErrByID map[string]error

	waitByID      map[string]containertypes.WaitResponse
	waitErrByID   map[string]error
	waitBlockByID map[string]chan struct{}

	logsByID    map[string][]byte
	logsErrByID map[string]error

	infoResult client.SystemInfoResult
	infoErr    error

	removeErrByID map[string]error
	removedIDs    []string
}

func (f *fakeDockerClient) ContainerList(context.Context, client.ContainerListOptions) (client.ContainerListResult, error) {
	f.listCalls++
	if f.listErr != nil {
		return client.ContainerListResult{}, f.listErr
	}
	return f.listResult, nil
}

func (f *fakeDockerClient) ContainerInspect(_ context.Context, containerID string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	if err, ok := f.inspectErrByID[containerID]; ok && err != nil {
		return client.ContainerInspectResult{}, err
	}
	if inspect, ok := f.inspectByID[containerID]; ok {
		return inspect, nil
	}
	return client.ContainerInspectResult{}, errors.New("missing inspect result")
}

func (f *fakeDockerClient) ContainerWait(_ context.Context, containerID string, _ client.ContainerWaitOptions) client.ContainerWaitResult {
	result := make(chan containertypes.WaitResponse, 1)
	errCh := make(chan error, 1)
	if gate, ok := f.waitBlockByID[containerID]; ok && gate != nil {
		<-gate
	}
	if err, ok := f.waitErrByID[containerID]; ok && err != nil {
		errCh <- err
		return client.ContainerWaitResult{Result: result, Error: errCh}
	}
	waitResp, ok := f.waitByID[containerID]
	if !ok {
		waitResp = containertypes.WaitResponse{StatusCode: 0}
	}
	result <- waitResp
	return client.ContainerWaitResult{Result: result, Error: errCh}
}

func (f *fakeDockerClient) ContainerLogs(_ context.Context, containerID string, _ client.ContainerLogsOptions) (client.ContainerLogsResult, error) {
	if err, ok := f.logsErrByID[containerID]; ok && err != nil {
		return nil, err
	}
	data, ok := f.logsByID[containerID]
	if !ok {
		data = nil
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (f *fakeDockerClient) Info(context.Context, client.InfoOptions) (client.SystemInfoResult, error) {
	if f.infoErr != nil {
		return client.SystemInfoResult{}, f.infoErr
	}
	return f.infoResult, nil
}

func (f *fakeDockerClient) ContainerRemove(_ context.Context, containerID string, _ client.ContainerRemoveOptions) (client.ContainerRemoveResult, error) {
	f.removedIDs = append(f.removedIDs, containerID)
	if err, ok := f.removeErrByID[containerID]; ok && err != nil {
		return client.ContainerRemoveResult{}, err
	}
	return client.ContainerRemoveResult{}, nil
}

// multiplexedDockerLogs builds Docker stdcopy-framed log output for testing.
func multiplexedDockerLogs(payload string, stream stdcopy.StdType) []byte {
	data := []byte(payload)
	frame := make([]byte, 8+len(data))
	frame[0] = byte(stream)
	binary.BigEndian.PutUint32(frame[4:8], uint32(len(data)))
	copy(frame[8:], data)
	return frame
}
