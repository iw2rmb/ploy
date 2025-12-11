package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// maybeScheduleMRJobForRun inspects the run spec and terminal status to decide
// whether an MR job should be created. MR jobs are auxiliary and run after the
// run reaches a terminal state; their failures must not alter the run status.
//
// MR creation rules:
//   - Inspect spec for mr_on_success / mr_on_fail booleans.
//   - When runStatus == succeeded and mr_on_success is true → schedule MR job.
//   - When runStatus == failed and mr_on_fail is true → schedule MR job.
//   - RunStatusCanceled never triggers MR creation.
func maybeScheduleMRJobForRun(
	ctx context.Context,
	st store.Store,
	run store.Run,
	runID domaintypes.RunID,
	runStatus store.RunStatus,
) error {
	// Only consider succeeded/failed runs.
	if runStatus != store.RunStatusSucceeded && runStatus != store.RunStatusFailed {
		return nil
	}

	// Parse run spec to extract MR wiring.
	var specMap map[string]any
	if len(run.Spec) > 0 && json.Valid(run.Spec) {
		if err := json.Unmarshal(run.Spec, &specMap); err != nil {
			return fmt.Errorf("parse run spec for MR scheduling: %w", err)
		}
	}

	// Extract mr_on_success / mr_on_fail flags when they are booleans.
	mrOnSuccess := false
	mrOnFail := false

	if raw, ok := specMap["mr_on_success"]; ok {
		if b, ok := raw.(bool); ok {
			mrOnSuccess = b
		}
	}
	if raw, ok := specMap["mr_on_fail"]; ok {
		if b, ok := raw.(bool); ok {
			mrOnFail = b
		}
	}

	// Determine if MR should be created for this run.
	shouldCreate := (runStatus == store.RunStatusSucceeded && mrOnSuccess) ||
		(runStatus == store.RunStatusFailed && mrOnFail)
	if !shouldCreate {
		return nil
	}

	// Check if an MR job already exists to avoid duplicates.
	jobs, err := st.ListJobsByRun(ctx, runID.String())
	if err != nil {
		return fmt.Errorf("list jobs for MR scheduling: %w", err)
	}
	for _, job := range jobs {
		if strings.TrimSpace(job.ModType) == "mr" {
			// MR job already scheduled or completed.
			return nil
		}
	}

	// Create a single MR job as a best-effort post-run task.
	const mrStepIndex = 9000
	if err := createJobWithIndex(ctx, st, runID, "mr", "mr", domaintypes.StepIndex(mrStepIndex), "", store.JobStatusCreated); err != nil {
		return fmt.Errorf("create MR job: %w", err)
	}

	slog.Info("scheduled MR job for run",
		"run_id", runID,
		"status", runStatus,
		"step_index", mrStepIndex,
	)

	return nil
}
