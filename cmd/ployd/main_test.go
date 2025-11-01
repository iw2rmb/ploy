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
    "github.com/jackc/pgx/v5/pgtype"
    "github.com/jackc/pgx/v5/pgconn"

	apiconfig "github.com/iw2rmb/ploy/internal/api/config"
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
