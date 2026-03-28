package nodeagent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestHealingWorkspacePolicyUploads(t *testing.T) {
	float64Ptr := func(v float64) *float64 { return &v }

	tests := []struct {
		name         string
		callFn       func(rc *runController, ctx context.Context, req StartRunRequest)
		wantWarning  string
		wantExitCode *float64
	}{
		{
			name: "no_workspace_changes uploads failed status with exit_code=1",
			callFn: func(rc *runController, ctx context.Context, req StartRunRequest) {
				stats := types.NewRunStatsBuilder().
					ExitCode(0).
					DurationMs(123).
					MustBuild()
				rc.uploadHealingNoWorkspaceChangesFailure(ctx, req, stats, 123*time.Millisecond)
			},
			wantWarning:  "no_workspace_changes",
			wantExitCode: float64Ptr(1),
		},
		{
			name: "unexpected_workspace_changes uploads failed status",
			callFn: func(rc *runController, ctx context.Context, req StartRunRequest) {
				rc.uploadHealingWorkspacePolicyFailure(ctx, req, "unexpected_workspace_changes", 123*time.Millisecond)
			},
			wantWarning: "unexpected_workspace_changes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
				RunID: types.RunID("run-heal-test"),
				JobID: types.JobID("job-heal-test"),
			}

			tt.callFn(rc, context.Background(), req)

			mu.Lock()
			defer mu.Unlock()

			wantPath := fmt.Sprintf("/v1/jobs/%s/complete", req.JobID)
			if capturedPath != wantPath {
				t.Fatalf("path = %q, want %q", capturedPath, wantPath)
			}
			if capturedHeaderNodeID != "node123" {
				t.Fatalf("PLOY_NODE_UUID header = %q, want %q", capturedHeaderNodeID, "node123")
			}

			// v1 uses capitalized job status values: Success, Fail, Cancelled.
			if got, _ := capturedPayload["status"].(string); got != types.JobStatusFail.String() {
				t.Fatalf("status = %v, want %q", capturedPayload["status"], types.JobStatusFail.String())
			}

			if tt.wantExitCode != nil {
				if got, ok := capturedPayload["exit_code"].(float64); !ok || got != *tt.wantExitCode {
					t.Fatalf("exit_code = %v, want %v", capturedPayload["exit_code"], *tt.wantExitCode)
				}
			}

			statsObj, ok := capturedPayload["stats"].(map[string]any)
			if !ok {
				t.Fatalf("stats = %T, want object", capturedPayload["stats"])
			}
			if got, _ := statsObj["healing_warning"].(string); got != tt.wantWarning {
				t.Fatalf("stats.healing_warning = %v, want %q", statsObj["healing_warning"], tt.wantWarning)
			}
		})
	}
}
