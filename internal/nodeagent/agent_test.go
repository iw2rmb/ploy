package nodeagent

import (
	"context"
	"fmt"
	"testing"
)

func TestRunControllerStartRun(t *testing.T) {
	cfg := Config{NodeID: "test-node", ServerURL: "https://server.example.com"}
	rc := &runController{
		cfg:  cfg,
		runs: make(map[string]*runContext),
	}

	req := StartRunRequest{
		RunID:   "run-001",
		RepoURL: "https://github.com/example/repo.git",
		BaseRef: "main",
	}

	ctx := context.Background()
	if err := rc.StartRun(ctx, req); err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}

	rc.mu.Lock()
	defer rc.mu.Unlock()

	if _, exists := rc.runs[req.RunID]; !exists {
		t.Errorf("run %s not found in controller", req.RunID)
	}
}

func TestRunControllerStartRunDuplicate(t *testing.T) {
	cfg := Config{NodeID: "test-node", ServerURL: "https://server.example.com"}
	rc := &runController{
		cfg:  cfg,
		runs: make(map[string]*runContext),
	}

	req := StartRunRequest{
		RunID:   "run-001",
		RepoURL: "https://github.com/example/repo.git",
		BaseRef: "main",
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

func TestRunControllerStopRun(t *testing.T) {
	cfg := Config{NodeID: "test-node", ServerURL: "https://server.example.com"}
	rc := &runController{
		cfg:  cfg,
		runs: make(map[string]*runContext),
	}

	// Start a run first.
	startReq := StartRunRequest{
		RunID:   "run-001",
		RepoURL: "https://github.com/example/repo.git",
		BaseRef: "main",
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

func TestRunControllerStopNonExistent(t *testing.T) {
	cfg := Config{NodeID: "test-node", ServerURL: "https://server.example.com"}
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
		t.Errorf("expected error for nonexistent run, got nil")
	}
	if err.Error() != fmt.Sprintf("run %s not found", stopReq.RunID) {
		t.Errorf("error = %v, want 'run %s not found'", err, stopReq.RunID)
	}
}
