package handlers

import (
	"testing"

	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
	"github.com/iw2rmb/ploy/internal/store"
)

// TestStageStatusFromStore verifies that store.JobStatus values are correctly
// converted to mods API StageState values.
func TestStageStatusFromStore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input store.JobStatus
		want  modsapi.StageState
	}{
		{name: "created->pending", input: store.JobStatusCreated, want: modsapi.StageStatePending},
		{name: "queued->pending", input: store.JobStatusQueued, want: modsapi.StageStatePending},
		{name: "running", input: store.JobStatusRunning, want: modsapi.StageStateRunning},
		{name: "success", input: store.JobStatusSuccess, want: modsapi.StageStateSucceeded},
		{name: "fail", input: store.JobStatusFail, want: modsapi.StageStateFailed},
		{name: "cancelled", input: store.JobStatusCancelled, want: modsapi.StageStateCancelled},
		{name: "unknown->pending", input: store.JobStatus("unknown"), want: modsapi.StageStatePending},
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
// converted to mods API RunState values.
func TestRunStatusFromStore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input store.RunStatus
		want  modsapi.RunState
	}{
		{name: "started->running", input: store.RunStatusStarted, want: modsapi.RunStateRunning},
		{name: "finished->succeeded", input: store.RunStatusFinished, want: modsapi.RunStateSucceeded},
		{name: "cancelled", input: store.RunStatusCancelled, want: modsapi.RunStateCancelled},
		{name: "unknown->running", input: store.RunStatus("unknown"), want: modsapi.RunStateRunning},
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

// TestStageStatusToStore verifies that mods API StageState values are correctly
// converted to store.JobStatus values.
func TestStageStatusToStore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input modsapi.StageState
		want  store.JobStatus
	}{
		{name: "pending->created", input: modsapi.StageStatePending, want: store.JobStatusCreated},
		{name: "queued->created", input: modsapi.StageStateQueued, want: store.JobStatusCreated},
		{name: "running", input: modsapi.StageStateRunning, want: store.JobStatusRunning},
		{name: "succeeded", input: modsapi.StageStateSucceeded, want: store.JobStatusSuccess},
		{name: "failed", input: modsapi.StageStateFailed, want: store.JobStatusFail},
		{name: "cancelling->cancelled", input: modsapi.StageStateCancelling, want: store.JobStatusCancelled},
		{name: "cancelled", input: modsapi.StageStateCancelled, want: store.JobStatusCancelled},
		{name: "unknown->created", input: modsapi.StageState("unknown"), want: store.JobStatusCreated},
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

// TestRunStatusToStore verifies that mods API RunState values are correctly
// converted to store.RunStatus values.
func TestRunStatusToStore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input modsapi.RunState
		want  store.RunStatus
	}{
		{name: "pending->started", input: modsapi.RunStatePending, want: store.RunStatusStarted},
		{name: "running->started", input: modsapi.RunStateRunning, want: store.RunStatusStarted},
		{name: "succeeded->finished", input: modsapi.RunStateSucceeded, want: store.RunStatusFinished},
		{name: "failed->finished", input: modsapi.RunStateFailed, want: store.RunStatusFinished},
		{name: "cancelling->cancelled", input: modsapi.RunStateCancelling, want: store.RunStatusCancelled},
		{name: "cancelled", input: modsapi.RunStateCancelled, want: store.RunStatusCancelled},
		{name: "unknown->started", input: modsapi.RunState("unknown"), want: store.RunStatusStarted},
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

// TestIsGateJob verifies that gate jobs are correctly identified by mod_type.
func TestIsGateJob(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		meta []byte
		want bool
	}{
		{name: "pre_gate is gate", meta: []byte(`{"mod_type":"pre_gate"}`), want: true},
		{name: "post_gate is gate", meta: []byte(`{"mod_type":"post_gate"}`), want: true},
		{name: "re_gate is gate", meta: []byte(`{"mod_type":"re_gate"}`), want: true},
		{name: "mod is not gate", meta: []byte(`{"mod_type":"mod"}`), want: false},
		{name: "heal is not gate", meta: []byte(`{"mod_type":"heal"}`), want: false},
		{name: "empty mod_type is not gate", meta: []byte(`{"mod_type":""}`), want: false},
		{name: "missing mod_type is not gate", meta: []byte(`{}`), want: false},
		{name: "empty meta is not gate", meta: []byte{}, want: false},
		{name: "nil meta is not gate", meta: nil, want: false},
		{name: "invalid json is not gate", meta: []byte(`{invalid`), want: false},
		{name: "extra fields ignored", meta: []byte(`{"mod_type":"pre_gate","other":"val"}`), want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := IsGateJob(tt.meta)
			if got != tt.want {
				t.Errorf("IsGateJob(%q) = %v, want %v", string(tt.meta), got, tt.want)
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
			if apiState == modsapi.StageStatePending {
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
