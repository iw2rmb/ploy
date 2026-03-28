// execution_orchestrator_jobs.go contains mig and healing job implementations,
// the shared standard job executor, and workspace lifecycle helpers.
package nodeagent

import (
	"context"
	"encoding/json"
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

	if req.StepSkip != nil {
		if err := req.StepSkip.Validate(); err != nil {
			err = fmt.Errorf("invalid step_skip metadata: %w", err)
			slog.Error("failed to apply step skip", "run_id", req.RunID, "job_id", req.JobID, "error", err)
			r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
			return
		}

		duration := time.Since(startTime)
		statsBuilder := types.NewRunStatsBuilder().
			ExitCode(0).
			DurationMs(duration.Milliseconds()).
			MetadataEntry("step_skip", "true").
			MetadataEntry("step_skip_ref_job_id", req.StepSkip.RefJobID.String())
		if strings.TrimSpace(req.StepSkip.Hash) != "" {
			statsBuilder.MetadataEntry("step_skip_hash", strings.TrimSpace(req.StepSkip.Hash))
		}
		stats := statsBuilder.MustBuild()

		var exitCodeZero int32 = 0
		if uploadErr := r.uploadStatus(ctx, req.RunID.String(), types.JobStatusSuccess.String(), &exitCodeZero, stats, req.JobID, strings.TrimSpace(req.StepSkip.RefRepoSHAOut)); uploadErr != nil {
			slog.Error("failed to upload step-skip success status", "run_id", req.RunID, "job_id", req.JobID, "error", uploadErr)
		}
		slog.Info("mig job skipped via step cache", "run_id", req.RunID, "job_id", req.JobID, "ref_job_id", req.StepSkip.RefJobID, "repo_sha_out", req.StepSkip.RefRepoSHAOut)
		return
	}

	// Load the persisted stack from the pre-gate phase for stack-aware image
	// selection. If no stack was persisted (e.g., gate skipped), defaults to
	// MigStackUnknown which falls back to "default" in stack maps.
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
		DiffType:                  types.DiffJobTypeMod,
		OutDirPattern:             "ploy-mig-out-*",
		WorkspacePolicy:           workspaceChangePolicyIgnore,
		UploadConfiguredArtifacts: true,
		UploadDiff: func(ctx context.Context, runID types.RunID, jobID types.JobID, jobName string, diffGen step.DiffGenerator, baseDir, workspace string, result step.Result) {
			r.uploadDiffWithBaseline(ctx, runID, jobID, jobName, diffGen, baseDir, workspace, result, types.DiffJobTypeMod, true)
		},
		StartTime:                 startTime,
	}
	if err := configureModAmataInDir(&cfg, typedOpts, stepIdx); err != nil {
		slog.Error("failed to configure /in/amata.yaml for mig job", "run_id", req.RunID, "job_id", req.JobID, "error", err)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
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
	// MigStackUnknown which falls back to "default" in stack maps.
	stack := r.loadPersistedStack(req.RunID)
	if req.RecoveryContext != nil && req.RecoveryContext.DetectedStack != "" {
		stack = req.RecoveryContext.DetectedStack
	}

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
	workspacePolicy := resolveHealingWorkspacePolicy(req.TypedOptions.HealingSelector)
	schemaJSON := ""
	if req.Env != nil {
		schemaJSON = strings.TrimSpace(req.Env[contracts.GateProfileSchemaJSONEnv])
	}

	cfg := standardJobConfig{
		Manifest:      manifest,
		DiffType:      types.DiffJobTypeHealing,
		OutDirPattern: "ploy-heal-out-*",
		InDirPattern:  "ploy-heal-in-*",
		PopulateInDir: func(inDir string) error {
			if err := r.populateHealingInDir(req.RunID, inDir, req.TypedOptions.HealingSelector, req.RecoveryContext, schemaJSON); err != nil {
				return err
			}
			return writeAmataSpecInDir(inDir, typedOpts.Healing.Mod.Amata)
		},
		PrepareManifest: func(m *contracts.StepManifest, ws string) {
			r.injectHealingEnvVars(m, ws)
			r.mountHealingTLSCerts(m)
		},
		WorkspacePolicy: workspacePolicy,
		UploadDiff: func(ctx context.Context, runID types.RunID, jobID types.JobID, jobName string, diffGen step.DiffGenerator, baseDir, workspace string, result step.Result) {
			r.uploadDiffWithBaseline(ctx, runID, jobID, jobName, diffGen, baseDir, workspace, result, types.DiffJobTypeHealing, false)
		},
		BuildJobMeta: func(outDir string) json.RawMessage {
			actionSummary := parseActionSummary(outDir)
			if actionSummary == "" {
				return nil
			}
			meta := &contracts.JobMeta{
				Kind:          contracts.JobKindMig,
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

func configureModAmataInDir(cfg *standardJobConfig, typedOpts RunOptions, stepIdx int) error {
	amata := selectedModAmata(typedOpts, stepIdx)
	if amata == nil || strings.TrimSpace(amata.Spec) == "" {
		return nil
	}
	cfg.InDirPattern = "ploy-mig-in-*"
	cfg.PopulateInDir = func(inDir string) error {
		return writeAmataSpecInDir(inDir, amata)
	}
	return nil
}

func selectedModAmata(typedOpts RunOptions, stepIdx int) *contracts.AmataRunSpec {
	if len(typedOpts.Steps) == 0 {
		return typedOpts.Execution.Amata
	}
	if stepIdx < 0 || stepIdx >= len(typedOpts.Steps) {
		return nil
	}
	return typedOpts.Steps[stepIdx].Amata
}

func writeAmataSpecInDir(inDir string, amata *contracts.AmataRunSpec) error {
	if amata == nil {
		return nil
	}
	specText := strings.TrimSpace(amata.Spec)
	if specText == "" {
		return nil
	}
	amataPath := filepath.Join(inDir, "amata.yaml")
	if err := os.WriteFile(amataPath, []byte(specText), 0o644); err != nil {
		return fmt.Errorf("write /in/amata.yaml: %w", err)
	}
	return nil
}

// standardJobConfig configures the execution of a standard container job (mig/heal).
type standardJobConfig struct {
	Manifest      contracts.StepManifest
	DiffType      types.DiffJobType
	OutDirPattern string
	InDirPattern  string

	PopulateInDir   func(inDir string) error
	PrepareManifest func(manifest *contracts.StepManifest, workspace string)

	WorkspacePolicy           workspaceChangePolicy
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
	workspace := wsResult.path

	var baselineDir string
	if execCtx.diffGenerator != nil {
		snapshot := snapshotWorkspaceForNoIndexDiff(req.RunID, req.JobID, cfg.DiffType, workspace)
		defer snapshot.cleanup()
		baselineDir = snapshot.path
	}

	outDirErr := withTempDir(cfg.OutDirPattern, func(outDir string) error {
		if cfg.InDirPattern != "" {
			return withTempDir(cfg.InDirPattern, func(inDir string) error {
				if cfg.PopulateInDir != nil {
					if err := cfg.PopulateInDir(inDir); err != nil {
						return fmt.Errorf("populate in dir: %w", err)
					}
				}
				return r.runContainerJob(ctx, req, cfg, execCtx, baselineDir, workspace, startTime, outDir, inDir)
			})
		}
		return r.runContainerJob(ctx, req, cfg, execCtx, baselineDir, workspace, startTime, outDir, "")
	})

	if outDirErr != nil {
		slog.Error("failed to create temp directories", "run_id", req.RunID, "error", outDirErr)
		r.uploadFailureStatus(ctx, req, outDirErr, time.Since(startTime))
	}
}

// runContainerJob executes the container, uploads artifacts/diffs, and reports terminal status.
// Extracted from executeStandardJob to keep function sizes under ~100 lines.
func (r *runController) runContainerJob(
	ctx context.Context,
	req StartRunRequest,
	cfg standardJobConfig,
	execCtx jobExecutionContext,
	baselineDir, workspace string,
	startTime time.Time,
	outDir, inDir string,
) error {
	manifest := cfg.Manifest
	disableManifestGate(&manifest)
	clearManifestHydration(&manifest)

	if cfg.PrepareManifest != nil {
		cfg.PrepareManifest(&manifest, workspace)
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
	if cfg.WorkspacePolicy != workspaceChangePolicyIgnore {
		preStatus, preStatusErr = gitpkg.WorkspaceStatus(ctx, workspace)
		if preStatusErr != nil {
			slog.Warn("failed to compute workspace status before execution", "run_id", req.RunID, "error", preStatusErr)
		}
	}
	preWorkspaceTree := ""
	if tree, treeErr := gitpkg.ComputeWorkspaceTreeSHA(ctx, workspace); treeErr != nil {
		slog.Warn("failed to compute pre-execution workspace tree", "run_id", req.RunID, "job_id", req.JobID, "error", treeErr)
	} else {
		preWorkspaceTree = tree
	}

	// Materialize the manifest tmp bundle into a staging directory.
	var result step.Result
	var runErr error
	var duration time.Duration
	if bundleErr := r.withMaterializedTmpBundle(ctx, manifest.TmpBundle, "ploy-tmpfiles-*", func(tmpStagingDir string) error {
		result, runErr = execCtx.runner.Run(ctx, step.Request{
			RunID:         req.RunID,
			JobID:         req.JobID,
			Manifest:      manifest,
			Workspace:     workspace,
			OutDir:        outDir,
			InDir:         inDir,
			TmpStagingDir: tmpStagingDir,
		})
		duration = time.Since(startTime)
		return nil
	}); bundleErr != nil {
		return bundleErr
	}

	if cfg.UploadDiff != nil {
		cfg.UploadDiff(ctx, req.RunID, req.JobID, req.JobName, execCtx.diffGenerator, baselineDir, workspace, result)
	}

	if err := r.uploadOutDirBundle(ctx, req.RunID, req.JobID, outDir, "mig-out"); err != nil {
		slog.Warn("/out artifact upload failed", "run_id", req.RunID, "job_id", req.JobID, "next_id", req.NextID, "error", err)
	}

	if cfg.UploadConfiguredArtifacts {
		r.uploadConfiguredArtifacts(ctx, req, req.TypedOptions, manifest, workspace, outDir)
	}

	if cfg.WorkspacePolicy != workspaceChangePolicyIgnore && runErr == nil && result.ExitCode == 0 && preStatusErr == nil {
		postStatus, postErr := gitpkg.WorkspaceStatus(ctx, workspace)
		if postErr == nil {
			if warning, violated := validateWorkspacePolicy(cfg.WorkspacePolicy, preStatus, postStatus); violated {
				r.uploadHealingWorkspacePolicyFailure(ctx, req, warning, duration)
				return nil
			}
		}
	}

	repoSHAOut := r.computeRepoSHAOut(ctx, req, workspace, preWorkspaceTree)

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
	if orwMeta, orwErr := parseORWFailureMetadata(outDir); orwErr != nil {
		slog.Warn("failed to parse ORW report metadata", "run_id", req.RunID, "job_id", req.JobID, "error", orwErr)
	} else {
		for k, v := range orwMeta {
			statsBuilder.MetadataEntry(k, v)
		}
	}

	stats := statsBuilder.MustBuild()

	r.reportTerminalStatus(ctx, req, runErr, result, stats, repoSHAOut, duration)
	return nil
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

// withMaterializedTmpBundle materializes a TmpBundle into a staging directory
// and passes the staging path to fn. When bundle is nil, fn receives "".
func (r *runController) withMaterializedTmpBundle(ctx context.Context, bundle *contracts.TmpBundleRef, prefix string, fn func(stagingDir string) error) error {
	if bundle == nil {
		return fn("")
	}
	return withTempDir(prefix, func(dir string) error {
		if err := r.materializeTmpBundle(ctx, bundle, dir); err != nil {
			return fmt.Errorf("materialize tmp bundle: %w", err)
		}
		return fn(dir)
	})
}

// tempResource holds a temporary path and its cleanup function.
// Used for workspace snapshots, rehydrated workspaces, and similar lifecycle-scoped directories.
type tempResource struct {
	path    string
	cleanup func()
}

// noopTempResource is a zero-value tempResource with a no-op cleanup.
var noopTempResource = tempResource{path: "", cleanup: func() {}}

// snapshotWorkspaceForNoIndexDiff creates a snapshot of the workspace for baseline comparison.
func snapshotWorkspaceForNoIndexDiff(runID types.RunID, jobID types.JobID, diffType types.DiffJobType, workspace string) tempResource {
	jobTypeStr := diffType.String()
	prefix := fmt.Sprintf("ploy-%s-base-*", jobTypeStr)
	snapshotDir, err := os.MkdirTemp("", prefix)
	if err != nil {
		slog.Warn(fmt.Sprintf("%s: failed to create baseline snapshot directory", jobTypeStr),
			"run_id", runID, "job_id", jobID, "error", err)
		return noopTempResource
	}

	if err := copyGitClone(workspace, snapshotDir); err != nil {
		slog.Warn(fmt.Sprintf("%s: failed to snapshot baseline workspace", jobTypeStr),
			"run_id", runID, "job_id", jobID, "error", err)
		if rmErr := os.RemoveAll(snapshotDir); rmErr != nil {
			slog.Warn("failed to remove snapshot dir after copy failure", "path", snapshotDir, "error", rmErr)
		}
		return noopTempResource
	}

	return tempResource{
		path: snapshotDir,
		cleanup: func() {
			if err := os.RemoveAll(snapshotDir); err != nil {
				slog.Warn("failed to remove snapshot dir", "path", snapshotDir, "error", err)
			}
		},
	}
}

// rehydrateWorkspaceWithCleanup wraps rehydrateWorkspaceForStep with automatic cleanup.
func (r *runController) rehydrateWorkspaceWithCleanup(
	ctx context.Context,
	req StartRunRequest,
	manifest contracts.StepManifest,
) (tempResource, error) {
	workspace, err := r.rehydrateWorkspaceForStep(ctx, req, manifest)
	if err != nil {
		return tempResource{}, err
	}

	return tempResource{
		path: workspace,
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
