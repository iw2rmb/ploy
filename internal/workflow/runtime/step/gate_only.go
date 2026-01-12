package step

import (
	"context"
	"fmt"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

// RunGateOnly executes only the gate validation phase without container execution.
// This helper allows the node agent orchestration layer to reuse gate logic for
// post-mod gates without invoking a mod container.
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
	hydrationStart := time.Now()
	if r.Workspace != nil {
		if err := r.Workspace.Hydrate(ctx, req.Manifest, req.Workspace); err != nil {
			return Result{}, fmt.Errorf("workspace hydration failed: %w", err)
		}
	}
	result.Timings.HydrationDuration = types.Duration(time.Since(hydrationStart))

	// Stage 2: Build Gate validation.
	// Run static validation on the workspace. This is the primary purpose of
	// RunGateOnly — validate code without running a mod container.
	gateStart := time.Now()
	gateSpec := req.Manifest.Gate
	if r.Gate != nil && gateSpec != nil && gateSpec.Enabled {
		gateMetadata, err := r.Gate.Execute(ctx, gateSpec, req.Workspace)
		if err != nil {
			return Result{}, fmt.Errorf("build gate execution failed: %w", err)
		}
		result.BuildGate = gateMetadata

		// Check if gate passed by inspecting StaticChecks.
		// Gate passes only if at least one check exists and the first one passed.
		gatePassed := false
		if len(gateMetadata.StaticChecks) > 0 {
			gatePassed = gateMetadata.StaticChecks[0].Passed
		}
		if !gatePassed {
			// Gate failed. Return error so the orchestration layer can handle
			// healing if configured.
			result.Timings.BuildGateDuration = types.Duration(time.Since(gateStart))
			result.Timings.TotalDuration = types.Duration(time.Since(totalStart))
			return result, fmt.Errorf("%w: %s", ErrBuildGateFailed, "gate validation failed")
		}
	}
	result.Timings.BuildGateDuration = types.Duration(time.Since(gateStart))

	// No container execution or diff generation — exit immediately.
	// ExitCode remains 0 since no mod was executed.
	result.Timings.TotalDuration = types.Duration(time.Since(totalStart))
	return result, nil
}
