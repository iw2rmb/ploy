// execution_orchestrator_jobs.go contains mig and healing job implementations,
// the shared standard job executor, and workspace lifecycle helpers.
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
	gitpkg "github.com/iw2rmb/ploy/internal/nodeagent/git"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/step"
)

// executeModJob runs a mig container job.
// Executes the container, uploads diff, and reports status.
//
// Stack-aware image selection: The job loads the persisted stack from the
// pre-gate phase and uses it for manifest building. This ensures mig steps
// use stack-specific images (e.g., java-maven, java-gradle) when configured.
func (r *runController) executeModJob(ctx context.Context, req StartRunRequest) {
	startTime := time.Now()

	// Load the persisted stack from the pre-gate phase for stack-aware image
	// selection. If no stack was persisted (e.g., gate skipped), defaults to
	// ModStackUnknown which falls back to "default" in stack maps.
	stack := r.loadPersistedStack(req.RunID)

	// Build manifest with stack-aware image resolution using typed options.
	typedOpts := req.TypedOptions
	stepIdx := 0
	if len(typedOpts.Steps) > 0 {
		idx, err := modStepIndexFromJobName(req.JobName, len(typedOpts.Steps))
		if err != nil {
			err = fmt.Errorf("derive mig step index from job_name: %w", err)
			slog.Error("failed to derive mig step index", "run_id", req.RunID, "job_id", req.JobID, "job_name", req.JobName, "error", err)
			r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
			return
		}
		if idx < 0 || idx >= len(typedOpts.Steps) {
			err := fmt.Errorf("derived mig step index out of range: job_name=%q derived=%d steps_len=%d", req.JobName, idx, len(typedOpts.Steps))
			slog.Error("derived mig step index out of range", "run_id", req.RunID, "job_id", req.JobID, "job_name", req.JobName, "derived_index", idx, "steps_len", len(typedOpts.Steps))
			r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
			return
		}
		stepIdx = idx
	}
	manifest, err := buildManifestFromRequest(req, typedOpts, stepIdx, stack)
	if err != nil {
		slog.Error("failed to build manifest", "run_id", req.RunID, "error", err)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
	}

	// Log the stack-aware image selection for observability.
	slog.Info("mig job using stack-aware image",
		"run_id", req.RunID,
		"job_id", req.JobID,
		"detected_stack", stack,
		"resolved_image", manifest.Image,
	)

	cfg := standardJobConfig{
		Manifest:                  manifest,
		DiffType:                  DiffJobTypeMod,
		OutDirPattern:             "ploy-mig-out-*",
		UploadConfiguredArtifacts: true,
		UploadDiff:                r.uploadModDiffWithBaseline,
		StartTime:                 startTime,
	}

	r.executeStandardJob(ctx, req, cfg)
}

// executeHealingJob runs a healing container job.
// Fetches gate logs from parent job, runs healing container, uploads diff.
//
// Stack-aware image selection: The job loads the persisted stack from the
// pre-gate phase and uses it for manifest building. This ensures healing
// migs use stack-specific images (e.g., java-maven, java-gradle) when configured.
func (r *runController) executeHealingJob(ctx context.Context, req StartRunRequest) {
	startTime := time.Now()

	// Load the persisted stack from the pre-gate phase for stack-aware image
	// selection. If no stack was persisted (e.g., gate skipped), defaults to
	// ModStackUnknown which falls back to "default" in stack maps.
	stack := r.loadPersistedStack(req.RunID)

	// Build manifest with stack-aware image resolution using typed options.
	// stepIndex=0 is used for manifest building; job configuration comes from req.TypedOptions.
	typedOpts := req.TypedOptions

	var manifest contracts.StepManifest
	var err error

	if typedOpts.Healing == nil || typedOpts.Healing.Mod.Image.IsEmpty() {
		err = fmt.Errorf("healing job missing selected strategy image")
	} else {
		healMod := typedOpts.Healing.Mod
		manifest, err = buildHealingManifest(req, healMod, 0, "", stack)
	}
	if err != nil {
		slog.Error("failed to build manifest", "run_id", req.RunID, "error", err)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
	}

	// Log the stack-aware image selection for observability.
	slog.Info("healing job using stack-aware image",
		"run_id", req.RunID,
		"job_id", req.JobID,
		"detected_stack", stack,
		"resolved_image", manifest.Image,
	)

	cfg := standardJobConfig{
		Manifest:      manifest,
		DiffType:      DiffJobTypeHealing,
		OutDirPattern: "ploy-heal-out-*",
		InDirPattern:  "ploy-heal-in-*",
		PopulateInDir: func(inDir string) error {
			return r.populateHealingInDir(req.RunID, inDir)
		},
		InjectEnv: func(m *contracts.StepManifest, ws string) {
			r.injectHealingEnvVars(m, ws)
		},
		MountCerts: func(m *contracts.StepManifest) {
			r.mountHealingTLSCerts(m)
		},
		CheckWorkspaceNoChange: true,
		UploadDiff:             r.uploadHealingJobDiff,
		BuildJobMeta: func(outDir string) json.RawMessage {
			actionSummary := parseActionSummary(outDir)
			if actionSummary == "" {
				return nil
			}
			meta := &contracts.JobMeta{
				Kind:          contracts.JobKindMod,
				ActionSummary: actionSummary,
			}
			data, err := contracts.MarshalJobMeta(meta)
			if err != nil {
				slog.Warn("failed to marshal healing job meta", "run_id", req.RunID, "job_id", req.JobID, "error", err)
				return nil
			}
			return data
		},
		StartTime: startTime,
	}

	r.executeStandardJob(ctx, req, cfg)
}

// uploadHealingNoWorkspaceChangesFailure uploads a terminal failure status when a healing job
// exits 0 but produces no workspace changes.
func (r *runController) uploadHealingNoWorkspaceChangesFailure(ctx context.Context, req StartRunRequest, baseStats types.RunStats, duration time.Duration) {
	// This is considered a failure: the healing mig promised to fix the issue but
	// didn't actually change anything. Upload a failed status with exit code 1 and
	// a stable stats marker so downstream observers can distinguish this from other
	// failure modes.
	//
	// Since RunStats is now json.RawMessage-backed, we build a new stats object
	// with the healing_warning field included.
	stats := types.NewRunStatsBuilder().
		ExitCode(1).
		DurationMs(duration.Milliseconds()).
		HealingWarning("no_workspace_changes").
		MustBuild()

	// v1 uses capitalized job status values: Success, Fail, Cancelled.
	var exitCodeOne int32 = 1
	if uploadErr := r.uploadStatus(ctx, req.RunID.String(), JobStatusFail.String(), &exitCodeOne, stats, req.JobID); uploadErr != nil {
		slog.Error("failed to upload healing failure status (no workspace changes)", "run_id", req.RunID, "job_id", req.JobID, "error", uploadErr)
	}
	slog.Info("healing job failed (no workspace changes)", "run_id", req.RunID, "job_id", req.JobID, "exit_code", 1, "duration", duration)
}

// populateHealingInDir copies the first failing gate log (when present) into
// the healing job's /in directory as build-gate.log for discrete healing jobs.
func (r *runController) populateHealingInDir(runID types.RunID, inDir string) error {
	if strings.TrimSpace(inDir) == "" {
		return nil
	}

	baseRoot := os.Getenv("PLOYD_CACHE_HOME")
	if baseRoot == "" {
		baseRoot = os.TempDir()
	}
	runDir := filepath.Join(baseRoot, "ploy", "run", runID.String())
	srcPath := filepath.Join(runDir, "build-gate-first.log")

	data, err := os.ReadFile(srcPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read first gate log: %w", err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil
	}

	destPath := filepath.Join(inDir, "build-gate.log")
	if err := os.WriteFile(destPath, data, 0o644); err != nil {
		return fmt.Errorf("write /in/build-gate.log: %w", err)
	}

	slog.Info("hydrated /in/build-gate.log for healing job", "run_id", runID, "path", destPath)
	return nil
}

// standardJobConfig configures the execution of a standard container job (mig/heal).
type standardJobConfig struct {
	Manifest      contracts.StepManifest
	DiffType      DiffJobType
	OutDirPattern string
	InDirPattern  string

	PopulateInDir func(inDir string) error
	InjectEnv     func(manifest *contracts.StepManifest, workspace string)
	MountCerts    func(manifest *contracts.StepManifest)

	CheckWorkspaceNoChange    bool
	UploadConfiguredArtifacts bool

	UploadDiff   func(ctx context.Context, runID types.RunID, jobID types.JobID, jobName string, diffGen step.DiffGenerator, baselineDir, workspace string, result step.Result)
	BuildJobMeta func(outDir string) json.RawMessage

	StartTime time.Time
}

// executeStandardJob orchestrates the common lifecycle of a container job (mig/heal):
// runtime init, rehydration, snapshots, directory prep, execution, and uploading.
func (r *runController) executeStandardJob(ctx context.Context, req StartRunRequest, cfg standardJobConfig) {
	startTime := cfg.StartTime
	if startTime.IsZero() {
		startTime = time.Now()
	}

	execCtx, cleanup, err := r.initJobExecutionContext(ctx, req.RunID, req.JobID)
	if err != nil {
		slog.Error("failed to initialize runtime", "run_id", req.RunID, "error", err)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
	}
	defer cleanup()

	wsResult, err := r.rehydrateWorkspaceWithCleanup(ctx, req, cfg.Manifest)
	if err != nil {
		slog.Error("failed to rehydrate workspace", "run_id", req.RunID, "error", err)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
	}
	defer wsResult.cleanup()
	workspace := wsResult.workspace

	var baselineDir string
	if execCtx.diffGenerator != nil {
		snapshot := snapshotWorkspaceForNoIndexDiff(req.RunID, req.JobID, cfg.DiffType, workspace)
		defer snapshot.cleanup()
		baselineDir = snapshot.dir
	}

	executeBody := func(outDir, inDir string) error {
		manifest := cfg.Manifest
		disableManifestGate(&manifest)
		clearManifestHydration(&manifest)

		if cfg.InjectEnv != nil {
			cfg.InjectEnv(&manifest, workspace)
		}
		if cfg.MountCerts != nil {
			cfg.MountCerts(&manifest)
		}

		imageName := strings.TrimSpace(manifest.Image)
		if imageName == "" {
			return fmt.Errorf("resolved job image is empty")
		}
		if err := r.SaveJobImageName(ctx, req.JobID, imageName); err != nil {
			return fmt.Errorf("save job image name: %w", err)
		}

		var preStatus string
		var preStatusErr error
		if cfg.CheckWorkspaceNoChange {
			preStatus, preStatusErr = workspaceStatus(ctx, workspace)
			if preStatusErr != nil {
				slog.Warn("failed to compute workspace status before execution", "run_id", req.RunID, "error", preStatusErr)
			}
		}

		result, runErr := execCtx.runner.Run(ctx, step.Request{
			RunID:     req.RunID,
			JobID:     req.JobID,
			Manifest:  manifest,
			Workspace: workspace,
			OutDir:    outDir,
			InDir:     inDir,
		})
		duration := time.Since(startTime)

		if cfg.UploadDiff != nil {
			cfg.UploadDiff(ctx, req.RunID, req.JobID, req.JobName, execCtx.diffGenerator, baselineDir, workspace, result)
		}

		if err := r.uploadOutDir(ctx, req.RunID, req.JobID, outDir); err != nil {
			slog.Warn("/out artifact upload failed", "run_id", req.RunID, "job_id", req.JobID, "next_id", req.NextID, "error", err)
		}

		if cfg.UploadConfiguredArtifacts {
			r.uploadConfiguredArtifacts(ctx, req, req.TypedOptions, manifest, workspace, outDir)
		}

		if cfg.CheckWorkspaceNoChange && runErr == nil && result.ExitCode == 0 && preStatusErr == nil {
			postStatus, postErr := workspaceStatus(ctx, workspace)
			if postErr == nil && postStatus == preStatus {
				r.uploadHealingNoWorkspaceChangesFailure(ctx, req, types.NewRunStatsBuilder().ExitCode(1).DurationMs(duration.Milliseconds()).HealingWarning("no_workspace_changes").MustBuild(), duration)
				return nil
			}
		}

		statsBuilder := types.NewRunStatsBuilder().
			ExitCode(result.ExitCode).
			DurationMs(duration.Milliseconds()).
			TimingsFromDurations(
				time.Duration(result.Timings.HydrationDuration).Milliseconds(),
				time.Duration(result.Timings.ExecutionDuration).Milliseconds(),
				time.Duration(result.Timings.DiffDuration).Milliseconds(),
				time.Duration(result.Timings.TotalDuration).Milliseconds(),
			)
		if resources := runStatsJobResourcesFromStepUsage(result.ContainerResources); resources != nil {
			statsBuilder.JobResources(resources)
		}

		if cfg.BuildJobMeta != nil {
			if meta := cfg.BuildJobMeta(outDir); len(meta) > 0 {
				statsBuilder.JobMeta(meta)
			}
		}

		stats := statsBuilder.MustBuild()

		if runErr != nil {
			status := JobStatusFail
			var exitCode *int32
			if errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded) {
				status = JobStatusCancelled
			} else {
				var runtimeExitCode int32 = -1
				exitCode = &runtimeExitCode
			}
			r.emitRunException(
				req,
				"node runtime execution error",
				runErr,
				map[string]any{
					"component":   "run_controller",
					"status":      status.String(),
					"duration_ms": duration.Milliseconds(),
				},
			)
			if uploadErr := r.uploadStatus(ctx, req.RunID.String(), status.String(), exitCode, stats, req.JobID); uploadErr != nil {
				slog.Error("failed to upload terminal status", "run_id", req.RunID, "job_id", req.JobID, "next_id", req.NextID, "error", uploadErr)
			}
			slog.Info("job terminated", "run_id", req.RunID, "job_id", req.JobID, "next_id", req.NextID, "status", status, "duration", duration, "error", runErr)
			return nil
		}

		if result.ExitCode != 0 {
			exitCode := int32(result.ExitCode)
			if uploadErr := r.uploadStatus(ctx, req.RunID.String(), JobStatusFail.String(), &exitCode, stats, req.JobID); uploadErr != nil {
				slog.Error("failed to upload failure status", "run_id", req.RunID, "job_id", req.JobID, "next_id", req.NextID, "error", uploadErr)
			}
			slog.Info("job failed", "run_id", req.RunID, "job_id", req.JobID, "next_id", req.NextID, "exit_code", result.ExitCode, "duration", duration)
			return nil
		}

		var exitCodeZero int32 = 0
		if uploadErr := r.uploadStatus(ctx, req.RunID.String(), JobStatusSuccess.String(), &exitCodeZero, stats, req.JobID); uploadErr != nil {
			slog.Error("failed to upload success status", "run_id", req.RunID, "job_id", req.JobID, "next_id", req.NextID, "error", uploadErr)
		}
		slog.Info("job succeeded", "run_id", req.RunID, "job_id", req.JobID, "next_id", req.NextID, "duration", duration)
		return nil
	}

	outDirErr := withTempDir(cfg.OutDirPattern, func(outDir string) error {
		if cfg.InDirPattern != "" {
			return withTempDir(cfg.InDirPattern, func(inDir string) error {
				if cfg.PopulateInDir != nil {
					if err := cfg.PopulateInDir(inDir); err != nil {
						slog.Warn("failed to populate in dir", "run_id", req.RunID, "job_id", req.JobID, "next_id", req.NextID, "error", err)
					}
				}
				return executeBody(outDir, inDir)
			})
		}
		return executeBody(outDir, "")
	})

	if outDirErr != nil {
		slog.Error("failed to create temp directories", "run_id", req.RunID, "error", outDirErr)
		r.uploadFailureStatus(ctx, req, outDirErr, time.Since(startTime))
	}
}

// withTempDir creates a temporary directory, calls fn, then removes the directory.
func withTempDir(prefix string, fn func(path string) error) error {
	dir, err := os.MkdirTemp("", prefix)
	if err != nil {
		return fmt.Errorf("create temp dir %s: %w", prefix, err)
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			slog.Warn("failed to remove temp dir", "path", dir, "error", err)
		}
	}()

	return fn(dir)
}

// snapshotResult holds a workspace snapshot path and its cleanup function.
type snapshotResult struct {
	dir     string
	cleanup func()
}

// snapshotWorkspaceForNoIndexDiff creates a snapshot of the workspace for baseline comparison.
func snapshotWorkspaceForNoIndexDiff(runID types.RunID, jobID types.JobID, diffType DiffJobType, workspace string) snapshotResult {
	jobTypeStr := diffType.String()
	prefix := fmt.Sprintf("ploy-%s-base-*", jobTypeStr)
	snapshotDir, err := os.MkdirTemp("", prefix)
	if err != nil {
		slog.Warn(fmt.Sprintf("%s: failed to create baseline snapshot directory", jobTypeStr),
			"run_id", runID, "job_id", jobID, "error", err)
		return snapshotResult{dir: "", cleanup: func() {}}
	}

	if err := copyGitClone(workspace, snapshotDir); err != nil {
		slog.Warn(fmt.Sprintf("%s: failed to snapshot baseline workspace", jobTypeStr),
			"run_id", runID, "job_id", jobID, "error", err)
		if rmErr := os.RemoveAll(snapshotDir); rmErr != nil {
			slog.Warn("failed to remove snapshot dir after copy failure", "path", snapshotDir, "error", rmErr)
		}
		return snapshotResult{dir: "", cleanup: func() {}}
	}

	return snapshotResult{
		dir: snapshotDir,
		cleanup: func() {
			if err := os.RemoveAll(snapshotDir); err != nil {
				slog.Warn("failed to remove snapshot dir", "path", snapshotDir, "error", err)
			}
		},
	}
}

// workspaceRehydrationResult holds a rehydrated workspace path and its cleanup function.
type workspaceRehydrationResult struct {
	workspace string
	cleanup   func()
}

// rehydrateWorkspaceWithCleanup wraps rehydrateWorkspaceForStep with automatic cleanup.
func (r *runController) rehydrateWorkspaceWithCleanup(
	ctx context.Context,
	req StartRunRequest,
	manifest contracts.StepManifest,
) (workspaceRehydrationResult, error) {
	workspace, err := r.rehydrateWorkspaceForStep(ctx, req, manifest)
	if err != nil {
		return workspaceRehydrationResult{}, err
	}

	return workspaceRehydrationResult{
		workspace: workspace,
		cleanup: func() {
			if err := os.RemoveAll(workspace); err != nil {
				slog.Warn("failed to remove workspace", "path", workspace, "error", err)
			}
		},
	}, nil
}

// clearManifestHydration removes hydration config from manifest inputs to prevent double-hydration.
func clearManifestHydration(manifest *contracts.StepManifest) {
	if len(manifest.Inputs) == 0 {
		return
	}
	inputs := make([]contracts.StepInput, len(manifest.Inputs))
	copy(inputs, manifest.Inputs)
	for i := range inputs {
		inputs[i].Hydration = nil
	}
	manifest.Inputs = inputs
}

// disableManifestGate sets Gate.Enabled=false on the manifest.
func disableManifestGate(manifest *contracts.StepManifest) {
	manifest.Gate = &contracts.StepGateSpec{Enabled: false}
}

// uploadConfiguredArtifacts uploads artifact bundles specified in the typed RunOptions.
// Relative paths resolve in workspace; /out/* paths resolve from outDir and keep
// deterministic archive names under out/.
func (r *runController) uploadConfiguredArtifacts(ctx context.Context, req StartRunRequest, typedOpts RunOptions, manifest contracts.StepManifest, workspace, outDir string) {
	if len(typedOpts.Artifacts.Paths) == 0 {
		return
	}

	entries := make([]ArtifactBundleEntry, 0, len(typedOpts.Artifacts.Paths))
	for _, p := range typedOpts.Artifacts.Paths {
		fullPath, archivePath, ok := resolveConfiguredArtifactPath(p, workspace, outDir)
		if !ok {
			slog.Warn("artifact path rejected",
				"run_id", req.RunID, "job_id", req.JobID, "path", p,
			)
			continue
		}

		if _, err := os.Stat(fullPath); err == nil {
			entries = append(entries, ArtifactBundleEntry{
				SourcePath:  fullPath,
				ArchivePath: archivePath,
			})
		} else {
			slog.Warn("artifact path not found", "run_id", req.RunID, "path", p)
		}
	}

	if len(entries) == 0 {
		return
	}

	if err := r.ensureUploaders(); err != nil {
		slog.Error("failed to initialize uploaders", "run_id", req.RunID, "error", err)
		return
	}

	if _, _, err := r.artifactUploader.UploadArtifactEntries(ctx, req.RunID, req.JobID, entries, typedOpts.Artifacts.Name); err != nil {
		slog.Error("failed to upload artifact bundle", "run_id", req.RunID, "job_id", req.JobID, "error", err)
	} else {
		slog.Info("artifact bundle uploaded successfully", "run_id", req.RunID, "job_id", req.JobID, "paths", len(entries))
	}
}

func resolveConfiguredArtifactPath(rawPath, workspace, outDir string) (fullPath string, archivePath string, ok bool) {
	p := strings.TrimSpace(rawPath)
	if p == "" {
		return "", "", false
	}

	if strings.HasPrefix(p, "/out/") {
		if strings.TrimSpace(outDir) == "" {
			return "", "", false
		}
		rel := strings.TrimPrefix(p, "/out/")
		cleanRel := filepath.Clean(rel)
		if cleanRel == "." || strings.HasPrefix(cleanRel, "..") || filepath.IsAbs(cleanRel) {
			return "", "", false
		}
		return filepath.Join(outDir, cleanRel), filepath.ToSlash(filepath.Join("out", cleanRel)), true
	}

	if !isValidArtifactPath(p, workspace) {
		return "", "", false
	}

	return filepath.Clean(filepath.Join(workspace, p)), "", true
}

// uploadOutDir bundles and uploads the /out directory when it contains files.
func (r *runController) uploadOutDir(ctx context.Context, runID types.RunID, jobID types.JobID, outDir string) error {
	if outDir == "" {
		return nil
	}

	hasFiles, _ := listFilesRecursive(outDir)
	if !hasFiles {
		return nil
	}

	if err := r.ensureUploaders(); err != nil {
		return fmt.Errorf("initialize uploaders: %w", err)
	}

	entries := []ArtifactBundleEntry{{
		SourcePath:  outDir,
		ArchivePath: "out",
	}}
	if _, _, err := r.artifactUploader.UploadArtifactEntries(ctx, runID, jobID, entries, "mig-out"); err != nil {
		return fmt.Errorf("upload /out bundle: %w", err)
	}

	return nil
}

// uploadStatus uploads terminal status and execution statistics to the control plane.
// Uses a detached context to ensure reporting even if the run context is cancelled.
func (r *runController) uploadStatus(ctx context.Context, runID, status string, exitCode *int32, stats types.RunStats, jobID types.JobID) error {
	if err := r.ensureUploaders(); err != nil {
		return fmt.Errorf("initialize uploaders: %w", err)
	}

	var loggedExitCode any
	if exitCode != nil {
		loggedExitCode = *exitCode
	}

	statusCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if uploadErr := r.statusUploader.UploadJobStatus(statusCtx, jobID, status, exitCode, stats); uploadErr != nil {
		return fmt.Errorf("upload job status: %w", uploadErr)
	}

	slog.Info("terminal status uploaded successfully", "run_id", runID, "job_id", jobID, "status", status, "exit_code", loggedExitCode)
	return nil
}

// uploadGateLogsArtifact uploads build gate logs as an artifact bundle and attaches
// artifact IDs to the gate stats payload.
func (r *runController) uploadGateLogsArtifact(runID types.RunID, jobID types.JobID, logsText, artifactNameSuffix string, phase *types.RunStatsGatePhase) {
	if phase == nil {
		return
	}

	if err := r.ensureUploaders(); err != nil {
		slog.Warn("failed to initialize uploaders, skipping gate logs upload", "run_id", runID, "job_id", jobID, "error", err)
		return
	}

	logFile, err := os.CreateTemp("", "ploy-gate-*.log")
	if err != nil {
		return
	}
	defer func() { _ = os.Remove(logFile.Name()) }()

	_, _ = logFile.WriteString(logsText)
	_ = logFile.Close()

	artifactName := "build-gate.log"
	if artifactNameSuffix != "" {
		artifactName = "build-gate-" + artifactNameSuffix + ".log"
	}

	if id, cid, uerr := r.artifactUploader.UploadArtifact(context.Background(), runID, jobID, []string{logFile.Name()}, artifactName); uerr == nil {
		phase.LogsArtifactID = id
		phase.LogsBundleCID = cid
	} else {
		slog.Warn("failed to upload "+artifactName, "run_id", runID, "job_id", jobID, "error", uerr)
	}
}

func workspaceStatus(ctx context.Context, workspace string) (string, error) {
	return gitpkg.WorkspaceStatus(ctx, workspace)
}

// uploadHealingJobDiff generates and uploads a diff for a discrete healing job.
func (r *runController) uploadHealingJobDiff(
	ctx context.Context,
	runID types.RunID,
	jobID types.JobID,
	jobName string,
	diffGenerator step.DiffGenerator,
	baseDir string,
	workspace string,
	result step.Result,
) {
	if diffGenerator == nil {
		return
	}
	if strings.TrimSpace(baseDir) == "" {
		return
	}

	diffBytes, err := diffGenerator.GenerateBetween(ctx, baseDir, workspace)
	if err != nil {
		slog.Error("failed to generate healing job diff", "run_id", runID, "job_id", jobID, "error", err)
		return
	}

	if len(diffBytes) == 0 {
		slog.Info("no diff to upload for healing job (no changes between baseline and workspace)", "run_id", runID, "job_id", jobID)
		return
	}

	summary := types.NewDiffSummaryBuilder().
		JobType(DiffJobTypeMod.String()).
		ExitCode(result.ExitCode).
		Timings(
			time.Duration(result.Timings.HydrationDuration).Milliseconds(),
			time.Duration(result.Timings.ExecutionDuration).Milliseconds(),
			time.Duration(result.Timings.DiffDuration).Milliseconds(),
			time.Duration(result.Timings.TotalDuration).Milliseconds(),
		).
		MustBuild()

	if err := r.ensureUploaders(); err != nil {
		slog.Error("failed to initialize uploaders", "run_id", runID, "job_id", jobID, "error", err)
		return
	}

	if err := r.diffUploader.UploadDiff(ctx, runID, jobID, diffBytes, summary); err != nil {
		slog.Error("failed to upload healing job diff", "run_id", runID, "job_id", jobID, "error", err)
		return
	}

	slog.Info("healing job diff uploaded successfully", "run_id", runID, "job_id", jobID, "size", len(diffBytes))
}

// isValidArtifactPath validates that an artifact path is safe for upload.
// Prevents path traversal by ensuring the path is relative and contained within workspace.
func isValidArtifactPath(artifactPath string, workspace string) bool {
	if artifactPath == "" || strings.TrimSpace(artifactPath) == "" {
		return false
	}
	if filepath.IsAbs(artifactPath) {
		return false
	}

	fullPath := filepath.Clean(filepath.Join(workspace, artifactPath))
	rel, err := filepath.Rel(workspace, fullPath)
	if err != nil {
		return false
	}

	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}

	return true
}
