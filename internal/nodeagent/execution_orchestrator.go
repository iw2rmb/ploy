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

// executeRun orchestrates job execution based on job type (ModType).
// Dispatches to specialized handlers: gate jobs, mod jobs, or healing jobs.
//
// Job types:
//   - pre_gate, post_gate, re_gate: Run build gate validation
//   - mod: Run container with mod execution
//   - heal: Run healing container after gate failure
//
// Each job is atomic - there's no multi-step loop. The server creates
// individual jobs (pre-gate, mod-0, ..., post-gate) and nodes execute
// them independently. Healing jobs are created by the server when
// gates fail, not run inline by the node.
func (r *runController) executeRun(ctx context.Context, req StartRunRequest) {
	defer func() {
		r.mu.Lock()
		delete(r.jobs, req.JobID.String())
		r.mu.Unlock()
	}()

	slog.Info("starting job execution",
		"run_id", req.RunID,
		"job_id", req.JobID,
		"mod_type", req.ModType,
		"step_index", req.StepIndex,
	)

	// Dispatch based on job type (ModType).
	switch req.ModType {
	case types.ModTypePreGate, types.ModTypePostGate, types.ModTypeReGate:
		r.executeGateJob(ctx, req)
	case types.ModTypeMod:
		r.executeModJob(ctx, req)
	case types.ModTypeHeal:
		r.executeHealingJob(ctx, req)
	case types.ModTypeMR:
		r.executeMRJob(ctx, req)
	default:
		// Fallback for legacy jobs without ModType - execute as mod job.
		slog.Warn("unknown mod_type, falling back to mod execution",
			"run_id", req.RunID,
			"mod_type", req.ModType,
		)
		r.executeModJob(ctx, req)
	}
}

// executeModJob runs a mod container job.
// Executes the container, uploads diff, and reports status.
//
// Stack-aware image selection: The job loads the persisted stack from the
// pre-gate phase and uses it for manifest building. This ensures mod steps
// use stack-specific images (e.g., java-maven, java-gradle) when configured.
func (r *runController) executeModJob(ctx context.Context, req StartRunRequest) {
	startTime := time.Now()

	// Initialize runtime components using shared helper.
	// The cleanup function closes logStreamer on exit.
	execCtx, cleanup, err := r.initJobExecutionContext(ctx, req.RunID.String())
	if err != nil {
		slog.Error("failed to initialize runtime", "run_id", req.RunID, "error", err)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
	}
	defer cleanup()

	// Load the persisted stack from the pre-gate phase for stack-aware image
	// selection. If no stack was persisted (e.g., gate skipped), defaults to
	// ModStackUnknown which falls back to "default" in stack maps.
	stack := r.loadPersistedStack(req.RunID)

	// Build manifest with stack-aware image resolution using typed options.
	// stepIndex is derived from server-injected mod_index for multi-step runs;
	// when absent, defaults to 0 (single-step or legacy behavior).
	typedOpts := req.TypedOptions
	stepIdx := 0
	if len(typedOpts.Steps) > 0 && typedOpts.ModIndexSet {
		// Use typed ModIndex from RunOptions (passed in from claimer_loop).
		if typedOpts.ModIndex >= 0 && typedOpts.ModIndex < len(typedOpts.Steps) {
			stepIdx = typedOpts.ModIndex
		} else {
			slog.Warn("mod_index out of range for steps",
				"run_id", req.RunID, "mod_index", typedOpts.ModIndex, "steps_len", len(typedOpts.Steps))
		}
	}
	manifest, err := buildManifestFromRequest(req, typedOpts, stepIdx, stack)
	if err != nil {
		slog.Error("failed to build manifest", "run_id", req.RunID, "error", err)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
	}

	// Log the stack-aware image selection for observability.
	slog.Info("mod job using stack-aware image",
		"run_id", req.RunID,
		"job_id", req.JobID,
		"detected_stack", stack,
		"resolved_image", manifest.Image,
	)

	// Rehydrate workspace from base + diffs using shared helper.
	wsResult, err := r.rehydrateWorkspaceWithCleanup(ctx, req, manifest, req.StepIndex)
	if err != nil {
		slog.Error("failed to rehydrate workspace", "run_id", req.RunID, "error", err)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
	}
	defer wsResult.cleanup()
	workspace := wsResult.workspace

	// Snapshot the pre-mod workspace so we can generate a diff that includes
	// untracked files (git diff --no-index semantics via GenerateBetween).
	// This snapshot represents the baseline state: repo clone plus all prior
	// diffs for this run and step index.
	var modBaselineDir string
	if execCtx.diffGenerator != nil {
		snapshot := snapshotWorkspaceForNoIndexDiff(req.RunID, req.JobID, "mod", workspace)
		defer snapshot.cleanup()
		modBaselineDir = snapshot.dir
	}

	// Prepare /out directory using shared helper.
	var outDir string
	outDirErr := withTempDir("ploy-mod-out-*", func(dir string) error {
		outDir = dir

		// Disable gate in manifest - mod jobs don't run gates.
		disableManifestGate(&manifest)

		// Clear hydration since workspace is already hydrated.
		clearManifestHydration(&manifest)

		// Run the mod container.
		// Pass RunID directly for consistent labeling and telemetry.
		result, runErr := execCtx.runner.Run(ctx, step.Request{
			RunID:     req.RunID,
			Manifest:  manifest,
			Workspace: workspace,
			OutDir:    outDir,
			InDir:     "",
		})
		duration := time.Since(startTime)

		// Upload diff for this mod using the pre-mod baseline snapshot so untracked
		// files are captured in the patch (repo+diff semantics).
		// E3: Pass job name for branch-local diff tagging (mainline mod jobs have empty branch).
		r.uploadModDiffWithBaseline(ctx, req.RunID, req.JobID, req.JobName, execCtx.diffGenerator, modBaselineDir, workspace, result, req.StepIndex)

		// Upload /out artifacts.
		if err := r.uploadOutDir(ctx, req.RunID, req.JobID, outDir); err != nil {
			slog.Warn("/out artifact upload failed", "run_id", req.RunID, "error", err)
		}

		// Upload configured artifacts using typed RunOptions.
		r.uploadConfiguredArtifacts(ctx, req, typedOpts, manifest, workspace)

		// Build stats using typed builder to eliminate map[string]any construction.
		stats := types.NewRunStatsBuilder().
			ExitCode(result.ExitCode).
			DurationMs(duration.Milliseconds()).
			TimingsFromDurations(
				result.Timings.HydrationDuration.Milliseconds(),
				result.Timings.ExecutionDuration.Milliseconds(),
				result.Timings.DiffDuration.Milliseconds(),
				result.Timings.TotalDuration.Milliseconds(),
			).
			MustBuild()

		// Determine status.
		// v1 uses capitalized job status values: Success, Fail, Cancelled.
		if runErr != nil {
			var exitCode int32 = -1 // Use -1 to indicate runtime error
			if uploadErr := r.uploadStatus(ctx, req.RunID.String(), "Fail", &exitCode, stats, req.StepIndex, req.JobID); uploadErr != nil {
				slog.Error("failed to upload mod failure status", "run_id", req.RunID, "job_id", req.JobID, "error", uploadErr)
			}
			slog.Info("mod job failed", "run_id", req.RunID, "job_id", req.JobID, "error", runErr, "duration", duration)
			return nil
		}

		if result.ExitCode != 0 {
			exitCode := int32(result.ExitCode)
			if uploadErr := r.uploadStatus(ctx, req.RunID.String(), "Fail", &exitCode, stats, req.StepIndex, req.JobID); uploadErr != nil {
				slog.Error("failed to upload mod failure status", "run_id", req.RunID, "job_id", req.JobID, "error", uploadErr)
			}
			slog.Info("mod job failed", "run_id", req.RunID, "job_id", req.JobID, "exit_code", result.ExitCode, "duration", duration)
			return nil
		}

		var exitCodeZero int32 = 0
		if uploadErr := r.uploadStatus(ctx, req.RunID.String(), "Success", &exitCodeZero, stats, req.StepIndex, req.JobID); uploadErr != nil {
			slog.Error("failed to upload mod success status", "run_id", req.RunID, "job_id", req.JobID, "error", uploadErr)
		}
		slog.Info("mod job succeeded", "run_id", req.RunID, "job_id", req.JobID, "exit_code", result.ExitCode, "duration", duration)
		return nil
	})

	if outDirErr != nil {
		slog.Error("failed to create /out directory", "run_id", req.RunID, "error", outDirErr)
		r.uploadFailureStatus(ctx, req, outDirErr, time.Since(startTime))
	}
}

// executeHealingJob runs a healing container job.
// Fetches gate logs from parent job, runs healing container, uploads diff.
//
// Stack-aware image selection: The job loads the persisted stack from the
// pre-gate phase and uses it for manifest building. This ensures healing
// mods use stack-specific images (e.g., java-maven, java-gradle) when configured.
func (r *runController) executeHealingJob(ctx context.Context, req StartRunRequest) {
	startTime := time.Now()

	// Initialize runtime components using shared helper.
	// The cleanup function closes logStreamer on exit.
	execCtx, cleanup, err := r.initJobExecutionContext(ctx, req.RunID.String())
	if err != nil {
		slog.Error("failed to initialize runtime", "run_id", req.RunID, "error", err)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
	}
	defer cleanup()

	// Load the persisted stack from the pre-gate phase for stack-aware image
	// selection. If no stack was persisted (e.g., gate skipped), defaults to
	// ModStackUnknown which falls back to "default" in stack maps.
	stack := r.loadPersistedStack(req.RunID)

	// Build manifest with stack-aware image resolution using typed options.
	// stepIndex=0 is used for manifest building; job configuration comes from req.TypedOptions.
	typedOpts := req.TypedOptions

	var manifest contracts.StepManifest

	// When build_gate_healing is configured, hydrate the healing manifest from the
	// typed HealingConfig so that discrete healing jobs use the correct image/env.
	if typedOpts.Healing != nil {
		healMod := typedOpts.Healing.Mod
		manifest, err = buildHealingManifest(req, healMod, 0, "", stack)
	}
	if manifest.Image == "" {
		manifest, err = buildManifestFromRequest(req, typedOpts, 0, stack)
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

	// Rehydrate workspace from base + diffs using shared helper.
	wsResult, err := r.rehydrateWorkspaceWithCleanup(ctx, req, manifest, req.StepIndex)
	if err != nil {
		slog.Error("failed to rehydrate workspace", "run_id", req.RunID, "error", err)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
	}
	defer wsResult.cleanup()
	workspace := wsResult.workspace

	// Snapshot the pre-healing workspace so we can generate a diff that includes
	// untracked files (git diff --no-index semantics via GenerateBetween). This
	// snapshot represents the baseline state: repo clone plus all prior diffs for
	// this run and step index.
	var healingBaselineDir string
	if execCtx.diffGenerator != nil {
		snapshot := snapshotWorkspaceForNoIndexDiff(req.RunID, req.JobID, "healing", workspace)
		defer snapshot.cleanup()
		healingBaselineDir = snapshot.dir
	}

	// Prepare /out directory using shared helper, and /in directory for healing-specific hydration.
	var outDir, inDir string
	outDirErr := withTempDir("ploy-heal-out-*", func(out string) error {
		outDir = out
		return withTempDir("ploy-heal-in-*", func(in string) error {
			inDir = in

			// Healing-specific: Hydrate /in/build-gate.log from the first failing gate log.
			// This gives healing containers (e.g., Codex) a trimmed failure view without
			// requiring them to re-run the gate themselves.
			if err := r.populateHealingInDir(req.RunID, inDir); err != nil {
				slog.Warn("failed to hydrate /in/build-gate.log for healing job", "run_id", req.RunID, "job_id", req.JobID, "error", err)
			}

			// Disable gate in manifest - healing jobs don't run gates.
			disableManifestGate(&manifest)

			// Clear hydration since workspace is already hydrated.
			clearManifestHydration(&manifest)

			// Healing-specific: Inject healing environment variables for Build Gate API access.
			r.injectHealingEnvVars(&manifest, workspace)

			// Healing-specific: Mount node TLS certificates into healing container.
			r.mountHealingTLSCerts(&manifest)

			slog.Info("starting healing job execution",
				"run_id", req.RunID,
				"job_id", req.JobID,
				"step_index", req.StepIndex,
			)

			// Capture workspace status before running healing so we can detect whether
			// this discrete healing job produced any net changes. This is used for
			// diagnostics and terminal status decisions (e.g., "exit 0 but no
			// workspace changes" is treated as a healing failure). This must not
			// mutate the container's own exit code; it only affects the status we
			// upload to the control plane.
			preStatus, preStatusErr := workspaceStatus(ctx, workspace)
			if preStatusErr != nil {
				slog.Warn("healing: failed to compute workspace status before healing; assuming changes may occur",
					"run_id", req.RunID,
					"job_id", req.JobID,
					"error", preStatusErr,
				)
			}

			// Run the healing container.
			// Pass RunID directly for consistent labeling and telemetry.
			result, runErr := execCtx.runner.Run(ctx, step.Request{
				RunID:     req.RunID,
				Manifest:  manifest,
				Workspace: workspace,
				OutDir:    outDir,
				InDir:     inDir,
			})
			duration := time.Since(startTime)

			// Determine whether this healing job produced any workspace changes.
			// A healing job that exits 0 but produces no diff is treated as a
			// failure: the healing mod promised to fix something but didn't
			// change anything. We set healingNoChange=true here and handle the
			// failed status upload below.
			healingNoChange := false
			if runErr == nil && result.ExitCode == 0 && preStatusErr == nil {
				if postStatus, postErr := workspaceStatus(ctx, workspace); postErr != nil {
					slog.Warn("healing: failed to compute workspace status after healing",
						"run_id", req.RunID,
						"job_id", req.JobID,
						"error", postErr,
					)
				} else if postStatus == preStatus {
					healingNoChange = true
					slog.Warn("healing job produced no workspace changes",
						"run_id", req.RunID,
						"job_id", req.JobID,
					)
				}
			}

			// Upload diff for this healing step using the pre-healing baseline snapshot.
			// E3: Pass job name for path-local diff tagging in multi-strategy healing.
			// We upload diffs even for healing jobs that will fail (due to no workspace
			// changes or non-zero exit code) so diagnostics are preserved.
			r.uploadHealingJobDiff(ctx, req.RunID, req.JobID, req.JobName, execCtx.diffGenerator, healingBaselineDir, workspace, result, req.StepIndex)

			// Upload /out artifacts.
			if err := r.uploadOutDir(ctx, req.RunID, req.JobID, outDir); err != nil {
				slog.Warn("/out artifact upload failed", "run_id", req.RunID, "error", err)
			}

			// Build stats using typed builder to eliminate map[string]any construction.
			stats := types.NewRunStatsBuilder().
				ExitCode(result.ExitCode).
				DurationMs(duration.Milliseconds()).
				MustBuild()

			// Determine status.
			// v1 uses capitalized job status values: Success, Fail, Cancelled.
			if runErr != nil {
				var exitCode int32 = -1 // Use -1 to indicate runtime error
				if uploadErr := r.uploadStatus(ctx, req.RunID.String(), "Fail", &exitCode, stats, req.StepIndex, req.JobID); uploadErr != nil {
					slog.Error("failed to upload healing failure status", "run_id", req.RunID, "job_id", req.JobID, "error", uploadErr)
				}
				slog.Info("healing job failed", "run_id", req.RunID, "job_id", req.JobID, "exit_code", result.ExitCode, "error", runErr, "duration", duration)
				return nil
			}

			if result.ExitCode != 0 {
				exitCode := int32(result.ExitCode)
				if uploadErr := r.uploadStatus(ctx, req.RunID.String(), "Fail", &exitCode, stats, req.StepIndex, req.JobID); uploadErr != nil {
					slog.Error("failed to upload healing failure status", "run_id", req.RunID, "job_id", req.JobID, "error", uploadErr)
				}
				slog.Info("healing job failed", "run_id", req.RunID, "job_id", req.JobID, "exit_code", result.ExitCode, "duration", duration)
				return nil
			}

			if healingNoChange {
				r.uploadHealingNoWorkspaceChangesFailure(ctx, req, stats, duration)
				return nil
			}

			var exitCodeZero int32 = 0
			if uploadErr := r.uploadStatus(ctx, req.RunID.String(), "Success", &exitCodeZero, stats, req.StepIndex, req.JobID); uploadErr != nil {
				slog.Error("failed to upload healing success status", "run_id", req.RunID, "job_id", req.JobID, "error", uploadErr)
			}
			slog.Info("healing job succeeded", "run_id", req.RunID, "job_id", req.JobID, "exit_code", result.ExitCode, "duration", duration)
			return nil
		})
	})

	if outDirErr != nil {
		slog.Error("failed to create temp directories for healing job", "run_id", req.RunID, "error", outDirErr)
		r.uploadFailureStatus(ctx, req, outDirErr, time.Since(startTime))
	}
}

// uploadHealingNoWorkspaceChangesFailure uploads a terminal failure status when a healing job
// exits 0 but produces no workspace changes.
func (r *runController) uploadHealingNoWorkspaceChangesFailure(ctx context.Context, req StartRunRequest, baseStats types.RunStats, duration time.Duration) {
	// This is considered a failure: the healing mod promised to fix the issue but
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
	if uploadErr := r.uploadStatus(ctx, req.RunID.String(), "Fail", &exitCodeOne, stats, req.StepIndex, req.JobID); uploadErr != nil {
		slog.Error("failed to upload healing failure status (no workspace changes)", "run_id", req.RunID, "job_id", req.JobID, "error", uploadErr)
	}
	slog.Info("healing job failed (no workspace changes)", "run_id", req.RunID, "job_id", req.JobID, "exit_code", 1, "duration", duration)
}

// injectHealingEnvVars adds healing-specific environment variables to the manifest.
// These variables provide Build Gate API access configuration to healing containers.
func (r *runController) injectHealingEnvVars(manifest *contracts.StepManifest, workspace string) {
	if manifest.Env == nil {
		manifest.Env = map[string]string{}
	}
	manifest.Env["PLOY_HOST_WORKSPACE"] = workspace
	manifest.Env["PLOY_SERVER_URL"] = r.cfg.ServerURL
	manifest.Env["PLOY_CA_CERT_PATH"] = "/etc/ploy/certs/ca.crt"
	manifest.Env["PLOY_CLIENT_CERT_PATH"] = "/etc/ploy/certs/client.crt"
	manifest.Env["PLOY_CLIENT_KEY_PATH"] = "/etc/ploy/certs/client.key"

	// Inject API token from environment or file fallback.
	if token := os.Getenv("PLOY_API_TOKEN"); token != "" {
		manifest.Env["PLOY_API_TOKEN"] = token
	} else if !r.cfg.HTTP.TLS.Enabled {
		if data, err := os.ReadFile(bearerTokenPath()); err == nil {
			if token := strings.TrimSpace(string(data)); token != "" {
				manifest.Env["PLOY_API_TOKEN"] = token
			}
		} else {
			slog.Warn("healing: failed to read bearer token for PLOY_API_TOKEN fallback", "error", err)
		}
	}
}

// mountHealingTLSCerts configures TLS certificate paths in manifest options.
// This enables healing containers to access the Build Gate API over mTLS.
func (r *runController) mountHealingTLSCerts(manifest *contracts.StepManifest) {
	if manifest.Options == nil {
		manifest.Options = make(map[string]any)
	}
	manifest.Options["ploy_ca_cert_path"] = r.cfg.HTTP.TLS.CAPath
	manifest.Options["ploy_client_cert_path"] = r.cfg.HTTP.TLS.CertPath
	manifest.Options["ploy_client_key_path"] = r.cfg.HTTP.TLS.KeyPath
}

// populateHealingInDir copies the first failing gate log (when present) into
// the healing job's /in directory as build-gate.log. This mirrors the behavior
// of executeWithHealing, which writes a trimmed failure view for Codex healers.
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

// uploadFailureStatus uploads a failure status for early errors.
// Uses exit code -1 to indicate pre-execution infrastructure failures.
// v1 uses capitalized job status values: Success, Fail, Cancelled.
func (r *runController) uploadFailureStatus(ctx context.Context, req StartRunRequest, err error, duration time.Duration) {
	var exitCode int32 = -1 // -1 indicates pre-execution failure
	// Build stats using typed builder to eliminate map[string]any construction.
	stats := types.NewRunStatsBuilder().
		DurationMs(duration.Milliseconds()).
		Error(err.Error()).
		MustBuild()
	if uploadErr := r.uploadStatus(ctx, req.RunID.String(), "Fail", &exitCode, stats, req.StepIndex, req.JobID); uploadErr != nil {
		slog.Error("failed to upload failure status", "run_id", req.RunID, "job_id", req.JobID, "error", uploadErr)
	}
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

	// Initialize gate executor using local Docker-based execution.
	// All gates run via the container runtime.
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
//
//nolint:unused // invoked by future orchestration entrypoints; kept for roadmap parity
func (r *runController) finalizeRun(ctx context.Context, req StartRunRequest, manifest contracts.StepManifest, execResult executionResult, execErr error, workspace string, duration time.Duration) {
	result := execResult.Result

	// Determine terminal status and exit code based on execution result.
	// v1 uses capitalized job status values: Success, Fail, Cancelled.
	terminalStatus := "Success"
	var exitCode int32

	switch {
	case execErr != nil:
		terminalStatus = "Fail"
		// Check if this is a build gate failure.
		if errors.Is(execErr, step.ErrBuildGateFailed) {
			// Exit code 1 signals gate failure for server-side healing detection.
			exitCode = 1
		} else {
			// Exit code -1 for other execution errors.
			exitCode = -1
		}
	case result.ExitCode != 0:
		terminalStatus = "Fail"
		exitCode = int32(result.ExitCode)
	default:
		exitCode = int32(result.ExitCode) // 0 for success
	}

	// Build stats with execution metrics and gate history.
	// Job ID is used to associate gate log artifacts with the current job.
	stats := r.buildExecutionStats(req.RunID, req.JobID, result, execResult, duration, "")

	// Phase 8: Upload terminal status to server.
	// Upload job completion status with job_id, step_index and exit_code.
	if uploadErr := r.uploadStatus(ctx, req.RunID.String(), terminalStatus, &exitCode, stats, req.StepIndex, req.JobID); uploadErr != nil {
		slog.Error("failed to upload terminal status", "run_id", req.RunID, "job_id", req.JobID, "error", uploadErr)
	}
}

// buildExecutionStats constructs the stats payload for terminal status upload.
// Includes execution timings, exit code, gate history (pre-gate, re-gates), and MR URL.
//
//nolint:unused // used by finalizeRun for roadmap-aligned metrics, kept for future wiring
func (r *runController) buildExecutionStats(runID types.RunID, jobID types.JobID, result step.Result, execResult executionResult, duration time.Duration, mrURL string) types.RunStats {
	// Build stats using typed builder to eliminate map[string]any construction.
	builder := types.NewRunStatsBuilder().
		ExitCode(result.ExitCode).
		DurationMs(duration.Milliseconds()).
		TimingsWithGate(
			result.Timings.HydrationDuration.Milliseconds(),
			result.Timings.ExecutionDuration.Milliseconds(),
			result.Timings.BuildGateDuration.Milliseconds(),
			result.Timings.DiffDuration.Milliseconds(),
			result.Timings.TotalDuration.Milliseconds(),
		)

	// Attach MR URL to metadata if created.
	if mrURL != "" {
		builder.MetadataEntry("mr_url", mrURL)
	}

	// Gate stats/logs: collect pass/fail, duration, resources, and upload logs artifact.
	// Include pre-gate and re-gate runs when healing was attempted.
	if execResult.PreGate != nil || len(execResult.ReGates) > 0 || result.BuildGate != nil {
		gate := r.buildGateStats(runID, jobID, result, execResult)
		builder.GateDetails(gate)
	}

	return builder.MustBuild()
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
