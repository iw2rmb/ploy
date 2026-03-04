package api

import (
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestStageStatusFromDomain(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   domaintypes.JobStatus
		want    StageState
		wantErr bool
	}{
		{name: "created", input: domaintypes.JobStatusCreated, want: StageStatePending},
		{name: "queued", input: domaintypes.JobStatusQueued, want: StageStatePending},
		{name: "running", input: domaintypes.JobStatusRunning, want: StageStateRunning},
		{name: "success", input: domaintypes.JobStatusSuccess, want: StageStateSucceeded},
		{name: "fail", input: domaintypes.JobStatusFail, want: StageStateFailed},
		{name: "cancelled", input: domaintypes.JobStatusCancelled, want: StageStateCancelled},
		{name: "unknown", input: domaintypes.JobStatus("unknown"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := StageStatusFromDomain(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("StageStatusFromDomain(%q) error=%v wantErr=%v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("StageStatusFromDomain(%q)=%q want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestRunStatusFromDomain(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   domaintypes.RunStatus
		want    RunState
		wantErr bool
	}{
		{name: "started", input: domaintypes.RunStatusStarted, want: RunStateRunning},
		{name: "finished", input: domaintypes.RunStatusFinished, want: RunStateSucceeded},
		{name: "cancelled", input: domaintypes.RunStatusCancelled, want: RunStateCancelled},
		{name: "unknown", input: domaintypes.RunStatus("unknown"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := RunStatusFromDomain(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("RunStatusFromDomain(%q) error=%v wantErr=%v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("RunStatusFromDomain(%q)=%q want %q", tt.input, got, tt.want)
			}
		})
	}
}
