package nodeagent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestRunController_uploadFailureStatus_UsesCancelledOnContextCanceled(t *testing.T) {
	t.Parallel()

	runID := types.NewRunID()
	jobID := types.NewJobID()

	var (
		statusPath    string
		statusPayload map[string]any
		eventPath     string
		eventPayload  map[string]any
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/jobs/" + jobID.String() + "/complete":
			statusPath = r.URL.Path
			_ = json.NewDecoder(r.Body).Decode(&statusPayload)
			w.WriteHeader(http.StatusNoContent)
		case "/v1/nodes/" + testNodeID + "/events":
			eventPath = r.URL.Path
			_ = json.NewDecoder(r.Body).Decode(&eventPayload)
			w.WriteHeader(http.StatusCreated)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg := Config{
		ServerURL: server.URL,
		NodeID:    testNodeID,
		HTTP:      HTTPConfig{TLS: TLSConfig{Enabled: false}},
	}
	rc := newTestController(t, cfg)

	req := StartRunRequest{
		RunID: runID,
		JobID: jobID,
	}

	rc.uploadFailureStatus(context.Background(), req, context.Canceled, 250*time.Millisecond)

	wantPath := "/v1/jobs/" + req.JobID.String() + "/complete"
	if statusPath != wantPath {
		t.Fatalf("status path = %q, want %q", statusPath, wantPath)
	}

	if statusPayload["status"] != types.JobStatusCancelled.String() {
		t.Fatalf("status = %v, want %q", statusPayload["status"], types.JobStatusCancelled.String())
	}

	if _, ok := statusPayload["exit_code"]; ok {
		t.Fatalf("did not expect exit_code in cancelled payload, got %v", statusPayload["exit_code"])
	}

	wantEventPath := "/v1/nodes/" + testNodeID + "/events"
	if eventPath != wantEventPath {
		t.Fatalf("event path = %q, want %q", eventPath, wantEventPath)
	}
	if eventPayload["run_id"] != req.RunID.String() {
		t.Fatalf("event run_id = %v, want %q", eventPayload["run_id"], req.RunID.String())
	}
}
