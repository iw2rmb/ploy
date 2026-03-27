package nodeagent

import (
	"context"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

// Run controller unit tests: StartRun / StopRun behavior.

func TestRunControllerStartRun(t *testing.T) {
	cfg := newTestConfig("http://127.0.0.1:8080")
	rc := &runController{
		cfg:  cfg,
		jobs: make(map[types.JobID]*jobContext),
	}

	req := StartRunRequest{
		RunID:   types.RunID("run-001"),
		JobID:   types.JobID("job-001"),
		RepoURL: types.RepoURL("https://github.com/example/repo.git"),
		BaseRef: types.GitRef("main"),
	}

	ctx := context.Background()
	if err := rc.StartRun(ctx, req); err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}

	rc.mu.Lock()
	defer rc.mu.Unlock()

	// Use typed JobID key directly — no .String() conversion needed.
	if _, exists := rc.jobs[req.JobID]; !exists {
		t.Errorf("job %s not found in controller", req.JobID)
	}
}

func TestRunControllerStartRunDuplicate(t *testing.T) {
	cfg := newTestConfig("http://127.0.0.1:8080")
	rc := &runController{
		cfg:  cfg,
		jobs: make(map[types.JobID]*jobContext),
	}

	req := StartRunRequest{
		RunID:   types.RunID("run-001"),
		JobID:   types.JobID("job-001"),
		RepoURL: types.RepoURL("https://github.com/example/repo.git"),
		BaseRef: types.GitRef("main"),
	}

	ctx := context.Background()

	// Start the job once.
	if err := rc.StartRun(ctx, req); err != nil {
		t.Fatalf("first StartRun() error = %v", err)
	}

	// Try to start the same job again.
	err := rc.StartRun(ctx, req)
	if err == nil {
		t.Errorf("expected error for duplicate job, got nil")
	}
}

func TestRunControllerStopRun(t *testing.T) {
	cfg := newTestConfig("http://127.0.0.1:8080")
	rc := &runController{
		cfg:  cfg,
		jobs: make(map[types.JobID]*jobContext),
	}

	// Start a job first.
	startReq := StartRunRequest{
		RunID:   types.RunID("run-001"),
		JobID:   types.JobID("job-001"),
		RepoURL: types.RepoURL("https://github.com/example/repo.git"),
		BaseRef: types.GitRef("main"),
	}

	ctx := context.Background()
	if err := rc.StartRun(ctx, startReq); err != nil {
		t.Fatalf("StartRun() error = %v", err)
	}

	// Stop the run (which stops all jobs).
	stopReq := StopRunRequest{
		RunID:  types.RunID("run-001"),
		Reason: "test",
	}

	if err := rc.StopRun(ctx, stopReq); err != nil {
		t.Errorf("StopRun() error = %v", err)
	}

	rc.mu.Lock()
	defer rc.mu.Unlock()

	// Use typed JobID key directly — no .String() conversion needed.
	if _, exists := rc.jobs[startReq.JobID]; exists {
		t.Errorf("job %s still exists after stop", startReq.JobID)
	}
}

func TestRunControllerStopNonExistent(t *testing.T) {
	cfg := newTestConfig("http://127.0.0.1:8080")
	rc := &runController{
		cfg:  cfg,
		jobs: make(map[types.JobID]*jobContext),
	}

	stopReq := StopRunRequest{
		RunID:  types.RunID("nonexistent"),
		Reason: "test",
	}

	ctx := context.Background()
	err := rc.StopRun(ctx, stopReq)
	if err == nil {
		t.Errorf("expected error for stopping nonexistent run, got nil")
	}
}
