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

	// Phase 1: Convert the StartRunRequest to a StepManifest.
	manifest, err := buildManifestFromRequest(req)
	if err != nil {
		slog.Error("failed to build manifest", "run_id", req.RunID, "error", err)
		return
	}

	// Phase 2: Create ephemeral workspace directory (honors PLOYD_CACHE_HOME when set).
	workspaceRoot, err := createWorkspaceDir()
	if err != nil {
		slog.Error("failed to create workspace", "run_id", req.RunID, "error", err)
		return
	}
	defer func() {
		_ = os.RemoveAll(workspaceRoot)
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

	// Phase 4: Execute the step with possible healing loop.
	startTime := time.Now()
	execResult, execErr := r.executeWithHealing(ctx, runner, req, manifest, workspaceRoot, outDir, &inDir)
	duration := time.Since(startTime)
	result := execResult.Result

	if execErr != nil {
		slog.Error("run execution failed",
			"run_id", req.RunID,
			"error", execErr,
			"duration", duration,
			"exit_code", result.ExitCode,
		)
		// Continue to emit terminal status even on failure.
	}

	// Phase 5: Generate and upload diff to server if diff generator is available.
	stageID, _ := manifest.OptionString("stage_id")
	r.uploadDiff(ctx, req.RunID.String(), stageID, diffGenerator, workspaceRoot, result)

	// Phase 6a: Upload configured artifact bundles (artifact_paths option).
	r.uploadConfiguredArtifacts(ctx, req, manifest, workspaceRoot)

	// Phase 6b: Always attempt to bundle and upload /out directory.
	if err := uploadOutDirIfPresent(ctx, r.cfg, req.RunID.String(), stageID, outDir); err != nil {
		slog.Error("/out artifact upload failed", "run_id", req.RunID, "error", err)
	}

	// Phase 7 & 8: Emit terminal status and conditionally create merge request.
	r.finalizeRun(ctx, req, manifest, execResult, execErr, workspaceRoot, duration)

	slog.Info("run execution completed",
		"run_id", req.RunID,
		"duration", duration,
		"exit_code", result.ExitCode,
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

// uploadDiff generates and uploads the workspace diff to the control plane.
// Also uploads diff as an artifact bundle for client download.
func (r *runController) uploadDiff(ctx context.Context, runID, stageID string, diffGenerator step.DiffGenerator, workspace string, result step.Result) {
	if diffGenerator == nil {
		return
	}

	diffBytes, err := diffGenerator.Generate(ctx, workspace)
	if err != nil {
		slog.Error("failed to generate diff", "run_id", runID, "error", err)
		return
	}

	if len(diffBytes) == 0 {
		return
	}

	// Upload diff with execution summary metadata.
	diffUploader, err := NewDiffUploader(r.cfg)
	if err != nil {
		slog.Error("failed to create diff uploader", "run_id", runID, "error", err)
		return
	}

	summary := map[string]interface{}{
		"exit_code": result.ExitCode,
		"timings": map[string]interface{}{
			"hydration_duration_ms":  result.Timings.HydrationDuration.Milliseconds(),
			"execution_duration_ms":  result.Timings.ExecutionDuration.Milliseconds(),
			"build_gate_duration_ms": result.Timings.BuildGateDuration.Milliseconds(),
			"diff_duration_ms":       result.Timings.DiffDuration.Milliseconds(),
			"total_duration_ms":      result.Timings.TotalDuration.Milliseconds(),
		},
	}

	if err := diffUploader.UploadDiff(ctx, runID, stageID, diffBytes, summary); err != nil {
		slog.Error("failed to upload diff", "run_id", runID, "error", err)
		return
	}

	slog.Info("diff uploaded successfully", "run_id", runID, "size", len(diffBytes))

	// Also upload diff as artifact bundle named "diff" for client download.
	diffFile, err := os.CreateTemp("", "ploy-diff-*.patch")
	if err != nil {
		return
	}
	defer func() { _ = os.Remove(diffFile.Name()) }()

	_, _ = diffFile.Write(diffBytes)
	_ = diffFile.Close()

	artUploader, err := NewArtifactUploader(r.cfg)
	if err != nil {
		return
	}

	if _, _, errU := artUploader.UploadArtifact(ctx, runID, stageID, []string{diffFile.Name()}, "diff"); errU != nil {
		slog.Warn("failed to upload diff artifact bundle", "run_id", runID, "error", errU)
	} else {
		slog.Info("diff artifact bundle uploaded", "run_id", runID)
	}
}

// uploadConfiguredArtifacts uploads artifact bundles specified in the artifact_paths option.
func (r *runController) uploadConfiguredArtifacts(ctx context.Context, req StartRunRequest, manifest contracts.StepManifest, workspace string) {
	// Accept either []any (from JSON) or []string (programmatic callers).
	var paths []string
	switch v := req.Options["artifact_paths"].(type) {
	case []any:
		for _, p := range v {
			if s, ok := p.(string); ok && s != "" {
				fullPath := filepath.Join(workspace, s)
				if _, err := os.Stat(fullPath); err == nil {
					paths = append(paths, fullPath)
				} else {
					slog.Warn("artifact path not found", "run_id", req.RunID, "path", s)
				}
			}
		}
	case []string:
		for _, s := range v {
			if strings.TrimSpace(s) == "" {
				continue
			}
			fullPath := filepath.Join(workspace, s)
			if _, err := os.Stat(fullPath); err == nil {
				paths = append(paths, fullPath)
			} else {
				slog.Warn("artifact path not found", "run_id", req.RunID, "path", s)
			}
		}
	}

	if len(paths) == 0 {
		return
	}

	artifactUploader, err := NewArtifactUploader(r.cfg)
	if err != nil {
		slog.Error("failed to create artifact uploader", "run_id", req.RunID, "error", err)
		return
	}

	stageID, _ := manifest.OptionString("stage_id")
	artifactName, _ := manifest.OptionString("artifact_name")

	if _, _, err := artifactUploader.UploadArtifact(ctx, req.RunID.String(), stageID, paths, artifactName); err != nil {
		slog.Error("failed to upload artifact bundle", "run_id", req.RunID, "error", err)
	} else {
		slog.Info("artifact bundle uploaded successfully", "run_id", req.RunID, "paths", len(paths))
	}
}

// finalizeRun handles terminal status determination, merge request creation, and status upload.
func (r *runController) finalizeRun(ctx context.Context, req StartRunRequest, manifest contracts.StepManifest, execResult executionResult, execErr error, workspace string, duration time.Duration) {
	statusUploader, err := NewStatusUploader(r.cfg)
	if err != nil {
		slog.Error("failed to create status uploader", "run_id", req.RunID, "error", err)
		return
	}

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
	stats := r.buildExecutionStats(req.RunID.String(), result, execResult, duration, mrURL)

	// Phase 8: Upload terminal status to server with a short, detached context so
	// we still attempt to report completion even if the run context is cancelled.
	statusCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if uploadErr := statusUploader.UploadStatus(statusCtx, req.RunID.String(), terminalStatus, reason, stats); uploadErr != nil {
		slog.Error("failed to upload terminal status", "run_id", req.RunID, "error", uploadErr)
	} else {
		slog.Info("terminal status uploaded successfully", "run_id", req.RunID, "status", terminalStatus)
	}
}

// buildExecutionStats constructs the stats payload for terminal status upload.
// Includes execution timings, exit code, gate history (pre-gate, re-gates), and MR URL.
func (r *runController) buildExecutionStats(runID string, result step.Result, execResult executionResult, duration time.Duration, mrURL string) map[string]interface{} {
	stats := map[string]interface{}{
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
		gate := r.buildGateStats(runID, result, execResult)
		stats["gate"] = gate
	}

	return stats
}

// buildGateStats constructs gate statistics including pre-gate, re-gates, and final gate runs.
// Uploads gate logs as artifact bundles for debugging.
func (r *runController) buildGateStats(runID string, result step.Result, execResult executionResult) map[string]any {
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
			r.uploadGateLogsArtifact(runID, meta.LogsText, artifactNameSuffix, gateStats)
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

// uploadGateLogsArtifact uploads gate logs as an artifact bundle and attaches IDs to stats.
func (r *runController) uploadGateLogsArtifact(runID, logsText, artifactNameSuffix string, gateStats map[string]any) {
	logFile, err := os.CreateTemp("", "ploy-gate-*.log")
	if err != nil {
		return
	}
	defer func() { _ = os.Remove(logFile.Name()) }()

	_, _ = logFile.WriteString(logsText)
	_ = logFile.Close()

	artUploader, err := NewArtifactUploader(r.cfg)
	if err != nil {
		return
	}

	// Stage ID is not available here; use empty string (server will handle).
	artifactName := "build-gate.log"
	if artifactNameSuffix != "" {
		artifactName = "build-gate-" + artifactNameSuffix + ".log"
	}

	if id, cid, uerr := artUploader.UploadArtifact(context.Background(), runID, "", []string{logFile.Name()}, artifactName); uerr == nil {
		gateStats["logs_artifact_id"] = id
		gateStats["logs_bundle_cid"] = cid
	} else {
		slog.Warn("failed to upload "+artifactName, "run_id", runID, "error", uerr)
	}
}
