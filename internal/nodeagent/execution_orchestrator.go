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
	stepCount := 1
	if len(typedOpts.Steps) > 0 {
		stepCount = len(typedOpts.Steps)
		slog.Info("multi-step run detected", "run_id", req.RunID, "step_count", stepCount)
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

	// Phase 4: Execute steps sequentially (gate+mod for each step).
	// For multi-step runs, loop over Steps; for single-step runs, execute once with stepIndex=0.
	// Each step runs in a fresh workspace created via rehydration (base + ordered diffs).
	// If any step fails, halt execution and report terminal status.
	var finalExecResult executionResult
	var finalExecErr error
	var finalManifest contracts.StepManifest
	var finalWorkspace string // Track final workspace for artifacts and MR.
	totalDuration := time.Duration(0)

	for stepIndex := 0; stepIndex < stepCount; stepIndex++ {
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
			finalExecResult = execResult
			finalExecErr = execErr
			finalManifest = manifest
			finalWorkspace = workspaceRoot
			// Stop execution on step failure.
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

		// Track the last successful execution result for final status reporting.
		finalExecResult = execResult
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

	// Initialize gate executor with container runtime.
	gateExecutor := step.NewDockerGateExecutor(containerRuntime)

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
	if uploadErr := r.uploadStatus(ctx, req.RunID.String(), terminalStatus, reason, stats); uploadErr != nil {
		slog.Error("failed to upload terminal status", "run_id", req.RunID, "error", uploadErr)
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

// buildGateStats constructs gate statistics including pre-gate, re-gates, and final gate runs.
// Uploads gate logs as artifact bundles for debugging.
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

	// Include re-gate stats if present.
	if len(execResult.ReGates) > 0 {
		reGatesList := make([]map[string]any, 0, len(execResult.ReGates))
		for i, rg := range execResult.ReGates {
			suffix := fmt.Sprintf("re%d", i+1)
			reGatesList = append(reGatesList, buildGateMetadata(rg.Metadata, rg.DurationMs, suffix))
		}
		gate["re_gates"] = reGatesList
	}

	// Always include final/post-mod gate stats under an explicit key when present.
	if result.BuildGate != nil {
		gate["final_gate"] = buildGateMetadata(result.BuildGate, result.Timings.BuildGateDuration.Milliseconds(), "")
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
		slog.Info("creating base clone for run", "run_id", runID)

		// Create temporary directory for base clone.
		baseClone, err := createWorkspaceDir()
		if err != nil {
			return "", fmt.Errorf("create base clone dir: %w", err)
		}

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

	// Step 2: For step 0, return a copy of the base clone (no diffs to apply).
	// For step k>0, fetch diffs for steps 0 through k-1 and apply them.
	workspacePath, err := createWorkspaceDir()
	if err != nil {
		return "", fmt.Errorf("create workspace dir: %w", err)
	}

	if stepIndex == 0 {
		// Step 0: Copy base clone to workspace (no diffs).
		slog.Info("rehydrating workspace for step 0 (base clone only)", "run_id", runID)
		if err := copyGitClone(*baseClonePathPtr, workspacePath); err != nil {
			_ = os.RemoveAll(workspacePath)
			return "", fmt.Errorf("copy base clone: %w", err)
		}
		return workspacePath, nil
	}

	// Step k>0: Fetch diffs from control plane and apply them.
	slog.Info("rehydrating workspace from base + diffs", "run_id", runID, "step_index", stepIndex)

	// Fetch diffs for steps 0 through k-1.
	diffFetcher, err := NewDiffFetcher(r.cfg)
	if err != nil {
		_ = os.RemoveAll(workspacePath)
		return "", fmt.Errorf("create diff fetcher: %w", err)
	}

	// FetchDiffsForStep returns diffs up to and including the specified step index.
	// Since we want diffs *before* stepIndex, we fetch up to stepIndex-1.
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
	// The step_index field enables ordering and rehydration across nodes.
	summary := types.DiffSummary{
		"step_index": stepIndex,
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

	if err := diffUploader.UploadDiff(ctx, runID, stageID, diffBytes, summary); err != nil {
		slog.Error("failed to upload step diff", "run_id", runID, "step_index", stepIndex, "error", err)
		return
	}

	slog.Info("step diff uploaded successfully", "run_id", runID, "step_index", stepIndex, "size", len(diffBytes))
}
