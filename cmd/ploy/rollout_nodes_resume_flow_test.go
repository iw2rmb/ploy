package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRolloutNodesResume(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping rollout nodes resume test in short mode")
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
				{"id": "node-2", "name": "worker-2", "ip_address": "10.0.0.2", "drained": false, "last_heartbeat": "2025-11-03T12:00:00Z"},
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

	// Override state directory to use temp dir.
	oldEnv := os.Getenv("PLOY_CONFIG_HOME")
	_ = os.Setenv("PLOY_CONFIG_HOME", tmpDir)
	defer func() { _ = os.Setenv("PLOY_CONFIG_HOME", oldEnv) }()

	stateDir := filepath.Join(tmpDir, "rollout")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("create state dir: %v", err)
	}

	// Create a pre-existing state file with one completed node.
	stateFile := filepath.Join(stateDir, "state.json")
	now := time.Now().UTC().Format(time.RFC3339)
	state := &rolloutState{
		Version:     1,
		RetryPolicy: rolloutRetryPolicy{MaxAttempts: 3},
		Nodes: map[string]nodeRolloutStatus{
			"node-1": {
				NodeID:      "node-1",
				NodeName:    "worker-1",
				InProgress:  false,
				Completed:   true,
				Attempts:    1,
				LastAttempt: now,
			},
		},
		CreatedAt:    now,
		LastModified: now,
	}
	data, _ := json.MarshalIndent(state, "", "  ")
	if err := os.WriteFile(stateFile, data, 0o644); err != nil {
		t.Fatalf("write state file: %v", err)
	}

	// Stub the rollout host function.
	var rolledOutNodes []string
	oldHost := rolloutNodesHost
	rolloutNodesHost = func(ctx context.Context, node nodeInfo, opts rolloutNodeOptions) error {
		rolledOutNodes = append(rolledOutNodes, node.Name)
		return nil
	}
	defer func() { rolloutNodesHost = oldHost }()

	buf := &bytes.Buffer{}
	cfg := rolloutNodesConfig{
		All:          true,
		BinaryPath:   binPath,
		IdentityFile: identityPath,
	}

	err := runRolloutNodes(cfg, buf)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}

	// Verify that only worker-2 was rolled out (worker-1 was already completed).
	if len(rolledOutNodes) != 1 || rolledOutNodes[0] != "worker-2" {
		t.Fatalf("expected only worker-2 to be rolled out, got: %v", rolledOutNodes)
	}

	// Verify state file was cleaned up on success.
	if _, err := os.Stat(stateFile); !os.IsNotExist(err) {
		t.Fatalf("expected state file to be cleaned up on success")
	}
}
