package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandleRolloutNodesRequiresSelector(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleRolloutNodes(nil, buf)
	if err == nil {
		t.Fatalf("expected error when no selector provided")
	}
	if !strings.Contains(err.Error(), "either --all or --selector is required") {
		t.Fatalf("unexpected error: %v", err)
	}
	// NOTE: Rollout nodes is now accessible via `ploy cluster rollout nodes`.
	if !strings.Contains(buf.String(), "Usage: ploy cluster rollout nodes") {
		t.Fatalf("expected cluster rollout nodes usage output, got: %q", buf.String())
	}
}

func TestHandleRolloutNodesRejectsBothAllAndSelector(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleRolloutNodes([]string{"--all", "--selector", "worker-*"}, buf)
	if err == nil {
		t.Fatalf("expected error when both --all and --selector provided")
	}
	if !strings.Contains(err.Error(), "--all and --selector are mutually exclusive") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandleRolloutNodesRejectsExtraArgs(t *testing.T) {
	buf := &bytes.Buffer{}
	err := handleRolloutNodes([]string{"--all", "extra"}, buf)
	if err == nil {
		t.Fatalf("expected error for unexpected args")
	}
	if !strings.Contains(err.Error(), "unexpected arguments:") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandleRolloutNodesValidatesConcurrency(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "ployd-test")
	if err := os.WriteFile(binPath, []byte("fake binary"), 0755); err != nil {
		t.Fatalf("create test binary: %v", err)
	}
	identityPath := filepath.Join(tmpDir, "id_test")
	if err := os.WriteFile(identityPath, []byte("fake key"), 0600); err != nil {
		t.Fatalf("create test identity: %v", err)
	}

	// Stub API client to return empty node list.
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	}))
	defer mockServer.Close()

	old := rolloutNodesAPIClient
	oldURL := rolloutNodesAPIBaseURL
	rolloutNodesAPIClient = mockServer.Client()
	rolloutNodesAPIBaseURL = mockServer.URL
	defer func() {
		rolloutNodesAPIClient = old
		rolloutNodesAPIBaseURL = oldURL
	}()

	tests := []struct {
		name        string
		concurrency int
		expectErr   bool
	}{
		{"valid concurrency 1", 1, false},
		{"valid concurrency 5", 5, false},
		{"default concurrency 0", 0, false}, // 0 defaults to 1.
		{"invalid concurrency -1", -1, true},
		{"invalid concurrency -10", -10, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := rolloutNodesConfig{
				All:          true,
				Concurrency:  tt.concurrency,
				BinaryPath:   binPath,
				IdentityFile: identityPath,
			}

			err := runRolloutNodes(cfg, io.Discard)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("expected error for concurrency %d", tt.concurrency)
				}
				if !strings.Contains(err.Error(), "concurrency must be positive") {
					t.Fatalf("expected concurrency validation error, got: %v", err)
				}
			} else if err != nil && strings.Contains(err.Error(), "concurrency must be positive") {
				// For valid concurrency with empty node list, we expect success.
				t.Fatalf("unexpected concurrency validation error for valid concurrency %d: %v", tt.concurrency, err)
			}
		})
	}
}

func TestHandleRolloutNodesValidatesTimeout(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "ployd-test")
	if err := os.WriteFile(binPath, []byte("fake binary"), 0755); err != nil {
		t.Fatalf("create test binary: %v", err)
	}
	identityPath := filepath.Join(tmpDir, "id_test")
	if err := os.WriteFile(identityPath, []byte("fake key"), 0600); err != nil {
		t.Fatalf("create test identity: %v", err)
	}

	// Stub API client to return empty node list.
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	}))
	defer mockServer.Close()

	old := rolloutNodesAPIClient
	oldURL := rolloutNodesAPIBaseURL
	rolloutNodesAPIClient = mockServer.Client()
	rolloutNodesAPIBaseURL = mockServer.URL
	defer func() {
		rolloutNodesAPIClient = old
		rolloutNodesAPIBaseURL = oldURL
	}()

	tests := []struct {
		name      string
		timeout   int
		expectErr bool
	}{
		{"valid timeout 90", 90, false},
		{"valid timeout 120", 120, false},
		{"default timeout 0", 0, false}, // 0 defaults to 90.
		{"invalid timeout -1", -1, true},
		{"invalid timeout -100", -100, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := rolloutNodesConfig{
				All:          true,
				Timeout:      tt.timeout,
				BinaryPath:   binPath,
				IdentityFile: identityPath,
			}

			err := runRolloutNodes(cfg, io.Discard)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("expected error for timeout %d", tt.timeout)
				}
				if !strings.Contains(err.Error(), "timeout must be positive") {
					t.Fatalf("expected timeout validation error, got: %v", err)
				}
			} else if err != nil && strings.Contains(err.Error(), "timeout must be positive") {
				// For valid timeout with empty node list, we expect success.
				t.Fatalf("unexpected timeout validation error for valid timeout %d: %v", tt.timeout, err)
			}
		})
	}
}

func TestHandleRolloutNodesValidatesSSHPort(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "ployd-test")
	if err := os.WriteFile(binPath, []byte("fake binary"), 0755); err != nil {
		t.Fatalf("create test binary: %v", err)
	}
	identityPath := filepath.Join(tmpDir, "id_test")
	if err := os.WriteFile(identityPath, []byte("fake key"), 0600); err != nil {
		t.Fatalf("create test identity: %v", err)
	}

	// Stub API client to return empty node list.
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	}))
	defer mockServer.Close()

	old := rolloutNodesAPIClient
	oldURL := rolloutNodesAPIBaseURL
	rolloutNodesAPIClient = mockServer.Client()
	rolloutNodesAPIBaseURL = mockServer.URL
	defer func() {
		rolloutNodesAPIClient = old
		rolloutNodesAPIBaseURL = oldURL
	}()

	tests := []struct {
		name      string
		port      int
		expectErr bool
	}{
		{"valid port 22", 22, false},
		{"valid port 2222", 2222, false},
		{"default port 0", 0, false}, // defaults to 22
		{"invalid port -1", -1, true},
		{"invalid port 70000", 70000, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := rolloutNodesConfig{
				All:          true,
				SSHPort:      tt.port,
				BinaryPath:   binPath,
				IdentityFile: identityPath,
			}
			err := runRolloutNodes(cfg, io.Discard)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("expected error for ssh-port %d", tt.port)
				}
				if !strings.Contains(err.Error(), "invalid SSH port") {
					t.Fatalf("expected SSH port validation error, got: %v", err)
				}
			} else if err != nil && strings.Contains(err.Error(), "invalid SSH port") {
				t.Fatalf("unexpected SSH port validation error for valid port %d: %v", tt.port, err)
			}
		})
	}
}
