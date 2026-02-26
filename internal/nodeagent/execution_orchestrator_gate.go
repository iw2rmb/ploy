package nodeagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/step"
)

// executeGateJob runs a build gate validation job.
// Reports pass/fail status to server. On failure with reason="build-gate",
// the server will create healing jobs if configured.
func (r *runController) executeGateJob(ctx context.Context, req StartRunRequest) {
	startTime := time.Now()

	// Initialize runtime components.
	// Pass jobID to associate log chunks with this specific gate job.
	runner, _, logStreamer, err := r.initializeRuntime(ctx, req.RunID, req.JobID)
	if err != nil {
		slog.Error("failed to initialize runtime", "run_id", req.RunID, "error", err)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
	}
	defer func() { _ = logStreamer.Close() }()

	// Build manifest using typed options from request.
	// stepIndex=0 is used for manifest building; job configuration comes from req.TypedOptions.
	typedOpts := req.TypedOptions

	// Thread Stack Gate expectation based on gate type without next_id dependence.
	if len(typedOpts.Steps) > 0 {
		stepIdx := 0
		switch req.JobType {
		case types.JobTypePreGate:
			stepIdx = 0
		case types.JobTypePostGate, types.JobTypeReGate:
			stepIdx = len(typedOpts.Steps) - 1
		}
		step := typedOpts.Steps[stepIdx]
		if step.Stack != nil {
			// Get mig-level images from BuildGate config for image resolution.
			modImages := typedOpts.BuildGate.Images

			switch req.JobType {
			case types.JobTypePreGate:
				if step.Stack.Inbound != nil && step.Stack.Inbound.Enabled {
					typedOpts.StackGate = stackGatePhaseSpecToStepGate(step.Stack.Inbound, modImages)
				}
			case types.JobTypePostGate:
				if step.Stack.Outbound != nil && step.Stack.Outbound.Enabled {
					typedOpts.StackGate = stackGatePhaseSpecToStepGate(step.Stack.Outbound, modImages)
				}
			// Note: re_gate uses same expectations as post_gate (verifying output after healing)
			case types.JobTypeReGate:
				if step.Stack.Outbound != nil && step.Stack.Outbound.Enabled {
					typedOpts.StackGate = stackGatePhaseSpecToStepGate(step.Stack.Outbound, modImages)
				}
			}
		}
	}

	manifest, err := buildGateManifestFromRequest(req, typedOpts)
	if err != nil {
		slog.Error("failed to build manifest", "run_id", req.RunID, "error", err)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
	}

	applyGateStackDetect(&manifest, req.JobType, typedOpts)

	// Rehydrate workspace from base + diffs.
	workspace, err := r.rehydrateWorkspaceForStep(ctx, req, manifest)
	if err != nil {
		slog.Error("failed to rehydrate workspace", "run_id", req.RunID, "error", err)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
	}
	defer func() {
		if err := os.RemoveAll(workspace); err != nil {
			slog.Warn("failed to remove workspace", "path", workspace, "error", err)
		}
	}()

	// Run the build gate.
	ctx = withGateExecutionLabels(ctx, req)
	ctx = step.WithGateRuntimeImageObserver(ctx, func(obsCtx context.Context, image string) {
		if err := r.SaveJobImageName(obsCtx, req.JobID, image); err != nil {
			slog.Warn("failed to save gate job image name", "run_id", req.RunID, "job_id", req.JobID, "error", err)
		}
	})
	gateResult, gateErr := r.runGate(ctx, runner, manifest, workspace)

	// Gate execution errors (e.g., Docker pull/create/start failures) are NOT build failures
	// and must not trigger healing. Treat them as terminal cancellations for this repo
	// attempt so the control plane cancels remaining jobs without scheduling heal/re-gate.
	if gateErr != nil || gateResult == nil {
		duration := time.Since(startTime)
		errMsg := gateErr
		if errMsg == nil {
			errMsg = errors.New("gate returned nil result with nil error")
		}

		stats := types.NewRunStatsBuilder().
			DurationMs(duration.Milliseconds()).
			Error(fmt.Sprintf("gate execution failed: %s", errMsg.Error())).
			MustBuild()

		if uploadErr := r.uploadStatus(ctx, req.RunID.String(), JobStatusCancelled.String(), nil, stats, req.JobID); uploadErr != nil {
			slog.Error("failed to upload gate cancellation status after gate execution error",
				"run_id", req.RunID,
				"job_id", req.JobID,
				"error", uploadErr,
			)
		}
		slog.Error("gate execution failed; cancelling repo attempt (no healing)",
			"run_id", req.RunID,
			"job_id", req.JobID,
			"job_type", req.JobType,
			"duration", duration,
			"error", errMsg,
		)
		return
	}

	// Persist the detected stack for this run so mig and healing jobs can
	// resolve stack-specific images consistently. This is done for all gate
	// results (pass or fail) to ensure deterministic image selection.
	r.persistGateStack(req.RunID, gateResult)

	// Persist the first failing gate log for this run so discrete healing jobs
	// can hydrate /in/build-gate.log with a trimmed failure view.
	if !gateResultPassed(gateResult) {
		r.persistFirstGateFailureLog(req.RunID, gateResult)
	}

	// When gate fails and healing is configured, run the router once to produce
	// bug_summary and attach it to the gate job metadata before uploading status.
	if !gateResultPassed(gateResult) {
		r.runRouterForGateFailure(ctx, runner, req, typedOpts, workspace, gateResult)
	}

	duration := time.Since(startTime)

	// Build stats with gate metadata.
	stats := r.buildGateJobStats(gateResult, duration)

	// Check if gate passed using shared helper.
	gatePassed := gateResultPassed(gateResult)

	// Determine status and exit code.
	// v1 uses capitalized job status values: Success, Fail, Cancelled.
	if !gatePassed {
		// Gate failed - exit code 1 signals gate failure for healing.
		var exitCode int32 = 1
		if uploadErr := r.uploadStatus(ctx, req.RunID.String(), JobStatusFail.String(), &exitCode, stats, req.JobID); uploadErr != nil {
			slog.Error("failed to upload gate failure status", "run_id", req.RunID, "job_id", req.JobID, "error", uploadErr)
		}
		slog.Info("gate job failed",
			"run_id", req.RunID,
			"job_id", req.JobID,
			"job_type", req.JobType,
			"duration", duration,
		)
		return
	}

	// Gate passed.
	var exitCodeZero int32 = 0
	if uploadErr := r.uploadStatus(ctx, req.RunID.String(), JobStatusSuccess.String(), &exitCodeZero, stats, req.JobID); uploadErr != nil {
		slog.Error("failed to upload gate success status", "run_id", req.RunID, "job_id", req.JobID, "error", uploadErr)
	}
	slog.Info("gate job succeeded",
		"run_id", req.RunID,
		"job_id", req.JobID,
		"job_type", req.JobType,
		"duration", duration,
	)
}

// runRouterForGateFailure executes the configured build_gate.router container (when present)
// to summarize the failing gate log into gateResult.BugSummary.
//
// This runs only when healing is configured (since router is required for healing) and the
// gateResult indicates failure.
func (r *runController) runRouterForGateFailure(
	ctx context.Context,
	runner step.Runner,
	req StartRunRequest,
	typedOpts RunOptions,
	workspace string,
	gateResult *contracts.BuildGateStageMetadata,
) {
	if gateResult == nil || gateResultPassed(gateResult) {
		return
	}
	if typedOpts.Healing == nil || typedOpts.Healing.Mod.Image.IsEmpty() {
		return
	}
	if typedOpts.Router == nil || typedOpts.Router.Image.IsEmpty() {
		return
	}

	// Create temp /in and /out for router.
	routerInDir, err := os.MkdirTemp("", "ploy-gate-router-in-*")
	if err != nil {
		slog.Warn("failed to create router /in directory", "run_id", req.RunID, "job_id", req.JobID, "error", err)
		return
	}
	defer os.RemoveAll(routerInDir)

	routerOutDir, err := os.MkdirTemp("", "ploy-gate-router-out-*")
	if err != nil {
		slog.Warn("failed to create router /out directory", "run_id", req.RunID, "job_id", req.JobID, "error", err)
		return
	}
	defer os.RemoveAll(routerOutDir)

	// Write /in/build-gate.log with the same trimmed preference used for healing hydration.
	logPayload := gateResult.LogsText
	if len(gateResult.LogFindings) > 0 {
		if trimmed := strings.TrimSpace(gateResult.LogFindings[0].Message); trimmed != "" {
			logPayload = trimmed
			if !strings.HasSuffix(logPayload, "\n") {
				logPayload += "\n"
			}
		}
	}
	if strings.TrimSpace(logPayload) != "" {
		if writeErr := os.WriteFile(filepath.Join(routerInDir, "build-gate.log"), []byte(logPayload), 0o644); writeErr != nil {
			slog.Warn("failed to write router /in/build-gate.log", "run_id", req.RunID, "job_id", req.JobID, "error", writeErr)
		}
	}

	stack := gateResult.DetectedStack()
	if stack == "" {
		stack = contracts.ModStackUnknown
	}

	routerManifest, buildErr := buildRouterManifest(req, *typedOpts.Router, stack)
	if buildErr != nil {
		slog.Warn("failed to build router manifest", "run_id", req.RunID, "job_id", req.JobID, "error", buildErr)
		return
	}
	r.injectHealingEnvVars(&routerManifest, workspace)
	r.mountHealingTLSCerts(&routerManifest)

	_, runErr := runner.Run(ctx, step.Request{
		RunID:     req.RunID,
		JobID:     req.JobID,
		Manifest:  routerManifest,
		Workspace: workspace,
		OutDir:    routerOutDir,
		InDir:     routerInDir,
	})
	if runErr != nil {
		slog.Warn("router execution failed", "run_id", req.RunID, "job_id", req.JobID, "error", runErr)
		return
	}

	if bugSummary := parseBugSummary(routerOutDir); bugSummary != "" {
		gateResult.BugSummary = bugSummary
		slog.Info("router produced bug_summary", "run_id", req.RunID, "job_id", req.JobID, "bug_summary", bugSummary)
	}
}

// applyGateStackDetect wires the optional StackDetect override into the gate manifest.
//
// Semantics:
//   - pre_gate may use build_gate.pre.stack as a fallback/override.
//   - post_gate may use build_gate.post.stack as a fallback/override.
//   - re_gate must *not* use build_gate.post.stack; it re-runs the gate using the
//     stackdetect output to select the runtime image/tool.
func applyGateStackDetect(manifest *contracts.StepManifest, jobType types.JobType, typedOpts RunOptions) {
	if manifest == nil || manifest.Gate == nil {
		return
	}

	switch jobType {
	case types.JobTypePreGate:
		if typedOpts.BuildGate.PreStack != nil && typedOpts.BuildGate.PreStack.Enabled {
			manifest.Gate.StackDetect = typedOpts.BuildGate.PreStack
		}
	case types.JobTypePostGate:
		if typedOpts.BuildGate.PostStack != nil && typedOpts.BuildGate.PostStack.Enabled {
			manifest.Gate.StackDetect = typedOpts.BuildGate.PostStack
		}
	}
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
// per-run path under the node's cache/temp home. This allows mig and healing
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
// This allows mig and healing jobs to use the same stack detected during the
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

	logPayload := gateLogPayloadFromMetadata(meta)
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
		// Use Gate helper for simple gate stats.
		builder.Gate(gateResultPassed(gateResult), duration.Milliseconds())

		// Attach structured job metadata so the control plane can persist
		// gate results in jobs.meta JSONB.
		if jobMetaBytes, err := json.Marshal(contracts.NewGateJobMeta(gateResult)); err == nil {
			builder.JobMeta(jobMetaBytes)
		}
	}

	return builder.MustBuild()
}

// buildGateStats constructs gate statistics including pre-gate, re-gates, and final gate runs.
// Uploads gate logs as artifact bundles for debugging.
//
// final_gate semantics:
//   - When a post-mig gate exists (result.BuildGate != nil), final_gate is the last post-mig gate.
//   - When no migs executed (no BuildGate), final_gate falls back to the pre-mig gate, ensuring
//     CLI/API gate summaries always have a final_gate to report on.
//   - This keeps gate summary behavior consistent: final_gate → last re-gate → pre_gate.
func (r *runController) buildGateStats(runID types.RunID, jobID types.JobID, result step.Result, execResult executionResult) *types.RunStatsGate {
	gate := &types.RunStatsGate{}

	// Helper function to build gate phase metadata and upload logs.
	buildGatePhase := func(meta *contracts.BuildGateStageMetadata, durationMs int64, artifactNameSuffix string) *types.RunStatsGatePhase {
		phase := &types.RunStatsGatePhase{
			DurationMs: durationMs,
		}

		// Determine pass/fail using shared helper.
		phase.Passed = gateResultPassed(meta)

		// Attach resource usage metrics when available.
		if meta != nil && meta.Resources != nil {
			ru := meta.Resources
			var sizeRwBytes int64
			if ru.SizeRwBytes != nil {
				sizeRwBytes = *ru.SizeRwBytes
			}
			phase.Resources = &types.RunStatsGateResources{
				Limits: &types.RunStatsResourceLimits{
					NanoCPUs:    ru.LimitNanoCPUs,
					MemoryBytes: ru.LimitMemoryBytes,
				},
				Usage: &types.RunStatsResourceUsage{
					CPUTotalNs:      ru.CPUTotalNs,
					MemUsageBytes:   ru.MemUsageBytes,
					MemMaxBytes:     ru.MemMaxBytes,
					BlkioReadBytes:  ru.BlkioReadBytes,
					BlkioWriteBytes: ru.BlkioWriteBytes,
					SizeRwBytes:     sizeRwBytes,
				},
			}
		}

		// Upload build logs as artifact when present.
		if meta != nil && strings.TrimSpace(meta.LogsText) != "" {
			r.uploadGateLogsArtifact(runID, jobID, meta.LogsText, artifactNameSuffix, phase)
		}

		return phase
	}

	// Include pre-gate stats if present.
	if execResult.PreGate != nil {
		gate.PreGate = buildGatePhase(execResult.PreGate.Metadata, execResult.PreGate.DurationMs, "pre")
	}

	// Include re-gate stats if present (healing attempts from both pre- and post-mig phases
	// in chronological order).
	if len(execResult.ReGates) > 0 {
		gate.ReGates = make([]types.RunStatsGatePhase, 0, len(execResult.ReGates))
		for i, rg := range execResult.ReGates {
			suffix := fmt.Sprintf("re%d", i+1)
			if phase := buildGatePhase(rg.Metadata, rg.DurationMs, suffix); phase != nil {
				gate.ReGates = append(gate.ReGates, *phase)
			}
		}
	}

	// Populate final_gate: use the post-mig gate (result.BuildGate) when present,
	// otherwise fall back to the pre-mig gate (for runs where no migs executed).
	// This ensures CLI/API gate summaries always have a final_gate to report on.
	if result.BuildGate != nil {
		gate.FinalGate = buildGatePhase(result.BuildGate, time.Duration(result.Timings.BuildGateDuration).Milliseconds(), "")
	} else if execResult.PreGate != nil {
		// No post-mig gate executed (run terminated at pre-mig phase or no migs).
		// Use the pre-mig gate as the final gate for consistent summary output.
		gate.FinalGate = buildGatePhase(execResult.PreGate.Metadata, execResult.PreGate.DurationMs, "pre-as-final")
	}

	if gate.PreGate == nil && gate.FinalGate == nil && len(gate.ReGates) == 0 {
		return nil
	}
	return gate
}
