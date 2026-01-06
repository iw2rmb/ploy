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
// Status mapping:
//   - store.JobStatusCreated -> StageStatePending (created jobs are pending from API perspective)
//   - store.JobStatusQueued -> StageStatePending (queued jobs are pending from API perspective)
//   - store.JobStatusRunning -> StageStateRunning
//   - store.JobStatusSuccess -> StageStateSucceeded
//   - store.JobStatusFail -> StageStateFailed
//   - store.JobStatusCancelled -> StageStateCancelled
//
// Note: "skipped" status is not used; jobs are never skipped, only cancelled or failed.
func StageStatusFromStore(status store.JobStatus) StageState {
	switch status {
	case store.JobStatusCreated, store.JobStatusQueued:
		return StageStatePending
	case store.JobStatusRunning:
		return StageStateRunning
	case store.JobStatusSuccess:
		return StageStateSucceeded
	case store.JobStatusFail:
		return StageStateFailed
	case store.JobStatusCancelled:
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
// Status mapping:
//   - store.RunStatusStarted -> RunStateRunning (started runs are running from API perspective)
//   - store.RunStatusCancelled -> RunStateCancelled
//   - store.RunStatusFinished -> RunStateSucceeded (finished runs are treated as succeeded by default)
//
// Run statuses are simplified to Started, Finished, and Cancelled.
func RunStatusFromStore(status store.RunStatus) RunState {
	switch status {
	case store.RunStatusStarted:
		// v1 Started means the run is active; map to Running for API.
		return RunStateRunning
	case store.RunStatusFinished:
		// v1 Finished is terminal; map to Succeeded for API.
		// NOTE: Future work may inspect repo-level outcomes to distinguish success/failure.
		return RunStateSucceeded
	case store.RunStatusCancelled:
		return RunStateCancelled
	default:
		// Default to running for unknown states (defensive).
		return RunStateRunning
	}
}

// StageStatusToStore converts mods API StageState to store.JobStatus.
// This provides a type-safe mapping from the external API representation
// to the database-authoritative status type.
//
// Status mapping:
//   - StageStatePending/StageStateQueued -> store.JobStatusCreated
//   - StageStateRunning -> store.JobStatusRunning
//   - StageStateSucceeded -> store.JobStatusSuccess
//   - StageStateFailed -> store.JobStatusFail
//   - StageStateCancelling/StageStateCancelled -> store.JobStatusCancelled
func StageStatusToStore(state StageState) store.JobStatus {
	switch state {
	case StageStatePending, StageStateQueued:
		return store.JobStatusCreated
	case StageStateRunning:
		return store.JobStatusRunning
	case StageStateSucceeded:
		return store.JobStatusSuccess
	case StageStateFailed:
		return store.JobStatusFail
	case StageStateCancelling, StageStateCancelled:
		return store.JobStatusCancelled
	default:
		// Default to created for unknown states (defensive).
		return store.JobStatusCreated
	}
}

// RunStatusToStore converts mods API RunState to store.RunStatus.
// This provides a type-safe mapping from the external API representation
// to the database-authoritative status type.
//
// Status mapping:
//   - RunStatePending -> store.RunStatusStarted (no separate pending state in store)
//   - RunStateRunning -> store.RunStatusStarted
//   - RunStateSucceeded -> store.RunStatusFinished
//   - RunStateFailed -> store.RunStatusFinished
//   - RunStateCancelling/RunStateCancelled -> store.RunStatusCancelled
func RunStatusToStore(state RunState) store.RunStatus {
	switch state {
	case RunStatePending, RunStateRunning:
		// v1 has no pending/running distinction; both map to Started.
		return store.RunStatusStarted
	case RunStateSucceeded, RunStateFailed:
		// v1 Finished is the terminal state for both success and failure.
		return store.RunStatusFinished
	case RunStateCancelling, RunStateCancelled:
		return store.RunStatusCancelled
	default:
		// Default to started for unknown states (defensive).
		return store.RunStatusStarted
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
