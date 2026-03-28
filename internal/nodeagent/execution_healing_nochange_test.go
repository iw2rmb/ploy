package nodeagent

import (
	"context"
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
				rc.uploadHealingWorkspacePolicyFailure(ctx, req, "no_workspace_changes", 123*time.Millisecond)
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
			server, cap := newStatusCaptureServer(t, "job-heal-test")
			rc := newTestController(t, newAgentConfig(server.URL, withNodeID("node123")))

			req := StartRunRequest{
				RunID: types.RunID("run-heal-test"),
				JobID: types.JobID("job-heal-test"),
			}

			tt.callFn(rc, context.Background(), req)

			if cap.Status != types.JobStatusFail.String() {
				t.Fatalf("status = %q, want %q", cap.Status, types.JobStatusFail.String())
			}
			if tt.wantExitCode != nil {
				if cap.ExitCode == nil || *cap.ExitCode != *tt.wantExitCode {
					t.Fatalf("exit_code = %v, want %v", cap.ExitCode, *tt.wantExitCode)
				}
			}
			if cap.Stats == nil {
				t.Fatal("stats is nil, want object")
			}
			if got, _ := cap.Stats["healing_warning"].(string); got != tt.wantWarning {
				t.Fatalf("stats.healing_warning = %q, want %q", got, tt.wantWarning)
			}
		})
	}
}
