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
	case "pre_gate", "post_gate", "re_gate":
		r.executeGateJob(ctx, req)
	case "mod":
		r.executeModJob(ctx, req)
	case "heal":
		r.executeHealingJob(ctx, req)
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

	// Initialize runtime components.
	runner, diffGenerator, logStreamer, err := r.initializeRuntime(ctx, req.RunID.String())
	if err != nil {
		slog.Error("failed to initialize runtime", "run_id", req.RunID, "error", err)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
	}
	defer func() { _ = logStreamer.Close() }()

	// Load the persisted stack from the pre-gate phase for stack-aware image
	// selection. If no stack was persisted (e.g., gate skipped), defaults to
	// ModStackUnknown which falls back to "default" in stack maps.
	stack := r.loadPersistedStack(req.RunID)

	// Parse options and build manifest with stack-aware image resolution.
	// stepIndex is derived from server-injected mod_index for multi-step runs;
	// when absent, defaults to 0 (single-step or legacy behavior).
	typedOpts := parseRunOptions(req.Options)
	stepIdx := 0
	if len(typedOpts.Steps) > 0 {
		if mi, ok := req.Options["mod_index"].(int); ok && mi >= 0 && mi < len(typedOpts.Steps) {
			stepIdx = mi
		} else if mf, ok := req.Options["mod_index"].(float64); ok {
			mi := int(mf)
			if mi >= 0 && mi < len(typedOpts.Steps) {
				stepIdx = mi
			} else {
				slog.Warn("mod_index out of range for steps",
					"run_id", req.RunID, "mod_index", mi, "steps_len", len(typedOpts.Steps))
			}
		}
	}
	manifest, err := buildManifestFromRequestWithStack(req, typedOpts, stepIdx, stack)
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

	// Rehydrate workspace from base + diffs.
	workspace, err := r.rehydrateWorkspaceForStep(ctx, req, manifest, req.StepIndex)
	if err != nil {
		slog.Error("failed to rehydrate workspace", "run_id", req.RunID, "error", err)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
	}
	defer func() { _ = os.RemoveAll(workspace) }()

	// Prepare /out directory.
	outDir, err := os.MkdirTemp("", "ploy-mod-out-*")
	if err != nil {
		slog.Error("failed to create /out directory", "run_id", req.RunID, "error", err)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
	}
	defer func() { _ = os.RemoveAll(outDir) }()

	// Disable gate in manifest - mod jobs don't run gates.
	manifest.Gate = &contracts.StepGateSpec{Enabled: false}
	manifest.Shift = nil //nolint:staticcheck // backward compatibility: clear deprecated Shift field

	// Clear hydration since workspace is already hydrated.
	if len(manifest.Inputs) > 0 {
		inputs := make([]contracts.StepInput, len(manifest.Inputs))
		copy(inputs, manifest.Inputs)
		for i := range inputs {
			inputs[i].Hydration = nil
		}
		manifest.Inputs = inputs
	}

	// Run the mod container.
	result, runErr := runner.Run(ctx, step.Request{
		TicketID:  types.TicketID(req.RunID),
		Manifest:  manifest,
		Workspace: workspace,
		OutDir:    outDir,
		InDir:     "",
	})
	duration := time.Since(startTime)

	// Upload diff for this mod.
	// E3: Pass job name for branch-local diff tagging (mainline mod jobs have empty branch).
	r.uploadDiffForStep(ctx, req.RunID, req.JobID, req.JobName, diffGenerator, workspace, result, req.StepIndex)

	// Upload /out artifacts.
	if err := r.uploadOutDir(ctx, req.RunID, req.JobID, outDir); err != nil {
		slog.Warn("/out artifact upload failed", "run_id", req.RunID, "error", err)
	}

	// Upload configured artifacts.
	r.uploadConfiguredArtifacts(ctx, req, manifest, workspace)

	// Build stats.
	stats := types.RunStats{
		"exit_code":   result.ExitCode,
		"duration_ms": duration.Milliseconds(),
		"timings": map[string]interface{}{
			"hydration_duration_ms": result.Timings.HydrationDuration.Milliseconds(),
			"execution_duration_ms": result.Timings.ExecutionDuration.Milliseconds(),
			"diff_duration_ms":      result.Timings.DiffDuration.Milliseconds(),
			"total_duration_ms":     result.Timings.TotalDuration.Milliseconds(),
		},
	}

	// Determine status.
	if runErr != nil {
		var exitCode int32 = -1 // Use -1 to indicate runtime error
		if uploadErr := r.uploadStatus(ctx, req.RunID.String(), "failed", &exitCode, stats, req.StepIndex, req.JobID); uploadErr != nil {
			slog.Error("failed to upload mod failure status", "run_id", req.RunID, "job_id", req.JobID, "error", uploadErr)
		}
		slog.Info("mod job failed", "run_id", req.RunID, "job_id", req.JobID, "error", runErr, "duration", duration)
		return
	}

	if result.ExitCode != 0 {
		exitCode := int32(result.ExitCode)
		if uploadErr := r.uploadStatus(ctx, req.RunID.String(), "failed", &exitCode, stats, req.StepIndex, req.JobID); uploadErr != nil {
			slog.Error("failed to upload mod failure status", "run_id", req.RunID, "job_id", req.JobID, "error", uploadErr)
		}
		slog.Info("mod job failed", "run_id", req.RunID, "job_id", req.JobID, "exit_code", result.ExitCode, "duration", duration)
		return
	}

	// Conditionally create MR on success.
	if shouldCreateMR("succeeded", manifest) {
		if url, mrErr := r.createMR(ctx, req, manifest, workspace); mrErr != nil {
			slog.Error("failed to create MR", "run_id", req.RunID, "job_id", req.JobID, "error", mrErr)
		} else {
			stats["metadata"] = map[string]interface{}{"mr_url": url}
			slog.Info("MR created", "run_id", req.RunID, "job_id", req.JobID, "mr_url", url)
		}
	}

	var exitCodeZero int32 = 0
	if uploadErr := r.uploadStatus(ctx, req.RunID.String(), "succeeded", &exitCodeZero, stats, req.StepIndex, req.JobID); uploadErr != nil {
		slog.Error("failed to upload mod success status", "run_id", req.RunID, "job_id", req.JobID, "error", uploadErr)
	}
	slog.Info("mod job succeeded", "run_id", req.RunID, "job_id", req.JobID, "exit_code", result.ExitCode, "duration", duration)
}

// executeHealingJob runs a healing container job.
// Fetches gate logs from parent job, runs healing container, uploads diff.
//
// Stack-aware image selection: The job loads the persisted stack from the
// pre-gate phase and uses it for manifest building. This ensures healing
// mods use stack-specific images (e.g., java-maven, java-gradle) when configured.
func (r *runController) executeHealingJob(ctx context.Context, req StartRunRequest) {
	startTime := time.Now()

	// Initialize runtime components.
	runner, diffGenerator, logStreamer, err := r.initializeRuntime(ctx, req.RunID.String())
	if err != nil {
		slog.Error("failed to initialize runtime", "run_id", req.RunID, "error", err)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
	}
	defer func() { _ = logStreamer.Close() }()

	// Load the persisted stack from the pre-gate phase for stack-aware image
	// selection. If no stack was persisted (e.g., gate skipped), defaults to
	// ModStackUnknown which falls back to "default" in stack maps.
	stack := r.loadPersistedStack(req.RunID)

	// Parse options and build manifest with stack-aware image resolution.
	// stepIndex=0 is used for manifest building; job configuration comes from req.Options.
	typedOpts := parseRunOptions(req.Options)

	var manifest contracts.StepManifest

	// When build_gate_healing is configured, hydrate the healing manifest from the
	// typed HealingConfig so that discrete healing jobs use the correct image/env.
	if typedOpts.Healing != nil {
		strategies := typedOpts.Healing.NormalizedStrategies()
		if len(strategies) > 0 {
			healMod, healIndex := selectHealingModForJob(req, typedOpts.Healing)
			manifest, err = buildHealingManifestWithStack(req, healMod, healIndex, "", stack)
		}
	}
	if manifest.Image == "" {
		manifest, err = buildManifestFromRequestWithStack(req, typedOpts, 0, stack)
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

	// Rehydrate workspace from base + diffs.
	workspace, err := r.rehydrateWorkspaceForStep(ctx, req, manifest, req.StepIndex)
	if err != nil {
		slog.Error("failed to rehydrate workspace", "run_id", req.RunID, "error", err)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
	}
	defer func() { _ = os.RemoveAll(workspace) }()

	// Prepare /out and /in directories.
	outDir, err := os.MkdirTemp("", "ploy-heal-out-*")
	if err != nil {
		slog.Error("failed to create /out directory", "run_id", req.RunID, "error", err)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
	}
	defer func() { _ = os.RemoveAll(outDir) }()

	inDir, err := os.MkdirTemp("", "ploy-heal-in-*")
	if err != nil {
		slog.Error("failed to create /in directory", "run_id", req.RunID, "error", err)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
	}
	defer func() { _ = os.RemoveAll(inDir) }()

	// Hydrate /in/build-gate.log from the first failing gate log when available.
	// This gives healing containers (e.g., Codex) a trimmed failure view without
	// requiring them to re-run the gate themselves.
	if err := r.populateHealingInDir(req.RunID, inDir); err != nil {
		slog.Warn("failed to hydrate /in/build-gate.log for healing job", "run_id", req.RunID, "job_id", req.JobID, "error", err)
	}

	// Disable gate in manifest - healing jobs don't run gates.
	manifest.Gate = &contracts.StepGateSpec{Enabled: false}
	manifest.Shift = nil //nolint:staticcheck // backward compatibility: clear deprecated Shift field

	// Clear hydration since workspace is already hydrated.
	if len(manifest.Inputs) > 0 {
		inputs := make([]contracts.StepInput, len(manifest.Inputs))
		copy(inputs, manifest.Inputs)
		for i := range inputs {
			inputs[i].Hydration = nil
		}
		manifest.Inputs = inputs
	}

	// Inject healing environment variables.
	if manifest.Env == nil {
		manifest.Env = map[string]string{}
	}
	manifest.Env["PLOY_HOST_WORKSPACE"] = workspace
	manifest.Env["PLOY_SERVER_URL"] = r.cfg.ServerURL
	manifest.Env["PLOY_CA_CERT_PATH"] = "/etc/ploy/certs/ca.crt"
	manifest.Env["PLOY_CLIENT_CERT_PATH"] = "/etc/ploy/certs/client.crt"
	manifest.Env["PLOY_CLIENT_KEY_PATH"] = "/etc/ploy/certs/client.key"
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

	// Mount node TLS certificates into healing container for Build Gate API access.
	if manifest.Options == nil {
		manifest.Options = make(map[string]any)
	}
	manifest.Options["ploy_ca_cert_path"] = r.cfg.HTTP.TLS.CAPath
	manifest.Options["ploy_client_cert_path"] = r.cfg.HTTP.TLS.CertPath
	manifest.Options["ploy_client_key_path"] = r.cfg.HTTP.TLS.KeyPath

	slog.Info("starting healing job execution",
		"run_id", req.RunID,
		"job_id", req.JobID,
		"step_index", req.StepIndex,
	)

	// Run the healing container.
	result, runErr := runner.Run(ctx, step.Request{
		TicketID:  types.TicketID(req.RunID),
		Manifest:  manifest,
		Workspace: workspace,
		OutDir:    outDir,
		InDir:     inDir,
	})
	duration := time.Since(startTime)

	// Upload diff for this healing step.
	// E3: Pass job name for branch-local diff tagging in multi-strategy healing.
	r.uploadDiffForStep(ctx, req.RunID, req.JobID, req.JobName, diffGenerator, workspace, result, req.StepIndex)

	// Upload /out artifacts.
	if err := r.uploadOutDir(ctx, req.RunID, req.JobID, outDir); err != nil {
		slog.Warn("/out artifact upload failed", "run_id", req.RunID, "error", err)
	}

	// Build stats.
	stats := types.RunStats{
		"exit_code":   result.ExitCode,
		"duration_ms": duration.Milliseconds(),
	}

	// Determine status.
	if runErr != nil {
		var exitCode int32 = -1 // Use -1 to indicate runtime error
		if uploadErr := r.uploadStatus(ctx, req.RunID.String(), "failed", &exitCode, stats, req.StepIndex, req.JobID); uploadErr != nil {
			slog.Error("failed to upload healing failure status", "run_id", req.RunID, "job_id", req.JobID, "error", uploadErr)
		}
		slog.Info("healing job failed", "run_id", req.RunID, "job_id", req.JobID, "exit_code", result.ExitCode, "error", runErr, "duration", duration)
		return
	}

	if result.ExitCode != 0 {
		exitCode := int32(result.ExitCode)
		if uploadErr := r.uploadStatus(ctx, req.RunID.String(), "failed", &exitCode, stats, req.StepIndex, req.JobID); uploadErr != nil {
			slog.Error("failed to upload healing failure status", "run_id", req.RunID, "job_id", req.JobID, "error", uploadErr)
		}
		slog.Info("healing job failed", "run_id", req.RunID, "job_id", req.JobID, "exit_code", result.ExitCode, "duration", duration)
		return
	}

	var exitCodeZero int32 = 0
	if uploadErr := r.uploadStatus(ctx, req.RunID.String(), "succeeded", &exitCodeZero, stats, req.StepIndex, req.JobID); uploadErr != nil {
		slog.Error("failed to upload healing success status", "run_id", req.RunID, "job_id", req.JobID, "error", uploadErr)
	}
	slog.Info("healing job succeeded", "run_id", req.RunID, "job_id", req.JobID, "exit_code", result.ExitCode, "duration", duration)
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

// selectHealingModForJob selects the HealingMod that should back this healing job.
// Preference order:
//  1. Match StartRunRequest.ModImage against any HealingStrategy.Mods[i].Image (resolved).
//  2. Fall back to the first configured healing mod in the first strategy.
//
// When matching, the mod image is resolved using ModStackUnknown since we don't
// have stack information at job selection time. For universal images, this returns
// the exact image string. For stack-specific images, this compares against the
// default fallback (if any).
func selectHealingModForJob(req StartRunRequest, healing *HealingConfig) (HealingMod, int) {
	if healing == nil {
		return HealingMod{}, 0
	}

	strategies := healing.NormalizedStrategies()
	if len(strategies) == 0 {
		return HealingMod{}, 0
	}

	if img := strings.TrimSpace(req.ModImage); img != "" {
		for _, strategy := range strategies {
			for i, mod := range strategy.Mods {
				// Resolve the mod image using unknown stack (fallback to default).
				// This matches universal images directly and stack maps via default.
				resolved, err := mod.Image.ResolveImage(contracts.ModStackUnknown)
				if err != nil {
					continue // Skip mods that can't be resolved without stack context.
				}
				if strings.TrimSpace(resolved) == img {
					return mod, i
				}
			}
		}
	}

	// Fallback to first mod of first strategy.
	if len(strategies[0].Mods) == 0 {
		return HealingMod{}, 0
	}
	return strategies[0].Mods[0], 0
}

// uploadFailureStatus uploads a failure status for early errors.
// Uses exit code -1 to indicate pre-execution infrastructure failures.
func (r *runController) uploadFailureStatus(ctx context.Context, req StartRunRequest, err error, duration time.Duration) {
	var exitCode int32 = -1 // -1 indicates pre-execution failure
	stats := types.RunStats{
		"duration_ms": duration.Milliseconds(),
		"error":       err.Error(),
	}
	if uploadErr := r.uploadStatus(ctx, req.RunID.String(), "failed", &exitCode, stats, req.StepIndex, req.JobID); uploadErr != nil {
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
	gateExecutor := step.NewGateExecutor("", containerRuntime)

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
	terminalStatus := "succeeded"
	var exitCode int32

	switch {
	case execErr != nil:
		terminalStatus = "failed"
		// Check if this is a build gate failure.
		if errors.Is(execErr, step.ErrBuildGateFailed) {
			// Exit code 1 signals gate failure for server-side healing detection.
			exitCode = 1
		} else {
			// Exit code -1 for other execution errors.
			exitCode = -1
		}
	case result.ExitCode != 0:
		terminalStatus = "failed"
		exitCode = int32(result.ExitCode)
	default:
		exitCode = int32(result.ExitCode) // 0 for success
	}

	// Phase 7: Create MR via GitLab API when conditions are met.
	// Hook runs after terminal status is determined but before uploading status.
	mrURL := ""
	if shouldCreateMR(terminalStatus, manifest) {
		if url, mrErr := r.createMR(ctx, req, manifest, workspace); mrErr != nil {
			slog.Error("failed to create MR", "run_id", req.RunID, "job_id", req.JobID, "error", mrErr)
		} else {
			mrURL = url
			slog.Info("MR created successfully", "run_id", req.RunID, "job_id", req.JobID, "mr_url", mrURL)
		}
	}

	// Build stats with execution metrics and gate history.
	// Job ID is used to associate gate log artifacts with the current job.
	stats := r.buildExecutionStats(req.RunID, req.JobID, result, execResult, duration, mrURL)

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
		gate := r.buildGateStats(runID, jobID, result, execResult)
		stats["gate"] = gate
	}

	return stats
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
