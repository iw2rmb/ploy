package mods

import (
	"context"
	"fmt"
	"time"
)

// runApplyAndBuildWithEvents applies diff and runs build gate with event emission and timeout.
// applyBuild is injected for testability (usually r.ApplyDiffAndBuild).
func runApplyAndBuildWithEvents(
	parent context.Context,
	r *TransflowRunner,
	repoPath, diffPath, stepID string,
	stepStart time.Time,
	applyBuild func(context.Context, string, string) error,
) (StepResult, error) {
	applyTimeout := ResolveDefaultsFromEnv().BuildApplyTimeout
	applyCtx, cancel := context.WithTimeout(parent, applyTimeout)
	defer cancel()

	r.emit(parent, "apply", "diff-apply-started", "info", "Applying diff to repository")
	r.emit(parent, "build", "build-gate-start", "info", "Running build gate")

	if err := applyBuild(applyCtx, repoPath, diffPath); err != nil {
		r.emit(parent, "build", "build-gate-failed", "error", fmt.Sprintf("apply/build failed: %v", err))
		return StepResult{
			StepID:   stepID,
			Success:  false,
			Message:  fmt.Sprintf("Apply/build failed: %v", err),
			Duration: time.Since(stepStart),
		}, err
	}

	r.emit(parent, "apply", "diff-applied", "info", "Diff applied and build gate passed")
	return StepResult{
		StepID:   stepID,
		Success:  true,
		Message:  "Applied ORW diff and passed build gate",
		Duration: time.Since(stepStart),
	}, nil
}
