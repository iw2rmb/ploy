package nodeagent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestUploadHealingNoWorkspaceChangesFailure_UploadsFailedStatus(t *testing.T) {
	var mu sync.Mutex
	var capturedPath string
	var capturedHeaderNodeID string
	var capturedPayload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		capturedPath = r.URL.Path
		capturedHeaderNodeID = r.Header.Get("PLOY_NODE_UUID")
		_ = json.NewDecoder(r.Body).Decode(&capturedPayload)

		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	cfg := Config{
		ServerURL: server.URL,
		NodeID:    types.NodeID("node123"),
		HTTP: HTTPConfig{
			TLS: TLSConfig{Enabled: false},
		},
	}

	rc := newTestController(t, cfg)

	req := StartRunRequest{
		RunID: types.RunID("run-heal-nochange"),
		JobID: types.JobID("job-heal-nochange"),
	}

	stats := types.NewRunStatsBuilder().
		ExitCode(0).
		DurationMs(123).
		MustBuild()

	rc.uploadHealingNoWorkspaceChangesFailure(context.Background(), req, stats, 123*time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if capturedPath != "/v1/jobs/job-heal-nochange/complete" {
		t.Fatalf("path = %q, want %q", capturedPath, "/v1/jobs/job-heal-nochange/complete")
	}
	if capturedHeaderNodeID != "node123" {
		t.Fatalf("PLOY_NODE_UUID header = %q, want %q", capturedHeaderNodeID, "node123")
	}

	// v1 uses capitalized job status values: Success, Fail, Cancelled.
	if got, _ := capturedPayload["status"].(string); got != JobStatusFail.String() {
		t.Fatalf("status = %v, want %q", capturedPayload["status"], JobStatusFail.String())
	}

	if got, ok := capturedPayload["exit_code"].(float64); !ok || got != 1 {
		t.Fatalf("exit_code = %v, want 1", capturedPayload["exit_code"])
	}

	statsObj, ok := capturedPayload["stats"].(map[string]any)
	if !ok {
		t.Fatalf("stats = %T, want object", capturedPayload["stats"])
	}
	if got, _ := statsObj["healing_warning"].(string); got != "no_workspace_changes" {
		t.Fatalf("stats.healing_warning = %v, want %q", statsObj["healing_warning"], "no_workspace_changes")
	}
}

func TestUploadHealingWorkspacePolicyFailure_UnexpectedChanges_UploadsFailedStatus(t *testing.T) {
	var mu sync.Mutex
	var capturedPayload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		_ = json.NewDecoder(r.Body).Decode(&capturedPayload)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	cfg := Config{
		ServerURL: server.URL,
		NodeID:    types.NodeID("node123"),
		HTTP: HTTPConfig{
			TLS: TLSConfig{Enabled: false},
		},
	}

	rc := newTestController(t, cfg)

	req := StartRunRequest{
		RunID: types.RunID("run-heal-unexpected-change"),
		JobID: types.JobID("job-heal-unexpected-change"),
	}

	rc.uploadHealingWorkspacePolicyFailure(context.Background(), req, "unexpected_workspace_changes", 123*time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	statsObj, ok := capturedPayload["stats"].(map[string]any)
	if !ok {
		t.Fatalf("stats = %T, want object", capturedPayload["stats"])
	}
	if got, _ := statsObj["healing_warning"].(string); got != "unexpected_workspace_changes" {
		t.Fatalf("stats.healing_warning = %v, want %q", statsObj["healing_warning"], "unexpected_workspace_changes")
	}
}
