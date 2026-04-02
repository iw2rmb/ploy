package nodeagent

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestRunController_watchRemoteCancellation_CancelsContext(t *testing.T) {
	t.Parallel()

	runID := types.NewRunID()
	jobID := types.NewJobID()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != fmt.Sprintf("/v1/jobs/%s/status", jobID) {
			http.NotFound(w, r)
			return
		}
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fmt.Sprintf(`{"job_id":"%s","status":"Cancelled"}`, jobID)))
	}))
	defer server.Close()

	rc := &runController{
		statusUploader: &baseUploader{
			cfg:    Config{ServerURL: server.URL, NodeID: testNodeID},
			client: server.Client(),
		},
	}

	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		rc.watchRemoteCancellation(runCtx, StartRunRequest{
			RunID:   runID,
			JobID:   jobID,
			JobType: types.JobTypeMig,
		}, cancel)
		close(done)
	}()

	select {
	case <-runCtx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for remote cancellation to cancel run context")
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("watchRemoteCancellation did not exit after cancel")
	}

	if got := calls.Load(); got == 0 {
		t.Fatal("expected at least one status poll call")
	}
}
