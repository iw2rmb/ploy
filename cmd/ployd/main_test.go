package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	apiconfig "github.com/iw2rmb/ploy/internal/api/config"
	"github.com/iw2rmb/ploy/internal/api/events"
	"github.com/iw2rmb/ploy/internal/controlplane/auth"
	internalPKI "github.com/iw2rmb/ploy/internal/pki"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestResolvePgDSN_Precedence(t *testing.T) {
	t.Setenv("PLOY_SERVER_PG_DSN", "postgres://primary-env")
	t.Setenv("PLOY_POSTGRES_DSN", "postgres://alias-env")
	cfg := apiconfig.Config{}
	cfg.Postgres.DSN = "postgres://from-config"

	got := resolvePgDSN(cfg)
	if got != "postgres://primary-env" {
		t.Fatalf("resolvePgDSN()=%q want primary env", got)
	}
}

func TestResolvePgDSN_Alias(t *testing.T) {
	t.Setenv("PLOY_SERVER_PG_DSN", "")
	t.Setenv("PLOY_POSTGRES_DSN", "postgres://alias-env")
	cfg := apiconfig.Config{}
	cfg.Postgres.DSN = "postgres://from-config"

	got := resolvePgDSN(cfg)
	if got != "postgres://alias-env" {
		t.Fatalf("resolvePgDSN()=%q want alias env", got)
	}
}

func TestResolvePgDSN_FromConfig(t *testing.T) {
	t.Setenv("PLOY_SERVER_PG_DSN", "")
	t.Setenv("PLOY_POSTGRES_DSN", "")
	cfg := apiconfig.Config{}
	cfg.Postgres.DSN = "  postgres://from-config  "

	got := resolvePgDSN(cfg)
	if got != "postgres://from-config" {
		t.Fatalf("resolvePgDSN()=%q want from config", got)
	}
}

func TestResolvePgDSN_TrimEnvPrimary(t *testing.T) {
	t.Setenv("PLOY_SERVER_PG_DSN", "  postgres://trimmed-primary  ")
	t.Setenv("PLOY_POSTGRES_DSN", "postgres://alias-env")
	cfg := apiconfig.Config{}
	cfg.Postgres.DSN = "postgres://from-config"

	got := resolvePgDSN(cfg)
	if got != "postgres://trimmed-primary" {
		t.Fatalf("resolvePgDSN()=%q want trimmed primary env", got)
	}
}

func TestResolvePgDSN_TrimEnvAlias(t *testing.T) {
	t.Setenv("PLOY_SERVER_PG_DSN", " ")
	t.Setenv("PLOY_POSTGRES_DSN", "  postgres://trimmed-alias  ")
	cfg := apiconfig.Config{}
	cfg.Postgres.DSN = "postgres://from-config"

	got := resolvePgDSN(cfg)
	if got != "postgres://trimmed-alias" {
		t.Fatalf("resolvePgDSN()=%q want trimmed alias env", got)
	}
}

func TestParseLogLevel(t *testing.T) {
	cases := map[string]slog.Level{
		"debug":   slog.LevelDebug,
		"info":    slog.LevelInfo,
		"WARN":    slog.LevelWarn,
		"warning": slog.LevelWarn,
		"error":   slog.LevelError,
		"unknown": slog.LevelInfo,
		"":        slog.LevelInfo,
	}
	for in, want := range cases {
		got := parseLogLevel(in)
		if got != want {
			t.Fatalf("parseLogLevel(%q)=%v want %v", in, got, want)
		}
	}
}

func TestInitLogging_FileWrites(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "ployd.log")

	cfg := apiconfig.LoggingConfig{File: logPath, JSON: false, Level: "debug"}
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	if err := initLogging(cfg); err != nil {
		t.Fatalf("initLogging() error: %v", err)
	}
	slog.Info("hello", "k", "v")

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if len(data) == 0 || !strings.Contains(string(data), "hello") {
		t.Fatalf("expected log file to contain entry, got: %q", string(data))
	}
}

func TestRun_Shutdown(t *testing.T) {
	// Ensure run returns after context cancellation without error.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var cfg apiconfig.Config
	var st store.Store // nil is fine; ttlworker.New handles nil store gracefully.
	authorizer := auth.NewAuthorizer(auth.Options{
		AllowInsecure: false,
		DefaultRole:   auth.RoleControlPlane,
	})

	// Create a temporary config file for the watcher.
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "ployd.yaml")
	if err := os.WriteFile(configPath, []byte("# minimal config\n"), 0644); err != nil {
		t.Fatalf("create temp config: %v", err)
	}

	if err := run(ctx, cfg, configPath, st, authorizer); err != nil {
		t.Fatalf("run() error: %v", err)
	}
}

func TestRun_SchedulerIntegration(t *testing.T) {
	// Verify scheduler and TTL worker are initialized and started/stopped correctly.
	// Use a nil store since ttlworker.New handles it gracefully by returning nil worker.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var cfg apiconfig.Config
	// Set custom TTL config to verify it's passed to the worker
	cfg.Scheduler.TTL = 0         // Will use default 30 days
	cfg.Scheduler.TTLInterval = 0 // Will use default 1 hour
	cfg.Scheduler.DropPartitions = false
	// Use random ports to avoid conflicts
	cfg.HTTP.Listen = "127.0.0.1:0"
	cfg.Metrics.Listen = "127.0.0.1:0"

	var st store.Store // nil store
	authorizer := auth.NewAuthorizer(auth.Options{
		AllowInsecure: false,
		DefaultRole:   auth.RoleControlPlane,
	})

	// Create a temporary config file for the watcher.
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "ployd.yaml")
	if err := os.WriteFile(configPath, []byte("# minimal config\n"), 0644); err != nil {
		t.Fatalf("create temp config: %v", err)
	}

	// Start run in a goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- run(ctx, cfg, configPath, st, authorizer)
	}()

	// Cancel context to trigger shutdown
	cancel()

	// Wait for run to complete
	err := <-errCh
	if err != nil {
		t.Fatalf("run() error: %v", err)
	}
}

func TestAuthorizer_DefaultConfig(t *testing.T) {
	// Verify authorizer is configured with mTLS enforcement and RoleControlPlane default.
	authorizer := auth.NewAuthorizer(auth.Options{
		AllowInsecure: false,
		DefaultRole:   auth.RoleControlPlane,
	})

	if authorizer == nil {
		t.Fatal("NewAuthorizer() returned nil")
	}

	// Test that insecure requests are rejected (no client certificate).
	// This verifies AllowInsecure=false is working.
	t.Run("RejectsInsecureRequest", func(t *testing.T) {
		req := newTestRequest(t, "GET", "/v1/test")
		rr := newTestRecorder()
		handler := authorizer.Middleware(auth.RoleControlPlane)(testHandler(t))

		handler.ServeHTTP(rr, req)
		if rr.Code != 403 {
			t.Fatalf("expected 403 for insecure request, got %d", rr.Code)
		}
	})
}

// Helper functions for testing
func newTestRequest(t *testing.T, method, path string) *http.Request {
	req, err := http.NewRequest(method, path, nil)
	if err != nil {
		t.Fatal(err)
	}
	return req
}

func newTestRecorder() *httptest.ResponseRecorder {
	return httptest.NewRecorder()
}

func testHandler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}
}

// mockStore is a minimal Store implementation for testing handlers.
type mockStore struct {
	store.Store
	updateCertMetadataCalled bool
	updateCertMetadataParams store.UpdateNodeCertMetadataParams
	updateCertMetadataErr    error

	createRepoCalled bool
	createRepoParams store.CreateRepoParams
	createRepoResult store.Repo
	createRepoErr    error

	listReposCalled bool
	listReposResult []store.Repo
	listReposErr    error

	createModCalled bool
	createModParams store.CreateModParams
	createModResult store.Mod
	createModErr    error

	listModsCalled bool
	listModsResult []store.Mod
	listModsErr    error

	listModsByRepoCalled bool
	listModsByRepoParams pgtype.UUID
	listModsByRepoResult []store.Mod
	listModsByRepoErr    error

	createRunCalled bool
	createRunParams store.CreateRunParams
	createRunResult store.Run
	createRunErr    error

	getRunCalled bool
	getRunParams pgtype.UUID
	getRunResult store.Run
	getRunErr    error

	getRunTimingCalled bool
	getRunTimingParams pgtype.UUID
	getRunTimingResult store.RunsTiming
	getRunTimingErr    error

	listRunsTimingsCalled bool
	listRunsTimingsParams store.ListRunsTimingsParams
	listRunsTimingsResult []store.RunsTiming
	listRunsTimingsErr    error

	deleteRunCalled bool
	deleteRunParams pgtype.UUID
	deleteRunErr    error
}

func (m *mockStore) UpdateNodeCertMetadata(ctx context.Context, params store.UpdateNodeCertMetadataParams) error {
	m.updateCertMetadataCalled = true
	m.updateCertMetadataParams = params
	return m.updateCertMetadataErr
}

func (m *mockStore) CreateRepo(ctx context.Context, params store.CreateRepoParams) (store.Repo, error) {
	m.createRepoCalled = true
	m.createRepoParams = params
	return m.createRepoResult, m.createRepoErr
}

func (m *mockStore) ListRepos(ctx context.Context) ([]store.Repo, error) {
	m.listReposCalled = true
	return m.listReposResult, m.listReposErr
}

func (m *mockStore) CreateMod(ctx context.Context, params store.CreateModParams) (store.Mod, error) {
	m.createModCalled = true
	m.createModParams = params
	return m.createModResult, m.createModErr
}

func (m *mockStore) ListMods(ctx context.Context) ([]store.Mod, error) {
	m.listModsCalled = true
	return m.listModsResult, m.listModsErr
}

func (m *mockStore) ListModsByRepo(ctx context.Context, repoID pgtype.UUID) ([]store.Mod, error) {
	m.listModsByRepoCalled = true
	m.listModsByRepoParams = repoID
	return m.listModsByRepoResult, m.listModsByRepoErr
}

func (m *mockStore) CreateRun(ctx context.Context, params store.CreateRunParams) (store.Run, error) {
	m.createRunCalled = true
	m.createRunParams = params
	return m.createRunResult, m.createRunErr
}

func (m *mockStore) GetRun(ctx context.Context, id pgtype.UUID) (store.Run, error) {
	m.getRunCalled = true
	m.getRunParams = id
	return m.getRunResult, m.getRunErr
}

func (m *mockStore) GetRunTiming(ctx context.Context, id pgtype.UUID) (store.RunsTiming, error) {
	m.getRunTimingCalled = true
	m.getRunTimingParams = id
	return m.getRunTimingResult, m.getRunTimingErr
}

func (m *mockStore) ListRunsTimings(ctx context.Context, arg store.ListRunsTimingsParams) ([]store.RunsTiming, error) {
	m.listRunsTimingsCalled = true
	m.listRunsTimingsParams = arg
	return m.listRunsTimingsResult, m.listRunsTimingsErr
}

func (m *mockStore) DeleteRun(ctx context.Context, id pgtype.UUID) error {
	m.deleteRunCalled = true
	m.deleteRunParams = id
	return m.deleteRunErr
}

// no-op

func TestPKISignHandler_Success(t *testing.T) {
	// Generate test CA.
	ca, err := internalPKI.GenerateCA("test-cluster", time.Now())
	if err != nil {
		t.Fatalf("generate CA: %v", err)
	}

	// Generate test CSR.
	nodeID := uuid.New().String()
	keyBundle, csrPEM, err := internalPKI.GenerateNodeCSR(nodeID, "test-cluster", "192.168.1.100")
	if err != nil {
		t.Fatalf("generate CSR: %v", err)
	}
	_ = keyBundle // Unused in this test, but would be stored by node.

	// Set CA environment variables.
	t.Setenv("PLOY_SERVER_CA_CERT", ca.CertPEM)
	t.Setenv("PLOY_SERVER_CA_KEY", ca.KeyPEM)

	// Create mock store.
	mockSt := &mockStore{}

	// Create request.
	reqBody := map[string]string{
		"node_id": nodeID,
		"csr":     string(csrPEM),
	}
	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/pki/sign", bytes.NewReader(reqJSON))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	// Call handler.
	handler := pkiSignHandler(mockSt)
	handler.ServeHTTP(rr, req)

	// Verify response status.
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify response body.
	var resp struct {
		Certificate string `json:"certificate"`
		CABundle    string `json:"ca_bundle"`
		Serial      string `json:"serial"`
		Fingerprint string `json:"fingerprint"`
		NotBefore   string `json:"not_before"`
		NotAfter    string `json:"not_after"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	// Verify response fields are non-empty.
	if resp.Certificate == "" {
		t.Error("expected non-empty certificate")
	}
	if resp.CABundle == "" {
		t.Error("expected non-empty ca_bundle")
	}
	if resp.Serial == "" {
		t.Error("expected non-empty serial")
	}
	if resp.Fingerprint == "" {
		t.Error("expected non-empty fingerprint")
	}
	if resp.NotBefore == "" {
		t.Error("expected non-empty not_before")
	}
	if resp.NotAfter == "" {
		t.Error("expected non-empty not_after")
	}

	// Verify CA bundle matches.
	if resp.CABundle != ca.CertPEM {
		t.Error("expected CA bundle to match generated CA")
	}

	// Verify timestamps are valid RFC3339 format.
	if _, err := time.Parse(time.RFC3339, resp.NotBefore); err != nil {
		t.Errorf("invalid not_before timestamp: %v", err)
	}
	if _, err := time.Parse(time.RFC3339, resp.NotAfter); err != nil {
		t.Errorf("invalid not_after timestamp: %v", err)
	}

	// Verify store was called with correct parameters.
	if !mockSt.updateCertMetadataCalled {
		t.Fatal("expected UpdateNodeCertMetadata to be called")
	}

	expectedUUID, _ := uuid.Parse(nodeID)
	if mockSt.updateCertMetadataParams.ID.Bytes != expectedUUID {
		t.Errorf("expected node ID %v, got %v", expectedUUID, mockSt.updateCertMetadataParams.ID.Bytes)
	}
	if mockSt.updateCertMetadataParams.CertSerial == nil || *mockSt.updateCertMetadataParams.CertSerial != resp.Serial {
		t.Errorf("expected serial %s, got %v", resp.Serial, mockSt.updateCertMetadataParams.CertSerial)
	}
	if mockSt.updateCertMetadataParams.CertFingerprint == nil || *mockSt.updateCertMetadataParams.CertFingerprint != resp.Fingerprint {
		t.Errorf("expected fingerprint %s, got %v", resp.Fingerprint, mockSt.updateCertMetadataParams.CertFingerprint)
	}
}

func TestPKISignHandler_InvalidJSON(t *testing.T) {
	mockSt := &mockStore{}
	req := httptest.NewRequest(http.MethodPost, "/v1/pki/sign", bytes.NewReader([]byte("invalid json")))
	rr := httptest.NewRecorder()

	handler := pkiSignHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "invalid request") {
		t.Errorf("expected error message about invalid request, got: %s", rr.Body.String())
	}
}

func TestPKISignHandler_InvalidNodeID(t *testing.T) {
	mockSt := &mockStore{}
	reqBody := map[string]string{
		"node_id": "not-a-uuid",
		"csr":     "some-csr",
	}
	reqJSON, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/pki/sign", bytes.NewReader(reqJSON))
	rr := httptest.NewRecorder()

	handler := pkiSignHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "invalid node_id") {
		t.Errorf("expected error message about invalid node_id, got: %s", rr.Body.String())
	}
}

func TestPKISignHandler_EmptyCSR(t *testing.T) {
	mockSt := &mockStore{}
	reqBody := map[string]string{
		"node_id": uuid.New().String(),
		"csr":     "  ",
	}
	reqJSON, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/pki/sign", bytes.NewReader(reqJSON))
	rr := httptest.NewRecorder()

	handler := pkiSignHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "csr field is required") {
		t.Errorf("expected error message about empty CSR, got: %s", rr.Body.String())
	}
}

func TestPKISignHandler_CANotConfigured(t *testing.T) {
	// Ensure CA environment variables are not set.
	t.Setenv("PLOY_SERVER_CA_CERT", "")
	t.Setenv("PLOY_SERVER_CA_KEY", "")

	mockSt := &mockStore{}
	reqBody := map[string]string{
		"node_id": uuid.New().String(),
		"csr":     "some-csr",
	}
	reqJSON, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/pki/sign", bytes.NewReader(reqJSON))
	rr := httptest.NewRecorder()

	handler := pkiSignHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "PKI not configured") {
		t.Errorf("expected error message about PKI not configured, got: %s", rr.Body.String())
	}
}

func TestPKISignHandler_CANotConfigured_Whitespace(t *testing.T) {
	// Whitespace-only env values should be treated as unset.
	t.Setenv("PLOY_SERVER_CA_CERT", "   \n\t  ")
	t.Setenv("PLOY_SERVER_CA_KEY", "  \t\n ")

	mockSt := &mockStore{}
	reqBody := map[string]string{
		"node_id": uuid.New().String(),
		"csr":     "some-csr",
	}
	reqJSON, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/pki/sign", bytes.NewReader(reqJSON))
	rr := httptest.NewRecorder()

	handler := pkiSignHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "PKI not configured") {
		t.Errorf("expected error message about PKI not configured, got: %s", rr.Body.String())
	}
}

func TestPKISignHandler_InvalidCSR(t *testing.T) {
	// Generate test CA.
	ca, err := internalPKI.GenerateCA("test-cluster", time.Now())
	if err != nil {
		t.Fatalf("generate CA: %v", err)
	}

	t.Setenv("PLOY_SERVER_CA_CERT", ca.CertPEM)
	t.Setenv("PLOY_SERVER_CA_KEY", ca.KeyPEM)

	mockSt := &mockStore{}
	reqBody := map[string]string{
		"node_id": uuid.New().String(),
		"csr":     "invalid-csr-pem-data",
	}
	reqJSON, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/pki/sign", bytes.NewReader(reqJSON))
	rr := httptest.NewRecorder()

	handler := pkiSignHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "sign failed") {
		t.Errorf("expected error message about sign failed, got: %s", rr.Body.String())
	}
}

func TestPKISignHandler_StoreError(t *testing.T) {
	// Generate test CA.
	ca, err := internalPKI.GenerateCA("test-cluster", time.Now())
	if err != nil {
		t.Fatalf("generate CA: %v", err)
	}

	// Generate test CSR.
	nodeID := uuid.New().String()
	_, csrPEM, err := internalPKI.GenerateNodeCSR(nodeID, "test-cluster", "192.168.1.100")
	if err != nil {
		t.Fatalf("generate CSR: %v", err)
	}

	t.Setenv("PLOY_SERVER_CA_CERT", ca.CertPEM)
	t.Setenv("PLOY_SERVER_CA_KEY", ca.KeyPEM)

	// Create mock store that returns an error.
	mockSt := &mockStore{
		updateCertMetadataErr: context.DeadlineExceeded,
	}

	reqBody := map[string]string{
		"node_id": nodeID,
		"csr":     string(csrPEM),
	}
	reqJSON, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/pki/sign", bytes.NewReader(reqJSON))
	rr := httptest.NewRecorder()

	handler := pkiSignHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "failed to persist certificate metadata") {
		t.Errorf("expected error message about persist failure, got: %s", rr.Body.String())
	}
}

func TestPKISignHandler_CSRNodeMismatch(t *testing.T) {
	// Generate test CA.
	ca, err := internalPKI.GenerateCA("test-cluster", time.Now())
	if err != nil {
		t.Fatalf("generate CA: %v", err)
	}

	// Generate CSR for a different node ID.
	actualNodeID := uuid.New().String()
	keyBundle, csrPEM, err := internalPKI.GenerateNodeCSR(actualNodeID, "test-cluster", "192.168.1.10")
	if err != nil {
		t.Fatalf("generate CSR: %v", err)
	}
	_ = keyBundle

	t.Setenv("PLOY_SERVER_CA_CERT", ca.CertPEM)
	t.Setenv("PLOY_SERVER_CA_KEY", ca.KeyPEM)

	// Prepare request with a different node_id than the CSR CN.
	reqBody := map[string]string{
		"node_id": uuid.New().String(), // mismatch
		"csr":     string(csrPEM),
	}
	reqJSON, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/pki/sign", bytes.NewReader(reqJSON))
	rr := httptest.NewRecorder()

	handler := pkiSignHandler(&mockStore{})
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 on mismatch, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "csr subject common name must match") {
		t.Errorf("expected mismatch error, got: %s", rr.Body.String())
	}
}

func TestCreateRepoHandler_Success(t *testing.T) {
	repoID := uuid.New()
	branch := "main"
	commitSha := "abc123"
	now := time.Now()

	mockSt := &mockStore{
		createRepoResult: store.Repo{
			ID: pgtype.UUID{
				Bytes: repoID,
				Valid: true,
			},
			Url:       "https://github.com/example/repo",
			Branch:    &branch,
			CommitSha: &commitSha,
			CreatedAt: pgtype.Timestamptz{
				Time:  now,
				Valid: true,
			},
		},
	}

	reqBody := map[string]interface{}{
		"url":        "https://github.com/example/repo",
		"branch":     "main",
		"commit_sha": "abc123",
	}
	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/repos", bytes.NewReader(reqJSON))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler := createRepoHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		ID        string  `json:"id"`
		URL       string  `json:"url"`
		Branch    *string `json:"branch"`
		CommitSha *string `json:"commit_sha"`
		CreatedAt string  `json:"created_at"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.ID != repoID.String() {
		t.Errorf("expected id %s, got %s", repoID.String(), resp.ID)
	}
	if resp.URL != "https://github.com/example/repo" {
		t.Errorf("expected url https://github.com/example/repo, got %s", resp.URL)
	}
	if resp.Branch == nil || *resp.Branch != "main" {
		t.Errorf("expected branch main, got %v", resp.Branch)
	}
	if resp.CommitSha == nil || *resp.CommitSha != "abc123" {
		t.Errorf("expected commit_sha abc123, got %v", resp.CommitSha)
	}

	if !mockSt.createRepoCalled {
		t.Fatal("expected CreateRepo to be called")
	}
	if mockSt.createRepoParams.Url != "https://github.com/example/repo" {
		t.Errorf("expected url https://github.com/example/repo, got %s", mockSt.createRepoParams.Url)
	}
}

func TestCreateRepoHandler_InvalidJSON(t *testing.T) {
	mockSt := &mockStore{}
	req := httptest.NewRequest(http.MethodPost, "/v1/repos", bytes.NewReader([]byte("invalid json")))
	rr := httptest.NewRecorder()

	handler := createRepoHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "invalid request") {
		t.Errorf("expected error message about invalid request, got: %s", rr.Body.String())
	}
}

func TestCreateRepoHandler_EmptyURL(t *testing.T) {
	mockSt := &mockStore{}
	reqBody := map[string]string{
		"url": "  ",
	}
	reqJSON, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/repos", bytes.NewReader(reqJSON))
	rr := httptest.NewRecorder()

	handler := createRepoHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "url field is required") {
		t.Errorf("expected error message about url required, got: %s", rr.Body.String())
	}
}

func TestCreateRepoHandler_DuplicateURL(t *testing.T) {
	mockSt := &mockStore{
		createRepoErr: &pgconn.PgError{Code: "23505", ConstraintName: "repos_url_unique"},
	}

	reqBody := map[string]string{
		"url": "https://github.com/example/repo",
	}
	reqJSON, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/repos", bytes.NewReader(reqJSON))
	rr := httptest.NewRecorder()

	handler := createRepoHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "repository with this url already exists") {
		t.Errorf("expected error message about duplicate url, got: %s", rr.Body.String())
	}
}

func TestListReposHandler_Success(t *testing.T) {
	repo1ID := uuid.New()
	repo2ID := uuid.New()
	branch := "main"
	now := time.Now()

	mockSt := &mockStore{
		listReposResult: []store.Repo{
			{
				ID: pgtype.UUID{
					Bytes: repo1ID,
					Valid: true,
				},
				Url:       "https://github.com/example/repo1",
				Branch:    &branch,
				CommitSha: nil,
				CreatedAt: pgtype.Timestamptz{
					Time:  now,
					Valid: true,
				},
			},
			{
				ID: pgtype.UUID{
					Bytes: repo2ID,
					Valid: true,
				},
				Url:       "https://github.com/example/repo2",
				Branch:    nil,
				CommitSha: nil,
				CreatedAt: pgtype.Timestamptz{
					Time:  now.Add(-1 * time.Hour),
					Valid: true,
				},
			},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/repos", nil)
	rr := httptest.NewRecorder()

	handler := listReposHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var wrapper struct {
		Repos []struct {
			ID        string  `json:"id"`
			URL       string  `json:"url"`
			Branch    *string `json:"branch,omitempty"`
			CommitSha *string `json:"commit_sha,omitempty"`
			CreatedAt string  `json:"created_at"`
		} `json:"repos"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&wrapper); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if len(wrapper.Repos) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(wrapper.Repos))
	}

	if wrapper.Repos[0].ID != repo1ID.String() {
		t.Errorf("expected first repo id %s, got %s", repo1ID.String(), wrapper.Repos[0].ID)
	}
	if wrapper.Repos[1].ID != repo2ID.String() {
		t.Errorf("expected second repo id %s, got %s", repo2ID.String(), wrapper.Repos[1].ID)
	}

	if !mockSt.listReposCalled {
		t.Fatal("expected ListRepos to be called")
	}
}

func TestListReposHandler_EmptyList(t *testing.T) {
	mockSt := &mockStore{
		listReposResult: []store.Repo{},
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/repos", nil)
	rr := httptest.NewRecorder()

	handler := listReposHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var resp struct {
		Repos []struct {
			ID        string  `json:"id"`
			URL       string  `json:"url"`
			Branch    *string `json:"branch,omitempty"`
			CommitSha *string `json:"commit_sha,omitempty"`
			CreatedAt string  `json:"created_at"`
		} `json:"repos"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if len(resp.Repos) != 0 {
		t.Fatalf("expected empty list, got %d repos", len(resp.Repos))
	}
}

func TestListReposHandler_DatabaseError(t *testing.T) {
	mockSt := &mockStore{
		listReposErr: context.DeadlineExceeded,
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/repos", nil)
	rr := httptest.NewRecorder()

	handler := listReposHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "failed to list repositories") {
		t.Errorf("expected error message about database error, got: %s", rr.Body.String())
	}
}

func TestCreateModHandler_Success(t *testing.T) {
	modID := uuid.New()
	repoID := uuid.New()
	createdBy := "user@example.com"
	now := time.Now()
	specJSON := []byte(`{"stages":["build","test"]}`)

	mockSt := &mockStore{
		createModResult: store.Mod{
			ID: pgtype.UUID{
				Bytes: modID,
				Valid: true,
			},
			RepoID: pgtype.UUID{
				Bytes: repoID,
				Valid: true,
			},
			Spec:      specJSON,
			CreatedBy: &createdBy,
			CreatedAt: pgtype.Timestamptz{
				Time:  now,
				Valid: true,
			},
		},
	}

	reqBody := map[string]interface{}{
		"repo_id":    repoID.String(),
		"spec":       json.RawMessage(specJSON),
		"created_by": createdBy,
	}
	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/mods/crud", bytes.NewReader(reqJSON))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler := createModHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		ID        string          `json:"id"`
		RepoID    string          `json:"repo_id"`
		Spec      json.RawMessage `json:"spec"`
		CreatedBy *string         `json:"created_by"`
		CreatedAt string          `json:"created_at"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.ID != modID.String() {
		t.Errorf("expected id %s, got %s", modID.String(), resp.ID)
	}
	if resp.RepoID != repoID.String() {
		t.Errorf("expected repo_id %s, got %s", repoID.String(), resp.RepoID)
	}
	if resp.CreatedBy == nil || *resp.CreatedBy != createdBy {
		t.Errorf("expected created_by %s, got %v", createdBy, resp.CreatedBy)
	}

	if !mockSt.createModCalled {
		t.Fatal("expected CreateMod to be called")
	}
	if uuid.UUID(mockSt.createModParams.RepoID.Bytes) != repoID {
		t.Errorf("expected repo_id %s, got %s", repoID.String(), uuid.UUID(mockSt.createModParams.RepoID.Bytes).String())
	}
}

func TestCreateModHandler_InvalidJSON(t *testing.T) {
	mockSt := &mockStore{}
	req := httptest.NewRequest(http.MethodPost, "/v1/mods/crud", bytes.NewReader([]byte("invalid json")))
	rr := httptest.NewRecorder()

	handler := createModHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "invalid request") {
		t.Errorf("expected error message about invalid request, got: %s", rr.Body.String())
	}
}

func TestCreateModHandler_EmptyRepoID(t *testing.T) {
	mockSt := &mockStore{}
	reqBody := map[string]interface{}{
		"repo_id": "  ",
		"spec":    json.RawMessage(`{}`),
	}
	reqJSON, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/mods/crud", bytes.NewReader(reqJSON))
	rr := httptest.NewRecorder()

	handler := createModHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "repo_id field is required") {
		t.Errorf("expected error message about repo_id required, got: %s", rr.Body.String())
	}
}

func TestCreateModHandler_InvalidRepoID(t *testing.T) {
	mockSt := &mockStore{}
	reqBody := map[string]interface{}{
		"repo_id": "not-a-uuid",
		"spec":    json.RawMessage(`{}`),
	}
	reqJSON, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/mods/crud", bytes.NewReader(reqJSON))
	rr := httptest.NewRecorder()

	handler := createModHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "invalid repo_id") {
		t.Errorf("expected error message about invalid repo_id, got: %s", rr.Body.String())
	}
}

func TestCreateModHandler_EmptySpec(t *testing.T) {
	mockSt := &mockStore{}
	reqBody := map[string]interface{}{
		"repo_id": uuid.New().String(),
	}
	reqJSON, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/mods/crud", bytes.NewReader(reqJSON))
	rr := httptest.NewRecorder()

	handler := createModHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "spec field is required") {
		t.Errorf("expected error message about spec required, got: %s", rr.Body.String())
	}
}

func TestCreateModHandler_RepoNotFound(t *testing.T) {
	mockSt := &mockStore{
		createModErr: &pgconn.PgError{Code: "23503"},
	}

	reqBody := map[string]interface{}{
		"repo_id": uuid.New().String(),
		"spec":    json.RawMessage(`{}`),
	}
	reqJSON, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/mods/crud", bytes.NewReader(reqJSON))
	rr := httptest.NewRecorder()

	handler := createModHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "repository not found") {
		t.Errorf("expected error message about repository not found, got: %s", rr.Body.String())
	}
}

func TestListModsHandler_AllMods(t *testing.T) {
	mod1ID := uuid.New()
	mod2ID := uuid.New()
	repo1ID := uuid.New()
	repo2ID := uuid.New()
	now := time.Now()
	spec1 := []byte(`{"stages":["build"]}`)
	spec2 := []byte(`{"stages":["test"]}`)

	mockSt := &mockStore{
		listModsResult: []store.Mod{
			{
				ID: pgtype.UUID{
					Bytes: mod1ID,
					Valid: true,
				},
				RepoID: pgtype.UUID{
					Bytes: repo1ID,
					Valid: true,
				},
				Spec:      spec1,
				CreatedBy: nil,
				CreatedAt: pgtype.Timestamptz{
					Time:  now,
					Valid: true,
				},
			},
			{
				ID: pgtype.UUID{
					Bytes: mod2ID,
					Valid: true,
				},
				RepoID: pgtype.UUID{
					Bytes: repo2ID,
					Valid: true,
				},
				Spec:      spec2,
				CreatedBy: nil,
				CreatedAt: pgtype.Timestamptz{
					Time:  now.Add(-1 * time.Hour),
					Valid: true,
				},
			},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/mods/crud", nil)
	rr := httptest.NewRecorder()

	handler := listModsHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var wrapper struct {
		Mods []struct {
			ID        string          `json:"id"`
			RepoID    string          `json:"repo_id"`
			Spec      json.RawMessage `json:"spec"`
			CreatedBy *string         `json:"created_by,omitempty"`
			CreatedAt string          `json:"created_at"`
		} `json:"mods"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&wrapper); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if len(wrapper.Mods) != 2 {
		t.Fatalf("expected 2 mods, got %d", len(wrapper.Mods))
	}

	if wrapper.Mods[0].ID != mod1ID.String() {
		t.Errorf("expected first mod id %s, got %s", mod1ID.String(), wrapper.Mods[0].ID)
	}
	if wrapper.Mods[1].ID != mod2ID.String() {
		t.Errorf("expected second mod id %s, got %s", mod2ID.String(), wrapper.Mods[1].ID)
	}

	if !mockSt.listModsCalled {
		t.Fatal("expected ListMods to be called")
	}
}

func TestListModsHandler_ByRepoID(t *testing.T) {
	modID := uuid.New()
	repoID := uuid.New()
	now := time.Now()
	spec := []byte(`{"stages":["build"]}`)
	createdBy := "user@example.com"

	mockSt := &mockStore{
		listModsByRepoResult: []store.Mod{
			{
				ID: pgtype.UUID{
					Bytes: modID,
					Valid: true,
				},
				RepoID: pgtype.UUID{
					Bytes: repoID,
					Valid: true,
				},
				Spec:      spec,
				CreatedBy: &createdBy,
				CreatedAt: pgtype.Timestamptz{
					Time:  now,
					Valid: true,
				},
			},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/mods/crud?repo_id="+repoID.String(), nil)
	rr := httptest.NewRecorder()

	handler := listModsHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var wrapper struct {
		Mods []struct {
			ID        string          `json:"id"`
			RepoID    string          `json:"repo_id"`
			Spec      json.RawMessage `json:"spec"`
			CreatedBy *string         `json:"created_by,omitempty"`
			CreatedAt string          `json:"created_at"`
		} `json:"mods"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&wrapper); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if len(wrapper.Mods) != 1 {
		t.Fatalf("expected 1 mod, got %d", len(wrapper.Mods))
	}

	if wrapper.Mods[0].ID != modID.String() {
		t.Errorf("expected mod id %s, got %s", modID.String(), wrapper.Mods[0].ID)
	}
	if wrapper.Mods[0].RepoID != repoID.String() {
		t.Errorf("expected repo_id %s, got %s", repoID.String(), wrapper.Mods[0].RepoID)
	}

	if !mockSt.listModsByRepoCalled {
		t.Fatal("expected ListModsByRepo to be called")
	}
	if uuid.UUID(mockSt.listModsByRepoParams.Bytes) != repoID {
		t.Errorf("expected repo_id param %s, got %s", repoID.String(), uuid.UUID(mockSt.listModsByRepoParams.Bytes).String())
	}
}

func TestListModsHandler_InvalidRepoID(t *testing.T) {
	mockSt := &mockStore{}

	req := httptest.NewRequest(http.MethodGet, "/v1/mods/crud?repo_id=not-a-uuid", nil)
	rr := httptest.NewRecorder()

	handler := listModsHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "invalid repo_id") {
		t.Errorf("expected error message about invalid repo_id, got: %s", rr.Body.String())
	}
}

func TestListModsHandler_EmptyList(t *testing.T) {
	mockSt := &mockStore{
		listModsResult: []store.Mod{},
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/mods/crud", nil)
	rr := httptest.NewRecorder()

	handler := listModsHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var resp struct {
		Mods []struct {
			ID        string          `json:"id"`
			RepoID    string          `json:"repo_id"`
			Spec      json.RawMessage `json:"spec"`
			CreatedBy *string         `json:"created_by,omitempty"`
			CreatedAt string          `json:"created_at"`
		} `json:"mods"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if len(resp.Mods) != 0 {
		t.Fatalf("expected empty list, got %d mods", len(resp.Mods))
	}
}

func TestListModsHandler_DatabaseError(t *testing.T) {
	mockSt := &mockStore{
		listModsErr: context.DeadlineExceeded,
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/mods/crud", nil)
	rr := httptest.NewRecorder()

	handler := listModsHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "failed to list mods") {
		t.Errorf("expected error message about database error, got: %s", rr.Body.String())
	}
}

func TestListModsHandler_DatabaseErrorByRepo(t *testing.T) {
	mockSt := &mockStore{
		listModsByRepoErr: context.DeadlineExceeded,
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/mods/crud?repo_id="+uuid.New().String(), nil)
	rr := httptest.NewRecorder()

	handler := listModsHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "failed to list mods") {
		t.Errorf("expected error message about database error, got: %s", rr.Body.String())
	}
}

func TestCreateRunHandler_Success(t *testing.T) {
	runID := uuid.New()
	modID := uuid.New()
	commitSha := "abc123def456"
	now := time.Now()

	mockSt := &mockStore{
		createRunResult: store.Run{
			ID: pgtype.UUID{
				Bytes: runID,
				Valid: true,
			},
			ModID: pgtype.UUID{
				Bytes: modID,
				Valid: true,
			},
			Status:    store.RunStatusQueued,
			BaseRef:   "main",
			TargetRef: "feature-branch",
			CommitSha: &commitSha,
			CreatedAt: pgtype.Timestamptz{
				Time:  now,
				Valid: true,
			},
		},
	}

	reqBody := map[string]interface{}{
		"mod_id":     modID.String(),
		"base_ref":   "main",
		"target_ref": "feature-branch",
		"commit_sha": commitSha,
	}
	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader(reqJSON))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler := createRunHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		RunID string `json:"run_id"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.RunID != runID.String() {
		t.Errorf("expected run_id %s, got %s", runID.String(), resp.RunID)
	}

	if !mockSt.createRunCalled {
		t.Fatal("expected CreateRun to be called")
	}
	if uuid.UUID(mockSt.createRunParams.ModID.Bytes) != modID {
		t.Errorf("expected mod_id %s, got %s", modID.String(), uuid.UUID(mockSt.createRunParams.ModID.Bytes).String())
	}
	if mockSt.createRunParams.Status != store.RunStatusQueued {
		t.Errorf("expected status queued, got %s", mockSt.createRunParams.Status)
	}
	if mockSt.createRunParams.BaseRef != "main" {
		t.Errorf("expected base_ref main, got %s", mockSt.createRunParams.BaseRef)
	}
	if mockSt.createRunParams.TargetRef != "feature-branch" {
		t.Errorf("expected target_ref feature-branch, got %s", mockSt.createRunParams.TargetRef)
	}
	if mockSt.createRunParams.CommitSha == nil || *mockSt.createRunParams.CommitSha != commitSha {
		t.Errorf("expected commit_sha %s, got %v", commitSha, mockSt.createRunParams.CommitSha)
	}
}

func TestCreateRunHandler_InvalidJSON(t *testing.T) {
	mockSt := &mockStore{}
	req := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader([]byte("invalid json")))
	rr := httptest.NewRecorder()

	handler := createRunHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "invalid request") {
		t.Errorf("expected error message about invalid request, got: %s", rr.Body.String())
	}
}

func TestCreateRunHandler_EmptyModID(t *testing.T) {
	mockSt := &mockStore{}
	reqBody := map[string]interface{}{
		"mod_id":     "  ",
		"base_ref":   "main",
		"target_ref": "feature",
	}
	reqJSON, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader(reqJSON))
	rr := httptest.NewRecorder()

	handler := createRunHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "mod_id field is required") {
		t.Errorf("expected error message about mod_id required, got: %s", rr.Body.String())
	}
}

func TestCreateRunHandler_EmptyBaseRef(t *testing.T) {
	mockSt := &mockStore{}
	reqBody := map[string]interface{}{
		"mod_id":     uuid.New().String(),
		"base_ref":   "  ",
		"target_ref": "feature",
	}
	reqJSON, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader(reqJSON))
	rr := httptest.NewRecorder()

	handler := createRunHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "base_ref field is required") {
		t.Errorf("expected error message about base_ref required, got: %s", rr.Body.String())
	}
}

func TestCreateRunHandler_EmptyTargetRef(t *testing.T) {
	mockSt := &mockStore{}
	reqBody := map[string]interface{}{
		"mod_id":     uuid.New().String(),
		"base_ref":   "main",
		"target_ref": "  ",
	}
	reqJSON, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader(reqJSON))
	rr := httptest.NewRecorder()

	handler := createRunHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "target_ref field is required") {
		t.Errorf("expected error message about target_ref required, got: %s", rr.Body.String())
	}
}

func TestCreateRunHandler_InvalidModID(t *testing.T) {
	mockSt := &mockStore{}
	reqBody := map[string]interface{}{
		"mod_id":     "not-a-uuid",
		"base_ref":   "main",
		"target_ref": "feature",
	}
	reqJSON, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader(reqJSON))
	rr := httptest.NewRecorder()

	handler := createRunHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "invalid mod_id") {
		t.Errorf("expected error message about invalid mod_id, got: %s", rr.Body.String())
	}
}

func TestCreateRunHandler_ModNotFound(t *testing.T) {
	mockSt := &mockStore{
		createRunErr: &pgconn.PgError{Code: "23503"},
	}

	reqBody := map[string]interface{}{
		"mod_id":     uuid.New().String(),
		"base_ref":   "main",
		"target_ref": "feature",
	}
	reqJSON, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader(reqJSON))
	rr := httptest.NewRecorder()

	handler := createRunHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "mod not found") {
		t.Errorf("expected error message about mod not found, got: %s", rr.Body.String())
	}
}

func TestCreateRunHandler_DatabaseError(t *testing.T) {
	mockSt := &mockStore{
		createRunErr: context.DeadlineExceeded,
	}

	reqBody := map[string]interface{}{
		"mod_id":     uuid.New().String(),
		"base_ref":   "main",
		"target_ref": "feature",
	}
	reqJSON, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader(reqJSON))
	rr := httptest.NewRecorder()

	handler := createRunHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "failed to create run") {
		t.Errorf("expected error message about database error, got: %s", rr.Body.String())
	}
}

func TestCreateRunHandler_WithoutCommitSha(t *testing.T) {
	runID := uuid.New()
	modID := uuid.New()
	now := time.Now()

	mockSt := &mockStore{
		createRunResult: store.Run{
			ID: pgtype.UUID{
				Bytes: runID,
				Valid: true,
			},
			ModID: pgtype.UUID{
				Bytes: modID,
				Valid: true,
			},
			Status:    store.RunStatusQueued,
			BaseRef:   "main",
			TargetRef: "feature-branch",
			CommitSha: nil,
			CreatedAt: pgtype.Timestamptz{
				Time:  now,
				Valid: true,
			},
		},
	}

	reqBody := map[string]interface{}{
		"mod_id":     modID.String(),
		"base_ref":   "main",
		"target_ref": "feature-branch",
	}
	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader(reqJSON))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler := createRunHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		RunID string `json:"run_id"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.RunID != runID.String() {
		t.Errorf("expected run_id %s, got %s", runID.String(), resp.RunID)
	}

	if !mockSt.createRunCalled {
		t.Fatal("expected CreateRun to be called")
	}
	if mockSt.createRunParams.CommitSha != nil {
		t.Errorf("expected commit_sha to be nil, got %v", mockSt.createRunParams.CommitSha)
	}
}

func TestGetRunHandler_Success(t *testing.T) {
	runID := uuid.New()
	modID := uuid.New()
	nodeID := uuid.New()
	createdAt := time.Now().UTC()
	startedAt := time.Now().UTC().Add(time.Second)
	commitSha := "abc123def456"
	reason := "test reason"
	stats := `{"foo":"bar"}`

	mockSt := &mockStore{
		getRunResult: store.Run{
			ID: pgtype.UUID{
				Bytes: runID,
				Valid: true,
			},
			ModID: pgtype.UUID{
				Bytes: modID,
				Valid: true,
			},
			Status: store.RunStatusRunning,
			Reason: &reason,
			CreatedAt: pgtype.Timestamptz{
				Time:  createdAt,
				Valid: true,
			},
			StartedAt: pgtype.Timestamptz{
				Time:  startedAt,
				Valid: true,
			},
			FinishedAt: pgtype.Timestamptz{
				Valid: false,
			},
			NodeID: pgtype.UUID{
				Bytes: nodeID,
				Valid: true,
			},
			BaseRef:   "main",
			TargetRef: "feature-branch",
			CommitSha: &commitSha,
			Stats:     []byte(stats),
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/runs?id="+runID.String(), nil)
	rr := httptest.NewRecorder()

	handler := getRunHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		ID         string          `json:"id"`
		ModID      string          `json:"mod_id"`
		Status     string          `json:"status"`
		Reason     *string         `json:"reason,omitempty"`
		CreatedAt  string          `json:"created_at"`
		StartedAt  *string         `json:"started_at,omitempty"`
		FinishedAt *string         `json:"finished_at,omitempty"`
		NodeID     *string         `json:"node_id,omitempty"`
		BaseRef    string          `json:"base_ref"`
		TargetRef  string          `json:"target_ref"`
		CommitSha  *string         `json:"commit_sha,omitempty"`
		Stats      json.RawMessage `json:"stats,omitempty"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.ID != runID.String() {
		t.Errorf("expected id %s, got %s", runID.String(), resp.ID)
	}
	if resp.ModID != modID.String() {
		t.Errorf("expected mod_id %s, got %s", modID.String(), resp.ModID)
	}
	if resp.Status != "running" {
		t.Errorf("expected status 'running', got %s", resp.Status)
	}
	if resp.Reason == nil || *resp.Reason != reason {
		t.Errorf("expected reason %q, got %v", reason, resp.Reason)
	}
	if resp.CommitSha == nil || *resp.CommitSha != commitSha {
		t.Errorf("expected commit_sha %q, got %v", commitSha, resp.CommitSha)
	}
	if resp.NodeID == nil || *resp.NodeID != nodeID.String() {
		t.Errorf("expected node_id %s, got %v", nodeID.String(), resp.NodeID)
	}
	if len(resp.Stats) == 0 || string(resp.Stats) != stats {
		t.Errorf("expected stats %q, got %s", stats, string(resp.Stats))
	}

	if !mockSt.getRunCalled {
		t.Fatal("expected GetRun to be called")
	}
}

func TestGetRunHandler_MissingID(t *testing.T) {
	mockSt := &mockStore{}

	req := httptest.NewRequest(http.MethodGet, "/v1/runs", nil)
	rr := httptest.NewRecorder()

	handler := getRunHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}

	body := strings.TrimSpace(rr.Body.String())
	if !strings.Contains(body, "id query parameter is required") {
		t.Errorf("expected error about missing id, got: %s", body)
	}

	if mockSt.getRunCalled {
		t.Fatal("expected GetRun not to be called")
	}
}

func TestGetRunHandler_InvalidID(t *testing.T) {
	mockSt := &mockStore{}

	req := httptest.NewRequest(http.MethodGet, "/v1/runs?id=not-a-uuid", nil)
	rr := httptest.NewRecorder()

	handler := getRunHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}

	body := strings.TrimSpace(rr.Body.String())
	if !strings.Contains(body, "invalid id") {
		t.Errorf("expected error about invalid id, got: %s", body)
	}

	if mockSt.getRunCalled {
		t.Fatal("expected GetRun not to be called")
	}
}

func TestGetRunHandler_NotFound(t *testing.T) {
	runID := uuid.New()
	mockSt := &mockStore{
		getRunErr: pgx.ErrNoRows,
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/runs?id="+runID.String(), nil)
	rr := httptest.NewRecorder()

	handler := getRunHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", rr.Code, rr.Body.String())
	}

	body := strings.TrimSpace(rr.Body.String())
	if !strings.Contains(body, "run not found") {
		t.Errorf("expected error about run not found, got: %s", body)
	}

	if !mockSt.getRunCalled {
		t.Fatal("expected GetRun to be called")
	}
}

func TestGetRunHandler_DatabaseError(t *testing.T) {
	runID := uuid.New()
	mockSt := &mockStore{
		getRunErr: &pgconn.PgError{Code: "08000"},
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/runs?id="+runID.String(), nil)
	rr := httptest.NewRecorder()

	handler := getRunHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d: %s", rr.Code, rr.Body.String())
	}

	body := strings.TrimSpace(rr.Body.String())
	if !strings.Contains(body, "failed to get run") {
		t.Errorf("expected error about database failure, got: %s", body)
	}

	if !mockSt.getRunCalled {
		t.Fatal("expected GetRun to be called")
	}
}

func TestGetRunHandler_PathParam_Success(t *testing.T) {
	runID := uuid.New()
	modID := uuid.New()
	now := time.Now().UTC()

	mockSt := &mockStore{
		getRunResult: store.Run{
			ID:        pgtype.UUID{Bytes: runID, Valid: true},
			ModID:     pgtype.UUID{Bytes: modID, Valid: true},
			Status:    store.RunStatusQueued,
			BaseRef:   "main",
			TargetRef: "feature",
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
			Stats:     []byte("{}"),
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+runID.String(), nil)
	req.SetPathValue("id", runID.String())
	rr := httptest.NewRecorder()

	handler := getRunHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		ID    string `json:"id"`
		ModID string `json:"mod_id"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ID != runID.String() {
		t.Errorf("expected id %s, got %s", runID.String(), resp.ID)
	}
	if resp.ModID != modID.String() {
		t.Errorf("expected mod_id %s, got %s", modID.String(), resp.ModID)
	}
}

func TestDeleteRunHandler_Success(t *testing.T) {
	runID := uuid.New()
	modID := uuid.New()

	mockSt := &mockStore{
		getRunResult: store.Run{
			ID: pgtype.UUID{
				Bytes: runID,
				Valid: true,
			},
			ModID: pgtype.UUID{
				Bytes: modID,
				Valid: true,
			},
			Status:    store.RunStatusQueued,
			BaseRef:   "main",
			TargetRef: "feature-branch",
			CreatedAt: pgtype.Timestamptz{
				Time:  time.Now().UTC(),
				Valid: true,
			},
			Stats: []byte("{}"),
		},
	}

	req := httptest.NewRequest(http.MethodDelete, "/v1/runs/"+runID.String(), nil)
	req.SetPathValue("id", runID.String())
	rr := httptest.NewRecorder()

	handler := deleteRunHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}

	if !mockSt.getRunCalled {
		t.Fatal("expected GetRun to be called")
	}
	if !mockSt.deleteRunCalled {
		t.Fatal("expected DeleteRun to be called")
	}
}

func TestDeleteRunHandler_MissingID(t *testing.T) {
	mockSt := &mockStore{}

	req := httptest.NewRequest(http.MethodDelete, "/v1/runs/", nil)
	req.SetPathValue("id", "")
	rr := httptest.NewRecorder()

	handler := deleteRunHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}

	body := strings.TrimSpace(rr.Body.String())
	if !strings.Contains(body, "id path parameter is required") {
		t.Errorf("expected error about missing id, got: %s", body)
	}

	if mockSt.getRunCalled {
		t.Fatal("expected GetRun not to be called")
	}
	if mockSt.deleteRunCalled {
		t.Fatal("expected DeleteRun not to be called")
	}
}

func TestDeleteRunHandler_InvalidID(t *testing.T) {
	mockSt := &mockStore{}

	req := httptest.NewRequest(http.MethodDelete, "/v1/runs/not-a-uuid", nil)
	req.SetPathValue("id", "not-a-uuid")
	rr := httptest.NewRecorder()

	handler := deleteRunHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}

	body := strings.TrimSpace(rr.Body.String())
	if !strings.Contains(body, "invalid id") {
		t.Errorf("expected error about invalid id, got: %s", body)
	}

	if mockSt.getRunCalled {
		t.Fatal("expected GetRun not to be called")
	}
	if mockSt.deleteRunCalled {
		t.Fatal("expected DeleteRun not to be called")
	}
}

func TestDeleteRunHandler_NotFound(t *testing.T) {
	runID := uuid.New()
	mockSt := &mockStore{
		getRunErr: pgx.ErrNoRows,
	}

	req := httptest.NewRequest(http.MethodDelete, "/v1/runs/"+runID.String(), nil)
	req.SetPathValue("id", runID.String())
	rr := httptest.NewRecorder()

	handler := deleteRunHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", rr.Code, rr.Body.String())
	}

	body := strings.TrimSpace(rr.Body.String())
	if !strings.Contains(body, "run not found") {
		t.Errorf("expected error about run not found, got: %s", body)
	}

	if !mockSt.getRunCalled {
		t.Fatal("expected GetRun to be called")
	}
	if mockSt.deleteRunCalled {
		t.Fatal("expected DeleteRun not to be called after GetRun failed")
	}
}

func TestDeleteRunHandler_DeleteError(t *testing.T) {
	runID := uuid.New()
	modID := uuid.New()

	mockSt := &mockStore{
		getRunResult: store.Run{
			ID: pgtype.UUID{
				Bytes: runID,
				Valid: true,
			},
			ModID: pgtype.UUID{
				Bytes: modID,
				Valid: true,
			},
			Status:    store.RunStatusQueued,
			BaseRef:   "main",
			TargetRef: "feature-branch",
			CreatedAt: pgtype.Timestamptz{
				Time:  time.Now().UTC(),
				Valid: true,
			},
			Stats: []byte("{}"),
		},
		deleteRunErr: &pgconn.PgError{Code: "08000"},
	}

	req := httptest.NewRequest(http.MethodDelete, "/v1/runs/"+runID.String(), nil)
	req.SetPathValue("id", runID.String())
	rr := httptest.NewRecorder()

	handler := deleteRunHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d: %s", rr.Code, rr.Body.String())
	}

	body := strings.TrimSpace(rr.Body.String())
	if !strings.Contains(body, "failed to delete run") {
		t.Errorf("expected error about delete failure, got: %s", body)
	}

	if !mockSt.getRunCalled {
		t.Fatal("expected GetRun to be called")
	}
	if !mockSt.deleteRunCalled {
		t.Fatal("expected DeleteRun to be called")
	}
}

func TestGetRunTimingHandler_Success(t *testing.T) {
	runID := uuid.New()

	mockSt := &mockStore{
		getRunTimingResult: store.RunsTiming{
			ID: pgtype.UUID{
				Bytes: runID,
				Valid: true,
			},
			QueueMs: 1500,
			RunMs:   3000,
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/runs?id="+runID.String()+"&view=timing", nil)
	rr := httptest.NewRecorder()

	handler := getRunHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		ID      string `json:"id"`
		QueueMs int64  `json:"queue_ms"`
		RunMs   int64  `json:"run_ms"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.ID != runID.String() {
		t.Errorf("expected id %s, got %s", runID.String(), resp.ID)
	}
	if resp.QueueMs != 1500 {
		t.Errorf("expected queue_ms 1500, got %d", resp.QueueMs)
	}
	if resp.RunMs != 3000 {
		t.Errorf("expected run_ms 3000, got %d", resp.RunMs)
	}

	if !mockSt.getRunTimingCalled {
		t.Fatal("expected GetRunTiming to be called")
	}
}

func TestGetRunTimingHandler_PathParam(t *testing.T) {
	runID := uuid.New()

	mockSt := &mockStore{
		getRunTimingResult: store.RunsTiming{
			ID: pgtype.UUID{
				Bytes: runID,
				Valid: true,
			},
			QueueMs: 500,
			RunMs:   2000,
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+runID.String()+"?view=timing", nil)
	req.SetPathValue("id", runID.String())
	rr := httptest.NewRecorder()

	handler := getRunHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		ID      string `json:"id"`
		QueueMs int64  `json:"queue_ms"`
		RunMs   int64  `json:"run_ms"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.ID != runID.String() {
		t.Errorf("expected id %s, got %s", runID.String(), resp.ID)
	}
	if resp.QueueMs != 500 {
		t.Errorf("expected queue_ms 500, got %d", resp.QueueMs)
	}
	if resp.RunMs != 2000 {
		t.Errorf("expected run_ms 2000, got %d", resp.RunMs)
	}

	if !mockSt.getRunTimingCalled {
		t.Fatal("expected GetRunTiming to be called")
	}
}

func TestGetRunTimingHandler_ZeroValues(t *testing.T) {
	runID := uuid.New()

	mockSt := &mockStore{
		getRunTimingResult: store.RunsTiming{
			ID: pgtype.UUID{
				Bytes: runID,
				Valid: true,
			},
			QueueMs: 0,
			RunMs:   0,
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/runs?id="+runID.String()+"&view=timing", nil)
	rr := httptest.NewRecorder()

	handler := getRunHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		ID      string `json:"id"`
		QueueMs int64  `json:"queue_ms"`
		RunMs   int64  `json:"run_ms"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.ID != runID.String() {
		t.Errorf("expected id %s, got %s", runID.String(), resp.ID)
	}
	if resp.QueueMs != 0 {
		t.Errorf("expected queue_ms 0, got %d", resp.QueueMs)
	}
	if resp.RunMs != 0 {
		t.Errorf("expected run_ms 0, got %d", resp.RunMs)
	}

	if !mockSt.getRunTimingCalled {
		t.Fatal("expected GetRunTiming to be called")
	}
}

func TestListRunTimingsHandler_Default(t *testing.T) {
	mockSt := &mockStore{listRunsTimingsResult: []store.RunsTiming{}}

	req := httptest.NewRequest(http.MethodGet, "/v1/runs?view=timing", nil)
	rr := httptest.NewRecorder()

	handler := getRunHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Timings []struct {
			ID      string `json:"id"`
			QueueMs int64  `json:"queue_ms"`
			RunMs   int64  `json:"run_ms"`
		} `json:"timings"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Timings) != 0 {
		t.Fatalf("expected empty timings, got %d", len(resp.Timings))
	}
	if !mockSt.listRunsTimingsCalled {
		t.Fatal("expected ListRunsTimings to be called")
	}
	if mockSt.listRunsTimingsParams.Limit != 100 || mockSt.listRunsTimingsParams.Offset != 0 {
		t.Fatalf("expected default limit=100 offset=0, got limit=%d offset=%d", mockSt.listRunsTimingsParams.Limit, mockSt.listRunsTimingsParams.Offset)
	}
}

func TestListRunTimingsHandler_WithPagination(t *testing.T) {
	runA := uuid.New()
	runB := uuid.New()
	mockSt := &mockStore{listRunsTimingsResult: []store.RunsTiming{
		{ID: pgtype.UUID{Bytes: runA, Valid: true}, QueueMs: 10, RunMs: 20},
		{ID: pgtype.UUID{Bytes: runB, Valid: true}, QueueMs: 30, RunMs: 40},
	}}

	req := httptest.NewRequest(http.MethodGet, "/v1/runs?view=timing&limit=5&offset=10", nil)
	rr := httptest.NewRecorder()

	handler := getRunHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Timings []struct {
			ID      string `json:"id"`
			QueueMs int64  `json:"queue_ms"`
			RunMs   int64  `json:"run_ms"`
		} `json:"timings"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Timings) != 2 {
		t.Fatalf("expected 2 timings, got %d", len(resp.Timings))
	}
	if resp.Timings[0].ID != runA.String() || resp.Timings[0].QueueMs != 10 || resp.Timings[0].RunMs != 20 {
		t.Fatalf("unexpected first timing: %+v", resp.Timings[0])
	}
	if !mockSt.listRunsTimingsCalled {
		t.Fatal("expected ListRunsTimings to be called")
	}
	if mockSt.listRunsTimingsParams.Limit != 5 || mockSt.listRunsTimingsParams.Offset != 10 {
		t.Fatalf("expected limit=5 offset=10, got limit=%d offset=%d", mockSt.listRunsTimingsParams.Limit, mockSt.listRunsTimingsParams.Offset)
	}
}

func TestListRunTimingsHandler_InvalidParams(t *testing.T) {
	mockSt := &mockStore{}
	// invalid limit
	req := httptest.NewRequest(http.MethodGet, "/v1/runs?view=timing&limit=zero", nil)
	rr := httptest.NewRecorder()
	handler := getRunHandler(mockSt)
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid limit, got %d", rr.Code)
	}
	// invalid offset
	req2 := httptest.NewRequest(http.MethodGet, "/v1/runs?view=timing&offset=-1", nil)
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid offset, got %d", rr2.Code)
	}
}

func TestGetRunTimingHandler_InvalidID(t *testing.T) {
	mockSt := &mockStore{}

	req := httptest.NewRequest(http.MethodGet, "/v1/runs?id=invalid-uuid&view=timing", nil)
	rr := httptest.NewRecorder()

	handler := getRunHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}

	body := strings.TrimSpace(rr.Body.String())
	if !strings.Contains(body, "invalid id") {
		t.Errorf("expected error about invalid id, got: %s", body)
	}

	if mockSt.getRunTimingCalled {
		t.Fatal("expected GetRunTiming not to be called")
	}
}

func TestGetRunTimingHandler_NotFound(t *testing.T) {
	runID := uuid.New()

	mockSt := &mockStore{
		getRunTimingErr: pgx.ErrNoRows,
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/runs?id="+runID.String()+"&view=timing", nil)
	rr := httptest.NewRecorder()

	handler := getRunHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", rr.Code, rr.Body.String())
	}

	body := strings.TrimSpace(rr.Body.String())
	if !strings.Contains(body, "run not found") {
		t.Errorf("expected error about run not found, got: %s", body)
	}

	if !mockSt.getRunTimingCalled {
		t.Fatal("expected GetRunTiming to be called")
	}
}

func TestGetRunTimingHandler_DatabaseError(t *testing.T) {
	runID := uuid.New()

	mockSt := &mockStore{
		getRunTimingErr: &pgconn.PgError{Code: "08000"},
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/runs?id="+runID.String()+"&view=timing", nil)
	rr := httptest.NewRecorder()

	handler := getRunHandler(mockSt)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d: %s", rr.Code, rr.Body.String())
	}

	body := strings.TrimSpace(rr.Body.String())
	if !strings.Contains(body, "failed to get run timing") {
		t.Errorf("expected error about database failure, got: %s", body)
	}

	if !mockSt.getRunTimingCalled {
		t.Fatal("expected GetRunTiming to be called")
	}
}

// TestParseLastEventID tests the parseLastEventID helper function.
func TestParseLastEventID(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   int64
	}{
		{"empty header", "", 0},
		{"valid id", "42", 42},
		{"with whitespace", "  123  ", 123},
		{"zero", "0", 0},
		{"large number", "9223372036854775807", 9223372036854775807},
		{"invalid string", "abc", 0},
		{"invalid mixed", "12abc", 0},
		{"negative", "-5", -5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseLastEventID(tt.header)
			if got != tt.want {
				t.Errorf("parseLastEventID(%q) = %d, want %d", tt.header, got, tt.want)
			}
		})
	}
}

// TestGetRunEventsHandler_RunNotFound tests that the handler returns 404 when the run does not exist.
func TestGetRunEventsHandler_RunNotFound(t *testing.T) {
	mockSt := &mockStore{
		getRunErr: pgx.ErrNoRows,
	}

	// Create events service.
	eventsService, err := createTestEventsService()
	if err != nil {
		t.Fatalf("create events service: %v", err)
	}

	runID := uuid.New().String()
	req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+runID+"/events", nil)
	req.SetPathValue("id", runID)
	rr := httptest.NewRecorder()

	handler := getRunEventsHandler(mockSt, eventsService)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d: %s", rr.Code, rr.Body.String())
	}

	if !mockSt.getRunCalled {
		t.Error("expected GetRun to be called")
	}
}

// TestGetRunEventsHandler_InvalidRunID tests that the handler returns 400 for invalid run IDs.
func TestGetRunEventsHandler_InvalidRunID(t *testing.T) {
	mockSt := &mockStore{}

	eventsService, err := createTestEventsService()
	if err != nil {
		t.Fatalf("create events service: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/runs/invalid-uuid/events", nil)
	req.SetPathValue("id", "invalid-uuid")
	rr := httptest.NewRecorder()

	handler := getRunEventsHandler(mockSt, eventsService)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}

	if mockSt.getRunCalled {
		t.Error("expected GetRun not to be called with invalid UUID")
	}
}

// TestGetRunEventsHandler_MissingID tests that the handler returns 400 when id is missing.
func TestGetRunEventsHandler_MissingID(t *testing.T) {
	mockSt := &mockStore{}

	eventsService, err := createTestEventsService()
	if err != nil {
		t.Fatalf("create events service: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/runs//events", nil)
	req.SetPathValue("id", "")
	rr := httptest.NewRecorder()

	handler := getRunEventsHandler(mockSt, eventsService)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestGetRunEventsHandler_DatabaseError tests that the handler returns 500 on database errors.
func TestGetRunEventsHandler_DatabaseError(t *testing.T) {
	mockSt := &mockStore{
		getRunErr: &pgconn.PgError{Code: "XX000", Message: "test db error"},
	}

	eventsService, err := createTestEventsService()
	if err != nil {
		t.Fatalf("create events service: %v", err)
	}

	runID := uuid.New().String()
	req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+runID+"/events", nil)
	req.SetPathValue("id", runID)
	rr := httptest.NewRecorder()

	handler := getRunEventsHandler(mockSt, eventsService)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d: %s", rr.Code, rr.Body.String())
	}

	if !mockSt.getRunCalled {
		t.Error("expected GetRun to be called")
	}
}

// createTestEventsService creates an events service for testing.
func createTestEventsService() (*events.Service, error) {
	return events.New(events.Options{
		BufferSize:  32,
		HistorySize: 256,
		Logger:      slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	})
}
