package api

import (
	"encoding/json"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// StageStatusFromStore converts store.JobStatus to mods API StageState.
// This provides a type-safe mapping from the database-authoritative status
// to the external API representation.
//
// Mapping:
//   - store.JobStatusCreated -> StageStatePending (created jobs are pending from API perspective)
//   - store.JobStatusPending -> StageStatePending (pending jobs are pending from API perspective)
//   - store.JobStatusRunning -> StageStateRunning
//   - store.JobStatusSucceeded -> StageStateSucceeded
//   - store.JobStatusFailed -> StageStateFailed
//   - store.JobStatusSkipped -> StageStateFailed (skipped jobs are represented as failed in mods API)
//   - store.JobStatusCanceled -> StageStateCancelled (UK spelling for mods API)
func StageStatusFromStore(status store.JobStatus) StageState {
	switch status {
	case store.JobStatusCreated, store.JobStatusPending:
		return StageStatePending
	case store.JobStatusRunning:
		return StageStateRunning
	case store.JobStatusSucceeded:
		return StageStateSucceeded
	case store.JobStatusFailed:
		return StageStateFailed
	case store.JobStatusSkipped:
		// Skipped jobs don't have a direct mods API equivalent;
		// map to failed for API consistency.
		return StageStateFailed
	case store.JobStatusCanceled:
		return StageStateCancelled
	default:
		// Default to pending for unknown states (defensive).
		return StageStatePending
	}
}

// RunStatusFromStore converts store.RunStatus to mods API RunState.
// This provides a type-safe mapping from the database-authoritative status
// to the external API representation.
//
// Mapping:
//   - store.RunStatusQueued -> RunStatePending
//   - store.RunStatusAssigned -> RunStatePending (assigned runs are still pending from API perspective)
//   - store.RunStatusRunning -> RunStateRunning
//   - store.RunStatusSucceeded -> RunStateSucceeded
//   - store.RunStatusFailed -> RunStateFailed
//   - store.RunStatusCanceled -> RunStateCancelled (UK spelling for mods API)
func RunStatusFromStore(status store.RunStatus) RunState {
	switch status {
	case store.RunStatusQueued, store.RunStatusAssigned:
		// Both queued and assigned map to pending in the mods API
		// since assignment is an internal scheduler state.
		return RunStatePending
	case store.RunStatusRunning:
		return RunStateRunning
	case store.RunStatusSucceeded:
		return RunStateSucceeded
	case store.RunStatusFailed:
		return RunStateFailed
	case store.RunStatusCanceled:
		return RunStateCancelled
	default:
		// Default to pending for unknown states (defensive).
		return RunStatePending
	}
}

// StageStatusToStore converts mods API StageState to store.JobStatus.
// This provides a type-safe mapping from the external API representation
// to the database-authoritative status type.
//
// Mapping:
//   - StageStatePending/StageStateQueued -> store.JobStatusCreated
//   - StageStateRunning -> store.JobStatusRunning
//   - StageStateSucceeded -> store.JobStatusSucceeded
//   - StageStateFailed -> store.JobStatusFailed
//   - StageStateCancelling/StageStateCancelled -> store.JobStatusCanceled
func StageStatusToStore(state StageState) store.JobStatus {
	switch state {
	case StageStatePending, StageStateQueued:
		return store.JobStatusCreated
	case StageStateRunning:
		return store.JobStatusRunning
	case StageStateSucceeded:
		return store.JobStatusSucceeded
	case StageStateFailed:
		return store.JobStatusFailed
	case StageStateCancelling, StageStateCancelled:
		return store.JobStatusCanceled
	default:
		// Default to created for unknown states (defensive).
		return store.JobStatusCreated
	}
}

// RunStatusToStore converts mods API RunState to store.RunStatus.
// This provides a type-safe mapping from the external API representation
// to the database-authoritative status type.
//
// Mapping:
//   - RunStatePending -> store.RunStatusQueued
//   - RunStateRunning -> store.RunStatusRunning
//   - RunStateSucceeded -> store.RunStatusSucceeded
//   - RunStateFailed -> store.RunStatusFailed
//   - RunStateCancelling/RunStateCancelled -> store.RunStatusCanceled
func RunStatusToStore(state RunState) store.RunStatus {
	switch state {
	case RunStatePending:
		return store.RunStatusQueued
	case RunStateRunning:
		return store.RunStatusRunning
	case RunStateSucceeded:
		return store.RunStatusSucceeded
	case RunStateFailed:
		return store.RunStatusFailed
	case RunStateCancelling, RunStateCancelled:
		return store.RunStatusCanceled
	default:
		// Default to queued for unknown states (defensive).
		return store.RunStatusQueued
	}
}

// IsGateJob parses job metadata JSON and returns true if the job is a gate job.
// Gate jobs are identified by mod_type being one of: pre_gate, post_gate, re_gate.
// Returns false if metadata is empty, invalid JSON, or mod_type is not a gate type.
//
// This helper enables gate-aware run completion logic to distinguish between
// gate jobs (whose failures may be recovered by healing) and mod/heal jobs
// (whose failures are terminal for the run).
func IsGateJob(meta []byte) bool {
	if len(meta) == 0 {
		return false
	}
	var sm StageMetadata
	if err := json.Unmarshal(meta, &sm); err != nil {
		return false
	}
	modType := domaintypes.ModType(strings.TrimSpace(sm.ModType))
	// Gate job types: pre_gate (initial), post_gate (after mods), re_gate (after healing).
	return modType == domaintypes.ModTypePreGate ||
		modType == domaintypes.ModTypePostGate ||
		modType == domaintypes.ModTypeReGate
}
