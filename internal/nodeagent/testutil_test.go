package nodeagent

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/pki"
	"github.com/iw2rmb/ploy/internal/workflow/backoff"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
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

// ---------------------------------------------------------------------------
// Agent mock-server helpers
// ---------------------------------------------------------------------------

type agentServerConfig struct {
	heartbeatStatus  int
	heartbeatCounter *int
}

type agentServerOption func(*agentServerConfig)

func withHeartbeatStatus(code int) agentServerOption {
	return func(c *agentServerConfig) { c.heartbeatStatus = code }
}

func withHeartbeatCounter(counter *int) agentServerOption {
	return func(c *agentServerConfig) { c.heartbeatCounter = counter }
}

// newAgentMockServer creates an httptest.Server that handles heartbeat and claim
// endpoints for the given nodeID. By default heartbeat returns 200 and claim
// returns 200 with an empty JSON object (no work).
func newAgentMockServer(t *testing.T, nodeID string, opts ...agentServerOption) *httptest.Server {
	t.Helper()
	sc := agentServerConfig{heartbeatStatus: http.StatusOK}
	for _, o := range opts {
		o(&sc)
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/nodes/" + nodeID + "/heartbeat":
			if sc.heartbeatCounter != nil {
				*sc.heartbeatCounter++
			}
			w.WriteHeader(sc.heartbeatStatus)
		case "/v1/nodes/" + nodeID + "/claim":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(ts.Close)
	return ts
}

// ---------------------------------------------------------------------------
// Agent config helpers
// ---------------------------------------------------------------------------

type configOption func(*Config)

func withNodeID(id string) configOption {
	return func(c *Config) { c.NodeID = types.NodeID(id) }
}

func withTLS(certPath, keyPath, caPath string) configOption {
	return func(c *Config) {
		c.HTTP.TLS = TLSConfig{Enabled: true, CertPath: certPath, KeyPath: keyPath, CAPath: caPath}
	}
}

func withHeartbeatInterval(d time.Duration) configOption {
	return func(c *Config) { c.Heartbeat.Interval = d }
}

func withHeartbeatTimeout(d time.Duration) configOption {
	return func(c *Config) { c.Heartbeat.Timeout = d }
}

func withConcurrency(n int) configOption {
	return func(c *Config) { c.Concurrency = n }
}

// newAgentConfig returns a full Config suitable for agent lifecycle tests.
// Defaults: testNodeID, Concurrency=1, TLS disabled, Heartbeat 100ms/5s, Listen=":0".
func newAgentConfig(serverURL string, opts ...configOption) Config {
	cfg := Config{
		ServerURL:   serverURL,
		NodeID:      testNodeID,
		Concurrency: 1,
		HTTP: HTTPConfig{
			Listen: ":0",
			TLS:    TLSConfig{Enabled: false},
		},
		Heartbeat: HeartbeatConfig{
			Interval: 100 * time.Millisecond,
			Timeout:  5 * time.Second,
		},
	}
	for _, o := range opts {
		o(&cfg)
	}
	return cfg
}

// runAgentUntil starts the agent, waits startup duration, calls check (if non-nil),
// then cancels and waits for shutdown within shutdownTimeout. Returns agent.Run error.
func runAgentUntil(t *testing.T, agent *Agent, startup, shutdownTimeout time.Duration, check func(t *testing.T, agent *Agent)) error {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- agent.Run(ctx) }()

	time.Sleep(startup)

	if check != nil {
		check(t, agent)
	}

	cancel()

	select {
	case err := <-errCh:
		return err
	case <-time.After(shutdownTimeout):
		t.Fatal("agent did not shut down within timeout")
		return nil
	}
}

// ---------------------------------------------------------------------------
// Claim response helpers
// ---------------------------------------------------------------------------

type claimOption func(*ClaimResponse)

func withClaimNodeID(id types.NodeID) claimOption {
	return func(c *ClaimResponse) { c.NodeID = id }
}

func withNextID(id types.JobID) claimOption {
	return func(c *ClaimResponse) { c.NextID = &id }
}

func withCommitSHA(sha types.CommitSHA) claimOption {
	return func(c *ClaimResponse) { c.CommitSha = &sha }
}

func withRecoveryContext(rc *contracts.RecoveryClaimContext) claimOption {
	return func(c *ClaimResponse) { c.RecoveryContext = rc }
}

func withClaimJobName(name string) claimOption {
	return func(c *ClaimResponse) { c.JobName = name }
}

// newClaimResponse returns a ClaimResponse with generated IDs and sensible defaults.
func newClaimResponse(opts ...claimOption) ClaimResponse {
	now := time.Now().UTC().Format(time.RFC3339)
	c := ClaimResponse{
		RunID:     types.NewRunID(),
		RepoID:    types.NewMigRepoID(),
		JobID:     types.NewJobID(),
		RepoURL:   types.RepoURL("https://github.com/test/repo"),
		Status:    "Started",
		NodeID:    types.NodeID(testNodeID),
		BaseRef:   types.GitRef("main"),
		TargetRef: types.GitRef("feature-branch"),
		StartedAt: now,
		CreatedAt: now,
	}
	for _, o := range opts {
		o(&c)
	}
	return c
}

// newSingleClaimServer returns a server that serves the given ClaimResponse
// on the /v1/nodes/{nodeID}/claim endpoint.
func newSingleClaimServer(t *testing.T, nodeID string, claim ClaimResponse) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/nodes/" + nodeID + "/claim":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(claim)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(ts.Close)
	return ts
}

// ---------------------------------------------------------------------------
// StartRunRequest helpers
// ---------------------------------------------------------------------------

type startRunOption func(*StartRunRequest)

func withRunRepoURL(u string) startRunOption {
	return func(r *StartRunRequest) { r.RepoURL = types.RepoURL(u) }
}

func withRunBaseRef(ref string) startRunOption {
	return func(r *StartRunRequest) { r.BaseRef = types.GitRef(ref) }
}

func withRunTargetRef(ref string) startRunOption {
	return func(r *StartRunRequest) { r.TargetRef = types.GitRef(ref) }
}

func withRunCommitSHA(sha string) startRunOption {
	return func(r *StartRunRequest) { r.CommitSHA = types.CommitSHA(sha) }
}

func withRunEnv(env map[string]string) startRunOption {
	return func(r *StartRunRequest) { r.Env = env }
}

func withRunOptions(opts RunOptions) startRunOption {
	return func(r *StartRunRequest) { r.TypedOptions = opts }
}

// newStartRunRequest returns a StartRunRequest with fixed IDs and sensible
// defaults (RepoURL, BaseRef "main", empty TypedOptions). Uses fixed IDs that
// satisfy manifest validation (lowercase alphanumeric, 3-64 chars).
func newStartRunRequest(opts ...startRunOption) StartRunRequest {
	r := StartRunRequest{
		RunID:        types.RunID("run-test-001"),
		JobID:        types.JobID("job-test-001"),
		RepoURL:      types.RepoURL("https://github.com/example/repo.git"),
		BaseRef:      types.GitRef("main"),
		TypedOptions: RunOptions{},
	}
	for _, o := range opts {
		o(&r)
	}
	return r
}

// ---------------------------------------------------------------------------
// Manifest builder helpers
// ---------------------------------------------------------------------------

// buildManifestDefault calls buildManifestFromRequest with stepIndex=0 and ModStackUnknown.
func buildManifestDefault(req StartRunRequest) (contracts.StepManifest, error) {
	return buildManifestFromRequest(req, req.TypedOptions, 0, contracts.ModStackUnknown)
}

// buildManifestAtStep calls buildManifestFromRequest with the given stepIndex and ModStackUnknown.
func buildManifestAtStep(req StartRunRequest, step int) (contracts.StepManifest, error) {
	return buildManifestFromRequest(req, req.TypedOptions, step, contracts.ModStackUnknown)
}

// assertCommand checks that got matches want element-by-element.
func assertCommand(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("command len=%d, want %d; got %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("command[%d]=%q, want %q", i, got[i], want[i])
		}
	}
}
