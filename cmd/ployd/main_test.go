package main

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	apiconfig "github.com/iw2rmb/ploy/internal/api/config"
	"github.com/iw2rmb/ploy/internal/controlplane/auth"
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
	var st store.Store // nil is fine; run() does not use it yet.
	authorizer := auth.NewAuthorizer(auth.Options{
		AllowInsecure: false,
		DefaultRole:   auth.RoleControlPlane,
	})

	if err := run(ctx, cfg, st, authorizer); err != nil {
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
