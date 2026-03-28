package nodeagent

import (
	"context"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestRunController(t *testing.T) {
	t.Parallel()

	newRC := func() *runController {
		return &runController{
			cfg:  newTestConfig("http://127.0.0.1:8080"),
			jobs: make(map[types.JobID]*jobContext),
		}
	}
	baseReq := newStartRunRequest()

	t.Run("StartRun registers job", func(t *testing.T) {
		rc := newRC()
		if err := rc.StartRun(context.Background(), baseReq); err != nil {
			t.Fatalf("StartRun() error = %v", err)
		}
		rc.mu.Lock()
		defer rc.mu.Unlock()
		if _, exists := rc.jobs[baseReq.JobID]; !exists {
			t.Errorf("job %s not found in controller", baseReq.JobID)
		}
	})

	t.Run("StartRun rejects duplicate", func(t *testing.T) {
		rc := newRC()
		ctx := context.Background()
		if err := rc.StartRun(ctx, baseReq); err != nil {
			t.Fatalf("first StartRun() error = %v", err)
		}
		if err := rc.StartRun(ctx, baseReq); err == nil {
			t.Error("expected error for duplicate job, got nil")
		}
	})

	t.Run("StopRun removes job", func(t *testing.T) {
		rc := newRC()
		ctx := context.Background()
		if err := rc.StartRun(ctx, baseReq); err != nil {
			t.Fatalf("StartRun() error = %v", err)
		}
		if err := rc.StopRun(ctx, StopRunRequest{RunID: baseReq.RunID, Reason: "test"}); err != nil {
			t.Errorf("StopRun() error = %v", err)
		}
		rc.mu.Lock()
		defer rc.mu.Unlock()
		if _, exists := rc.jobs[baseReq.JobID]; exists {
			t.Errorf("job %s still exists after stop", baseReq.JobID)
		}
	})

	t.Run("StopRun errors for nonexistent run", func(t *testing.T) {
		rc := newRC()
		err := rc.StopRun(context.Background(), StopRunRequest{RunID: "nonexistent", Reason: "test"})
		if err == nil {
			t.Error("expected error for stopping nonexistent run, got nil")
		}
	})
}
