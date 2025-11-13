package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRolloutNodesDryRun verifies dry-run output for nodes rollout.
func TestRolloutNodesDryRun(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "ployd-node-test")
	if err := os.WriteFile(binPath, []byte("fake binary"), 0755); err != nil {
		t.Fatalf("create test binary: %v", err)
	}
	identityPath := filepath.Join(tmpDir, "id_test")
	if err := os.WriteFile(identityPath, []byte("fake key"), 0600); err != nil {
		t.Fatalf("create test identity: %v", err)
	}

	// Mock API responses.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/nodes" && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			resp := []struct {
				ID        string `json:"id"`
				Name      string `json:"name"`
				IPAddress string `json:"ip_address"`
				Drained   bool   `json:"drained"`
			}{
				{ID: "node1", Name: "worker-1", IPAddress: "10.0.0.101", Drained: false},
				{ID: "node2", Name: "worker-2", IPAddress: "10.0.0.102", Drained: false},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	oldClient := rolloutNodesAPIClient
	oldBaseURL := rolloutNodesAPIBaseURL
	rolloutNodesAPIClient = srv.Client()
	rolloutNodesAPIBaseURL = srv.URL
	defer func() {
		rolloutNodesAPIClient = oldClient
		rolloutNodesAPIBaseURL = oldBaseURL
	}()

	var stderr bytes.Buffer
	cfg := rolloutNodesConfig{
		All:          true,
		Concurrency:  2,
		User:         "root",
		IdentityFile: identityPath,
		BinaryPath:   binPath,
		SSHPort:      22,
		Timeout:      90,
		DryRun:       true,
	}

	err := runRolloutNodes(cfg, &stderr)
	if err != nil {
		t.Fatalf("dry-run should not error: %v", err)
	}

	out := stderr.String()
	if !strings.Contains(out, "DRY RUN: Rollout Ploy nodes") {
		t.Errorf("expected 'DRY RUN' header, got: %q", out)
	}
	if !strings.Contains(out, "Matched 2 node(s)") {
		t.Errorf("expected node count, got: %q", out)
	}
	if !strings.Contains(out, "worker-1") || !strings.Contains(out, "worker-2") {
		t.Errorf("expected node names in output, got: %q", out)
	}
	if !strings.Contains(out, "Planned actions per node:") {
		t.Errorf("expected planned actions header, got: %q", out)
	}
	if !strings.Contains(out, "Drain node via API") {
		t.Errorf("expected drain message, got: %q", out)
	}
	if !strings.Contains(out, "Wait for node to be idle") {
		t.Errorf("expected idle wait message, got: %q", out)
	}
	if !strings.Contains(out, "Upload new ployd-node binary") {
		t.Errorf("expected upload message, got: %q", out)
	}
	if !strings.Contains(out, "Install binary") {
		t.Errorf("expected install message, got: %q", out)
	}
	if !strings.Contains(out, "Restart ployd-node service") {
		t.Errorf("expected restart message, got: %q", out)
	}
	if !strings.Contains(out, "Wait for heartbeat") {
		t.Errorf("expected heartbeat message, got: %q", out)
	}
	if !strings.Contains(out, "Undrain node via API") {
		t.Errorf("expected undrain message, got: %q", out)
	}
	if !strings.Contains(out, "Batching: nodes will be updated in batches of 2") {
		t.Errorf("expected batching info, got: %q", out)
	}
	if !strings.Contains(out, "Dry run complete. No changes have been made.") {
		t.Errorf("expected completion message, got: %q", out)
	}
}
