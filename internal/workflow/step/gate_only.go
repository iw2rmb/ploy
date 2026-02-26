package step

import (
	"context"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

// RunGateOnly executes only the gate validation phase without container execution.
// This helper allows the node agent orchestration layer to reuse gate logic for
// post-mig gates without invoking a mig container.
//
// Execution stages:
//  1. Hydration — Prepare the workspace via WorkspaceHydrator when configured.
//  2. Build Gate — Run static validation using GateExecutor when enabled.
//
// Unlike Runner.Run, this function:
//   - Does NOT create or start any containers.
//   - Does NOT generate diffs.
//   - Returns immediately after gate validation (pass or fail).
//
// The returned Result contains:
//   - BuildGate metadata (if gate was executed).
//   - Timings for hydration and gate phases only.
//   - ExitCode is always 0 (no container was executed).
//
// On gate failure, returns ErrBuildGateFailed so callers can detect failures
// and trigger healing if configured.
func RunGateOnly(ctx context.Context, r *Runner, req Request) (Result, error) {
	totalStart := time.Now()
	var result Result

	// Stage 1: Hydrate workspace (optional).
	// Prepares the workspace by fetching repository sources if hydrator is configured.
	hydrationDuration, err := runHydrationStage(ctx, r, req)
	if err != nil {
		return Result{}, err
	}
	result.Timings.HydrationDuration = hydrationDuration

	// Stage 2: Build Gate validation.
	// Run static validation on the workspace. This is the primary purpose of
	// RunGateOnly — validate code without running a mig container.
	gateMetadata, gateDuration, err := runGateStage(ctx, r, req, "gate validation failed")
	result.BuildGate = gateMetadata
	result.Timings.BuildGateDuration = gateDuration
	if err != nil {
		result.Timings.TotalDuration = types.Duration(time.Since(totalStart))
		return result, err
	}

	// No container execution or diff generation — exit immediately.
	// ExitCode remains 0 since no mig was executed.
	result.Timings.TotalDuration = types.Duration(time.Since(totalStart))
	return result, nil
}
