// execution_orchestrator.go contains the high-level run lifecycle orchestration.
//
// This file owns executeRun, the main entry point for executing a single run.
// It coordinates workspace setup, runtime initialization, healing execution,
// artifact collection, and terminal status reporting. The orchestrator ensures
// cleanup of ephemeral resources and delegates domain-specific concerns to
// specialized execution files (healing, MR creation, uploads).
package nodeagent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/runtime/step"
)

// executeRun orchestrates the high-level lifecycle of a single run execution.
// It coordinates workspace setup, runtime initialization, step execution with healing,
// artifact collection, and terminal status reporting. This function owns the run's
// lifecycle from start to completion and ensures cleanup of ephemeral resources.
//
// Lifecycle phases:
//  1. Manifest construction from run request
//  2. Workspace directory creation and cleanup registration
//  3. Runtime component initialization (git, hydrator, container, diff, gate, logs)
//  4. Step execution with optional healing loop via executeWithHealing
//  5. Diff generation and upload (when available)
//  6. Artifact bundle uploads (configured paths + /out directory)
//  7. Merge request creation (when conditions are met)
//  8. Terminal status emission to control plane
//
// The function uses defer to ensure cleanup occurs even on early returns or panics.
// It removes the run from the controller's tracking map on exit.
func (r *runController) executeRun(ctx context.Context, req StartRunRequest) {
	defer func() {
		r.mu.Lock()
		delete(r.runs, req.RunID.String())
		r.mu.Unlock()
	}()

	slog.Info("starting run execution", "run_id", req.RunID, "repo_url", req.RepoURL)

	// Phase 1: Parse typed options from the raw options map to reduce map[string]any casts.
	// Determine if this is a multi-step run (mods[] array) or single-step run.
	typedOpts := parseRunOptions(req.Options)

	// Detect multi-step vs single-step execution mode.
	// Multi-step runs loop over Steps; single-step runs execute once with stepIndex=0.
	// When req.StepIndex is present, constrain execution to that single step (multi-node execution).
	stepCount := 1
	startStepIndex := 0
	if len(typedOpts.Steps) > 0 {
		stepCount = len(typedOpts.Steps)
		slog.Info("multi-step run detected", "run_id", req.RunID, "step_count", stepCount)
	}

	// If a specific step was claimed (multi-node execution), constrain to that step only.
	if req.StepIndex != nil {
		startStepIndex = int(*req.StepIndex)
		stepCount = startStepIndex + 1 // Execute only this step
		slog.Info("single-step execution (multi-node claim)", "run_id", req.RunID, "step_index", startStepIndex)
	}

	// Phase 2: Prepare base clone cache for rehydration.
	// Instead of a single long-lived workspace, we'll create a fresh workspace per step
	// by copying the base clone and applying ordered diffs. This enables multi-node execution
	// where different steps can run on different nodes.
	//
	// The baseClonePath is created once and cached for all steps in this run.
	// Each step gets its own ephemeral workspace via rehydration.
	baseClonePath := ""
	defer func() {
		if baseClonePath != "" {
			_ = os.RemoveAll(baseClonePath)
		}
	}()

	// Phase 3: Initialize runtime components (git fetcher, workspace hydrator, container runtime, diff generator, gate executor).
	runner, diffGenerator, logStreamer, err := r.initializeRuntime(ctx, req.RunID.String())
	if err != nil {
		slog.Error("failed to initialize runtime", "run_id", req.RunID, "error", err)
		return
	}
	defer func() {
		if closeErr := logStreamer.Close(); closeErr != nil {
			slog.Warn("failed to close log streamer", "run_id", req.RunID, "error", closeErr)
		}
	}()

	// Prepare ephemeral /out directory for the container to write additional artifacts.
	outDir, err := os.MkdirTemp("", "ploy-mod-out-*")
	if err != nil {
		slog.Error("failed to create /out directory", "run_id", req.RunID, "error", err)
		return
	}
	defer func() { _ = os.RemoveAll(outDir) }()

	// Prepare ephemeral /in directory for cross-phase inputs (created on-demand by healing logic).
	var inDir string
	defer func() {
		if inDir != "" {
			_ = os.RemoveAll(inDir)
		}
	}()

	// Phase 4a: Run a single pre-mod gate with healing BEFORE executing any mods.
	// This ensures the baseline compiles before any changes are applied, providing
	// early feedback if the repository's current state is broken.
	//
	// The pre-mod gate uses step 0's manifest and a freshly hydrated workspace.
	// If the gate fails and cannot be healed, the run terminates immediately with
	// ErrBuildGateFailed (reason="build-gate") and no mods are executed.
	//
	// PreGate and ReGates from this phase are accumulated into a shared result
	// that gets merged with per-step execution results for final stats reporting.
	var preModGateResult *gateRunMetadata // Captures initial pre-mod gate outcome.
	var preModReGates []gateRunMetadata   // Captures re-gate attempts after healing.
	var preModGateWorkspace string        // Workspace used for pre-mod gate (cleaned up after).

	// Build manifest for step 0 to determine gate configuration.
	step0Manifest, step0ManifestErr := buildManifestFromRequest(req, typedOpts, 0)
	if step0ManifestErr != nil {
		slog.Error("failed to build manifest for pre-mod gate", "run_id", req.RunID, "error", step0ManifestErr)
		// Report failure and exit early.
		r.reportPreModGateFailure(ctx, req, step0ManifestErr, nil, nil, 0)
		return
	}

	// Check if gate is enabled before hydrating workspace (avoid unnecessary work).
	gateEnabled := step0Manifest.Gate != nil && step0Manifest.Gate.Enabled
	//lint:ignore SA1019 Backward compatibility: support deprecated Shift field.
	if !gateEnabled && step0Manifest.Shift != nil && step0Manifest.Shift.Enabled {
		gateEnabled = true
	}

	if gateEnabled {
		slog.Info("running pre-mod gate with healing", "run_id", req.RunID)

		// Hydrate workspace for step 0 to run the pre-mod gate.
		var err error
		preModGateWorkspace, err = r.rehydrateWorkspaceForStep(ctx, req, step0Manifest, 0, &baseClonePath)
		if err != nil {
			slog.Error("failed to rehydrate workspace for pre-mod gate", "run_id", req.RunID, "error", err)
			r.reportPreModGateFailure(ctx, req, fmt.Errorf("rehydrate workspace for pre-mod gate: %w", err), nil, nil, 0)
			return
		}
		// Cleanup pre-mod gate workspace when done (separate from per-step workspaces).
		defer func() {
			if preModGateWorkspace != "" {
				_ = os.RemoveAll(preModGateWorkspace)
			}
		}()

		// Execute the pre-mod gate with healing via runGateWithHealing.
		// Pass empty inDir pointer; healing will create it if needed.
		preModInDir := "" // Separate /in for pre-mod gate healing.
		defer func() {
			if preModInDir != "" {
				_ = os.RemoveAll(preModInDir)
			}
		}()

		// C2: Pre-mod gate healing uses step_index=0 since it runs before any mods.
		preModGateResult, preModReGates, err = r.runGateWithHealing(
			ctx, runner, req, step0Manifest, preModGateWorkspace, outDir, &preModInDir, "pre", 0,
		)

		if err != nil {
			// Pre-mod gate failed and could not be healed.
			// Terminate the run with ErrBuildGateFailed and reason="build-gate".
			slog.Error("pre-mod gate failed, no mods will be executed",
				"run_id", req.RunID,
				"error", err,
			)
			r.reportPreModGateFailure(ctx, req, err, preModGateResult, preModReGates, 0)
			return
		}

		slog.Info("pre-mod gate passed, proceeding to mod execution", "run_id", req.RunID)

		// C2: Upload pre-mod healing diff so step 0 runs on the healed baseline.
		// After pre-mod healing succeeds, compute the diff between baseClonePath and
		// preModGateWorkspace, then upload it with step_index=-1 and mod_type="pre_gate".
		// Using step_index=-1 ensures it's included when any step fetches "all diffs from previous steps".
		// For step 0: fetch step_index <= -1 → gets pre_gate diff
		// For step k: fetch step_index <= k-1 → gets pre_gate + all prior mod diffs
		if len(preModReGates) > 0 {
			// Healing occurred - upload the accumulated changes as a pre_gate diff.
			// If diff is empty, fail the run (healing claimed success but made no changes).
			stageID, _ := step0Manifest.OptionString("stage_id")
			if err := r.uploadBaselineDiff(ctx, req.RunID.String(), stageID, diffGenerator, baseClonePath, preModGateWorkspace, -1, "pre_gate"); err != nil {
				slog.Error("pre-mod healing produced invalid state",
					"run_id", req.RunID,
					"error", err,
				)
				r.reportPreModGateFailure(ctx, req, err, preModGateResult, preModReGates, 0)
				return
			}
		}
	}

	// Phase 4b: Execute steps sequentially (container execution + post-gates per step).
	// For multi-step runs, loop over Steps; for single-step runs, execute once with stepIndex=0.
	// For multi-node step-level claims, execute only the claimed step (startStepIndex..stepCount).
	// Each step runs in a fresh workspace created via rehydration (base + ordered diffs).
	// If any step fails, halt execution and report terminal status.
	//
	// GATE CONTRACT (ROADMAP Phase G):
	// - The pre-run gate (Phase 4a above) is the ONLY pre-gate for the entire run.
	// - Per-step execution via executeWithHealing does NOT run additional pre-step gates;
	//   it disables Gate.Enabled on the cloned manifest before calling Runner.Run.
	// - Only post-mod gates (gatePhase="post") are executed per step to validate changes.
	// - This ensures exactly one pre-run gate per run, avoiding redundant validation.
	var finalExecResult executionResult
	var finalExecErr error
	var finalManifest contracts.StepManifest
	var finalWorkspace string // Track final workspace for artifacts and MR.
	totalDuration := time.Duration(0)

	// Merge pre-mod gate results into final execution result for stats reporting.
	// These are set once before the loop and preserved through step execution.
	if preModGateResult != nil {
		finalExecResult.PreGate = preModGateResult
	}
	if len(preModReGates) > 0 {
		finalExecResult.ReGates = preModReGates
	}

	for stepIndex := startStepIndex; stepIndex < stepCount; stepIndex++ {
		slog.Info("executing step", "run_id", req.RunID, "step_index", stepIndex, "step_total", stepCount)

		// Build manifest for this step.
		manifest, err := buildManifestFromRequest(req, typedOpts, stepIndex)
		if err != nil {
			slog.Error("failed to build manifest for step", "run_id", req.RunID, "step_index", stepIndex, "error", err)
			finalExecErr = err
			break
		}

		// Rehydrate workspace for this step from base clone + ordered diffs.
		// For step 0: fresh clone (no diffs).
		// For step k>0: base clone + apply diffs from steps 0 through k-1.
		workspaceRoot, rehydrateErr := r.rehydrateWorkspaceForStep(ctx, req, manifest, stepIndex, &baseClonePath)
		if rehydrateErr != nil {
			slog.Error("failed to rehydrate workspace for step", "run_id", req.RunID, "step_index", stepIndex, "error", rehydrateErr)
			finalExecErr = fmt.Errorf("rehydrate workspace: %w", rehydrateErr)
			break
		}
		// Cleanup workspace after this step completes (unless it's the final step).
		// Keep final workspace for artifact collection and MR creation.
		defer func(ws string, isFinal bool) {
			if !isFinal {
				_ = os.RemoveAll(ws)
			}
		}(workspaceRoot, stepIndex == stepCount-1)

		// Execute this step with possible healing loop.
		startTime := time.Now()
		execResult, execErr := r.executeWithHealing(ctx, runner, req, manifest, workspaceRoot, outDir, &inDir, stepIndex)
		duration := time.Since(startTime)
		totalDuration += duration
		result := execResult.Result

		if execErr != nil {
			slog.Error("step execution failed",
				"run_id", req.RunID,
				"step_index", stepIndex,
				"error", execErr,
				"duration", duration,
				"exit_code", result.ExitCode,
			)
			finalExecResult = mergeExecutionResults(finalExecResult, execResult)
			finalExecErr = execErr
			finalManifest = manifest
			finalWorkspace = workspaceRoot

			// Stop execution on step failure. This includes post-mod gate failures:
			// when executeWithHealing returns ErrBuildGateFailed from a post-mod gate
			// that cannot be healed, we halt the multi-step loop to prevent subsequent
			// mods from running on an invalid workspace state.
			break
		}

		slog.Info("step execution succeeded",
			"run_id", req.RunID,
			"step_index", stepIndex,
			"duration", duration,
			"exit_code", result.ExitCode,
		)

		// Upload diff for this step to enable rehydration of workspaces from base + ordered diff chain.
		// Tag diff with step_index for ordering in multi-step/multi-node scenarios.
		stageID, _ := manifest.OptionString("stage_id")
		r.uploadDiffForStep(ctx, req.RunID.String(), stageID, diffGenerator, workspaceRoot, result, stepIndex)

		// Track the last successful execution result (merged with prior gate history)
		// for final status reporting.
		finalExecResult = mergeExecutionResults(finalExecResult, execResult)
		finalExecErr = nil
		finalManifest = manifest
		finalWorkspace = workspaceRoot
	}

	// Cleanup final workspace after artifact collection.
	defer func() {
		if finalWorkspace != "" {
			_ = os.RemoveAll(finalWorkspace)
		}
	}()

	// Phase 6a: Upload configured artifact bundles (artifact_paths option).
	// Use the final manifest for artifact paths (same across steps).
	r.uploadConfiguredArtifacts(ctx, req, finalManifest, finalWorkspace)

	// Phase 6b: Always attempt to bundle and upload /out directory.
	stageID, _ := finalManifest.OptionString("stage_id")
	if err := r.uploadOutDir(ctx, req.RunID.String(), stageID, outDir); err != nil {
		slog.Error("/out artifact upload failed", "run_id", req.RunID, "error", err)
	}

	// Phase 7 & 8: Emit terminal status and conditionally create merge request.
	// Use the final step's result and total duration for status reporting.
	r.finalizeRun(ctx, req, finalManifest, finalExecResult, finalExecErr, finalWorkspace, totalDuration)

	// Log final execution summary with total duration and final exit code.
	finalExitCode := 0
	if finalExecResult.Result.ExitCode != 0 {
		finalExitCode = finalExecResult.Result.ExitCode
	}
	slog.Info("run execution completed",
		"run_id", req.RunID,
		"duration", totalDuration,
		"exit_code", finalExitCode,
		"step_count", stepCount,
	)
}

// initializeRuntime creates and configures all runtime components needed for step execution.
// Returns a configured step.Runner, diff generator, and log streamer.
func (r *runController) initializeRuntime(ctx context.Context, runID string) (step.Runner, step.DiffGenerator, *LogStreamer, error) {
	// Initialize git fetcher without snapshot publishing (node agent operates on ephemeral workspaces).
	gitFetcher, err := r.createGitFetcher()
	if err != nil {
		return step.Runner{}, nil, nil, fmt.Errorf("create git fetcher: %w", err)
	}

	// Initialize workspace hydrator with git fetcher.
	workspaceHydrator, err := r.createWorkspaceHydrator(gitFetcher)
	if err != nil {
		return step.Runner{}, nil, nil, fmt.Errorf("create workspace hydrator: %w", err)
	}

	// Initialize container runtime with image pull enabled.
	// Fallback to nil if Docker is unavailable (simulated execution mode).
	containerRuntime, err := r.createContainerRuntime()
	if err != nil {
		slog.Warn("docker unavailable; falling back to stub runtime", "run_id", runID, "error", err)
		containerRuntime = nil
	}

	// Initialize diff generator for workspace change detection.
	diffGenerator := r.createDiffGenerator()

	// Initialize gate executor with configuration-driven mode selection.
	// PLOY_BUILDGATE_MODE controls whether gates run locally (docker) or remotely (http).
	// Default is "local-docker" for backward compatibility.
	gateExecutor, gateErr := r.createGateExecutor(containerRuntime)
	if gateErr != nil {
		// Log warning but continue; gate executor may be nil if HTTP client creation fails.
		// This allows runs to proceed without gate validation when misconfigured.
		slog.Warn("failed to create gate executor, gate validation will be skipped",
			"run_id", runID,
			"error", gateErr,
		)
	}

	// Initialize log streamer to stream logs as gzipped chunks to the server.
	logStreamer := NewLogStreamer(r.cfg, runID, "")

	// Assemble the step runner with all components.
	runner := step.Runner{
		Workspace:  workspaceHydrator,
		Containers: containerRuntime,
		Diffs:      diffGenerator,
		Gate:       gateExecutor,
		LogWriter:  logStreamer,
	}

	return runner, diffGenerator, logStreamer, nil
}

// finalizeRun handles terminal status determination, merge request creation, and status upload.
func (r *runController) finalizeRun(ctx context.Context, req StartRunRequest, manifest contracts.StepManifest, execResult executionResult, execErr error, workspace string, duration time.Duration) {
	result := execResult.Result

	// Determine terminal status based on execution result.
	terminalStatus := "succeeded"
	var reason *string
	if execErr != nil {
		terminalStatus = "failed"
		errMsg := execErr.Error()
		// Check if this is a build gate failure.
		if errors.Is(execErr, step.ErrBuildGateFailed) {
			// Set reason to "build-gate" for pre-mod gate failures.
			gateReason := "build-gate"
			reason = &gateReason
		} else {
			reason = &errMsg
		}
	} else if result.ExitCode != 0 {
		terminalStatus = "failed"
		failureMsg := fmt.Sprintf("exit code %d", result.ExitCode)
		reason = &failureMsg
	}

	// Phase 7: Create MR via GitLab API when conditions are met.
	// Hook runs after terminal status is determined but before uploading status.
	mrURL := ""
	if shouldCreateMR(terminalStatus, manifest) {
		if url, mrErr := r.createMR(ctx, req, manifest, workspace); mrErr != nil {
			slog.Error("failed to create MR", "run_id", req.RunID, "error", mrErr)
		} else {
			mrURL = url
			slog.Info("MR created successfully", "run_id", req.RunID, "mr_url", mrURL)
		}
	}

	// Build stats with execution metrics and gate history.
	// Stage ID is used to associate gate log artifacts with the current stage.
	stageID, _ := manifest.OptionString("stage_id")
	stats := r.buildExecutionStats(req.RunID.String(), stageID, result, execResult, duration, mrURL)

	// Phase 8: Upload terminal status to server.
	// Pass req.StepIndex to trigger step-level completion for multi-step runs.
	if uploadErr := r.uploadStatus(ctx, req.RunID.String(), terminalStatus, reason, stats, req.StepIndex); uploadErr != nil {
		slog.Error("failed to upload terminal status", "run_id", req.RunID, "error", uploadErr)
	}
}

// reportPreModGateFailure reports a terminal failure when the pre-mod gate fails.
// This is called when the baseline cannot be validated before any mods execute.
// The run terminates with status="failed" and reason="build-gate".
//
// Parameters:
//   - ctx: Context for API calls.
//   - req: StartRunRequest for run metadata.
//   - gateErr: The error that caused the gate failure.
//   - preGate: Pre-mod gate metadata (may be nil if gate never ran).
//   - reGates: Re-gate attempts after healing (may be empty).
//   - duration: Total time spent on pre-mod gate phase.
func (r *runController) reportPreModGateFailure(
	ctx context.Context,
	req StartRunRequest,
	gateErr error,
	preGate *gateRunMetadata,
	reGates []gateRunMetadata,
	duration time.Duration,
) {
	// Build execution result with gate history for stats.
	execResult := executionResult{
		PreGate: preGate,
		ReGates: reGates,
	}

	// Build stats payload with gate history.
	// Use empty step.Result since no mods were executed.
	stats := types.RunStats{
		"exit_code":   -1, // Indicate no mod execution.
		"duration_ms": duration.Milliseconds(),
		"timings": map[string]interface{}{
			"hydration_duration_ms":  0,
			"execution_duration_ms":  0,
			"build_gate_duration_ms": duration.Milliseconds(),
			"diff_duration_ms":       0,
			"total_duration_ms":      duration.Milliseconds(),
		},
	}

	// Include gate stats if gate metadata is available.
	if preGate != nil || len(reGates) > 0 {
		gate := r.buildGateStats(req.RunID.String(), "", step.Result{}, execResult)
		stats["gate"] = gate
	}

	// Set terminal status with reason="build-gate".
	reason := "build-gate"
	if uploadErr := r.uploadStatus(ctx, req.RunID.String(), "failed", &reason, stats, req.StepIndex); uploadErr != nil {
		slog.Error("failed to upload pre-mod gate failure status", "run_id", req.RunID, "error", uploadErr)
	}
}

// buildExecutionStats constructs the stats payload for terminal status upload.
// Includes execution timings, exit code, gate history (pre-gate, re-gates), and MR URL.
func (r *runController) buildExecutionStats(runID, stageID string, result step.Result, execResult executionResult, duration time.Duration, mrURL string) types.RunStats {
	stats := types.RunStats{
		"exit_code":   result.ExitCode,
		"duration_ms": duration.Milliseconds(),
		"timings": map[string]interface{}{
			"hydration_duration_ms":  result.Timings.HydrationDuration.Milliseconds(),
			"execution_duration_ms":  result.Timings.ExecutionDuration.Milliseconds(),
			"build_gate_duration_ms": result.Timings.BuildGateDuration.Milliseconds(),
			"diff_duration_ms":       result.Timings.DiffDuration.Milliseconds(),
			"total_duration_ms":      result.Timings.TotalDuration.Milliseconds(),
		},
	}

	// Attach MR URL to metadata if created.
	if mrURL != "" {
		stats["metadata"] = map[string]interface{}{
			"mr_url": mrURL,
		}
	}

	// Gate stats/logs: collect pass/fail, duration, resources, and upload logs artifact.
	// Include pre-gate and re-gate runs when healing was attempted.
	if execResult.PreGate != nil || len(execResult.ReGates) > 0 || result.BuildGate != nil {
		gate := r.buildGateStats(runID, stageID, result, execResult)
		stats["gate"] = gate
	}

	return stats
}

// mergeExecutionResults aggregates gate history across phases (pre-mod + per-step)
// while keeping the latest step.Result for terminal status reporting.
// - PreGate is preserved from the accumulator when present (pre-mod gate).
// - ReGates are appended in call order to accumulate healing re-gates.
func mergeExecutionResults(acc executionResult, next executionResult) executionResult {
	merged := executionResult{
		Result:  next.Result,
		PreGate: acc.PreGate,
		ReGates: acc.ReGates,
	}

	// If there is no pre-mod gate recorded yet, fall back to the next result's PreGate.
	if merged.PreGate == nil && next.PreGate != nil {
		merged.PreGate = next.PreGate
	}

	// Append any re-gates from the next execution in order.
	if len(next.ReGates) > 0 {
		merged.ReGates = append(merged.ReGates, next.ReGates...)
	}

	return merged
}

// buildGateStats constructs gate statistics including pre-gate, re-gates, and final gate runs.
// Uploads gate logs as artifact bundles for debugging.
//
// final_gate semantics:
//   - When a post-mod gate exists (result.BuildGate != nil), final_gate is the last post-mod gate.
//   - When no mods executed (no BuildGate), final_gate falls back to the pre-mod gate, ensuring
//     CLI/API gate summaries always have a final_gate to report on.
//   - This keeps gate summary behavior consistent: final_gate → last re-gate → pre_gate.
func (r *runController) buildGateStats(runID, stageID string, result step.Result, execResult executionResult) map[string]any {
	gate := map[string]any{}

	// Helper function to build gate metadata and upload logs.
	buildGateMetadata := func(meta *contracts.BuildGateStageMetadata, durationMs int64, artifactNameSuffix string) map[string]any {
		gateStats := map[string]any{
			"duration_ms": durationMs,
		}

		// Determine pass/fail.
		passed := false
		if meta != nil && len(meta.StaticChecks) > 0 {
			passed = meta.StaticChecks[0].Passed
		}
		gateStats["passed"] = passed

		// Attach resource usage metrics when available.
		if meta != nil && meta.Resources != nil {
			ru := meta.Resources
			gateStats["resources"] = map[string]any{
				"limits": map[string]any{"nano_cpus": ru.LimitNanoCPUs, "memory_bytes": ru.LimitMemoryBytes},
				"usage":  map[string]any{"cpu_total_ns": ru.CPUTotalNs, "mem_usage_bytes": ru.MemUsageBytes, "mem_max_bytes": ru.MemMaxBytes, "blkio_read_bytes": ru.BlkioReadBytes, "blkio_write_bytes": ru.BlkioWriteBytes, "size_rw_bytes": ru.SizeRwBytes},
			}
		}

		// Upload build logs as artifact when present.
		if meta != nil && strings.TrimSpace(meta.LogsText) != "" {
			r.uploadGateLogsArtifact(runID, stageID, meta.LogsText, artifactNameSuffix, gateStats)
		}

		return gateStats
	}

	// Include pre-gate stats if present.
	if execResult.PreGate != nil {
		gate["pre_gate"] = buildGateMetadata(execResult.PreGate.Metadata, execResult.PreGate.DurationMs, "pre")
	}

	// Include re-gate stats if present (healing attempts from both pre- and post-mod phases
	// in chronological order).
	if len(execResult.ReGates) > 0 {
		reGatesList := make([]map[string]any, 0, len(execResult.ReGates))
		for i, rg := range execResult.ReGates {
			suffix := fmt.Sprintf("re%d", i+1)
			reGatesList = append(reGatesList, buildGateMetadata(rg.Metadata, rg.DurationMs, suffix))
		}
		gate["re_gates"] = reGatesList
	}

	// Populate final_gate: use the post-mod gate (result.BuildGate) when present,
	// otherwise fall back to the pre-mod gate (for runs where no mods executed).
	// This ensures CLI/API gate summaries always have a final_gate to report on.
	if result.BuildGate != nil {
		gate["final_gate"] = buildGateMetadata(result.BuildGate, result.Timings.BuildGateDuration.Milliseconds(), "")
	} else if execResult.PreGate != nil {
		// No post-mod gate executed (run terminated at pre-mod phase or no mods).
		// Use the pre-mod gate as the final gate for consistent summary output.
		gate["final_gate"] = buildGateMetadata(execResult.PreGate.Metadata, execResult.PreGate.DurationMs, "pre-as-final")
	}

	return gate
}

// rehydrateWorkspaceForStep creates a fresh workspace for the given step by rehydrating
// from the base clone and applying ordered diffs from prior steps.
//
// For step 0: Creates base clone (or reuses cached base if available).
// For step k>0: Copies base clone + applies diffs from steps 0 through k-1.
//
// This function implements the core rehydration strategy that enables multi-node execution:
// each step can run on any node by reconstructing workspace state from base + diff chain.
//
// Parameters:
//   - ctx: Context for cancellation and deadlines.
//   - req: StartRunRequest containing repo URL, base_ref, and commit_sha.
//   - manifest: StepManifest for this step (contains hydration config).
//   - stepIndex: Zero-based index of the step being executed.
//   - baseClonePathPtr: Pointer to cached base clone path (updated on first step).
//
// Returns:
//   - workspacePath: Path to the rehydrated workspace ready for execution.
//   - error: Non-nil if rehydration fails (clone, copy, or patch application error).
func (r *runController) rehydrateWorkspaceForStep(
	ctx context.Context,
	req StartRunRequest,
	manifest contracts.StepManifest,
	stepIndex int,
	baseClonePathPtr *string,
) (string, error) {
	runID := req.RunID.String()

	// Step 1: Ensure base clone exists (create on first step, reuse for subsequent steps).
	if *baseClonePathPtr == "" {
		// Create deterministic base clone path per run. Using a stable path rooted
		// under the node's cache/temp location allows idempotent hydration for
		// a given ticket: if the path already contains a valid clone for this
		// repo, the git fetcher will detect it and skip re-cloning.
		baseRoot := os.Getenv("PLOYD_CACHE_HOME")
		if baseRoot == "" {
			baseRoot = os.TempDir()
		}
		baseClone := filepath.Join(baseRoot, "ploy", "run", runID, "base")
		if err := os.MkdirAll(baseClone, 0o755); err != nil {
			return "", fmt.Errorf("create base clone dir: %w", err)
		}

		slog.Info("creating base clone for run", "run_id", runID, "path", baseClone)

		// Hydrate base clone using the runner's workspace hydrator.
		// This performs a shallow git clone of the base_ref + optional commit_sha.
		gitFetcher, err := r.createGitFetcher()
		if err != nil {
			return "", fmt.Errorf("create git fetcher: %w", err)
		}

		hydrator, err := r.createWorkspaceHydrator(gitFetcher)
		if err != nil {
			return "", fmt.Errorf("create workspace hydrator: %w", err)
		}

		// Hydrate the base clone using the manifest.
		// The hydrator will use the first input from the manifest for repository cloning.
		hydrateErr := hydrator.Hydrate(ctx, manifest, baseClone)
		if hydrateErr != nil {
			_ = os.RemoveAll(baseClone)
			return "", fmt.Errorf("hydrate base clone: %w", hydrateErr)
		}

		*baseClonePathPtr = baseClone
		slog.Info("base clone created", "run_id", runID, "path", baseClone)
	}

	// Step 2: Rehydrate workspace from base clone + ordered diffs.
	// C2: For ALL steps (including step 0), fetch diffs and apply them.
	// This ensures step 0 runs on the healed baseline if pre-mod healing occurred.
	workspacePath, err := createWorkspaceDir()
	if err != nil {
		return "", fmt.Errorf("create workspace dir: %w", err)
	}

	slog.Info("rehydrating workspace from base + diffs", "run_id", runID, "step_index", stepIndex)

	diffFetcher, err := NewDiffFetcher(r.cfg)
	if err != nil {
		_ = os.RemoveAll(workspacePath)
		return "", fmt.Errorf("create diff fetcher: %w", err)
	}

	// C2: Uniform rehydration query for ALL steps.
	// Fetch diffs where step_index <= stepIndex-1 (all diffs from previous steps).
	// - Pre_gate diffs have step_index=-1, so they're included for all steps:
	//   - Step 0: fetch step_index <= -1 → gets pre_gate diff only
	//   - Step 1: fetch step_index <= 0 → gets pre_gate + step 0 mod diff
	//   - Step k: fetch step_index <= k-1 → gets pre_gate + all prior mod diffs
	gzippedDiffs, err := diffFetcher.FetchDiffsForStep(ctx, runID, int32(stepIndex-1))
	if err != nil {
		_ = os.RemoveAll(workspacePath)
		return "", fmt.Errorf("fetch diffs for step: %w", err)
	}

	slog.Info("fetched diffs for rehydration", "run_id", runID, "step_index", stepIndex, "diff_count", len(gzippedDiffs))

	// Rehydrate workspace from base + diffs using the helper from execution.go.
	if err := RehydrateWorkspaceFromBaseAndDiffs(ctx, *baseClonePathPtr, workspacePath, gzippedDiffs); err != nil {
		_ = os.RemoveAll(workspacePath)
		return "", fmt.Errorf("rehydrate from base and diffs: %w", err)
	}

	slog.Info("workspace rehydrated successfully", "run_id", runID, "step_index", stepIndex, "workspace", workspacePath)

	// Create baseline commit after rehydration to enable incremental diffs.
	// C2: Now applies to ALL steps (including step 0) when diffs were applied.
	// This commit establishes a new HEAD so that "git diff HEAD" generates
	// only the changes from this step, not cumulative changes from prior steps.
	if len(gzippedDiffs) > 0 {
		if err := ensureBaselineCommitForRehydration(ctx, workspacePath, stepIndex); err != nil {
			_ = os.RemoveAll(workspacePath)
			return "", fmt.Errorf("create baseline commit for rehydration: %w", err)
		}
		slog.Info("baseline commit created for incremental diff", "run_id", runID, "step_index", stepIndex)
	}

	return workspacePath, nil
}

// uploadDiffForStep generates and uploads a diff for the given step with step_index metadata.
// This replaces the older uploadDiff method and tags each diff with its step index for
// ordered rehydration in multi-step/multi-node scenarios.
func (r *runController) uploadDiffForStep(
	ctx context.Context,
	runID string,
	stageID string,
	diffGenerator step.DiffGenerator,
	workspace string,
	result step.Result,
	stepIndex int,
) {
	if diffGenerator == nil {
		return
	}

	// Generate workspace diff for this step.
	diffBytes, err := diffGenerator.Generate(ctx, workspace)
	if err != nil {
		slog.Error("failed to generate step diff", "run_id", runID, "step_index", stepIndex, "error", err)
		return
	}

	if len(diffBytes) == 0 {
		// No changes from this step; skip upload.
		slog.Info("no diff to upload for step (no changes)", "run_id", runID, "step_index", stepIndex)
		return
	}

	// Build diff summary with step metadata for database storage.
	// C2: Every diff is tagged with step_index + mod_type for unified rehydration.
	// - step_index: 0-based step number for ordering and rehydration queries.
	// - mod_type: "mod" for main mod diffs (healing diffs use "healing" in execution_healing.go).
	summary := types.DiffSummary{
		"step_index": stepIndex,
		"mod_type":   "mod", // Identifies this diff as a main mod step diff.
		"exit_code":  result.ExitCode,
		"timings": map[string]interface{}{
			"hydration_duration_ms":  result.Timings.HydrationDuration.Milliseconds(),
			"execution_duration_ms":  result.Timings.ExecutionDuration.Milliseconds(),
			"build_gate_duration_ms": result.Timings.BuildGateDuration.Milliseconds(),
			"diff_duration_ms":       result.Timings.DiffDuration.Milliseconds(),
			"total_duration_ms":      result.Timings.TotalDuration.Milliseconds(),
		},
	}

	// Upload diff with step metadata to control plane.
	diffUploader, err := NewDiffUploader(r.cfg)
	if err != nil {
		slog.Error("failed to create diff uploader", "run_id", runID, "step_index", stepIndex, "error", err)
		return
	}

	// Convert stepIndex to *int32 for API compatibility.
	stepIdx := int32(stepIndex)
	if err := diffUploader.UploadDiff(ctx, runID, stageID, diffBytes, summary, &stepIdx); err != nil {
		slog.Error("failed to upload step diff", "run_id", runID, "step_index", stepIndex, "error", err)
		return
	}

	slog.Info("step diff uploaded successfully", "run_id", runID, "step_index", stepIndex, "size", len(diffBytes))
}

// uploadBaselineDiff uploads a diff between two directories with the specified mod_type and step_index.
// Used by C2 to persist pre-mod or post-mod healing changes so subsequent steps run on the healed baseline.
//
// Parameters:
//   - baseDir: the reference directory (e.g., base clone before healing)
//   - modifiedDir: the modified directory (e.g., healed workspace)
//   - stepIndex: the step index to tag the diff with (0 for pre-mod healing)
//   - modType: the mod_type to tag the diff with (e.g., "pre_gate", "post_gate")
//
// Returns error if diff generation fails or diff is empty (healing claimed success but made no changes).
func (r *runController) uploadBaselineDiff(
	ctx context.Context,
	runID string,
	stageID string,
	diffGenerator step.DiffGenerator,
	baseDir string,
	modifiedDir string,
	stepIndex int,
	modType string,
) error {
	if diffGenerator == nil {
		return fmt.Errorf("diff generator nil, cannot upload baseline diff")
	}

	diffBytes, err := diffGenerator.GenerateBetween(ctx, baseDir, modifiedDir)
	if err != nil {
		return fmt.Errorf("failed to generate baseline diff: %w", err)
	}

	if len(diffBytes) == 0 {
		// Empty diff after healing passed means something is wrong:
		// either the gate is flaky or healing made no actual changes.
		// Fail the run to surface this inconsistency.
		return fmt.Errorf("healing produced empty diff but gate passed - possible flaky gate or healing made no changes")
	}

	summary := types.DiffSummary{
		"step_index": stepIndex,
		"mod_type":   modType,
	}

	diffUploader, err := NewDiffUploader(r.cfg)
	if err != nil {
		return fmt.Errorf("failed to create diff uploader: %w", err)
	}

	stepIdx := int32(stepIndex)
	if err := diffUploader.UploadDiff(ctx, runID, stageID, diffBytes, summary, &stepIdx); err != nil {
		return fmt.Errorf("failed to upload baseline diff: %w", err)
	}

	slog.Info("baseline diff uploaded successfully", "run_id", runID, "mod_type", modType, "step_index", stepIndex, "size", len(diffBytes))
	return nil
}

// createGateExecutor creates a GateExecutor based on PLOY_BUILDGATE_MODE configuration.
// Supports two modes:
//   - "local-docker" (default): Uses container runtime for local gate execution.
//   - "remote-http": Delegates to Build Gate workers via HTTP API.
//
// Returns the executor and any error from HTTP client creation (for remote-http mode).
func (r *runController) createGateExecutor(containerRuntime step.ContainerRuntime) (step.GateExecutor, error) {
	mode := os.Getenv("PLOY_BUILDGATE_MODE")

	// For local-docker mode (default), use container runtime directly.
	if mode == "" || mode == step.GateExecutorModeLocalDocker {
		return step.NewGateExecutor(mode, containerRuntime, nil), nil
	}

	// For remote-http mode, create HTTP client from environment configuration.
	if mode == step.GateExecutorModeRemoteHTTP {
		cfg, err := step.BuildGateHTTPClientConfigFromEnv()
		if err != nil {
			// Return error but allow caller to decide whether to proceed without gate.
			return nil, fmt.Errorf("load build gate http client config: %w", err)
		}

		httpClient, err := step.NewBuildGateHTTPClient(cfg)
		if err != nil {
			return nil, fmt.Errorf("create build gate http client: %w", err)
		}

		return step.NewGateExecutor(mode, containerRuntime, httpClient), nil
	}

	// Unrecognized mode: factory will log warning and fall back to local-docker.
	return step.NewGateExecutor(mode, containerRuntime, nil), nil
}
