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
			migImages := typedOpts.BuildGate.Images

			switch req.JobType {
			case types.JobTypePreGate:
				if step.Stack.Inbound != nil && step.Stack.Inbound.Enabled {
					typedOpts.StackGate = stackGatePhaseSpecToStepGate(step.Stack.Inbound, migImages)
				}
			case types.JobTypePostGate, types.JobTypeReGate:
				if step.Stack.Outbound != nil && step.Stack.Outbound.Enabled {
					typedOpts.StackGate = stackGatePhaseSpecToStepGate(step.Stack.Outbound, migImages)
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
	if err := r.prepareGateJavaClasspathInput(ctx, req, workspace); err != nil {
		slog.Error("failed to prepare gate java classpath input", "run_id", req.RunID, "job_id", req.JobID, "error", err)
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
	gateResult, gateErr := r.runGate(ctx, runner, manifest, workspace)

	// Gate execution errors (e.g., Docker pull/create/start failures) are NOT build failures
	// and must not trigger healing. Treat them as terminal runtime errors for this repo
	// attempt so the control plane cancels remaining jobs without scheduling heal/re-gate.
	if gateErr != nil || gateResult == nil {
		duration := time.Since(startTime)
		r.cleanupGateOutDir(workspace)
		repoSHAOut := r.computeRepoSHAOut(ctx, req, workspace, "")
		errMsg := gateErr
		if errMsg == nil {
			errMsg = errors.New("gate returned nil result with nil error")
		}

		stats := types.NewRunStatsBuilder().
			DurationMs(duration.Milliseconds()).
			Error(fmt.Sprintf("gate execution failed: %s", errMsg.Error())).
			MustBuild()

		if uploadErr := r.uploadStatus(ctx, req.RunID.String(), types.JobStatusError.String(), nil, stats, req.JobID, repoSHAOut); uploadErr != nil {
			slog.Error("failed to upload gate error status after gate execution error",
				"run_id", req.RunID,
				"job_id", req.JobID,
				"error", uploadErr,
			)
		}
		slog.Error("gate execution failed; marking repo attempt as error (no healing)",
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
	if err := r.captureJavaClasspathAfterGateJob(req, workspace); err != nil {
		slog.Error("failed to capture gate java classpath output", "run_id", req.RunID, "job_id", req.JobID, "error", err)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
	}

	if err := r.uploadOutDirBundle(ctx, req.RunID, req.JobID, filepath.Join(workspace, step.BuildGateWorkspaceOutDir), "build-gate-out"); err != nil {
		slog.Warn("failed to upload gate /out bundle", "run_id", req.RunID, "job_id", req.JobID, "error", err)
	}
	r.uploadGateReportArtifacts(ctx, req.RunID, req.JobID, workspace, gateResult)
	r.cleanupGateOutDir(workspace)

	duration := time.Since(startTime)
	repoSHAOut := r.computeRepoSHAOut(ctx, req, workspace, "")

	// Build stats with gate metadata.
	stats := r.buildGateJobStats(gateResult, duration)

	// Determine status and exit code based on gate outcome.
	status := types.JobStatusSuccess
	var exitCode int32 = 0
	logVerb := "succeeded"
	if !gateResultPassed(gateResult) {
		status = types.JobStatusFail
		exitCode = 1
		logVerb = "failed"
	}
	if uploadErr := r.uploadStatus(ctx, req.RunID.String(), status.String(), &exitCode, stats, req.JobID, repoSHAOut); uploadErr != nil {
		slog.Error("failed to upload gate status", "run_id", req.RunID, "job_id", req.JobID, "status", status, "error", uploadErr)
	}
	slog.Info("gate job "+logVerb, "run_id", req.RunID, "job_id", req.JobID, "job_type", req.JobType, "duration", duration)
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
// Target semantics:
//   - pre_gate uses build_gate.pre.target.
//   - post_gate uses build_gate.post.target.
//   - re_gate reuses build_gate.post.target.
func applyGatePhaseOverrides(manifest *contracts.StepManifest, req StartRunRequest, typedOpts RunOptions) {
	if manifest == nil || manifest.Gate == nil {
		return
	}
	manifest.Gate.Target = ""
	manifest.Gate.EnforceTargetLock = false

	switch req.JobType {
	case types.JobTypePreGate:
		contracts.ApplyBuildGatePhaseToGateSpec(manifest.Gate, typedOpts.BuildGate.Pre)
		if typedOpts.BuildGate.Pre != nil {
			manifest.CA = mergeUniqueStringEntries(manifest.CA, typedOpts.BuildGate.Pre.CA)
		}
	case types.JobTypePostGate:
		contracts.ApplyBuildGatePhaseToGateSpec(manifest.Gate, typedOpts.BuildGate.Post)
		if typedOpts.BuildGate.Post != nil {
			manifest.CA = mergeUniqueStringEntries(manifest.CA, typedOpts.BuildGate.Post.CA)
		}
	case types.JobTypeReGate:
		contracts.ApplyBuildGatePhaseToGateSpec(manifest.Gate, typedOpts.BuildGate.Post)
		if typedOpts.BuildGate.Post != nil {
			manifest.CA = mergeUniqueStringEntries(manifest.CA, typedOpts.BuildGate.Post.CA)
		}
		manifest.Gate.StackDetect = nil // re-gate uses persisted stack from original gate run
		if req.RecoveryContext != nil {
			if strings.TrimSpace(req.RecoveryContext.GateProfileSchemaJSON) != "" &&
				strings.TrimSpace(manifest.Gate.Target) != "" {
				manifest.Gate.EnforceTargetLock = true
			}
		}
	}
}

func mergeUniqueStringEntries(base, extra []string) []string {
	if len(extra) == 0 {
		return base
	}
	seen := make(map[string]struct{}, len(base)+len(extra))
	out := make([]string, 0, len(base)+len(extra))
	for _, v := range base {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	for _, v := range extra {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
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
	return runner.Gate.Execute(step.WithExecutionLogWriter(ctx, runner.LogWriter), gateSpec, workspace)
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

// runCacheDir returns the per-run cache directory under PLOYD_CACHE_HOME (or os.TempDir).
func runCacheRootDir() string {
	baseRoot := os.Getenv("PLOYD_CACHE_HOME")
	if baseRoot == "" {
		baseRoot = os.TempDir()
	}
	return filepath.Join(baseRoot, "ploy", "run")
}

// runCacheDir returns the per-run cache directory under PLOYD_CACHE_HOME (or os.TempDir).
func runCacheDir(runID types.RunID) string {
	return filepath.Join(runCacheRootDir(), runID.String())
}

// persistOnce writes data to dir/filename idempotently: if the file already
// exists the call is a no-op, preserving the first write.
func persistOnce(dir, filename string, data []byte, label string, runID types.RunID) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		slog.Warn("failed to create dir for "+label, "run_id", runID, "error", err)
		return
	}
	p := filepath.Join(dir, filename)
	if _, err := os.Stat(p); err == nil {
		return
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
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
	persistOnce(runCacheDir(runID), "build-gate-stack.txt", []byte(string(stack)), "build gate stack", runID)
}

// loadPersistedStack reads the persisted stack for a run.
// Returns MigStackUnknown if no stack file exists or on error.
func (r *runController) loadPersistedStack(runID types.RunID) contracts.MigStack {
	stackPath := filepath.Join(runCacheDir(runID), "build-gate-stack.txt")
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
	persistOnce(runCacheDir(runID), "build-gate-first.log", []byte(logPayload), "first build gate failure log", runID)
}

func (r *runController) persistGateProfileSnapshot(
	runID types.RunID,
	jobType types.JobType,
	gateSpec *contracts.StepGateSpec,
	meta *contracts.BuildGateStageMetadata,
) {
	dir := runCacheDir(runID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		slog.Warn("failed to create run dir for gate profile snapshot", "run_id", runID, "error", err)
		return
	}
	profilePath := filepath.Join(dir, "build-gate-profile.json")

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
