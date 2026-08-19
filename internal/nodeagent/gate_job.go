package nodeagent

import (
	"context"
	"encoding/json"
	"errors"
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
// Reports pass/fail status to server.
func (r *runController) executeGateJob(ctx context.Context, req StartRunRequest, mounts step.JobMounts) {
	startTime := time.Now()

	jobDirs := jobDirectories(req.RunID, req.JobID)
	uploadRepoArtifactsOnReturn := false
	closeArtifactLogs := func() {}
	defer func() {
		closeArtifactLogs()
		if uploadRepoArtifactsOnReturn {
			r.uploadRepoArtifactsIfPresent(req.RunID, req.RepoID, req.JobID)
		}
	}()

	// Initialize runtime components.
	// Pass jobID to associate log chunks with this specific gate job.
	runner, _, logStreamer, err := r.initializeRuntime(ctx, req.RunID, req.JobID)
	if err != nil {
		uploadRepoArtifactsOnReturn = true
		slog.Error("failed to initialize runtime", "run_id", req.RunID, "error", err)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
	}
	defer func() { _ = logStreamer.Close() }()
	artifactLogs, err := newArtifactLogWriter(logStreamer, jobDirs)
	if err != nil {
		uploadRepoArtifactsOnReturn = true
		slog.Error("failed to prepare job artifact logs", "run_id", req.RunID, "job_id", req.JobID, "error", err)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
	}
	runner.LogWriter = artifactLogs
	closeArtifactLogs = func() {
		if err := artifactLogs.Close(); err != nil {
			slog.Warn("failed to close job artifact logs", "run_id", req.RunID, "job_id", req.JobID, "error", err)
		}
	}

	// Build manifest using typed options from request.
	// stepIndex=0 is used for manifest building; job configuration comes from req.TypedOptions.
	typedOpts := req.TypedOptions

	// Thread Stack Gate expectation based on gate type without next_id dependence.
	if phase := stackGatePhaseForJob(typedOpts.Steps, req.JobType); phase != nil {
		typedOpts.StackGate = phase
	}

	manifest, err := buildGateManifest(req, typedOpts)
	if err != nil {
		uploadRepoArtifactsOnReturn = true
		slog.Error("failed to build manifest", "run_id", req.RunID, "error", err)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
	}

	applyGatePhaseOverrides(&manifest, req, typedOpts)

	workspace, err := r.prepareStickyWorkspace(ctx, req, manifest)
	if err != nil {
		uploadRepoArtifactsOnReturn = true
		slog.Error("failed to prepare sticky workspace", "run_id", req.RunID, "error", err)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
	}
	// Run the build gate.
	ctx = withGateExecutionLabels(ctx, req)
	ctx = step.WithGateRuntimeImageObserver(ctx, func(obsCtx context.Context, image string) {
		if err := r.SaveJobImageName(obsCtx, req.JobID, image); err != nil {
			slog.Warn("failed to save gate job image name", "run_id", req.RunID, "job_id", req.JobID, "error", err)
		}
	})
	gateResult, gateErr := r.runGate(ctx, runner, manifest, workspace, mounts)

	// Gate execution errors (e.g., Docker pull/create/start failures) are NOT build failures
	// and are treated as terminal runtime errors for this repo attempt so the
	// control plane cancels remaining jobs.
	if gateErr != nil || gateResult == nil {
		duration := time.Since(startTime)
		uploadRepoArtifactsOnReturn = true
		errMsg := gateErr
		if errMsg == nil {
			errMsg = errors.New("gate returned nil result with nil error")
		}
		r.uploadGateErrorStatus(ctx, req, errMsg, duration)
		slog.Error("gate execution failed; marking repo attempt as error",
			"run_id", req.RunID,
			"job_id", req.JobID,
			"job_type", req.JobType,
			"duration", duration,
			"error", errMsg,
		)
		return
	}

	// Persist the detected stack for this run so mig jobs can
	// resolve stack-specific images consistently. This is done for all gate
	// results (pass or fail) to ensure deterministic image selection.
	r.persistGateStack(req.RunID, gateResult)

	// Persist the first failing gate log for this run.
	if !gateResultPassed(gateResult) {
		r.persistFirstGateFailureLog(req.RunID, gateResult)
	}

	if err := r.persistGateSBOM(ctx, req, mounts.Share); err != nil {
		duration := time.Since(startTime)
		uploadRepoArtifactsOnReturn = true
		slog.Error("gate sbom persistence failed; marking gate job as error",
			"run_id", req.RunID,
			"job_id", req.JobID,
			"job_type", req.JobType,
			"duration", duration,
			"error", err,
		)
		r.uploadGateErrorStatus(ctx, req, err, duration)
		return
	}

	duration := time.Since(startTime)

	// Determine status and exit code based on gate outcome.
	status := types.JobStatusSuccess
	var exitCode int32 = 0
	logVerb := "succeeded"
	if !gateResultPassed(gateResult) {
		status = types.JobStatusFail
		exitCode = 1
		logVerb = "failed"
		uploadRepoArtifactsOnReturn = true
	}
	repoSHAOut := ""
	if status == types.JobStatusSuccess {
		var repoSHAErr error
		repoSHAOut, repoSHAErr = r.computeRepoSHAOut(ctx, req, workspace, "")
		if repoSHAErr != nil {
			uploadRepoArtifactsOnReturn = true
			stats := r.buildGateStats(gateResult, duration)
			var errorExitCode int32 = -1
			if uploadErr := r.uploadStatus(ctx, req.RunID.String(), types.JobStatusError.String(), &errorExitCode, stats, req.JobID); uploadErr != nil {
				slog.Error("failed to upload gate error status after repo_sha_out failure",
					"run_id", req.RunID,
					"job_id", req.JobID,
					"error", uploadErr,
				)
			}
			slog.Error("gate job errored due to repo_sha_out failure",
				"run_id", req.RunID,
				"job_id", req.JobID,
				"duration", duration,
				"error", repoSHAErr,
			)
			return
		}
	}
	if status == types.JobStatusSuccess && req.JobType == types.JobTypePostGate {
		uploadRepoArtifactsOnReturn = true
	}

	// Build stats with gate metadata.
	stats := r.buildGateStats(gateResult, duration)
	if uploadErr := r.uploadStatus(ctx, req.RunID.String(), status.String(), &exitCode, stats, req.JobID, repoSHAOut); uploadErr != nil {
		slog.Error("failed to upload gate status", "run_id", req.RunID, "job_id", req.JobID, "status", status, "error", uploadErr)
	} else {
		r.cleanupRunShareOnTerminalSuccess(req, status)
	}
	slog.Info("gate job "+logVerb, "run_id", req.RunID, "job_id", req.JobID, "job_type", req.JobType, "duration", duration)
}

func stackGatePhaseForJob(steps []StepOptions, jobType types.JobType) *contracts.StackGatePhaseSpec {
	if len(steps) == 0 {
		return nil
	}
	var phase *contracts.StackGatePhaseSpec
	switch jobType {
	case types.JobTypePreGate:
		if steps[0].Stack != nil {
			phase = steps[0].Stack.Inbound
		}
	case types.JobTypePostGate:
		if steps[len(steps)-1].Stack != nil {
			phase = steps[len(steps)-1].Stack.Outbound
		}
	}
	if phase == nil || !phase.Enabled {
		return nil
	}
	return phase
}

func (r *runController) uploadGateErrorStatus(ctx context.Context, req StartRunRequest, err error, duration time.Duration) {
	r.uploadFailureStatus(ctx, req, err, duration)
}

// applyGatePhaseOverrides wires optional per-phase stack policy into the gate manifest.
func applyGatePhaseOverrides(manifest *contracts.StepManifest, req StartRunRequest, typedOpts RunOptions) {
	if manifest == nil || manifest.Gate == nil {
		return
	}

	switch req.JobType {
	case types.JobTypePreGate:
		contracts.ApplyBuildGatePhaseToGateSpec(manifest.Gate, typedOpts.BuildGate.Pre)
	case types.JobTypePostGate:
		contracts.ApplyBuildGatePhaseToGateSpec(manifest.Gate, typedOpts.BuildGate.Post)
	}
}

// runGate executes the build gate and returns the result.
func (r *runController) runGate(ctx context.Context, runner step.Runner, manifest contracts.StepManifest, workspace string, mounts step.JobMounts) (*contracts.BuildGateStageMetadata, error) {
	gateSpec := manifest.Gate
	if runner.Gate == nil || gateSpec == nil || !gateSpec.Enabled {
		// No gate configured - return success.
		return &contracts.BuildGateStageMetadata{
			StaticChecks: []contracts.BuildGateStaticCheckReport{{Passed: true, Tool: "none"}},
		}, nil
	}
	return runner.Gate.Execute(step.WithExecutionLogWriter(ctx, runner.LogWriter), gateSpec, workspace, mounts)
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

// persistOnce writes data to dir/filename idempotently: if the file already
// exists the call is a no-op, preserving the first write.
func persistOnce(dir, filename string, data []byte, label string, runID types.RunID) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		slog.Warn("failed to create dir for "+label, "run_id", runID, "error", err)
		return
	}
	p := filepath.Join(dir, filename)
	if _, err := os.Stat(p); err == nil {
		return
	}
	if err := os.WriteFile(p, data, 0o600); err != nil {
		slog.Warn("failed to persist "+label, "run_id", runID, "path", p, "error", err)
		return
	}
	slog.Info("persisted "+label, "run_id", runID, "path", p)
}

// persistGateStack writes the detected stack from a gate result to a stable
// per-run path. Idempotent: preserves the first detection result.
func (r *runController) persistGateStack(runID types.RunID, meta *contracts.BuildGateStageMetadata) {
	if meta == nil {
		return
	}
	stack := meta.DetectedStack()
	if stack == "" {
		stack = contracts.MigStackUnknown
	}
	persistOnce(runDir(runID), "build-gate-stack.txt", []byte(string(stack)), "build gate stack", runID)
}

// loadPersistedStack reads the persisted stack for a run.
// Returns MigStackUnknown if no stack file exists or on error.
func (r *runController) loadPersistedStack(runID types.RunID) contracts.MigStack {
	stackPath := filepath.Join(runDir(runID), "build-gate-stack.txt")
	data, err := os.ReadFile(stackPath)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("failed to read persisted stack", "run_id", runID, "error", err)
		}
		return contracts.MigStackUnknown
	}
	stack := contracts.MigStack(strings.TrimSpace(string(data)))
	if stack == "" {
		return contracts.MigStackUnknown
	}
	slog.Debug("loaded persisted stack for run", "run_id", runID, "stack", stack)
	return stack
}

// persistFirstGateFailureLog writes the first failing gate log for a run.
// Idempotent: preserves the original failure context.
func (r *runController) persistFirstGateFailureLog(runID types.RunID, meta *contracts.BuildGateStageMetadata) {
	if meta == nil {
		return
	}
	logPayload := gateLogPayloadFromMetadata(meta)
	if strings.TrimSpace(logPayload) == "" {
		return
	}
	persistOnce(runDir(runID), "build-gate-first.log", []byte(logPayload), "first build gate failure log", runID)
}

// buildGateStats constructs stats payload for gate job completion.
// Uses typed builder to eliminate map[string]any construction.
func (r *runController) buildGateStats(gateResult *contracts.BuildGateStageMetadata, duration time.Duration) types.RunStats {
	builder := types.NewRunStatsBuilder().
		DurationMs(duration.Milliseconds())

	if gateResult != nil {
		// Use Gate helper for simple gate stats.
		passed := gateResultPassed(gateResult)
		builder.Gate(passed, duration.Milliseconds())
		if !passed {
			if errorText := gateFailureErrorFromMetadata(gateResult); errorText != "" {
				builder.Error(errorText)
			}
		}
		if resources := step.NormalizeContainerResourceUsage(gateResult.Resources); resources != nil {
			builder.JobResources(resources)
		}

		// Attach structured job metadata so the control plane can persist
		// gate results in jobs.meta JSONB.
		if jobMetaBytes, err := json.Marshal(contracts.NewGateJobMeta(gateResult)); err == nil {
			builder.JobMeta(jobMetaBytes)
		}
	}

	return builder.MustBuild()
}
