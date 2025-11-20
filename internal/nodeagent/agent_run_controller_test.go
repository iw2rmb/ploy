package nodeagent

import (
	"context"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

// Run controller unit tests: StartRun / StopRun behavior.

// TestRunControllerStartRun verifies that starting a run registers it in the controller.
func TestRunControllerStartRun(t *testing.T) {
	cfg := Config{NodeID: "test-node", ServerURL: "http://127.0.0.1:8080"}
	rc := &runController{
		cfg:  cfg,
		runs: make(map[string]*runContext),
	}

	req := StartRunRequest{
		RunID:   types.RunID("run-001"),
		RepoURL: types.RepoURL("https://github.com/example/repo.git"),
		BaseRef: types.GitRef("main"),
	}

	ctx := context.Background()
	if err := rc.StartRun(ctx, req); err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}

	rc.mu.Lock()
	defer rc.mu.Unlock()

	if _, exists := rc.runs[req.RunID.String()]; !exists {
		t.Errorf("run %s not found in controller", req.RunID)
	}
}

// TestRunControllerStartRunDuplicate verifies that starting a duplicate run returns an error.
func TestRunControllerStartRunDuplicate(t *testing.T) {
	cfg := Config{NodeID: "test-node", ServerURL: "http://127.0.0.1:8080"}
	rc := &runController{
		cfg:  cfg,
		runs: make(map[string]*runContext),
	}

	req := StartRunRequest{
		RunID:   types.RunID("run-001"),
		RepoURL: types.RepoURL("https://github.com/example/repo.git"),
		BaseRef: types.GitRef("main"),
	}

	ctx := context.Background()

	// Start the run once.
	if err := rc.StartRun(ctx, req); err != nil {
		t.Fatalf("first StartRun() error = %v", err)
	}

	// Try to start the same run again.
	err := rc.StartRun(ctx, req)
	if err == nil {
		t.Errorf("expected error for duplicate run, got nil")
	}
}

// TestRunControllerStopRun verifies that stopping a run removes it from the controller.
func TestRunControllerStopRun(t *testing.T) {
	cfg := Config{NodeID: "test-node", ServerURL: "http://127.0.0.1:8080"}
	rc := &runController{
		cfg:  cfg,
		runs: make(map[string]*runContext),
	}

	// Start a run first.
	startReq := StartRunRequest{
		RunID:   types.RunID("run-001"),
		RepoURL: types.RepoURL("https://github.com/example/repo.git"),
		BaseRef: types.GitRef("main"),
	}

	ctx := context.Background()
	if err := rc.StartRun(ctx, startReq); err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}

	// Stop the run.
	stopReq := StopRunRequest{
		RunID:  "run-001",
		Reason: "test",
	}

	if err := rc.StopRun(ctx, stopReq); err != nil {
		t.Errorf("StopRun() error = %v", err)
	}

	rc.mu.Lock()
	defer rc.mu.Unlock()

	if _, exists := rc.runs[stopReq.RunID]; exists {
		t.Errorf("run %s still exists after stop", stopReq.RunID)
	}
}

// TestRunControllerStopNonExistent verifies that stopping a nonexistent run returns an error.
func TestRunControllerStopNonExistent(t *testing.T) {
	cfg := Config{NodeID: "test-node", ServerURL: "http://127.0.0.1:8080"}
	rc := &runController{
		cfg:  cfg,
		runs: make(map[string]*runContext),
	}

	stopReq := StopRunRequest{
		RunID:  "nonexistent",
		Reason: "test",
	}

	ctx := context.Background()
	err := rc.StopRun(ctx, stopReq)
	if err == nil {
		t.Errorf("expected error for stopping nonexistent run, got nil")
	}
}
