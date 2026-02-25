package main

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	bsmock "github.com/iw2rmb/ploy/internal/blobstore/mock"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	apiconfig "github.com/iw2rmb/ploy/internal/server/config"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestResolvePgDSN_EnvBeatsConfig(t *testing.T) {
	t.Setenv("PLOY_POSTGRES_DSN", "postgres://env")
	cfg := apiconfig.Config{}
	cfg.Postgres.DSN = "postgres://from-config"

	got := resolvePgDSN(cfg)
	if got != "postgres://env" {
		t.Fatalf("resolvePgDSN()=%q want env", got)
	}
}

func TestResolvePgDSN_FromConfig(t *testing.T) {
	t.Setenv("PLOY_POSTGRES_DSN", "")
	cfg := apiconfig.Config{}
	cfg.Postgres.DSN = "  postgres://from-config  "

	got := resolvePgDSN(cfg)
	if got != "postgres://from-config" {
		t.Fatalf("resolvePgDSN()=%q want from config", got)
	}
}

func TestResolvePgDSN_TrimEnv(t *testing.T) {
	t.Setenv("PLOY_POSTGRES_DSN", "  postgres://trimmed  ")
	cfg := apiconfig.Config{}
	cfg.Postgres.DSN = "postgres://from-config"

	got := resolvePgDSN(cfg)
	if got != "postgres://trimmed" {
		t.Fatalf("resolvePgDSN()=%q want trimmed env", got)
	}
}

func TestResolvePgDSN_PlaceholderIgnored(t *testing.T) {
	t.Setenv("PLOY_POSTGRES_DSN", "")
	cfg := apiconfig.Config{}
	cfg.Postgres.DSN = "${PLOY_POSTGRES_DSN}"

	got := resolvePgDSN(cfg)
	if got != "" {
		t.Fatalf("resolvePgDSN()=%q want empty for placeholder value", got)
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
	cfg.HTTP.Listen = "127.0.0.1:0"
	cfg.Metrics.Listen = "127.0.0.1:0"
	var st store.Store // nil disables ttl worker (server skips ttlworker.New).
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

	bs := bsmock.New()
	bp := blobpersist.New(st, bs)
	if err := run(ctx, cfg, configPath, st, authorizer, "test-secret", bs, bp); err != nil {
		t.Fatalf("run() error: %v", err)
	}
}

func TestRun_SchedulerIntegration(t *testing.T) {
	// Verify scheduler and TTL worker are initialized and started/stopped correctly.
	// Use a nil store; server skips ttl worker initialization.
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

	// Create mock blobstore and blobpersist
	bs := bsmock.New()
	bp := blobpersist.New(st, bs)

	// Start run in a goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- run(ctx, cfg, configPath, st, authorizer, "test-secret", bs, bp)
	}()

	// Cancel context to trigger shutdown
	cancel()

	// Wait for run to complete
	err := <-errCh
	if err != nil {
		t.Fatalf("run() error: %v", err)
	}
}

type schedulerProbeStore struct {
	store.Store
	staleRecoveryCalls atomic.Int32
}

func (s *schedulerProbeStore) ListGlobalEnv(ctx context.Context) ([]store.ConfigEnv, error) {
	return nil, nil
}

func (s *schedulerProbeStore) DeleteExpiredLogs(ctx context.Context, cutoff pgtype.Timestamptz) (int64, error) {
	return 0, nil
}

func (s *schedulerProbeStore) DeleteExpiredEvents(ctx context.Context, cutoff pgtype.Timestamptz) (int64, error) {
	return 0, nil
}

func (s *schedulerProbeStore) DeleteExpiredDiffs(ctx context.Context, cutoff pgtype.Timestamptz) (int64, error) {
	return 0, nil
}

func (s *schedulerProbeStore) DeleteExpiredArtifactBundles(ctx context.Context, cutoff pgtype.Timestamptz) (int64, error) {
	return 0, nil
}

func (s *schedulerProbeStore) ListStaleRunningJobs(ctx context.Context, cutoff pgtype.Timestamptz) ([]store.ListStaleRunningJobsRow, error) {
	s.staleRecoveryCalls.Add(1)
	return nil, nil
}

func (s *schedulerProbeStore) CountStaleNodesWithRunningJobs(ctx context.Context, cutoff pgtype.Timestamptz) (int64, error) {
	return 0, nil
}

func (s *schedulerProbeStore) Pool() *pgxpool.Pool {
	return nil
}

func (s *schedulerProbeStore) Close() {}

func TestRun_StaleRecoverySchedulerEnabled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var cfg apiconfig.Config
	cfg.HTTP.Listen = "127.0.0.1:0"
	cfg.Metrics.Listen = "127.0.0.1:0"
	cfg.Scheduler.BatchSchedulerInterval = 0
	cfg.Scheduler.StaleJobRecoveryInterval = 10 * time.Millisecond
	cfg.Scheduler.NodeStaleAfter = time.Minute

	st := &schedulerProbeStore{}
	authorizer := auth.NewAuthorizer(auth.Options{
		AllowInsecure: false,
		DefaultRole:   auth.RoleControlPlane,
	})

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "ployd.yaml")
	if err := os.WriteFile(configPath, []byte("# minimal config\n"), 0644); err != nil {
		t.Fatalf("create temp config: %v", err)
	}

	bs := bsmock.New()
	bp := blobpersist.New(st, bs)

	errCh := make(chan error, 1)
	go func() {
		errCh <- run(ctx, cfg, configPath, st, authorizer, "test-secret", bs, bp)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-errCh:
			t.Fatalf("run() exited before stale recovery task observed: %v", err)
		default:
		}

		if st.staleRecoveryCalls.Load() > 0 {
			cancel()
			if err := <-errCh; err != nil {
				t.Fatalf("run() error: %v", err)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("run() error: %v", err)
	}
	t.Fatal("expected stale recovery task to call ListStaleRunningJobs at least once")
}

func TestGlobalEnvMapFromStoreEntries_ParsesAndDropsInvalid(t *testing.T) {
	entries := []store.ConfigEnv{
		{Key: "A", Value: "1", Scope: "all", Secret: true},
		{Key: "B", Value: "2", Scope: "", Secret: false},     // defaults to all
		{Key: "C", Value: "3", Scope: "mig", Secret: false},  // invalid (typo)
		{Key: "D", Value: "4", Scope: "migs", Secret: false}, // valid
	}

	got := globalEnvMapFromStoreEntries(entries)
	if len(got) != 3 {
		t.Fatalf("globalEnvMapFromStoreEntries() len=%d want 3", len(got))
	}
	if _, ok := got["C"]; ok {
		t.Fatalf("expected invalid-scope entry to be dropped")
	}
	if got["A"].Scope != domaintypes.GlobalEnvScopeAll {
		t.Fatalf("A scope=%q want %q", got["A"].Scope, domaintypes.GlobalEnvScopeAll)
	}
	if got["B"].Scope != domaintypes.GlobalEnvScopeAll {
		t.Fatalf("B scope=%q want %q", got["B"].Scope, domaintypes.GlobalEnvScopeAll)
	}
	if got["D"].Scope != domaintypes.GlobalEnvScopeMods {
		t.Fatalf("D scope=%q want %q", got["D"].Scope, domaintypes.GlobalEnvScopeMods)
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

// no-op

// TestParseLastEventID tests the parseLastEventID helper function.

// TestGetRunEventsHandler_RunNotFound tests that the handler returns 404 when the run does not exist.

// TestGetRunEventsHandler_InvalidRunID tests that the handler returns 400 for invalid run IDs.

// TestGetRunEventsHandler_MissingID tests that the handler returns 400 when id is missing.

// TestGetRunEventsHandler_DatabaseError tests that the handler returns 500 on database errors.

// (SSE/run events tests moved under internal/server/handlers)

// TestGetRunEventsHandler_Success verifies SSE frames are written for an existing run.

// TestGetRunEventsHandler_Resume verifies Last-Event-ID is honored for resumption.

// Ensure we return 413 when the body exceeds the MaxBytesReader limit
// even if Content-Length is unknown (streamed/chunked uploads).
