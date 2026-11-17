package nodeagent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRunController(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	newRC := func() *runController {
		r := newTestController(t, newAgentConfig(server.URL))
		t.Cleanup(func() {
			deadline := time.Now().Add(5 * time.Second)
			for len(r.jobSem) != 0 {
				if time.Now().After(deadline) {
					t.Error("execution did not release permit")
					return
				}
				time.Sleep(time.Millisecond)
			}
		})
		return r
	}
	baseReq := newStartRunRequest()

	t.Run("StartRun registers job", func(t *testing.T) {
		rc := newRC()
		if err := rc.StartRun(context.Background(), baseReq, testRunSlot(t, rc)); err != nil {
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
		rc.jobs[baseReq.JobID] = &jobContext{runID: baseReq.RunID}
		slot := testRunSlot(t, rc)
		defer rc.ReleaseSlot(slot)
		if err := rc.StartRun(ctx, baseReq, slot); err == nil {
			t.Error("expected error for duplicate job, got nil")
		}
	})

	t.Run("StopRun removes job", func(t *testing.T) {
		rc := newRC()
		ctx := context.Background()
		if err := rc.StartRun(ctx, baseReq, testRunSlot(t, rc)); err != nil {
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
