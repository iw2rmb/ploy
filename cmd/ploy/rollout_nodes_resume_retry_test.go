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
	"time"
)

// TestRolloutNodesResumeWithRetryMetadata verifies that resume state tracks attempts and enforces max attempts.
func TestRolloutNodesResumeWithRetryMetadata(t *testing.T) {
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
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/nodes" && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			resp := []struct {
				ID        string `json:"id"`
				Name      string `json:"name"`
				IPAddress string `json:"ip_address"`
				Drained   bool   `json:"drained"`
			}{
				{ID: "node-1", Name: "worker-1", IPAddress: "10.0.0.101", Drained: false},
				{ID: "node-2", Name: "worker-2", IPAddress: "10.0.0.102", Drained: false},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/v1/nodes/") && strings.HasSuffix(r.URL.Path, "/drain") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/v1/nodes/") && strings.HasSuffix(r.URL.Path, "/undrain") {
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

	oldEnv := os.Getenv("PLOY_CONFIG_HOME")
	_ = os.Setenv("PLOY_CONFIG_HOME", tmpDir)
	defer func() { _ = os.Setenv("PLOY_CONFIG_HOME", oldEnv) }()

	stateDir := filepath.Join(tmpDir, "rollout")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("create state dir: %v", err)
	}

	// Create state with node-1 having 2 failed attempts already.
	stateFile := filepath.Join(stateDir, "state.json")
	state := &rolloutState{
		Version: 1,
		RetryPolicy: rolloutRetryPolicy{
			MaxAttempts: 3,
		},
		Nodes: map[string]nodeRolloutStatus{
			"node-1": {
				NodeID:      "node-1",
				NodeName:    "worker-1",
				InProgress:  false,
				Completed:   false,
				Error:       "previous error",
				Attempts:    2,
				LastAttempt: time.Now().UTC().Add(-5 * time.Minute).Format(time.RFC3339),
			},
		},
		CreatedAt:    time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339),
		LastModified: time.Now().UTC().Add(-5 * time.Minute).Format(time.RFC3339),
	}
	data, _ := json.MarshalIndent(state, "", "  ")
	if err := os.WriteFile(stateFile, data, 0o644); err != nil {
		t.Fatalf("write state file: %v", err)
	}

	// Stub rollout to fail on node-1 and succeed on node-2.
	var attemptedNodes []string
	oldHost := rolloutNodesHost
	rolloutNodesHost = func(ctx context.Context, node nodeInfo, opts rolloutNodeOptions) error {
		attemptedNodes = append(attemptedNodes, node.Name)
		if node.Name == "worker-1" {
			return errors.New("rollout failed again")
		}
		return nil
	}
	defer func() { rolloutNodesHost = oldHost }()

	buf := &bytes.Buffer{}
	cfg := rolloutNodesConfig{
		All:          true,
		BinaryPath:   binPath,
		IdentityFile: identityPath,
		MaxAttempts:  3,
	}

	err := runRolloutNodes(cfg, buf)
	if err == nil {
		t.Fatalf("expected error due to failed rollouts")
	}

	output := buf.String()
	t.Logf("Output:\n%s", output)

	// Verify both nodes were attempted.
	if len(attemptedNodes) != 2 {
		t.Fatalf("expected 2 nodes attempted, got %d: %v\nOutput: %s", len(attemptedNodes), attemptedNodes, output)
	}

	// Load final state.
	finalState, err := loadRolloutState(stateFile)
	if err != nil {
		t.Fatalf("load final state: %v", err)
	}

	// Check node-1: should have 3 attempts now and still not completed.
	if finalState.Nodes["node-1"].Attempts != 3 {
		t.Errorf("expected node-1 to have 3 attempts, got %d", finalState.Nodes["node-1"].Attempts)
	}
	if finalState.Nodes["node-1"].Completed {
		t.Errorf("expected node-1 to not be completed")
	}

	// Check node-2: should have 1 attempt and be completed.
	if finalState.Nodes["node-2"].Attempts != 1 {
		t.Errorf("expected node-2 to have 1 attempt, got %d", finalState.Nodes["node-2"].Attempts)
	}
	if !finalState.Nodes["node-2"].Completed {
		t.Errorf("expected node-2 to be completed")
	}

	// Now run again - node-1 should be skipped (max attempts reached).
	attemptedNodes = nil
	buf = &bytes.Buffer{}

	err = runRolloutNodes(cfg, buf)
	if err == nil {
		t.Fatalf("expected error message about max attempts")
	}

	// Verify no nodes were attempted (node-1 skipped, node-2 already complete).
	if len(attemptedNodes) != 0 {
		t.Fatalf("expected 0 nodes attempted on retry, got %d: %v", len(attemptedNodes), attemptedNodes)
	}

	output2 := buf.String()
	if !strings.Contains(output2, "Max attempts") {
		t.Errorf("expected 'Max attempts' message in output, got: %q", output2)
	}
	if !strings.Contains(output2, "Already completed") {
		t.Errorf("expected 'Already completed' message in output, got: %q", output2)
	}
}
