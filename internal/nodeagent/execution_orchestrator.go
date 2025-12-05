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

	// Parse options and build manifest.
	// stepIndex=0 is used for manifest building; job configuration comes from req.Options.
	typedOpts := parseRunOptions(req.Options)
	manifest, err := buildManifestFromRequest(req, typedOpts, 0)
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
	// Resolve gate spec from manifest.
	gateSpec := manifest.Gate
	//lint:ignore SA1019 Backward compatibility: support deprecated Shift by mapping to Gate.
	if gateSpec == nil && manifest.Shift != nil {
		gateSpec = &contracts.StepGateSpec{
			Enabled: manifest.Shift.Enabled,
			Profile: manifest.Shift.Profile,
			Env:     manifest.Shift.Env,
		}
	}

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
func (r *runController) buildGateJobStats(gateResult *contracts.BuildGateStageMetadata, duration time.Duration) types.RunStats {
	stats := types.RunStats{
		"duration_ms": duration.Milliseconds(),
	}

	if gateResult != nil {
		passed := false
		if len(gateResult.StaticChecks) > 0 {
			passed = gateResult.StaticChecks[0].Passed
		}
		stats["gate"] = map[string]any{
			"passed":      passed,
			"duration_ms": duration.Milliseconds(),
		}
	}

	return stats
}

// executeModJob runs a mod container job.
// Executes the container, uploads diff, and reports status.
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

	// Parse options and build manifest.
	// stepIndex=0 is used for manifest building; job configuration comes from req.Options.
	typedOpts := parseRunOptions(req.Options)
	manifest, err := buildManifestFromRequest(req, typedOpts, 0)
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
	//lint:ignore SA1019 Backward compatibility
	manifest.Shift = nil

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
	r.uploadDiffForStep(ctx, req.RunID, req.JobID, diffGenerator, workspace, result, req.StepIndex)

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

	// Parse options and build manifest.
	// stepIndex=0 is used for manifest building; job configuration comes from req.Options.
	typedOpts := parseRunOptions(req.Options)

	var manifest contracts.StepManifest

	// When build_gate_healing is configured, hydrate the healing manifest from the
	// typed HealingConfig so that discrete healing jobs use the correct image/env.
	if typedOpts.Healing != nil && len(typedOpts.Healing.Mods) > 0 {
		healMod, healIndex := selectHealingModForJob(req, typedOpts.Healing)
		manifest, err = buildHealingManifest(req, healMod, healIndex, "")
	} else {
		manifest, err = buildManifestFromRequest(req, typedOpts, 0)
	}
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
	//lint:ignore SA1019 Backward compatibility
	manifest.Shift = nil

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
	r.uploadDiffForStep(ctx, req.RunID, req.JobID, diffGenerator, workspace, result, req.StepIndex)

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
//  1. Match StartRunRequest.ModImage against HealingConfig.Mods[i].Image (resolved).
//  2. Fall back to the first configured healing mod.
//
// When matching, the mod image is resolved using ModStackUnknown since we don't
// have stack information at job selection time. For universal images, this returns
// the exact image string. For stack-specific images, this compares against the
// default fallback (if any).
func selectHealingModForJob(req StartRunRequest, healing *HealingConfig) (HealingMod, int) {
	if healing == nil || len(healing.Mods) == 0 {
		return HealingMod{}, 0
	}

	if img := strings.TrimSpace(req.ModImage); img != "" {
		for i, mod := range healing.Mods {
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

	return healing.Mods[0], 0
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

	// Initialize gate executor with configuration-driven mode selection.
	// PLOY_BUILDGATE_MODE controls whether gates run locally (docker) or remotely (http).
	// Default is "local-docker" for backward compatibility.
	gateExecutor, gateErr := r.createGateExecutor(containerRuntime)
	if gateErr != nil {
		// Log warning but continue; gate executor may be nil if HTTP client creation fails.
		// This allows runs to proceed without gate validation when misconfigured.
		slog.Warn("failed to create gate executor, gate validation will be skipped",
			"run_id", runID,
			"error", gateErr,
		)
	}

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
func (r *runController) finalizeRun(ctx context.Context, req StartRunRequest, manifest contracts.StepManifest, execResult executionResult, execErr error, workspace string, duration time.Duration) {
	result := execResult.Result

	// Determine terminal status and exit code based on execution result.
	terminalStatus := "succeeded"
	var exitCode int32
	if execErr != nil {
		terminalStatus = "failed"
		// Check if this is a build gate failure.
		if errors.Is(execErr, step.ErrBuildGateFailed) {
			// Exit code 1 signals gate failure for server-side healing detection.
			exitCode = 1
		} else {
			// Exit code -1 for other execution errors.
			exitCode = -1
		}
	} else if result.ExitCode != 0 {
		terminalStatus = "failed"
		exitCode = int32(result.ExitCode)
	} else {
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

// rehydrateWorkspaceForStep creates a fresh workspace for the given step by rehydrating
// from the base clone and applying ordered diffs from prior steps.
//
// For step 0: Creates base clone (or reuses cached base if available).
// For step k>0: Copies base clone + applies diffs from steps 0 through k-1.
//
// This function implements the core rehydration strategy that enables multi-node execution:
// each step can run on any node by reconstructing workspace state from base + diff chain.
//
// Parameters:
//   - ctx: Context for cancellation and deadlines.
//   - req: StartRunRequest containing repo URL, base_ref, and commit_sha.
//   - manifest: StepManifest for this step.
//   - stepIndex: Job step index for execution tracking.
//
// Returns:
//   - workspacePath: Path to the rehydrated workspace ready for execution.
//   - error: Non-nil if rehydration fails (clone, copy, or patch application error).
func (r *runController) rehydrateWorkspaceForStep(
	ctx context.Context,
	req StartRunRequest,
	manifest contracts.StepManifest,
	stepIndex types.StepIndex,
) (string, error) {
	runID := req.RunID.String()

	// Step 1: Ensure base clone exists (create on first use, reuse on subsequent calls).
	// Base clone path is deterministic per run and node.
	baseRoot := os.Getenv("PLOYD_CACHE_HOME")
	if baseRoot == "" {
		baseRoot = os.TempDir()
	}
	baseClone := filepath.Join(baseRoot, "ploy", "run", runID, "base")
	if err := os.MkdirAll(baseClone, 0o755); err != nil {
		return "", fmt.Errorf("create base clone dir: %w", err)
	}

	slog.Info("creating base clone for run", "run_id", runID, "path", baseClone)

	// Initialize git fetcher for repository hydration. The fetcher is responsible for
	// reusing cached clones when PLOYD_CACHE_HOME is configured.
	gitFetcher, err := r.createGitFetcher()
	if err != nil {
		return "", fmt.Errorf("create git fetcher: %w", err)
	}

	// Determine repo materialization:
	// - Prefer manifest inputs that already carry hydration.Repo (gate/mod jobs).
	// - Fallback to StartRunRequest repo fields (healing jobs and other callers).
	var repo *contracts.RepoMaterialization
	for _, input := range manifest.Inputs {
		if input.Hydration != nil && input.Hydration.Repo != nil {
			repo = input.Hydration.Repo
			break
		}
	}

	if repo == nil {
		// Derive repo materialization from StartRunRequest, mirroring
		// buildManifestFromRequest semantics.
		targetRef := strings.TrimSpace(req.TargetRef.String())
		if targetRef == "" && strings.TrimSpace(req.BaseRef.String()) != "" {
			targetRef = strings.TrimSpace(req.BaseRef.String())
		}

		tmp := contracts.RepoMaterialization{
			URL:       req.RepoURL,
			BaseRef:   req.BaseRef,
			TargetRef: types.GitRef(targetRef),
			Commit:    req.CommitSHA,
		}
		repo = &tmp
	}

	if err := gitFetcher.Fetch(ctx, repo, baseClone); err != nil {
		return "", fmt.Errorf("hydrate base clone: %w", err)
	}

	slog.Info("base clone created", "run_id", runID, "path", baseClone)

	// Step 2: Rehydrate workspace from base clone + ordered diffs.
	// C2: For ALL steps (including step 0), fetch diffs and apply them.
	// This ensures step 0 runs on the healed baseline if pre-mod healing occurred.
	workspacePath, err := createWorkspaceDir()
	if err != nil {
		return "", fmt.Errorf("create workspace dir: %w", err)
	}

	slog.Info("rehydrating workspace from base + diffs", "run_id", runID, "step_index", stepIndex)

	diffFetcher, err := NewDiffFetcher(r.cfg)
	if err != nil {
		_ = os.RemoveAll(workspacePath)
		return "", fmt.Errorf("create diff fetcher: %w", err)
	}

	// C2: Uniform rehydration query for ALL steps.
	// Fetch diffs where step_index < stepIndex (all diffs from previous jobs).
	// Jobs are ordered by step_index (e.g., 1000=pre-gate, 2000=mod-0, 3000=post-gate).
	gzippedDiffs, err := diffFetcher.FetchDiffsForStep(ctx, runID, stepIndex-1)
	if err != nil {
		_ = os.RemoveAll(workspacePath)
		return "", fmt.Errorf("fetch diffs for step: %w", err)
	}

	slog.Info("fetched diffs for rehydration", "run_id", runID, "step_index", stepIndex, "diff_count", len(gzippedDiffs))

	// Rehydrate workspace from base + diffs using the helper from execution.go.
	if err := RehydrateWorkspaceFromBaseAndDiffs(ctx, baseClone, workspacePath, gzippedDiffs); err != nil {
		_ = os.RemoveAll(workspacePath)
		return "", fmt.Errorf("rehydrate from base and diffs: %w", err)
	}

	slog.Info("workspace rehydrated successfully", "run_id", runID, "step_index", stepIndex, "workspace", workspacePath)

	// Create baseline commit after rehydration to enable incremental diffs.
	// C2: Now applies to ALL steps (including step 0) when diffs were applied.
	// This commit establishes a new HEAD so that "git diff HEAD" generates
	// only the changes from this step, not cumulative changes from prior steps.
	if len(gzippedDiffs) > 0 {
		if err := ensureBaselineCommitForRehydration(ctx, workspacePath, stepIndex); err != nil {
			_ = os.RemoveAll(workspacePath)
			return "", fmt.Errorf("create baseline commit for rehydration: %w", err)
		}
		slog.Info("baseline commit created for incremental diff", "run_id", runID, "step_index", stepIndex)
	}

	return workspacePath, nil
}

// uploadDiffForStep generates and uploads a diff for the given step with step_index metadata.
// This replaces the older uploadDiff method and tags each diff with its step index for
// ordered rehydration in multi-step/multi-node scenarios.
func (r *runController) uploadDiffForStep(
	ctx context.Context,
	runID types.RunID,
	jobID types.JobID,
	diffGenerator step.DiffGenerator,
	workspace string,
	result step.Result,
	stepIndex types.StepIndex,
) {
	if diffGenerator == nil {
		return
	}

	// Generate workspace diff for this step.
	diffBytes, err := diffGenerator.Generate(ctx, workspace)
	if err != nil {
		slog.Error("failed to generate step diff", "run_id", runID, "step_index", stepIndex, "error", err)
		return
	}

	if len(diffBytes) == 0 {
		// No changes from this step; skip upload.
		slog.Info("no diff to upload for step (no changes)", "run_id", runID, "step_index", stepIndex)
		return
	}

	// Build diff summary with step metadata for database storage.
	// C2: Every diff is tagged with step_index + mod_type for unified rehydration.
	// - step_index: Job step index for ordering and rehydration queries.
	// - mod_type: "mod" for main mod diffs (healing diffs use "healing" in execution_healing.go).
	summary := types.DiffSummary{
		"step_index": stepIndex,
		"mod_type":   "mod", // Identifies this diff as a main mod step diff.
		"exit_code":  result.ExitCode,
		"timings": map[string]interface{}{
			"hydration_duration_ms":  result.Timings.HydrationDuration.Milliseconds(),
			"execution_duration_ms":  result.Timings.ExecutionDuration.Milliseconds(),
			"build_gate_duration_ms": result.Timings.BuildGateDuration.Milliseconds(),
			"diff_duration_ms":       result.Timings.DiffDuration.Milliseconds(),
			"total_duration_ms":      result.Timings.TotalDuration.Milliseconds(),
		},
	}

	// Upload diff with step metadata to control plane.
	diffUploader, err := NewDiffUploader(r.cfg)
	if err != nil {
		slog.Error("failed to create diff uploader", "run_id", runID, "step_index", stepIndex, "error", err)
		return
	}

	// Upload diff to job-scoped endpoint. Step ordering is tracked in the summary metadata.
	if err := diffUploader.UploadDiff(ctx, runID, jobID, diffBytes, summary); err != nil {
		slog.Error("failed to upload step diff", "run_id", runID, "job_id", jobID, "step_index", stepIndex, "error", err)
		return
	}

	slog.Info("step diff uploaded successfully", "run_id", runID, "job_id", jobID, "step_index", stepIndex, "size", len(diffBytes))
}

// uploadBaselineDiff uploads a diff between two directories with the specified mod_type and step_index.
// Used by C2 to persist pre-mod or post-mod healing changes so subsequent steps run on the healed baseline.
//
// Parameters:
//   - baseDir: the reference directory (e.g., base clone before healing)
//   - modifiedDir: the modified directory (e.g., healed workspace)
//   - stepIndex: the step index to tag the diff with
//   - modType: the mod_type to tag the diff with (e.g., "pre_gate", "post_gate")
//
// Returns error if diff generation fails or diff is empty (healing claimed success but made no changes).
func (r *runController) uploadBaselineDiff(
	ctx context.Context,
	runID types.RunID,
	jobID types.JobID,
	diffGenerator step.DiffGenerator,
	baseDir string,
	modifiedDir string,
	stepIndex types.StepIndex,
	modType string,
) error {
	if diffGenerator == nil {
		return fmt.Errorf("diff generator nil, cannot upload baseline diff")
	}

	diffBytes, err := diffGenerator.GenerateBetween(ctx, baseDir, modifiedDir)
	if err != nil {
		return fmt.Errorf("failed to generate baseline diff: %w", err)
	}

	if len(diffBytes) == 0 {
		// Empty diff after healing passed means something is wrong:
		// either the gate is flaky or healing made no actual changes.
		// Fail the run to surface this inconsistency.
		return fmt.Errorf("healing produced empty diff but gate passed - possible flaky gate or healing made no changes")
	}

	summary := types.DiffSummary{
		"step_index": stepIndex,
		"mod_type":   modType,
	}

	diffUploader, err := NewDiffUploader(r.cfg)
	if err != nil {
		return fmt.Errorf("failed to create diff uploader: %w", err)
	}

	// Upload diff to job-scoped endpoint. Step ordering is tracked in the summary metadata.
	if err := diffUploader.UploadDiff(ctx, runID, jobID, diffBytes, summary); err != nil {
		return fmt.Errorf("failed to upload baseline diff: %w", err)
	}

	slog.Info("baseline diff uploaded successfully", "run_id", runID, "job_id", jobID, "mod_type", modType, "step_index", stepIndex, "size", len(diffBytes))
	return nil
}

// createGateExecutor creates a GateExecutor based on PLOY_BUILDGATE_MODE configuration.
// Supports two modes:
//   - "local-docker" (default): Uses container runtime for local gate execution.
//   - "remote-http": Delegates to Build Gate workers via HTTP API.
//
// Returns the executor and any error from HTTP client creation (for remote-http mode).
func (r *runController) createGateExecutor(containerRuntime step.ContainerRuntime) (step.GateExecutor, error) {
	mode := os.Getenv("PLOY_BUILDGATE_MODE")

	// For local-docker mode (default), use container runtime directly.
	if mode == "" || mode == step.GateExecutorModeLocalDocker {
		return step.NewGateExecutor(mode, containerRuntime, nil), nil
	}

	// For remote-http mode, create HTTP client from environment configuration.
	if mode == step.GateExecutorModeRemoteHTTP {
		cfg, err := step.BuildGateHTTPClientConfigFromEnv()
		if err != nil {
			// Return error but allow caller to decide whether to proceed without gate.
			return nil, fmt.Errorf("load build gate http client config: %w", err)
		}

		httpClient, err := step.NewBuildGateHTTPClient(cfg)
		if err != nil {
			return nil, fmt.Errorf("create build gate http client: %w", err)
		}

		return step.NewGateExecutor(mode, containerRuntime, httpClient), nil
	}

	// Unrecognized mode: factory will log warning and fall back to local-docker.
	return step.NewGateExecutor(mode, containerRuntime, nil), nil
}
