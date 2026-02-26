package step

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// Runner executes workflow steps.
//
// # Execution Stages (Pre-mig Gate per Call)
//
// Runner.Run processes each step call through the following stages in order:
//
//  1. Hydration — Prepare the workspace by fetching repository sources via
//     WorkspaceHydrator. Errors here abort the run immediately.
//
//  2. Pre-mig Build Gate — When Gate is enabled (Manifest.Gate.Enabled), run static validation on the
//     workspace before executing the mig container. If the gate fails,
//     Runner.Run returns ErrBuildGateFailed without executing container
//     stages. The node agent orchestration layer handles healing when
//     configured; Runner itself does not perform healing.
//
//  3. Container Execution — Create, start, and wait on the container via
//     ContainerRuntime. Logs are forwarded to LogWriter if present.
//     Container cleanup is owned by node-runtime pre-claim disk-pressure flow.
//
//  4. Diff Generation — Generate a unified diff of workspace changes via
//     DiffGenerator. The diff is not published here; the node agent
//     uploads artifacts independently.
//
// # Gate Ownership Contract
//
// Runner supports an optional pre-mig gate when Manifest.Gate.Enabled=true.
// This capability exists for direct invocations (e.g., standalone testing)
// where Runner manages its own gate lifecycle.
//
// However, nodeagent step execution MUST pass manifests with Gate.Enabled=false.
// The nodeagent orchestration layer owns all gate lifecycle management via
// runGateWithHealing, which handles:
//   - A single pre-run gate before the step loop begins.
//   - Per-step post-mig gates after each container execution.
//   - Healing retries when gates fail and healing is configured.
//
// Passing Gate.Enabled=true from nodeagent would cause duplicate pre-mig gates
// (one from runGateWithHealing, one from Runner.Run) and break the single-gate-
// per-run invariant. The nodeagent is the authoritative gate orchestrator.
type Runner struct {
	Workspace  WorkspaceHydrator
	Containers ContainerRuntime
	Diffs      DiffGenerator
	Gate       GateExecutor
	LogWriter  io.Writer // Optional: streams logs to server as gzipped chunks.
}

// Request describes a step execution request.
type Request struct {
	// RunID threads the workflow run identifier for correlation/labels.
	// Container labels and telemetry use this value via LabelRunID.
	RunID     types.RunID
	Manifest  contracts.StepManifest
	Workspace string
	OutDir    string
	// InDir is an optional read-only directory mounted at /in for cross-phase inputs.
	InDir string
}

// Result contains the outcome of a step execution.
type Result struct {
	ExitCode int
	// Per-stage timings captured during execution.
	Timings   StageTiming
	BuildGate *contracts.BuildGateStageMetadata
}

// StageTiming captures duration of each execution stage.
type StageTiming struct {
	HydrationDuration types.Duration
	ExecutionDuration types.Duration
	BuildGateDuration types.Duration
	DiffDuration      types.Duration
	PublishDuration   types.Duration
	TotalDuration     types.Duration
}

// ErrBuildGateFailed is returned when the pre-mig Build Gate fails
// and no healing is configured to continue.
var ErrBuildGateFailed = errors.New("build gate failed")

// Run executes a step and returns the result.
func (r *Runner) Run(ctx context.Context, req Request) (Result, error) {
	totalStart := time.Now()
	var result Result

	// Stage 1: Hydrate workspace.
	hydrationStart := time.Now()
	if r.Workspace != nil {
		if err := r.Workspace.Hydrate(ctx, req.Manifest, req.Workspace); err != nil {
			return Result{}, fmt.Errorf("workspace hydration failed: %w", err)
		}
	}
	result.Timings.HydrationDuration = types.Duration(time.Since(hydrationStart))

	// Stage 2: Pre-mig Build Gate validation.
	gateStart := time.Now()
	gateSpec := req.Manifest.Gate
	if r.Gate != nil && gateSpec != nil && gateSpec.Enabled {
		gateMetadata, err := r.Gate.Execute(ctx, gateSpec, req.Workspace)
		if err != nil {
			return Result{}, fmt.Errorf("build gate execution failed: %w", err)
		}
		result.BuildGate = gateMetadata

		gatePassed := false
		if len(gateMetadata.StaticChecks) > 0 {
			gatePassed = gateMetadata.StaticChecks[0].Passed
		}
		if !gatePassed {
			result.Timings.BuildGateDuration = types.Duration(time.Since(gateStart))
			result.Timings.TotalDuration = types.Duration(time.Since(totalStart))
			return result, fmt.Errorf("%w: %s", ErrBuildGateFailed, "pre-mig validation failed")
		}
	}
	result.Timings.BuildGateDuration = types.Duration(time.Since(gateStart))

	// Stage 3: Execute container via configured runtime.
	executionStart := time.Now()
	if r.Containers == nil {
		if r.LogWriter != nil {
			_, _ = fmt.Fprintf(r.LogWriter, "Starting execution for manifest %s\n", req.Manifest.ID)
		}
		result.ExitCode = 0
		result.Timings.ExecutionDuration = types.Duration(time.Since(executionStart))
	} else {
		spec, err := buildContainerSpec(req.RunID, req.Manifest, req.Workspace, req.OutDir, req.InDir)
		if err != nil {
			return Result{}, fmt.Errorf("build container spec: %w", err)
		}
		handle, err := r.Containers.Create(ctx, spec)
		if err != nil {
			return Result{}, fmt.Errorf("container create failed: %w", err)
		}
		if err := r.Containers.Start(ctx, handle); err != nil {
			return Result{}, fmt.Errorf("container start failed: %w", err)
		}
		cRes, err := r.Containers.Wait(ctx, handle)
		if err != nil {
			return Result{}, fmt.Errorf("container wait failed: %w", err)
		}
		if r.LogWriter != nil {
			if logs, err := r.Containers.Logs(ctx, handle); err == nil && len(logs) > 0 {
				_, _ = r.LogWriter.Write(logs)
			}
		}
		result.ExitCode = cRes.ExitCode
		result.Timings.ExecutionDuration = types.Duration(time.Since(executionStart))

	}

	// Stage 4: Generate diff.
	diffStart := time.Now()
	if r.Diffs != nil {
		diffBytes, err := r.Diffs.Generate(ctx, req.Workspace)
		if err != nil {
			return Result{}, fmt.Errorf("diff generation failed: %w", err)
		}
		_ = diffBytes
	}
	result.Timings.DiffDuration = types.Duration(time.Since(diffStart))

	result.Timings.TotalDuration = types.Duration(time.Since(totalStart))
	return result, nil
}
