package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestLoadAndSaveRolloutState(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, "state.json")

	// Test saving state.
	now := time.Now().UTC().Format(time.RFC3339)
	nodeID := domaintypes.NodeID("a1b2c3")
	state := &rolloutState{
		Version:     1,
		RetryPolicy: rolloutRetryPolicy{MaxAttempts: 3},
		Nodes: map[string]nodeRolloutStatus{
			nodeID.String(): {
				NodeID:      nodeID,
				NodeName:    "worker-1",
				InProgress:  true,
				Completed:   false,
				Attempts:    1,
				LastAttempt: now,
			},
		},
		CreatedAt:    now,
		LastModified: now,
	}

	if err := saveRolloutState(stateFile, state); err != nil {
		t.Fatalf("save state: %v", err)
	}

	// Test loading state.
	loaded, err := loadRolloutState(stateFile)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}

	if loaded.Version != 1 {
		t.Errorf("expected version 1, got %d", loaded.Version)
	}

	if loaded.RetryPolicy.MaxAttempts != 3 {
		t.Errorf("expected max attempts 3, got %d", loaded.RetryPolicy.MaxAttempts)
	}

	if len(loaded.Nodes) != 1 {
		t.Fatalf("expected 1 node in state, got %d", len(loaded.Nodes))
	}

	status, ok := loaded.Nodes[nodeID.String()]
	if !ok {
		t.Fatalf("expected %s in state", nodeID.String())
	}

	if status.NodeName != "worker-1" {
		t.Errorf("expected node name worker-1, got %s", status.NodeName)
	}

	if !status.InProgress {
		t.Errorf("expected in_progress to be true")
	}

	if status.Completed {
		t.Errorf("expected completed to be false")
	}

	if status.Attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", status.Attempts)
	}

	if status.LastAttempt == "" {
		t.Errorf("expected LastAttempt to be set")
	}
}

func TestLoadRolloutStateNonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, "non-existent.json")

	_, err := loadRolloutState(stateFile)
	if err == nil {
		t.Fatalf("expected error for non-existent state file")
	}
	if !os.IsNotExist(err) {
		t.Fatalf("expected os.IsNotExist error, got: %v", err)
	}
}

// TestRolloutNodesVersionValidation verifies that state files with unsupported versions are rejected.
func TestRolloutStateVersionValidation(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, "state.json")

	// Write state with unsupported version.
	state := map[string]interface{}{
		"version": 99,
		"nodes":   map[string]interface{}{},
	}
	data, _ := json.Marshal(state)
	if err := os.WriteFile(stateFile, data, 0o644); err != nil {
		t.Fatalf("write state file: %v", err)
	}

	_, err := loadRolloutState(stateFile)
	if err == nil {
		t.Fatalf("expected error for unsupported version")
	}
	if !strings.Contains(err.Error(), "unsupported state version") {
		t.Errorf("expected 'unsupported state version' error, got: %v", err)
	}
}

// TestRolloutStateSavesTimestamps verifies that state saves update LastModified.
func TestRolloutStateSavesTimestamps(t *testing.T) {
	tmpDir := t.TempDir()
	stateFile := filepath.Join(tmpDir, "state.json")

	now := time.Now().UTC()
	state := &rolloutState{
		Version:      1,
		RetryPolicy:  rolloutRetryPolicy{MaxAttempts: 3},
		Nodes:        make(map[string]nodeRolloutStatus),
		CreatedAt:    now.Format(time.RFC3339),
		LastModified: now.Format(time.RFC3339),
	}

	// Wait to ensure timestamp difference (RFC3339 has second precision).
	time.Sleep(1100 * time.Millisecond)

	if err := saveRolloutState(stateFile, state); err != nil {
		t.Fatalf("save state: %v", err)
	}

	loaded, err := loadRolloutState(stateFile)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}

	// LastModified should be updated.
	loadedTime, _ := time.Parse(time.RFC3339, loaded.LastModified)
	originalTime, _ := time.Parse(time.RFC3339, now.Format(time.RFC3339))

	if !loadedTime.After(originalTime) {
		t.Errorf("expected LastModified to be updated, original=%v, loaded=%v", originalTime, loadedTime)
	}

	// CreatedAt should remain unchanged.
	if loaded.CreatedAt != now.Format(time.RFC3339) {
		t.Errorf("expected CreatedAt to remain unchanged")
	}
}
