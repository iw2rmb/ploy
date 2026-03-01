package nodeagent

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
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

// multiplexedDockerLogs builds Docker stdcopy-framed log output for testing.
func multiplexedDockerLogs(payload string, stream stdcopy.StdType) []byte {
	data := []byte(payload)
	frame := make([]byte, 8+len(data))
	frame[0] = byte(stream)
	binary.BigEndian.PutUint32(frame[4:8], uint32(len(data)))
	copy(frame[8:], data)
	return frame
}
