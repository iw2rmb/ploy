// execution_healing.go isolates gate-heal-regate execution complexity.
//
// This file contains executeWithHealing, which implements the retry loop for
// healing mods when gate validation fails. It orchestrates pre-gate execution,
// healing mod execution on gate failure, and re-gate validation. The healing
// logic is separated from core orchestration to maintain clear boundaries
// between run lifecycle (orchestrator) and healing retry mechanics.
//
// ## Gate Execution Model
//
// Gate validation runs via the Docker-based GateExecutor (gate_docker.go) which
// executes validation containers locally. After healing mods complete, the node
// agent ALWAYS re-runs the gate to verify the fix.
//
// Key guarantees:
//
//  1. Re-gate execution: After healing mods complete, the node agent re-runs
//     the gate via runner.Gate.Execute. This ensures the canonical gate result
//     is produced by the gate system triggered by the node agent.
//
//  2. Full gate history capture: The node agent records all gate executions:
//     - PreGate: The initial gate run before healing (BuildGateStageMetadata)
//     - ReGates: All subsequent re-gate attempts after each healing iteration
//     This history enables telemetry, debugging, and audit trails.
//
// ## Repo+Diff Semantics
//
// Healing verification uses a repo+diff model:
//
//   - Initial workspace: The Build Gate validates code cloned from repo_url+ref.
//     Healing mods operate on this same workspace.
//
//   - Healing modifications: Healing containers modify the workspace in-place.
//     Each healing mod's changes accumulate on top of the repo baseline.
//
//   - Re-gate verification: After healing completes, the gate re-runs against
//     the same workspace (repo_url+ref + healing modifications).
//
//   - Diff chain semantics: Workspace state at any point equals base clone plus
//     an ordered sequence of diffs. This model matches Mods multi-step execution
//     where each step's changes are captured and can be replayed for rehydration.
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
	"unicode/utf8"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/runtime/step"
)

// runGateWithHealing executes the build gate with optional healing loop when validation fails.
// This helper centralizes gate+healing orchestration logic for reuse by both pre-mod and
// post-mod gate phases.
//
// ## Parameters
//
//   - ctx: Context for cancellation and deadlines.
//   - runner: Step runner with gate executor and container runtime.
//   - req: Start run request containing healing configuration in Options.
//   - manifest: Step manifest with gate spec.
//   - workspace: Path to the workspace directory (repo_url+ref clone).
//   - outDir: Path to the /out directory for artifacts.
//   - inDir: Pointer to the /in directory path; created if empty and healing is triggered.
//   - gatePhase: "pre" or "post" to indicate which gate phase is executing.
//   - stepIndex: 0-based step number used for logging and gate statistics (matches Mods step index).
//
// ## Returns
//
//   - initialGate: Metadata from the first gate execution (always populated if gate runs).
//   - reGates: Slice of re-gate metadata after each healing attempt (empty if gate passes).
//   - error: nil if gate passes (with or without healing), ErrBuildGateFailed if exhausted.
//
// ## Healing Workflow
//
//  1. Execute initial gate via runner.Gate.Execute
//  2. If gate passes, return immediately with initialGate metadata
//  3. If gate fails and healing is configured:
//     a. Create /in directory if not already created
//     b. Write /in/build-gate.log for healer inspection
//     c. For each retry attempt:
//     - Execute each healing mod in sequence
//     - Upload healing mod diffs for rehydration (tagged with stepIndex per C2)
//     - Re-run gate after healing mods complete
//     - If gate passes, return success
//     d. If all retries exhausted, return ErrBuildGateFailed
//
// ## Repo+Diff Verification Model
//
// Healing verification uses a repo+diff model consistent with the Docker-based gate:
// the workspace contains repo_url+ref plus accumulated healing modifications. The
// GateExecutor validates this workspace directly, and DiffPatch is derived from it
// for telemetry and potential future distributed gate scenarios.
//
// ## Configuration
//
// Healing configuration is specified via req.TypedOptions.Healing (derived from
// build_gate_healing in the run spec) using the canonical single-mod schema:
//   - retries (int): maximum number of healing attempts (default: 1)
//   - mod (object): healing mod spec (image, command, env, retain_container)
func (r *runController) runGateWithHealing(
	ctx context.Context,
	runner step.Runner,
	req StartRunRequest,
	manifest contracts.StepManifest,
	workspace, outDir string,
	inDir *string,
	gatePhase string, // "pre" or "post"
	stepIndex int, // logical step index for logging and stats
) (*gateRunMetadata, []gateRunMetadata, string, error) {
	gateSpec := manifest.Gate

	// If gate is disabled or executor unavailable, return immediately.
	if runner.Gate == nil || gateSpec == nil || !gateSpec.Enabled {
		return nil, nil, "", nil
	}

	// Execute initial gate.
	gateStart := time.Now()
	gateMetadata, gateErr := runner.Gate.Execute(ctx, gateSpec, workspace)
	gateDuration := time.Since(gateStart)

	if gateErr != nil {
		return nil, nil, "", fmt.Errorf("gate execution failed: %w", gateErr)
	}

	// Capture initial gate metadata for stats.
	initialGate := &gateRunMetadata{
		Metadata:   gateMetadata,
		DurationMs: gateDuration.Milliseconds(),
	}

	// Check if gate passed using shared helper.
	if gateResultPassed(gateMetadata) {
		slog.Info("gate passed", "run_id", req.RunID, "phase", gatePhase)
		return initialGate, nil, "", nil
	}

	// Gate failed. Check if healing is configured using typed options from request.
	typedOpts := req.TypedOptions
	if typedOpts.Healing == nil || typedOpts.Healing.Mod.Image.IsEmpty() {
		// No healing configured (or healing configured without a mod); return the gate failure.
		slog.Info("gate failed, no healing configured", "run_id", req.RunID, "phase", gatePhase)
		return initialGate, nil, "", step.ErrBuildGateFailed
	}

	healingConfig := typedOpts.Healing
	retries := healingConfig.Retries

	// Create /in directory if not already created (for build-gate.log).
	if *inDir == "" {
		tmpInDir, dirErr := os.MkdirTemp("", "ploy-mod-in-*")
		if dirErr != nil {
			slog.Error("failed to create /in directory for healing", "run_id", req.RunID, "error", dirErr)
			return initialGate, nil, "", step.ErrBuildGateFailed
		}
		*inDir = tmpInDir
		// Caller handles cleanup via defer.
	}

	// Write build-gate.log to /in for healing containers.
	// Prefer trimmed log view (LogFindings) when available so Codex and
	// other healing mods see a focused failure slice instead of the full truncated gate log.
	logPayload := gateMetadata.LogsText
	if len(gateMetadata.LogFindings) > 0 {
		if trimmed := strings.TrimSpace(gateMetadata.LogFindings[0].Message); trimmed != "" {
			logPayload = trimmed
			if !strings.HasSuffix(logPayload, "\n") {
				logPayload += "\n"
			}
		}
	}
	if logPayload != "" {
		inLogPath := filepath.Join(*inDir, "build-gate.log")
		if writeErr := os.WriteFile(inLogPath, []byte(logPayload), 0o644); writeErr != nil {
			slog.Warn("failed to write /in/build-gate.log", "run_id", req.RunID, "error", writeErr)
		} else {
			slog.Info("build-gate.log persisted to /in for healing", "run_id", req.RunID, "path", inLogPath, "phase", gatePhase)
		}
	}

	// Track re-gate runs for stats.
	var reGates []gateRunMetadata

	// Track the last action_summary produced by healing containers across iterations.
	var lastActionSummary string

	// Track healing session state across healing loop iterations.
	// When a healing agent writes a session file (codex-session.txt) to /out,
	// the agent reads and persists this session ID to /in for subsequent attempts.
	// The concrete env/filename contract (e.g. CODEX_RESUME + codex-session.txt)
	// is defined in buildHealingManifest; this loop is agnostic to the agent.
	var healingSession string

	// Attempt healing loop.
	// Note: This is a domain-specific healing retry loop (not a transient error retry).
	// It executes a single healing mod between gate validation attempts based on user-configured retries.
	// We intentionally do not use internal/workflow/backoff here because:
	//  1. This is not a retry-on-failure pattern (each iteration does useful work: running a healing mod).
	//  2. The retry count is user-configured (manifest-specified retries parameter).
	//  3. No exponential backoff is needed; each healing attempt runs immediately after the healing mod completes.
	// healingLogBuf accumulates the /in/healing-log.md content across iterations.
	var healingLogBuf strings.Builder

	for attempt := 1; attempt <= retries; attempt++ {
		slog.Info("starting healing attempt", "run_id", req.RunID, "attempt", attempt, "max_retries", retries, "phase", gatePhase)

		// Stack-aware image selection uses the stack derived from the gate metadata
		// that triggered this healing attempt.
		stackForAttempt := contracts.ModStackUnknown
		switch {
		case attempt == 1 && initialGate != nil && initialGate.Metadata != nil:
			stackForAttempt = initialGate.Metadata.DetectedStack()
		case len(reGates) > 0 && reGates[len(reGates)-1].Metadata != nil:
			stackForAttempt = reGates[len(reGates)-1].Metadata.DetectedStack()
		}
		if stackForAttempt == "" {
			stackForAttempt = contracts.ModStackUnknown
		}

		// --- Per-iteration artifact: /in/build-gate-iteration-N.log ---
		// Write the current gate failure log as a per-iteration snapshot.
		if logPayload != "" {
			iterGateLogPath := filepath.Join(*inDir, fmt.Sprintf("build-gate-iteration-%d.log", attempt))
			if writeErr := os.WriteFile(iterGateLogPath, []byte(logPayload), 0o644); writeErr != nil {
				slog.Warn("failed to write build-gate-iteration log", "run_id", req.RunID, "attempt", attempt, "error", writeErr)
			}
		}

		// --- Per-iteration router execution (bug_summary) ---
		// Run router before healing so we capture a one-line summary for the current
		// failing build-gate.log (which may change across re-gate attempts).
		bugSummary := ""
		if typedOpts.Router != nil && !typedOpts.Router.Image.IsEmpty() {
			slog.Info("running router for bug_summary", "run_id", req.RunID, "attempt", attempt, "phase", gatePhase)

			func() {
				routerOutDir, routerDirErr := os.MkdirTemp("", "ploy-router-out-*")
				if routerDirErr != nil {
					slog.Error("failed to create router /out directory", "run_id", req.RunID, "attempt", attempt, "error", routerDirErr)
					return
				}
				defer os.RemoveAll(routerOutDir)

				routerManifest, routerBuildErr := buildRouterManifest(req, *typedOpts.Router, stackForAttempt)
				if routerBuildErr != nil {
					slog.Error("failed to build router manifest", "run_id", req.RunID, "attempt", attempt, "error", routerBuildErr)
					return
				}

				r.injectHealingEnvVars(&routerManifest, workspace)
				r.mountHealingTLSCerts(&routerManifest)

				_, routerErr := runner.Run(ctx, step.Request{
					RunID:     req.RunID,
					Manifest:  routerManifest,
					Workspace: workspace,
					OutDir:    routerOutDir,
					InDir:     *inDir,
				})
				if routerErr != nil {
					slog.Warn("router execution failed", "run_id", req.RunID, "attempt", attempt, "error", routerErr)
					return
				}

				bugSummary = parseBugSummary(routerOutDir)
				if bugSummary == "" {
					return
				}

				// Attach bug_summary to the initial gate metadata for downstream stats.
				// Inline healing does not have a separate gate job, but this preserves
				// the first failing gate's bug_summary in run stats.
				if attempt == 1 && initialGate != nil && initialGate.Metadata != nil {
					initialGate.Metadata.BugSummary = bugSummary
				}
				slog.Info("router produced bug_summary", "run_id", req.RunID, "attempt", attempt, "bug_summary", bugSummary)
			}()
		}

		// Capture workspace status before running the healing mod so we can detect
		// whether this healing attempt produced any net changes.
		preStatus, preStatusErr := workspaceStatus(ctx, workspace)
		if preStatusErr != nil {
			slog.Warn("healing: failed to compute workspace status before healing; assuming changes may occur",
				"run_id", req.RunID,
				"attempt", attempt,
				"phase", gatePhase,
				"error", preStatusErr,
			)
		}

		// Build healing manifest from the single configured HealingMod.
		mod := healingConfig.Mod
		const healingIndex = 0

		// Pass healingSession through so agent-specific session env (for example,
		// CODEX_RESUME=1 for Codex-based healers) can be injected by
		// buildHealingManifest when appropriate.
		// Stack is unknown during inline healing loops since the gate result
		// that detected the stack is the one that just failed. Use stackForAttempt
		// (derived from that gate metadata) for deterministic stack-aware image selection.
		healManifest, buildErr := buildHealingManifest(req, mod, healingIndex, healingSession, stackForAttempt)
		if buildErr != nil {
			slog.Error("failed to build healing manifest", "run_id", req.RunID, "healing_index", healingIndex, "error", buildErr)
			return initialGate, reGates, lastActionSummary, fmt.Errorf("build healing manifest[%d]: %w", healingIndex, buildErr)
		}

		slog.Info("executing healing mod", "run_id", req.RunID, "attempt", attempt, "healing_index", healingIndex, "image", healManifest.Image, "phase", gatePhase)

		// Inject healing-specific environment variables and TLS certificate mounts.
		// Uses shared helpers (injectHealingEnvVars, mountHealingTLSCerts) to ensure
		// consistent configuration between inline healing (runGateWithHealing) and
		// discrete healing jobs (executeHealingJob).
		r.injectHealingEnvVars(&healManifest, workspace)
		r.mountHealingTLSCerts(&healManifest)

		// Run the healing mod container.
		// Pass RunID directly for consistent labeling and telemetry.
		healResult, healErr := runner.Run(ctx, step.Request{
			RunID:     req.RunID,
			Manifest:  healManifest,
			Workspace: workspace,
			OutDir:    outDir,
			InDir:     *inDir,
		})

		if healErr != nil {
			slog.Error("healing mod execution failed", "run_id", req.RunID, "healing_index", healingIndex, "error", healErr)
			return initialGate, reGates, lastActionSummary, fmt.Errorf("healing mod[%d] failed: %w", healingIndex, healErr)
		}

		if healResult.ExitCode != 0 {
			slog.Warn("healing mod exited with non-zero code", "run_id", req.RunID, "healing_index", healingIndex, "exit_code", healResult.ExitCode)
			// Continue; re-gate will still run so callers see gate status.
		}

		// Upload /out artifacts for this healing mod if present.
		if uploadErr := r.uploadOutDir(ctx, req.RunID, req.JobID, outDir); uploadErr != nil {
			slog.Warn("failed to upload /out for healing mod", "run_id", req.RunID, "job_id", req.JobID, "healing_index", healingIndex, "error", uploadErr)
		}

		// Read session artifacts from /out for propagation across retries.
		if sessionBytes, readErr := os.ReadFile(filepath.Join(outDir, "codex-session.txt")); readErr == nil {
			if session := strings.TrimSpace(string(sessionBytes)); session != "" {
				healingSession = session
				slog.Info("healing: captured session from /out", "run_id", req.RunID, "healing_index", healingIndex, "session_id", healingSession)
			}
		}

		// Persist codex-session.txt to /in for subsequent healing attempts.
		// The filename is part of the current session contract; callers inside
		// containers remain free to interpret it as needed.
		if healingSession != "" && *inDir != "" {
			sessionPath := filepath.Join(*inDir, "codex-session.txt")
			if writeErr := os.WriteFile(sessionPath, []byte(healingSession), 0o644); writeErr != nil {
				slog.Warn("healing: failed to persist codex-session.txt into /in", "run_id", req.RunID, "error", writeErr)
			} else {
				slog.Info("healing: persisted codex-session.txt to /in for resume", "run_id", req.RunID, "session_id", healingSession)
			}
		}

		// --- Capture action_summary from healing container ---
		actionSummary := parseActionSummary(outDir)
		if actionSummary != "" {
			lastActionSummary = actionSummary
			slog.Info("healing: captured action_summary", "run_id", req.RunID, "attempt", attempt, "action_summary", actionSummary)
		}

		// --- Per-iteration artifact: /in/healing-iteration-N.log ---
		// Copy healing agent output (codex.log or equivalent) to /in for history.
		healingIterLogPath := filepath.Join(*inDir, fmt.Sprintf("healing-iteration-%d.log", attempt))
		if codexLog, readErr := os.ReadFile(filepath.Join(outDir, "codex.log")); readErr == nil {
			if writeErr := os.WriteFile(healingIterLogPath, codexLog, 0o644); writeErr != nil {
				slog.Warn("failed to write healing-iteration log", "run_id", req.RunID, "attempt", attempt, "error", writeErr)
			}
		}

		// --- Append iteration block to /in/healing-log.md ---
		if attempt == 1 {
			healingLogBuf.WriteString("# Healing Log\n\n")
		}
		fmt.Fprintf(&healingLogBuf, "## Iteration %d\n\n", attempt)
		if bugSummary != "" {
			fmt.Fprintf(&healingLogBuf, "- Bug Summary: %s\n", bugSummary)
		} else {
			healingLogBuf.WriteString("- Bug Summary: N/A\n")
		}
		fmt.Fprintf(&healingLogBuf, "  Build Log: /in/build-gate-iteration-%d.log\n", attempt)
		if actionSummary != "" {
			fmt.Fprintf(&healingLogBuf, "- Healing Attempt: %s\n", actionSummary)
		} else {
			healingLogBuf.WriteString("- Healing Attempt: N/A\n")
		}
		fmt.Fprintf(&healingLogBuf, "  Agent Log: /in/healing-iteration-%d.log\n\n", attempt)

		// Write healing-log.md after each iteration so it's available even if we bail out early.
		healingLogPath := filepath.Join(*inDir, "healing-log.md")
		if writeErr := os.WriteFile(healingLogPath, []byte(healingLogBuf.String()), 0o644); writeErr != nil {
			slog.Warn("failed to write healing-log.md", "run_id", req.RunID, "error", writeErr)
		}

		// Capture workspace status after the healing mod completes and compare with
		// the pre-healing status. If both are available and identical, then this
		// healing attempt produced no net workspace changes and there is no point
		// in re-running the gate. Treat this as a terminal build gate failure.
		postStatus, postStatusErr := workspaceStatus(ctx, workspace)
		if postStatusErr != nil {
			slog.Warn("healing: failed to compute workspace status after healing; proceeding with re-gate",
				"run_id", req.RunID,
				"attempt", attempt,
				"phase", gatePhase,
				"error", postStatusErr,
			)
		}

		if preStatusErr == nil && postStatusErr == nil && preStatus == postStatus {
			slog.Warn("healing: no workspace changes detected after healing attempt; skipping re-gate",
				"run_id", req.RunID,
				"attempt", attempt,
				"phase", gatePhase,
			)
			// Retries are effectively exhausted because healing cannot make further progress.
			return initialGate, reGates, lastActionSummary, fmt.Errorf("%w: healing produced no workspace changes", step.ErrBuildGateFailed)
		}

		// Re-run the gate after the healing mod completes.
		// The node agent ALWAYS re-runs the gate via runner.Gate.Execute to verify
		// that healing modifications have resolved the validation failure.
		slog.Info("re-running build gate after healing", "run_id", req.RunID, "attempt", attempt, "phase", gatePhase)

		// Clone gateSpec for re-gate execution. DiffPatch is intentionally left empty:
		// gate validation runs directly against the mutated workspace, and discrete
		// healing jobs publish baseline-based diffs for rehydration.
		regateSpec := &contracts.StepGateSpec{
			Enabled:        gateSpec.Enabled,
			Env:            gateSpec.Env,
			ImageOverrides: gateSpec.ImageOverrides,
			RepoURL:        gateSpec.RepoURL,
			Ref:            gateSpec.Ref,
			StackGate:      gateSpec.StackGate,
		}

		regateStart := time.Now()
		reGateMetadata, regateErr := runner.Gate.Execute(ctx, regateSpec, workspace)
		regateDuration := time.Since(regateStart)

		if regateErr != nil {
			slog.Error("re-gate execution failed", "run_id", req.RunID, "error", regateErr)
			return initialGate, reGates, lastActionSummary, fmt.Errorf("re-gate execution failed: %w", regateErr)
		}

		// Capture re-gate metadata for stats.
		reGates = append(reGates, gateRunMetadata{
			Metadata:   reGateMetadata,
			DurationMs: regateDuration.Milliseconds(),
		})

		// Check if gate passed using shared helper.
		if gateResultPassed(reGateMetadata) {
			slog.Info("build gate passed after healing", "run_id", req.RunID, "attempt", attempt, "phase", gatePhase)
			return initialGate, reGates, lastActionSummary, nil
		}

		// Re-gate failed; update logPayload and /in/build-gate.log for the next iteration.
		slog.Warn("build gate still failing after healing", "run_id", req.RunID, "attempt", attempt, "phase", gatePhase)

		// Refresh logPayload from the re-gate result so the next iteration's
		// build-gate-iteration-N.log and healing-log.md reflect the latest failure.
		logPayload = reGateMetadata.LogsText
		if len(reGateMetadata.LogFindings) > 0 {
			if trimmed := strings.TrimSpace(reGateMetadata.LogFindings[0].Message); trimmed != "" {
				logPayload = trimmed
				if !strings.HasSuffix(logPayload, "\n") {
					logPayload += "\n"
				}
			}
		}
		if logPayload != "" {
			inLogPath := filepath.Join(*inDir, "build-gate.log")
			if writeErr := os.WriteFile(inLogPath, []byte(logPayload), 0o644); writeErr != nil {
				slog.Warn("failed to update /in/build-gate.log after re-gate", "run_id", req.RunID, "error", writeErr)
			}
		}
	}

	// Retries exhausted; return the gate failure.
	slog.Error("healing retries exhausted, build gate still failing", "run_id", req.RunID, "phase", gatePhase)
	return initialGate, reGates, lastActionSummary, fmt.Errorf("%w: healing retries exhausted", step.ErrBuildGateFailed)
}

// executeWithHealing runs the main step with optional healing loop when the build gate fails.
// It handles the gate-heal-regate orchestration as specified in build_gate_healing options.
//
// ## Execution Flow (Phase G: Pre-run Gate Only)
//
// executeWithHealing disables per-step pre-gate in Runner.Run calls:
//
//  1. Run pre-mod gate via runGateWithHealing (handles healing if gate fails)
//  2. Clone manifest into manifestForMainMod with Gate disabled and Hydration cleared
//  3. Execute main mod via runner.Run(manifestForMainMod) — container-only, no gate
//  4. If main mod succeeds (ExitCode == 0), run post-mod gate via runGateWithHealing
//  5. Return execution result with full gate history (pre-gate + re-gates + post-gate)
//
// This ensures Runner.Run is used only for container execution during steps, while all
// gate failures come exclusively from runGateWithHealing calls.
//
// ## Post-Mod Gate
//
// When the main mod completes with ExitCode == 0, executeWithHealing invokes
// runGateWithHealing with gatePhase="post" to validate the workspace after
// modifications. This ensures the same healing behavior for both pre- and
// post-mod gates, keeping gate orchestration consistent.
//
// Post-mod gate metadata is appended to the ReGates slice and the final gate
// result is stored in result.BuildGate for downstream telemetry.
//
// ## Repo+Diff Verification Model
//
// Healing verification uses a repo+diff model:
//
//   - The workspace is initialized by cloning repo_url at ref (see manifest hydration).
//   - Healing mods modify the workspace in-place; changes accumulate as diffs.
//   - Re-gate validation runs against workspace = repo_url+ref + healing changes.
//
// ## Configuration
//
// Healing configuration is specified via build_gate_healing option:
//   - retries (int): maximum number of healing attempts (default: 1)
//   - mods ([]map): list of healing mod specs (image, command, env)
//
// The healing loop injects additional environment variables into healing containers:
//   - PLOY_HOST_WORKSPACE: host filesystem path to workspace for in-container tooling
//   - PLOY_SERVER_URL, PLOY_*_CERT_PATH: server connection details for API access
//   - PLOY_REPO_URL, PLOY_BASE_REF, PLOY_TARGET_REF, PLOY_COMMIT_SHA: repo metadata
//     enabling healers to derive the same Git baseline used by the Mods run
//
// The function also mounts node TLS certificates into healing containers to enable
// authenticated API calls to the control plane for artifact uploads.
//
// The stepIndex parameter is used for logging and diff upload correlation in multi-step runs.
func (r *runController) executeWithHealing(
	ctx context.Context,
	runner step.Runner,
	req StartRunRequest,
	manifest contracts.StepManifest,
	workspace string,
	outDir string,
	inDir *string,
	stepIndex int,
) (executionResult, error) {
	// Phase G: Run pre-mod gate via runGateWithHealing (not via Runner.Run).
	// This centralizes all gate execution in runGateWithHealing, ensuring gate failures
	// are handled uniformly with healing support. Runner.Run is reserved for container execution.
	// Pass stepIndex so gate history and stats remain aligned with the Mods step index.
	preGate, preReGates, preActionSummary, preGateErr := r.runGateWithHealing(
		ctx, runner, req, manifest, workspace, outDir, inDir, "pre", stepIndex,
	)

	// Build the initial ReGates slice from any pre-mod healing attempts.
	var reGates []gateRunMetadata
	reGates = append(reGates, preReGates...)

	// If pre-mod gate failed (with or without healing), return the failure.
	if preGateErr != nil {
		// Construct a minimal Result to hold gate metadata for downstream stats.
		result := step.Result{}
		if preGate != nil {
			result.BuildGate = preGate.Metadata
		}
		return executionResult{
			Result:        result,
			PreGate:       preGate,
			ReGates:       reGates,
			ActionSummary: preActionSummary,
		}, preGateErr
	}

	// Pre-mod gate passed. Clone manifest for main mod execution with gate disabled.
	// Set Gate.Enabled=false and clear Inputs[i].Hydration entries so Runner.Run performs
	// container execution only.
	manifestForMainMod := manifest
	manifestForMainMod.Gate = &contracts.StepGateSpec{Enabled: false}

	// Clear Hydration on each input to skip re-hydration (workspace already hydrated by pre-gate).
	if len(manifestForMainMod.Inputs) > 0 {
		inputs := make([]contracts.StepInput, len(manifestForMainMod.Inputs))
		copy(inputs, manifestForMainMod.Inputs)
		for i := range inputs {
			inputs[i].Hydration = nil
		}
		manifestForMainMod.Inputs = inputs
	}

	// Execute main mod container via Runner.Run. Gate is disabled, so this call
	// will not produce ErrBuildGateFailed — it only runs the container.
	// Pass RunID directly for consistent labeling and telemetry.
	result, err := runner.Run(ctx, step.Request{
		RunID:     req.RunID,
		Manifest:  manifestForMainMod,
		Workspace: workspace,
		OutDir:    outDir,
		InDir:     *inDir,
	})

	// Propagate the final pre-mod gate result into result.BuildGate for downstream stats.
	// When healing occurred, the final gate is the last successful re-gate (preReGates).
	// When no healing occurred, the final gate is the initial pre-gate (preGate).
	if result.BuildGate == nil {
		if len(preReGates) > 0 {
			// Healing occurred; use the last re-gate result (the successful one).
			result.BuildGate = preReGates[len(preReGates)-1].Metadata
		} else if preGate != nil {
			// No healing; use initial pre-gate result.
			result.BuildGate = preGate.Metadata
		}
	}

	// Handle execution error (not a gate failure since gate is disabled).
	if err != nil {
		return executionResult{
			Result:        result,
			PreGate:       preGate,
			ReGates:       reGates,
			ActionSummary: preActionSummary,
		}, err
	}

	// Run post-mod gate only if main mod succeeded (ExitCode == 0).
	// This validates the workspace after modifications using the same healing behavior
	// as pre-mod gates, keeping gate orchestration consistent.
	// Pass stepIndex so post-mod gate history and stats remain aligned with the Mods step index.
	if result.ExitCode == 0 {
		postGate, postReGates, postActionSummary, postErr := r.runGateWithHealing(
			ctx, runner, req, manifest, workspace, outDir, inDir, "post", stepIndex,
		)
		// Append initial post-gate run to history.
		if postGate != nil {
			reGates = append(reGates, *postGate)
		}
		// Append any post-mod healing re-gates to history.
		reGates = append(reGates, postReGates...)

		// Update result.BuildGate to reflect the final post-mod gate outcome.
		// This provides downstream telemetry with the canonical gate result.
		if len(postReGates) > 0 {
			// Use the last re-gate result (final healing attempt).
			result.BuildGate = postReGates[len(postReGates)-1].Metadata
		} else if postGate != nil {
			// No re-gates; use initial post-gate result.
			result.BuildGate = postGate.Metadata
		}

		// Prefer post-gate action_summary; fall back to pre-gate.
		actionSummary := postActionSummary
		if actionSummary == "" {
			actionSummary = preActionSummary
		}
		return executionResult{
			Result:        result,
			PreGate:       preGate,
			ReGates:       reGates,
			ActionSummary: actionSummary,
		}, postErr
	}

	// Main mod exited with non-zero code; no post-gate runs.
	return executionResult{
		Result:        result,
		PreGate:       preGate,
		ReGates:       reGates,
		ActionSummary: preActionSummary,
	}, nil
}

// parseBugSummary reads /out/codex-last.txt and extracts the "bug_summary" field
// from a JSON one-liner. Returns an empty string if the file is missing, unreadable,
// or does not contain a bug_summary field.
func parseBugSummary(outDir string) string {
	return parseCodexLastField(outDir, "bug_summary")
}

// parseActionSummary reads /out/codex-last.txt and extracts the "action_summary"
// field from a JSON one-liner. Returns an empty string if the file is missing,
// unreadable, or does not contain an action_summary field.
func parseActionSummary(outDir string) string {
	return parseCodexLastField(outDir, "action_summary")
}

// parseCodexLastField reads codex-last.txt from outDir and extracts a named string
// field from the JSON content. The file is expected to contain one or more lines;
// each line is tried as a JSON object. The first line containing the requested field
// wins. The returned value is trimmed and truncated to 200 characters.
func parseCodexLastField(outDir, field string) string {
	data, err := os.ReadFile(filepath.Join(outDir, "codex-last.txt"))
	if err != nil {
		return ""
	}

	truncateOneLine := func(s string, maxRunes int) string {
		s = strings.TrimSpace(s)
		s = strings.ReplaceAll(s, "\r", " ")
		s = strings.ReplaceAll(s, "\n", " ")
		if maxRunes <= 0 {
			return ""
		}
		if utf8.RuneCountInString(s) <= maxRunes {
			return s
		}
		// Reserve 1 rune for an ellipsis.
		if maxRunes == 1 {
			return "…"
		}
		r := []rune(s)
		return string(r[:maxRunes-1]) + "…"
	}

	// Try each line as a potential JSON object.
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] != '{' {
			continue
		}
		var obj map[string]interface{}
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			continue
		}
		if val, ok := obj[field]; ok {
			if s, ok := val.(string); ok {
				return truncateOneLine(s, 200)
			}
		}
	}

	// If line-by-line didn't work, try the entire content as a single JSON object
	// (in case the file is a single-line JSON without trailing newline).
	var obj map[string]interface{}
	if err := json.Unmarshal(data, &obj); err == nil {
		if val, ok := obj[field]; ok {
			if s, ok := val.(string); ok {
				return truncateOneLine(s, 200)
			}
		}
	}

	return ""
}
