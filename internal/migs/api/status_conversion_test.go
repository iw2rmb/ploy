package api

import (
	"testing"

	"github.com/iw2rmb/ploy/internal/store"
)

// TestStageStatusFromStore verifies that store.JobStatus values are correctly
// converted to migs API StageState values.
func TestStageStatusFromStore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input store.JobStatus
		want  StageState
	}{
		{name: "created->pending", input: store.JobStatusCreated, want: StageStatePending},
		{name: "queued->pending", input: store.JobStatusQueued, want: StageStatePending},
		{name: "running", input: store.JobStatusRunning, want: StageStateRunning},
		{name: "success", input: store.JobStatusSuccess, want: StageStateSucceeded},
		{name: "fail", input: store.JobStatusFail, want: StageStateFailed},
		{name: "cancelled", input: store.JobStatusCancelled, want: StageStateCancelled},
		{name: "unknown->pending", input: store.JobStatus("unknown"), want: StageStatePending},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := StageStatusFromStore(tt.input)
			if got != tt.want {
				t.Errorf("StageStatusFromStore(%v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestRunStatusFromStore verifies that store.RunStatus values are correctly
// converted to migs API RunState values.
func TestRunStatusFromStore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input store.RunStatus
		want  RunState
	}{
		{name: "started->running", input: store.RunStatusStarted, want: RunStateRunning},
		{name: "finished->succeeded", input: store.RunStatusFinished, want: RunStateSucceeded},
		{name: "cancelled", input: store.RunStatusCancelled, want: RunStateCancelled},
		{name: "unknown->running", input: store.RunStatus("unknown"), want: RunStateRunning},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := RunStatusFromStore(tt.input)
			if got != tt.want {
				t.Errorf("RunStatusFromStore(%v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestStageStatusToStore verifies that migs API StageState values are correctly
// converted to store.JobStatus values.
func TestStageStatusToStore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input StageState
		want  store.JobStatus
	}{
		{name: "pending->created", input: StageStatePending, want: store.JobStatusCreated},
		{name: "queued->created", input: StageStateQueued, want: store.JobStatusCreated},
		{name: "running", input: StageStateRunning, want: store.JobStatusRunning},
		{name: "succeeded", input: StageStateSucceeded, want: store.JobStatusSuccess},
		{name: "failed", input: StageStateFailed, want: store.JobStatusFail},
		{name: "cancelling->cancelled", input: StageStateCancelling, want: store.JobStatusCancelled},
		{name: "cancelled", input: StageStateCancelled, want: store.JobStatusCancelled},
		{name: "unknown->created", input: StageState("unknown"), want: store.JobStatusCreated},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := StageStatusToStore(tt.input)
			if got != tt.want {
				t.Errorf("StageStatusToStore(%v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestRunStatusToStore verifies that migs API RunState values are correctly
// converted to store.RunStatus values.
func TestRunStatusToStore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input RunState
		want  store.RunStatus
	}{
		{name: "pending->started", input: RunStatePending, want: store.RunStatusStarted},
		{name: "running->started", input: RunStateRunning, want: store.RunStatusStarted},
		{name: "succeeded->finished", input: RunStateSucceeded, want: store.RunStatusFinished},
		{name: "failed->finished", input: RunStateFailed, want: store.RunStatusFinished},
		{name: "cancelling->cancelled", input: RunStateCancelling, want: store.RunStatusCancelled},
		{name: "cancelled", input: RunStateCancelled, want: store.RunStatusCancelled},
		{name: "unknown->started", input: RunState("unknown"), want: store.RunStatusStarted},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := RunStatusToStore(tt.input)
			if got != tt.want {
				t.Errorf("RunStatusToStore(%v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestRoundTripConversion verifies that converting from store types to API types
// and back produces expected results.
func TestRoundTripConversion(t *testing.T) {
	t.Parallel()

	t.Run("stage status round trip", func(t *testing.T) {
		t.Parallel()
		// StageStatePending maps back to JobStatusCreated; other statuses round-trip.
		storeStatuses := []store.JobStatus{
			store.JobStatusCreated,
			store.JobStatusQueued,
			store.JobStatusRunning,
			store.JobStatusSuccess,
			store.JobStatusFail,
			store.JobStatusCancelled,
		}

		for _, orig := range storeStatuses {
			apiState := StageStatusFromStore(orig)
			backToStore := StageStatusToStore(apiState)
			if apiState == StageStatePending {
				if backToStore != store.JobStatusCreated {
					t.Errorf("Stage status pending round trip failed: %v -> %v -> %v", orig, apiState, backToStore)
				}
				continue
			}
			if backToStore != orig {
				t.Errorf("Stage status round trip failed: %v -> %v -> %v", orig, apiState, backToStore)
			}
		}
	})

	t.Run("run status round trip", func(t *testing.T) {
		t.Parallel()
		roundTripStatuses := []store.RunStatus{store.RunStatusStarted, store.RunStatusFinished, store.RunStatusCancelled}

		for _, orig := range roundTripStatuses {
			apiState := RunStatusFromStore(orig)
			backToStore := RunStatusToStore(apiState)
			// Finished maps to succeeded/failed API state, which maps back to Finished.
			if backToStore != orig {
				t.Errorf("Run status round trip failed: %v -> %v -> %v", orig, apiState, backToStore)
			}
		}
	})
}
