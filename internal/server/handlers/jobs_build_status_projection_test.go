package handlers

import (
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestProjectJobBuildStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		status       domaintypes.JobStatus
		wantTerminal bool
		wantSuccess  bool
	}{
		{name: "created", status: domaintypes.JobStatusCreated, wantTerminal: false, wantSuccess: false},
		{name: "queued", status: domaintypes.JobStatusQueued, wantTerminal: false, wantSuccess: false},
		{name: "running", status: domaintypes.JobStatusRunning, wantTerminal: false, wantSuccess: false},
		{name: "success", status: domaintypes.JobStatusSuccess, wantTerminal: true, wantSuccess: true},
		{name: "fail", status: domaintypes.JobStatusFail, wantTerminal: true, wantSuccess: false},
		{name: "error", status: domaintypes.JobStatusError, wantTerminal: true, wantSuccess: false},
		{name: "cancelled", status: domaintypes.JobStatusCancelled, wantTerminal: true, wantSuccess: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := projectJobBuildStatus(tt.status)
			if got.Status != string(tt.status) {
				t.Fatalf("status = %q, want %q", got.Status, tt.status)
			}
			if got.Terminal != tt.wantTerminal {
				t.Fatalf("terminal = %v, want %v", got.Terminal, tt.wantTerminal)
			}
			if got.Success != tt.wantSuccess {
				t.Fatalf("success = %v, want %v", got.Success, tt.wantSuccess)
			}
		})
	}
}
