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
	"github.com/iw2rmb/ploy/internal/workflow/gateprofile"
	"github.com/iw2rmb/ploy/internal/workflow/step"
)

const (
	gateGradleJUnitXMLReportPath = "/out/gradle-test-results"
	gateGradleHTMLReportPath     = "/out/gradle-test-report"
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

	applyGatePhaseOverrides(&manifest, req, typedOpts)

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
		r.cleanupGateOutDir(workspace)
		repoSHAOut := r.computeRepoSHAOut(ctx, req, workspace)
		errMsg := gateErr
		if errMsg == nil {
			errMsg = errors.New("gate returned nil result with nil error")
		}

		stats := types.NewRunStatsBuilder().
			DurationMs(duration.Milliseconds()).
			Error(fmt.Sprintf("gate execution failed: %s", errMsg.Error())).
			MustBuild()

		if uploadErr := r.uploadStatus(ctx, req.RunID.String(), types.JobStatusCancelled.String(), nil, stats, req.JobID, repoSHAOut); uploadErr != nil {
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
	r.persistGateProfileSnapshot(req.RunID, req.JobType, manifest.Gate, gateResult)

	// When gate fails and healing is configured, run the router once to produce
	// bug_summary and attach it to the gate job metadata before uploading status.
	if !gateResultPassed(gateResult) {
		r.runRouterForGateFailure(ctx, runner, req, typedOpts, workspace, gateResult)
	}
	if err := r.uploadOutDirBundle(ctx, req.RunID, req.JobID, filepath.Join(workspace, step.BuildGateWorkspaceOutDir), "build-gate-out"); err != nil {
		slog.Warn("failed to upload gate /out bundle", "run_id", req.RunID, "job_id", req.JobID, "error", err)
	}
	r.uploadGateReportArtifacts(ctx, req.RunID, req.JobID, workspace, gateResult)
	r.cleanupGateOutDir(workspace)

	duration := time.Since(startTime)
	repoSHAOut := r.computeRepoSHAOut(ctx, req, workspace)

	// Build stats with gate metadata.
	stats := r.buildGateJobStats(gateResult, duration)

	// Check if gate passed using shared helper.
	gatePassed := gateResultPassed(gateResult)

	// Determine status and exit code.
	// v1 uses capitalized job status values: Success, Fail, Cancelled.
	if !gatePassed {
		// Gate failed - exit code 1 signals gate failure for healing.
		var exitCode int32 = 1
		if uploadErr := r.uploadStatus(ctx, req.RunID.String(), types.JobStatusFail.String(), &exitCode, stats, req.JobID, repoSHAOut); uploadErr != nil {
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
	if uploadErr := r.uploadStatus(ctx, req.RunID.String(), types.JobStatusSuccess.String(), &exitCodeZero, stats, req.JobID, repoSHAOut); uploadErr != nil {
		slog.Error("failed to upload gate success status", "run_id", req.RunID, "job_id", req.JobID, "error", uploadErr)
	}
	slog.Info("gate job succeeded",
		"run_id", req.RunID,
		"job_id", req.JobID,
		"job_type", req.JobType,
		"duration", duration,
	)
}

// applyGatePhaseOverrides wires optional per-phase overrides into the gate manifest.
//
// Semantics:
//   - pre_gate may use build_gate.pre.stack as a fallback/override.
//   - post_gate may use build_gate.post.stack as a fallback/override.
//   - re_gate must *not* use build_gate.post.stack; it re-runs the gate using the
//     stackdetect output to select the runtime image/tool.
//
// Prep override semantics:
//   - pre_gate may use build_gate.pre.gate_profile command/env override.
//   - post_gate may use build_gate.post.gate_profile command/env override.
//   - re_gate reuses build_gate.post.gate_profile command/env override.
//
// Target/always semantics:
//   - pre_gate uses build_gate.pre.target and build_gate.pre.always.
//   - post_gate uses build_gate.post.target and build_gate.post.always.
//   - re_gate reuses build_gate.post.target and build_gate.post.always.
func applyGatePhaseOverrides(manifest *contracts.StepManifest, req StartRunRequest, typedOpts RunOptions) {
	if manifest == nil || manifest.Gate == nil {
		return
	}
	manifest.Gate.Target = ""
	manifest.Gate.Always = false
	manifest.Gate.Skip = nil
	manifest.Gate.EnforceTargetLock = false

	switch req.JobType {
	case types.JobTypePreGate:
		contracts.ApplyBuildGatePhaseToGateSpec(manifest.Gate, typedOpts.BuildGate.Pre)
		manifest.Gate.Skip = req.GateSkip
	case types.JobTypePostGate:
		contracts.ApplyBuildGatePhaseToGateSpec(manifest.Gate, typedOpts.BuildGate.Post)
		manifest.Gate.Skip = req.GateSkip
	case types.JobTypeReGate:
		contracts.ApplyBuildGatePhaseToGateSpec(manifest.Gate, typedOpts.BuildGate.Post)
		manifest.Gate.StackDetect = nil // re-gate uses persisted stack from original gate run
		manifest.Gate.Skip = req.GateSkip
		if req.RecoveryContext != nil {
			if kind, ok := contracts.ParseRecoveryErrorKind(req.RecoveryContext.SelectedErrorKind); ok &&
				contracts.IsInfraRecoveryErrorKind(kind) &&
				strings.TrimSpace(manifest.Gate.Target) != "" {
				manifest.Gate.EnforceTargetLock = true
			}
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

func (r *runController) persistGateProfileSnapshot(
	runID types.RunID,
	jobType types.JobType,
	gateSpec *contracts.StepGateSpec,
	meta *contracts.BuildGateStageMetadata,
) {
	baseRoot := os.Getenv("PLOYD_CACHE_HOME")
	if baseRoot == "" {
		baseRoot = os.TempDir()
	}
	runDir := filepath.Join(baseRoot, "ploy", "run", runID.String())
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		slog.Warn("failed to create run dir for gate profile snapshot", "run_id", runID, "error", err)
		return
	}
	profilePath := filepath.Join(runDir, "build-gate-profile.json")

	raw := resolveGateProfileSnapshotRaw(jobType, gateSpec, meta)
	if len(raw) == 0 {
		if err := os.Remove(profilePath); err != nil && !os.IsNotExist(err) {
			slog.Warn("failed to remove stale gate profile snapshot", "run_id", runID, "path", profilePath, "error", err)
		}
		return
	}

	if err := os.WriteFile(profilePath, raw, 0o644); err != nil {
		slog.Warn("failed to persist gate profile snapshot", "run_id", runID, "path", profilePath, "error", err)
		return
	}
	slog.Info("persisted gate profile snapshot", "run_id", runID, "path", profilePath)
}

func resolveGateProfileSnapshotRaw(
	jobType types.JobType,
	gateSpec *contracts.StepGateSpec,
	meta *contracts.BuildGateStageMetadata,
) json.RawMessage {
	if gateSpec == nil || gateSpec.GateProfile == nil || gateSpec.RepoID.IsZero() {
		return nil
	}
	raw, err := gateprofile.DeriveProfileSnapshotFromOverride(
		gateSpec.RepoID.String(),
		gateSpec.GateProfile,
		strings.TrimSpace(gateSpec.Target),
		jobType,
		meta,
	)
	if err != nil {
		return nil
	}
	return raw
}

func (r *runController) cleanupGateOutDir(workspace string) {
	outDir := filepath.Join(workspace, step.BuildGateWorkspaceOutDir)
	if err := os.RemoveAll(outDir); err != nil && !os.IsNotExist(err) {
		slog.Warn("failed to remove gate out dir", "path", outDir, "error", err)
	}
}

type gateReportUploadSpec struct {
	reportType   string
	reportPath   string
	relativePath string
	artifactName string
}

func (r *runController) uploadGateReportArtifacts(
	ctx context.Context,
	runID types.RunID,
	jobID types.JobID,
	workspace string,
	gateResult *contracts.BuildGateStageMetadata,
) {
	if r.artifactUploader == nil || gateResult == nil {
		return
	}
	if !isGradleGateResult(gateResult) {
		return
	}

	gateOutDir := filepath.Join(workspace, step.BuildGateWorkspaceOutDir)
	reportSpecs := []gateReportUploadSpec{
		{
			reportType:   contracts.BuildGateReportTypeGradleJUnitXML,
			reportPath:   gateGradleJUnitXMLReportPath,
			relativePath: strings.TrimPrefix(gateGradleJUnitXMLReportPath, "/out/"),
			artifactName: "build-gate-gradle-junit-xml",
		},
		{
			reportType:   contracts.BuildGateReportTypeGradleHTML,
			reportPath:   gateGradleHTMLReportPath,
			relativePath: strings.TrimPrefix(gateGradleHTMLReportPath, "/out/"),
			artifactName: "build-gate-gradle-html-report",
		},
	}

	for _, spec := range reportSpecs {
		sourcePath := filepath.Join(gateOutDir, spec.relativePath)
		if _, err := os.Stat(sourcePath); err != nil {
			continue
		}

		hasFiles, _ := listFilesRecursive(sourcePath)
		if !hasFiles {
			continue
		}

		archivePath := filepath.ToSlash(filepath.Join("out", spec.relativePath))
		artifactID, bundleCID, err := r.artifactUploader.UploadArtifactEntries(
			ctx,
			runID,
			jobID,
			[]ArtifactBundleEntry{{
				SourcePath:  sourcePath,
				ArchivePath: archivePath,
			}},
			spec.artifactName,
		)
		if err != nil {
			slog.Warn(
				"failed to upload gate report artifact",
				"run_id", runID,
				"job_id", jobID,
				"path", spec.reportPath,
				"error", err,
			)
			continue
		}

		artifactURL := gateArtifactURL(r.cfg.ServerURL, artifactID, false)
		downloadURL := gateArtifactURL(r.cfg.ServerURL, artifactID, true)
		gateResult.ReportLinks = append(gateResult.ReportLinks, contracts.BuildGateReportLink{
			Type:        spec.reportType,
			Path:        spec.reportPath,
			ArtifactID:  artifactID,
			BundleCID:   bundleCID,
			URL:         artifactURL,
			DownloadURL: downloadURL,
		})
	}
}

func isGradleGateResult(meta *contracts.BuildGateStageMetadata) bool {
	if meta == nil {
		return false
	}
	if meta.Detected != nil && strings.EqualFold(strings.TrimSpace(meta.Detected.Tool), "gradle") {
		return true
	}
	if len(meta.StaticChecks) == 0 {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(meta.StaticChecks[0].Tool), "gradle")
}

func gateArtifactURL(serverURL, artifactID string, download bool) string {
	trimmedID := strings.TrimSpace(artifactID)
	if trimmedID == "" {
		return ""
	}
	artifactPath := "/v1/artifacts/" + trimmedID
	if download {
		artifactPath += "?download=true"
	}
	u, err := BuildURL(serverURL, artifactPath)
	if err != nil {
		return artifactPath
	}
	return u
}

// buildGateJobStats constructs stats payload for gate job completion.
// Uses typed builder to eliminate map[string]any construction.
func (r *runController) buildGateJobStats(gateResult *contracts.BuildGateStageMetadata, duration time.Duration) types.RunStats {
	builder := types.NewRunStatsBuilder().
		DurationMs(duration.Milliseconds())

	if gateResult != nil {
		// Use Gate helper for simple gate stats.
		builder.Gate(gateResultPassed(gateResult), duration.Milliseconds())
		if resources := runStatsJobResourcesFromGateUsage(gateResult.Resources); resources != nil {
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
