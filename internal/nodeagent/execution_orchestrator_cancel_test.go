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

	var gotPath string
	var gotPayload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotPayload)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	// Initialize test infrastructure.
	// Uploaders are lazily initialized by ensureUploaders() when needed.
	rc := &runController{
		cfg: Config{
			ServerURL: server.URL,
			NodeID:    testNodeID,
			HTTP:      HTTPConfig{TLS: TLSConfig{Enabled: false}},
		},
	}

	req := StartRunRequest{
		RunID: types.RunID("test-run"),
		JobID: types.JobID("test-job-id"),
	}

	rc.uploadFailureStatus(context.Background(), req, context.Canceled, 250*time.Millisecond)

	wantPath := "/v1/jobs/" + req.JobID.String() + "/complete"
	if gotPath != wantPath {
		t.Fatalf("path = %q, want %q", gotPath, wantPath)
	}

	if gotPayload["status"] != JobStatusCancelled.String() {
		t.Fatalf("status = %v, want %q", gotPayload["status"], JobStatusCancelled.String())
	}

	if _, ok := gotPayload["exit_code"]; ok {
		t.Fatalf("did not expect exit_code in cancelled payload, got %v", gotPayload["exit_code"])
	}
}
