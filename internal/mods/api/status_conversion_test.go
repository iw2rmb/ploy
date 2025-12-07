package api

import (
	"testing"

	"github.com/iw2rmb/ploy/internal/store"
)

// TestStageStatusFromStore verifies that store.JobStatus values are correctly
// converted to mods API StageState values.
func TestStageStatusFromStore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input store.JobStatus
		want  StageState
	}{
		{name: "created->pending", input: store.JobStatusCreated, want: StageStatePending},
		{name: "pending->pending", input: store.JobStatusPending, want: StageStatePending},
		{name: "running", input: store.JobStatusRunning, want: StageStateRunning},
		{name: "succeeded", input: store.JobStatusSucceeded, want: StageStateSucceeded},
		{name: "failed", input: store.JobStatusFailed, want: StageStateFailed},
		{name: "skipped->failed", input: store.JobStatusSkipped, want: StageStateFailed},
		{name: "canceled->cancelled", input: store.JobStatusCanceled, want: StageStateCancelled},
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
// converted to mods API RunState values.
func TestRunStatusFromStore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input store.RunStatus
		want  RunState
	}{
		{name: "queued->pending", input: store.RunStatusQueued, want: RunStatePending},
		{name: "assigned->pending", input: store.RunStatusAssigned, want: RunStatePending},
		{name: "running", input: store.RunStatusRunning, want: RunStateRunning},
		{name: "succeeded", input: store.RunStatusSucceeded, want: RunStateSucceeded},
		{name: "failed", input: store.RunStatusFailed, want: RunStateFailed},
		{name: "canceled->cancelled", input: store.RunStatusCanceled, want: RunStateCancelled},
		{name: "unknown->pending", input: store.RunStatus("unknown"), want: RunStatePending},
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
		input StageState
		want  store.JobStatus
	}{
		{name: "pending->created", input: StageStatePending, want: store.JobStatusCreated},
		{name: "queued->created", input: StageStateQueued, want: store.JobStatusCreated},
		{name: "running", input: StageStateRunning, want: store.JobStatusRunning},
		{name: "succeeded", input: StageStateSucceeded, want: store.JobStatusSucceeded},
		{name: "failed", input: StageStateFailed, want: store.JobStatusFailed},
		{name: "cancelling->canceled", input: StageStateCancelling, want: store.JobStatusCanceled},
		{name: "cancelled->canceled", input: StageStateCancelled, want: store.JobStatusCanceled},
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

// TestRunStatusToStore verifies that mods API RunState values are correctly
// converted to store.RunStatus values.
func TestRunStatusToStore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input RunState
		want  store.RunStatus
	}{
		{name: "pending->queued", input: RunStatePending, want: store.RunStatusQueued},
		{name: "running", input: RunStateRunning, want: store.RunStatusRunning},
		{name: "succeeded", input: RunStateSucceeded, want: store.RunStatusSucceeded},
		{name: "failed", input: RunStateFailed, want: store.RunStatusFailed},
		{name: "cancelling->canceled", input: RunStateCancelling, want: store.RunStatusCanceled},
		{name: "cancelled->canceled", input: RunStateCancelled, want: store.RunStatusCanceled},
		{name: "unknown->queued", input: RunState("unknown"), want: store.RunStatusQueued},
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
		// Most statuses should round-trip cleanly
		storeStatuses := []store.JobStatus{
			store.JobStatusCreated,
			store.JobStatusRunning,
			store.JobStatusSucceeded,
			store.JobStatusFailed,
			store.JobStatusCanceled,
		}

		for _, orig := range storeStatuses {
			apiState := StageStatusFromStore(orig)
			backToStore := StageStatusToStore(apiState)
			if backToStore != orig {
				t.Errorf("Stage status round trip failed: %v -> %v -> %v", orig, apiState, backToStore)
			}
		}

		// Skipped doesn't round-trip (maps to failed in API, which maps back to failed in store)
		skipped := store.JobStatusSkipped
		apiState := StageStatusFromStore(skipped)
		backToStore := StageStatusToStore(apiState)
		if apiState != StageStateFailed {
			t.Errorf("Skipped should map to failed in API, got %v", apiState)
		}
		if backToStore != store.JobStatusFailed {
			t.Errorf("Failed API state should map back to failed in store, got %v", backToStore)
		}
	})

	t.Run("ticket status round trip", func(t *testing.T) {
		t.Parallel()
		// Running, succeeded, failed, and canceled should round-trip cleanly
		roundTripStatuses := []store.RunStatus{
			store.RunStatusRunning,
			store.RunStatusSucceeded,
			store.RunStatusFailed,
			store.RunStatusCanceled,
		}

		for _, orig := range roundTripStatuses {
			apiState := RunStatusFromStore(orig)
			backToStore := RunStatusToStore(apiState)
			if backToStore != orig {
				t.Errorf("Ticket status round trip failed: %v -> %v -> %v", orig, apiState, backToStore)
			}
		}

		// Queued and assigned both map to pending in API, which maps back to queued in store
		for _, orig := range []store.RunStatus{store.RunStatusQueued, store.RunStatusAssigned} {
			apiState := RunStatusFromStore(orig)
			if apiState != RunStatePending {
				t.Errorf("%v should map to pending in API, got %v", orig, apiState)
			}
			backToStore := RunStatusToStore(apiState)
			if backToStore != store.RunStatusQueued {
				t.Errorf("Pending API state should map back to queued in store, got %v", backToStore)
			}
		}
	})
}
