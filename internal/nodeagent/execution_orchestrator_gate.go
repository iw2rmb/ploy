package nodeagent

import (
	"context"
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

// executeGateJob runs a build gate validation job.
// Reports pass/fail status to server. On failure with reason="build-gate",
// the server will create healing jobs if configured.
func (r *runController) executeGateJob(ctx context.Context, req StartRunRequest) {
	startTime := time.Now()

	// Initialize runtime components.
	runner, _, logStreamer, err := r.initializeRuntime(ctx, req.RunID.String())
	if err != nil {
		slog.Error("failed to initialize runtime", "run_id", req.RunID, "error", err)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
	}
	defer func() { _ = logStreamer.Close() }()

	// Build manifest using typed options from request.
	// stepIndex=0 is used for manifest building; job configuration comes from req.TypedOptions.
	typedOpts := req.TypedOptions
	manifest, err := buildGateManifestFromRequest(req, typedOpts)
	if err != nil {
		slog.Error("failed to build manifest", "run_id", req.RunID, "error", err)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
	}

	// Rehydrate workspace from base + diffs.
	workspace, err := r.rehydrateWorkspaceForStep(ctx, req, manifest, req.StepIndex)
	if err != nil {
		slog.Error("failed to rehydrate workspace", "run_id", req.RunID, "error", err)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
	}
	defer func() { _ = os.RemoveAll(workspace) }()

	// Run the build gate.
	gateResult, gateErr := r.runGate(ctx, runner, manifest, workspace)

	// Persist the detected stack for this run so mod and healing jobs can
	// resolve stack-specific images consistently. This is done for all gate
	// results (pass or fail) to ensure deterministic image selection.
	if gateResult != nil {
		r.persistGateStack(req.RunID, gateResult)
	}

	// Persist the first failing gate log for this run so discrete healing jobs
	// can hydrate /in/build-gate.log with a trimmed failure view.
	if gateErr != nil || (gateResult != nil && !gateResultPassed(gateResult)) {
		r.persistFirstGateFailureLog(req.RunID, gateResult)
	}

	duration := time.Since(startTime)

	// Build stats with gate metadata.
	stats := r.buildGateJobStats(gateResult, duration)

	// Check if gate passed.
	gatePassed := false
	if gateResult != nil && len(gateResult.StaticChecks) > 0 {
		gatePassed = gateResult.StaticChecks[0].Passed
	}

	// Determine status and exit code.
	if gateErr != nil || !gatePassed {
		// Gate failed - exit code 1 signals gate failure for healing.
		var exitCode int32 = 1
		if uploadErr := r.uploadStatus(ctx, req.RunID.String(), "failed", &exitCode, stats, req.StepIndex, req.JobID); uploadErr != nil {
			slog.Error("failed to upload gate failure status", "run_id", req.RunID, "job_id", req.JobID, "error", uploadErr)
		}
		slog.Info("gate job failed",
			"run_id", req.RunID,
			"job_id", req.JobID,
			"mod_type", req.ModType,
			"duration", duration,
		)
		return
	}

	// Gate passed.
	var exitCodeZero int32 = 0
	if uploadErr := r.uploadStatus(ctx, req.RunID.String(), "succeeded", &exitCodeZero, stats, req.StepIndex, req.JobID); uploadErr != nil {
		slog.Error("failed to upload gate success status", "run_id", req.RunID, "job_id", req.JobID, "error", uploadErr)
	}
	slog.Info("gate job succeeded",
		"run_id", req.RunID,
		"job_id", req.JobID,
		"mod_type", req.ModType,
		"duration", duration,
	)
}

// runGate executes the build gate and returns the result.
func (r *runController) runGate(ctx context.Context, runner step.Runner, manifest contracts.StepManifest, workspace string) (*contracts.BuildGateStageMetadata, error) {
	gateSpec := manifest.Gate
	if runner.Gate == nil || gateSpec == nil || !gateSpec.Enabled {
		// No gate configured - return success.
		return &contracts.BuildGateStageMetadata{
			StaticChecks: []contracts.BuildGateStaticCheckReport{{Passed: true, Tool: "none"}},
		}, nil
	}

	return runner.Gate.Execute(ctx, gateSpec, workspace)
}

// gateResultPassed reports whether the gate result indicates a passing gate.
func gateResultPassed(gateResult *contracts.BuildGateStageMetadata) bool {
	if gateResult == nil {
		return false
	}
	if len(gateResult.StaticChecks) == 0 {
		return false
	}
	return gateResult.StaticChecks[0].Passed
}

// persistGateStack writes the detected stack from a gate result to a stable
// per-run path under the node's cache/temp home. This allows mod and healing
// jobs to resolve stack-specific images consistently, even when executed as
// separate jobs on the same or different nodes.
//
// The function is idempotent: once a stack has been written for a run,
// subsequent calls are no-ops to preserve the original detection result.
// This ensures stability across re-gates and healing retries.
func (r *runController) persistGateStack(runID types.RunID, meta *contracts.BuildGateStageMetadata) {
	if meta == nil {
		return
	}

	stack := meta.DetectedStack()
	if stack == "" {
		stack = contracts.ModStackUnknown
	}

	baseRoot := os.Getenv("PLOYD_CACHE_HOME")
	if baseRoot == "" {
		baseRoot = os.TempDir()
	}
	runDir := filepath.Join(baseRoot, "ploy", "run", runID.String())
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		slog.Warn("failed to create run dir for gate stack", "run_id", runID, "error", err)
		return
	}

	stackPath := filepath.Join(runDir, "build-gate-stack.txt")
	if _, err := os.Stat(stackPath); err == nil {
		// Stack already persisted for this run; keep the first detection.
		return
	}

	if err := os.WriteFile(stackPath, []byte(string(stack)), 0o644); err != nil {
		slog.Warn("failed to persist build gate stack", "run_id", runID, "path", stackPath, "error", err)
		return
	}

	slog.Info("persisted build gate stack", "run_id", runID, "stack", stack, "path", stackPath)
}

// loadPersistedStack reads the persisted stack for a run from the node's
// cache/temp home. Returns ModStackUnknown if no stack file exists or on error.
//
// This allows mod and healing jobs to use the same stack detected during the
// initial gate execution, ensuring deterministic image selection across jobs.
func (r *runController) loadPersistedStack(runID types.RunID) contracts.ModStack {
	baseRoot := os.Getenv("PLOYD_CACHE_HOME")
	if baseRoot == "" {
		baseRoot = os.TempDir()
	}
	stackPath := filepath.Join(baseRoot, "ploy", "run", runID.String(), "build-gate-stack.txt")

	data, err := os.ReadFile(stackPath)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("failed to read persisted stack", "run_id", runID, "error", err)
		}
		return contracts.ModStackUnknown
	}

	stack := contracts.ModStack(strings.TrimSpace(string(data)))
	if stack == "" {
		return contracts.ModStackUnknown
	}

	slog.Debug("loaded persisted stack for run", "run_id", runID, "stack", stack)
	return stack
}

// persistFirstGateFailureLog writes the first failing gate log for a run to a
// stable per-run path under the node's cache/temp home. Healing jobs later read
// this file to hydrate /in/build-gate.log without re-running the gate.
//
// The function is idempotent: once a log has been written for a run, subsequent
// calls are no-ops to preserve the original failure context.
func (r *runController) persistFirstGateFailureLog(runID types.RunID, meta *contracts.BuildGateStageMetadata) {
	if meta == nil {
		return
	}

	// Prefer trimmed LogFindings view when available; fall back to full LogsText.
	logPayload := meta.LogsText
	if len(meta.LogFindings) > 0 {
		if trimmed := strings.TrimSpace(meta.LogFindings[0].Message); trimmed != "" {
			logPayload = trimmed
			if !strings.HasSuffix(logPayload, "\n") {
				logPayload += "\n"
			}
		}
	}
	if strings.TrimSpace(logPayload) == "" {
		return
	}

	baseRoot := os.Getenv("PLOYD_CACHE_HOME")
	if baseRoot == "" {
		baseRoot = os.TempDir()
	}
	runDir := filepath.Join(baseRoot, "ploy", "run", runID.String())
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		slog.Warn("failed to create run dir for gate log", "run_id", runID, "error", err)
		return
	}

	logPath := filepath.Join(runDir, "build-gate-first.log")
	if _, err := os.Stat(logPath); err == nil {
		// Log already persisted for this run; keep the first failure view.
		return
	}

	if err := os.WriteFile(logPath, []byte(logPayload), 0o644); err != nil {
		slog.Warn("failed to persist first build gate failure log", "run_id", runID, "path", logPath, "error", err)
		return
	}

	slog.Info("persisted first build gate failure log", "run_id", runID, "path", logPath)
}

// buildGateJobStats constructs stats payload for gate job completion.
// Uses typed builder to eliminate map[string]any construction.
func (r *runController) buildGateJobStats(gateResult *contracts.BuildGateStageMetadata, duration time.Duration) types.RunStats {
	builder := types.NewRunStatsBuilder().
		DurationMs(duration.Milliseconds())

	if gateResult != nil {
		passed := false
		if len(gateResult.StaticChecks) > 0 {
			passed = gateResult.StaticChecks[0].Passed
		}
		// Use Gate helper for simple gate stats.
		builder.Gate(passed, duration.Milliseconds())

		// Attach structured job metadata so the control plane can persist
		// gate results in jobs.meta JSONB.
		builder.JobMetaAny(contracts.NewGateJobMeta(gateResult))
	}

	return builder.MustBuild()
}

// buildGateStats constructs gate statistics including pre-gate, re-gates, and final gate runs.
// Uploads gate logs as artifact bundles for debugging.
//
// final_gate semantics:
//   - When a post-mod gate exists (result.BuildGate != nil), final_gate is the last post-mod gate.
//   - When no mods executed (no BuildGate), final_gate falls back to the pre-mod gate, ensuring
//     CLI/API gate summaries always have a final_gate to report on.
//   - This keeps gate summary behavior consistent: final_gate → last re-gate → pre_gate.
func (r *runController) buildGateStats(runID types.RunID, jobID types.JobID, result step.Result, execResult executionResult) map[string]any {
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
			r.uploadGateLogsArtifact(runID, jobID, meta.LogsText, artifactNameSuffix, gateStats)
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
