package nodeagent

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/pki"
	"github.com/iw2rmb/ploy/internal/workflow/backoff"
	"github.com/moby/moby/api/pkg/stdcopy"
	containertypes "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

// testBackoffPolicy returns a fast backoff policy suitable for tests.
func testBackoffPolicy() backoff.Policy {
	return backoff.Policy{
		InitialInterval: types.Duration(10 * time.Millisecond),
		MaxInterval:     types.Duration(100 * time.Millisecond),
		Multiplier:      2.0,
		MaxElapsedTime:  0,
		MaxAttempts:     0,
	}
}

// gzipBytes compresses input bytes using gzip (test helper).
func gzipBytes(t *testing.T, input []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(input); err != nil {
		t.Fatalf("gzip write failed: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("gzip close failed: %v", err)
	}
	return buf.Bytes()
}

// writeTempFile creates a temporary file with content for testing.
func writeTempFile(t *testing.T, content []byte) string {
	t.Helper()
	f, err := os.CreateTemp("", "test-*")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer func() {
		_ = f.Close()
	}()

	if _, err := f.Write(content); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	t.Cleanup(func() {
		_ = os.Remove(f.Name())
	})

	return f.Name()
}

// generateTestCerts creates test CA, node certificate, and key for mTLS testing.
func generateTestCerts(t *testing.T) (certPEM, keyPEM, caPEM []byte) {
	t.Helper()

	now := time.Now().UTC()

	ca, err := pki.GenerateCA("test-cluster", now)
	if err != nil {
		t.Fatalf("generate CA: %v", err)
	}

	nodeKey, nodeCSR, err := pki.GenerateNodeCSR(testNodeID, "test-cluster", "127.0.0.1")
	if err != nil {
		t.Fatalf("generate node CSR: %v", err)
	}

	nodeCert, err := pki.SignNodeCSR(ca, nodeCSR, now)
	if err != nil {
		t.Fatalf("sign node CSR: %v", err)
	}

	certPEM = []byte(nodeCert.CertPEM)
	keyPEM = []byte(nodeKey.KeyPEM)
	caPEM = []byte(ca.CertPEM)

	return certPEM, keyPEM, caPEM
}

// containsError checks if an error message contains a substring.
func containsError(err error, substr string) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), substr)
}

// newTestConfig returns a Config with sensible test defaults (TLS disabled).
func newTestConfig(serverURL string) Config {
	return Config{
		ServerURL: serverURL,
		NodeID:    testNodeID,
		HTTP:      HTTPConfig{TLS: TLSConfig{Enabled: false}},
	}
}

// newTestController creates a runController with all uploaders initialized,
// suitable for tests that call upload methods.
func newTestController(t *testing.T, cfg Config) *runController {
	t.Helper()
	uploader, err := newBaseUploader(cfg)
	if err != nil {
		t.Fatalf("newBaseUploader: %v", err)
	}
	return &runController{
		cfg:               cfg,
		jobs:              make(map[types.JobID]*jobContext),
		diffUploader:      uploader,
		artifactUploader:  uploader,
		statusUploader:    uploader,
		jobImageNameSaver: uploader,
		nodeEventUploader: uploader,
		httpClient:        uploader.client,
	}
}

// setupClaimer creates a ClaimManager ready for testing with noop pre-claim
// cleanup, noop startup reconciler, and fast backoff policy.
func setupClaimer(t *testing.T, cfg Config, controller RunController) *ClaimManager {
	t.Helper()
	claimer, err := NewClaimManager(cfg, controller)
	if err != nil {
		t.Fatalf("NewClaimManager: %v", err)
	}
	claimer.preClaimCleanup = nil // nil means always proceed
	installNoopStartupReconciler(claimer)
	claimer.backoff = backoff.NewStatefulBackoff(testBackoffPolicy())
	return claimer
}

// runClaimerUntil runs the claim loop in a background goroutine and blocks
// until the timeout expires.
func runClaimerUntil(t *testing.T, claimer *ClaimManager, timeout time.Duration) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = claimer.Start(ctx)
	}()
	wg.Wait()
}

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
