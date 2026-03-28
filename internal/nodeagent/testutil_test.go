package nodeagent

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/pki"
	"github.com/iw2rmb/ploy/internal/workflow/backoff"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// ---------------------------------------------------------------------------
// Simple utilities
// ---------------------------------------------------------------------------

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

// populateTestFiles creates files with the given content under dir.
func populateTestFiles(t *testing.T, dir string, files []string, content string) {
	t.Helper()
	for _, f := range files {
		fullPath := filepath.Join(dir, f)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to create file: %v", err)
		}
	}
}

// ---------------------------------------------------------------------------
// Tar inspection helpers
// ---------------------------------------------------------------------------

// tarEntry holds the key attributes of a single tar archive entry.
type tarEntry struct {
	Typeflag byte
	Linkname string
	Content  []byte
}

// tarEntriesFromBundle decompresses a gzipped tar bundle and returns a map
// of entry name to tarEntry (typeflag, linkname, content).
func tarEntriesFromBundle(t *testing.T, bundle []byte) map[string]tarEntry {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(bundle))
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	entries := make(map[string]tarEntry)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar read: %v", err)
		}
		var content []byte
		if hdr.Typeflag == tar.TypeReg {
			content, err = io.ReadAll(tr)
			if err != nil {
				t.Fatalf("read content for %s: %v", hdr.Name, err)
			}
		}
		entries[hdr.Name] = tarEntry{
			Typeflag: hdr.Typeflag,
			Linkname: hdr.Linkname,
			Content:  content,
		}
	}
	return entries
}

// ---------------------------------------------------------------------------
// Config builders
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

// newTestConfig returns a Config with sensible test defaults (TLS disabled).
func newTestConfig(serverURL string) Config {
	return newAgentConfig(serverURL)
}

// ---------------------------------------------------------------------------
// Component builders
// ---------------------------------------------------------------------------

// newTestUploader returns a baseUploader with sensible test defaults (TLS disabled).
func newTestUploader(t *testing.T, serverURL string) *baseUploader {
	t.Helper()
	u, err := newBaseUploader(newTestConfig(serverURL))
	if err != nil {
		t.Fatalf("newBaseUploader: %v", err)
	}
	return u
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
	claimer.backoff = backoff.NewStatefulBackoff(backoff.Policy{
		InitialInterval: types.Duration(10 * time.Millisecond),
		MaxInterval:     types.Duration(100 * time.Millisecond),
		Multiplier:      2.0,
		MaxElapsedTime:  0,
		MaxAttempts:     0,
	})
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
// Claim response builders
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

// ---------------------------------------------------------------------------
// StartRunRequest builders
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
