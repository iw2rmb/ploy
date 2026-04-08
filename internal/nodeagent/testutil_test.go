package nodeagent

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"math/rand"
	"net/http"
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

// checkErr fails the test if the error doesn't match expectations.
func checkErr(t *testing.T, wantErr bool, err error) {
	t.Helper()
	if wantErr && err == nil {
		t.Error("expected error but got none")
	}
	if !wantErr && err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// incompressibleBytes returns deterministic pseudo-random bytes that resist
// gzip compression. Useful for testing upload size caps.
func incompressibleBytes(size int) []byte {
	data := make([]byte, size)
	rand.New(rand.NewSource(1)).Read(data)
	return data
}

// mapKeys returns the keys of a tarEntry map for diagnostic output.
func mapKeys(m map[string]tarEntry) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// ---------------------------------------------------------------------------
// TLS / PKI helpers
// ---------------------------------------------------------------------------

// testPKI holds all PKI material generated for a test.
type testPKI struct {
	CA         *pki.CABundle
	ServerCert *pki.IssuedCert
	NodeCert   *pki.IssuedCert
	NodeKey    *pki.IssuedCert // from GenerateNodeCSR (holds KeyPEM)
}

// generateTestPKI creates a full set of PKI material: CA, server cert, and
// node cert signed by the same CA. The server cert covers 127.0.0.1.
func generateTestPKI(t *testing.T) *testPKI {
	t.Helper()
	now := time.Now().UTC()

	ca, err := pki.GenerateCA("test-cluster", now)
	if err != nil {
		t.Fatalf("generate CA: %v", err)
	}

	serverCert, err := pki.IssueServerCert(ca, "test-cluster", "127.0.0.1", now)
	if err != nil {
		t.Fatalf("issue server cert: %v", err)
	}

	nodeKey, nodeCSR, err := pki.GenerateNodeCSR(string(testNodeID), "test-cluster", "127.0.0.1")
	if err != nil {
		t.Fatalf("generate node CSR: %v", err)
	}

	nodeCert, err := pki.SignNodeCSR(ca, nodeCSR, now)
	if err != nil {
		t.Fatalf("sign node CSR: %v", err)
	}

	return &testPKI{CA: ca, ServerCert: serverCert, NodeCert: nodeCert, NodeKey: nodeKey}
}

// testPKIPaths holds file paths for written PKI material.
type testPKIPaths struct {
	ServerCert string
	ServerKey  string
	NodeCert   string
	NodeKey    string
	CA         string
}

// writeFiles writes all PKI material to dir and returns the paths.
func (p *testPKI) writeFiles(t *testing.T, dir string) testPKIPaths {
	t.Helper()
	paths := testPKIPaths{
		ServerCert: filepath.Join(dir, "server.crt"),
		ServerKey:  filepath.Join(dir, "server.key"),
		NodeCert:   filepath.Join(dir, "node.crt"),
		NodeKey:    filepath.Join(dir, "node.key"),
		CA:         filepath.Join(dir, "ca.crt"),
	}
	for _, pair := range []struct{ path, content string }{
		{paths.ServerCert, p.ServerCert.CertPEM},
		{paths.ServerKey, p.ServerCert.KeyPEM},
		{paths.NodeCert, p.NodeCert.CertPEM},
		{paths.NodeKey, p.NodeKey.KeyPEM},
		{paths.CA, p.CA.CertPEM},
	} {
		if err := os.WriteFile(pair.path, []byte(pair.content), 0600); err != nil {
			t.Fatalf("write %s: %v", pair.path, err)
		}
	}
	return paths
}

// startTestTLSServer starts an HTTPS server using the provided server cert.
// If ca is non-nil, the server requires and verifies client certificates (mTLS).
// Returns the listener address. Server and listener are cleaned up via t.Cleanup.
func startTestTLSServer(t *testing.T, ca *pki.CABundle, serverCert *pki.IssuedCert, handler http.Handler) string {
	t.Helper()

	tlsCert, err := tls.X509KeyPair([]byte(serverCert.CertPEM), []byte(serverCert.KeyPEM))
	if err != nil {
		t.Fatalf("load server cert: %v", err)
	}

	serverTLS := &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		MinVersion:   tls.VersionTLS13,
	}
	if ca != nil {
		pool := x509.NewCertPool()
		pool.AddCert(ca.Cert)
		serverTLS.ClientAuth = tls.RequireAndVerifyClientCert
		serverTLS.ClientCAs = pool
	}

	listener, err := tls.Listen("tcp", "127.0.0.1:0", serverTLS)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	srv := &http.Server{Handler: handler}
	go func() { _ = srv.Serve(listener) }()

	t.Cleanup(func() {
		_ = srv.Close()
	})

	return listener.Addr().String()
}

// startNodeServer creates, starts, and returns a Server with t.Cleanup shutdown.
func startNodeServer(t *testing.T, cfg Config) *Server {
	t.Helper()
	server, err := NewServer(cfg, &mockController{})
	if err != nil {
		t.Fatalf("create server: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Start(ctx); err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer shutdownCancel()
		_ = server.Stop(shutdownCtx)
	})
	if server.Address() == "" {
		t.Fatal("server address is empty")
	}
	return server
}

// bootstrapHandler returns an http.Handler that serves the standard bootstrap
// response on /v1/pki/bootstrap.
func bootstrapHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/pki/bootstrap", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"certificate":"cert-data","ca_bundle":"ca-data"}`))
	})
	return mux
}

// ---------------------------------------------------------------------------
// File helpers
// ---------------------------------------------------------------------------

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

// assertTarContains verifies that every name in wantHeaders exists as an
// entry in the gzipped tar bundle.
func assertTarContains(t *testing.T, bundle []byte, wantHeaders []string) {
	t.Helper()
	entries := tarEntriesFromBundle(t, bundle)
	for _, h := range wantHeaders {
		if _, ok := entries[h]; !ok {
			t.Fatalf("expected tar entry %q, got %v", h, mapKeys(entries))
		}
	}
}

// assertUpload verifies the first upload call: whether it occurred, its artifact
// name, and (optionally) that the tar bundle contains the expected headers.
// Pass empty wantName/wantHeaders to skip those checks.
func assertUpload(t *testing.T, calls *[]artifactUploadCall, wantUpload bool, wantName string, wantHeaders []string) {
	t.Helper()
	if got := len(*calls) > 0; got != wantUpload {
		t.Fatalf("upload occurred = %v, want %v", got, wantUpload)
	}
	if !wantUpload {
		return
	}
	if wantName != "" && (*calls)[0].Name != wantName {
		t.Errorf("artifact name = %q, want %q", (*calls)[0].Name, wantName)
	}
	if len(wantHeaders) > 0 {
		assertTarContains(t, (*calls)[0].Bundle, wantHeaders)
	}
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

func withListen(addr string) configOption {
	return func(c *Config) { c.HTTP.Listen = addr }
}

func withHTTPTimeouts(read, write, idle time.Duration) configOption {
	return func(c *Config) {
		c.HTTP.ReadTimeout = read
		c.HTTP.WriteTimeout = write
		c.HTTP.IdleTimeout = idle
	}
}

func withBootstrapCA(path string) configOption {
	return func(c *Config) {
		c.HTTP.TLS.Enabled = true
		c.HTTP.TLS.BootstrapCAPath = path
	}
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

// ---------------------------------------------------------------------------
// Component builders
// ---------------------------------------------------------------------------

// newTestUploader returns a baseUploader with sensible test defaults (TLS disabled).
func newTestUploader(t *testing.T, serverURL string) *baseUploader {
	t.Helper()
	u, err := newBaseUploader(newAgentConfig(serverURL))
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

func withClaimHookRuntime(decision *contracts.HookRuntimeDecision) claimOption {
	return func(c *ClaimResponse) { c.HookRuntime = decision }
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

func withRunID(id string) startRunOption {
	return func(r *StartRunRequest) { r.RunID = types.RunID(id) }
}

func withJobID(id string) startRunOption {
	return func(r *StartRunRequest) { r.JobID = types.JobID(id) }
}

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

// buildManifestDefault calls buildManifestFromRequest with stepIndex=0 and MigStackUnknown.
func buildManifestDefault(req StartRunRequest) (contracts.StepManifest, error) {
	return buildManifestFromRequest(req, req.TypedOptions, 0, contracts.MigStackUnknown)
}

// buildManifestAtStep calls buildManifestFromRequest with the given stepIndex and MigStackUnknown.
func buildManifestAtStep(req StartRunRequest, step int) (contracts.StepManifest, error) {
	return buildManifestFromRequest(req, req.TypedOptions, step, contracts.MigStackUnknown)
}

// ---------------------------------------------------------------------------
// Upload test environment
// ---------------------------------------------------------------------------

// uploadTestEnv bundles the artifact upload mock server, its call recorder,
// and the runController — the trio that every upload test needs.
type uploadTestEnv struct {
	Controller *runController
	Calls      *[]artifactUploadCall
}

// newUploadTestEnv creates a mock artifact-upload server for the given
// runID/jobID pair and a runController wired to it.
func newUploadTestEnv(t *testing.T, runID, jobID string, opts ...artifactServerOption) uploadTestEnv {
	t.Helper()
	server, calls := newArtifactUploadServer(t, runID, jobID, opts...)
	controller := newTestController(t, newAgentConfig(server.URL))
	return uploadTestEnv{Controller: controller, Calls: calls}
}

// ---------------------------------------------------------------------------
// Upload assertion helpers
// ---------------------------------------------------------------------------

// assertGatePhaseIDs verifies that LogsArtifactID and LogsBundleCID on a
// RunStatsGatePhase are populated (when wantSet is true) or empty (when false).
func assertGatePhaseIDs(t *testing.T, phase *types.RunStatsGatePhase, wantSet bool) {
	t.Helper()
	if wantSet {
		if phase.LogsArtifactID == "" {
			t.Error("LogsArtifactID not set in gate phase")
		}
		if phase.LogsBundleCID == "" {
			t.Error("LogsBundleCID not set in gate phase")
		}
	} else {
		if phase.LogsArtifactID != "" {
			t.Error("LogsArtifactID should not be set on upload failure")
		}
		if phase.LogsBundleCID != "" {
			t.Error("LogsBundleCID should not be set on upload failure")
		}
	}
}

// assertArtifactNames verifies that every name in wantNames appears among the
// recorded artifact upload calls (order-independent).
func assertArtifactNames(t *testing.T, calls *[]artifactUploadCall, wantNames []string) {
	t.Helper()
	seen := make(map[string]bool, len(*calls))
	for _, c := range *calls {
		seen[c.Name] = true
	}
	for _, name := range wantNames {
		if !seen[name] {
			got := make([]string, 0, len(*calls))
			for _, c := range *calls {
				got = append(got, c.Name)
			}
			t.Fatalf("expected artifact upload name %q; got %v", name, got)
		}
	}
}

// wantReportLink describes an expected BuildGateReportLink for assertion.
type wantReportLink struct {
	Type string
	Path string
}

// assertReportLinks verifies that links contains exactly the expected entries
// (matched by Type), each with the correct Path and non-empty ID fields.
func assertReportLinks(t *testing.T, links []contracts.BuildGateReportLink, want []wantReportLink) {
	t.Helper()
	if len(links) != len(want) {
		t.Fatalf("report_links count = %d, want %d", len(links), len(want))
	}
	linkByType := make(map[string]contracts.BuildGateReportLink, len(links))
	for _, link := range links {
		linkByType[link.Type] = link
	}
	for _, wl := range want {
		link, ok := linkByType[wl.Type]
		if !ok {
			t.Fatalf("missing report link type %q in %+v", wl.Type, links)
		}
		if link.Path != wl.Path {
			t.Fatalf("%s link path = %q, want %q", wl.Type, link.Path, wl.Path)
		}
		if link.ArtifactID == "" || link.BundleCID == "" || link.URL == "" || link.DownloadURL == "" {
			t.Fatalf("expected populated link fields, got %+v", link)
		}
	}
}
