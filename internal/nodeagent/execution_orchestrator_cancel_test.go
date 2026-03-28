package nodeagent

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestRunController_uploadFailureStatus_UsesCancelledOnContextCanceled(t *testing.T) {
	t.Parallel()

	runID := types.NewRunID()
	jobID := types.NewJobID()

	var eventPayload map[string]any

	eventsHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/nodes/"+testNodeID+"/events" {
			_ = json.NewDecoder(r.Body).Decode(&eventPayload)
			w.WriteHeader(http.StatusCreated)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	server, cap := newStatusCaptureServer(t, jobID.String(), withStatusExtraHandler(eventsHandler))
	rc := newTestController(t, newAgentConfig(server.URL))

	req := StartRunRequest{
		RunID: runID,
		JobID: jobID,
	}

	rc.uploadFailureStatus(context.Background(), req, context.Canceled, 250*time.Millisecond)

	if cap.Status != types.JobStatusCancelled.String() {
		t.Fatalf("status = %q, want %q", cap.Status, types.JobStatusCancelled.String())
	}
	if cap.ExitCode != nil {
		t.Fatalf("did not expect exit_code in cancelled payload, got %v", *cap.ExitCode)
	}
	if eventPayload == nil {
		t.Fatal("expected event payload, got nil")
	}
	if eventPayload["run_id"] != req.RunID.String() {
		t.Fatalf("event run_id = %v, want %q", eventPayload["run_id"], req.RunID.String())
	}
}
