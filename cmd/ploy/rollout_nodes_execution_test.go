package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRolloutNodesEmptyList(t *testing.T) {
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

	buf := &bytes.Buffer{}
	cfg := rolloutNodesConfig{
		All:          true,
		BinaryPath:   binPath,
		IdentityFile: identityPath,
	}

	err := runRolloutNodes(cfg, buf)
	if err != nil {
		t.Fatalf("expected success for empty node list, got: %v", err)
	}
	if !strings.Contains(buf.String(), "No nodes matched the selector") {
		t.Fatalf("expected message about no matching nodes, got: %q", buf.String())
	}
}

func TestRolloutNodesWithSelector(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping rollout nodes test in short mode")
	}

	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "ployd-test")
	if err := os.WriteFile(binPath, []byte("fake binary"), 0755); err != nil {
		t.Fatalf("create test binary: %v", err)
	}
	identityPath := filepath.Join(tmpDir, "id_test")
	if err := os.WriteFile(identityPath, []byte("fake key"), 0600); err != nil {
		t.Fatalf("create test identity: %v", err)
	}

	// Create a mock server for API calls.
	drainCalled := 0
	undrainCalled := 0
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/nodes" {
			nodes := []map[string]interface{}{
				{"id": "node-1", "name": "worker-1", "ip_address": "10.0.0.1", "drained": false, "last_heartbeat": "2025-11-03T12:00:00Z"},
				{"id": "node-2", "name": "worker-2", "ip_address": "10.0.0.2", "drained": false, "last_heartbeat": "2025-11-03T12:00:00Z"},
				{"id": "node-3", "name": "server-1", "ip_address": "10.0.0.3", "drained": false, "last_heartbeat": "2025-11-03T12:00:00Z"},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(nodes)
			return
		}
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/drain") {
			drainCalled++
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/undrain") {
			undrainCalled++
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
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

	// Stub the rollout host function to avoid real SSH.
	oldHost := rolloutNodesHost
	rolloutNodesHost = func(ctx context.Context, node nodeInfo, opts rolloutNodeOptions) error {
		return nil
	}
	defer func() { rolloutNodesHost = oldHost }()

	buf := &bytes.Buffer{}
	cfg := rolloutNodesConfig{
		Selector:     "worker-*",
		BinaryPath:   binPath,
		IdentityFile: identityPath,
	}

	err := runRolloutNodes(cfg, buf)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}

	// Verify that two worker nodes were selected.
	if !strings.Contains(buf.String(), "Matched 2 node(s)") {
		t.Fatalf("expected 2 nodes matched, got: %q", buf.String())
	}

	// Verify drain and undrain were called for each node.
	if drainCalled != 2 {
		t.Fatalf("expected 2 drain calls, got %d", drainCalled)
	}
	if undrainCalled != 2 {
		t.Fatalf("expected 2 undrain calls, got %d", undrainCalled)
	}
}

func TestRolloutNodesWithFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping rollout nodes failure test in short mode")
	}

	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "ployd-test")
	if err := os.WriteFile(binPath, []byte("fake binary"), 0755); err != nil {
		t.Fatalf("create test binary: %v", err)
	}
	identityPath := filepath.Join(tmpDir, "id_test")
	if err := os.WriteFile(identityPath, []byte("fake key"), 0600); err != nil {
		t.Fatalf("create test identity: %v", err)
	}

	// Create a mock server for API calls.
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/nodes" {
			nodes := []map[string]interface{}{
				{"id": "node-1", "name": "worker-1", "ip_address": "10.0.0.1", "drained": false, "last_heartbeat": "2025-11-03T12:00:00Z"},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(nodes)
			return
		}
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/drain") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/undrain") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
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

	// Stub the rollout host function to simulate a failure.
	oldHost := rolloutNodesHost
	rolloutNodesHost = func(ctx context.Context, node nodeInfo, opts rolloutNodeOptions) error {
		return errors.New("simulated SSH failure")
	}
	defer func() { rolloutNodesHost = oldHost }()

	buf := &bytes.Buffer{}
	cfg := rolloutNodesConfig{
		All:          true,
		BinaryPath:   binPath,
		IdentityFile: identityPath,
	}

	err := runRolloutNodes(cfg, buf)
	if err == nil {
		t.Fatalf("expected error for rollout failure")
	}
	if !strings.Contains(err.Error(), "1 node(s) failed") {
		t.Fatalf("expected error about failed nodes, got: %v", err)
	}
	if !strings.Contains(buf.String(), "Rollout failed") {
		t.Fatalf("expected failure message in output, got: %q", buf.String())
	}
}
