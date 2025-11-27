// execution_healing.go isolates gate-heal-regate execution complexity.
//
// This file contains executeWithHealing, which implements the retry loop for
// healing mods when gate validation fails. It orchestrates pre-gate execution,
// healing mod execution on gate failure, and re-gate validation. The healing
// logic is separated from core orchestration to maintain clear boundaries
// between run lifecycle (orchestrator) and healing retry mechanics.
//
// ## HTTP Build Gate and Docker Gate Consistency
//
// The node agent maintains consistent gate behavior whether validation runs via:
//   - The HTTP Build Gate API (POST /v1/buildgate/validate)
//   - The Docker-based GateExecutor (gate_docker.go)
//
// Key consistency guarantees:
//
//  1. Re-gate execution: After healing mods complete, the node agent ALWAYS
//     re-runs the gate via runner.Gate.Execute (the Docker GateExecutor).
//     This ensures the canonical gate result is produced by the node agent,
//     not by in-container scripts that may call the HTTP Build Gate API.
//
//  2. Full gate history capture: The node agent records all gate executions:
//     - PreGate: The initial gate run before healing (BuildGateStageMetadata)
//     - ReGates: All subsequent re-gate attempts after each healing iteration
//     This history enables telemetry, debugging, and audit trails.
//
//  3. Healing mod flexibility: Healing containers MAY call the HTTP Build Gate
//     API directly for intermediate validation (e.g., testing if a fix works
//     before committing). However, these in-container calls are advisory only.
//     The authoritative gate result is always from the node agent's re-gate.
//     Note: Direct HTTP Build Gate calls from healing mods are now DISCOURAGED
//     for mods-codex; the node agent handles all gate orchestration.
//
//  4. Workspace semantics: Both gate execution paths use identical semantics:
//     - HTTP API: Validates repo_url+ref with optional diff_patch parameter
//     - Docker gate: Validates workspace directory (repo_url+ref + modifications)
//     The Docker gate is semantically equivalent to HTTP API with diff_patch.
//
// ## Repo+Diff Build Gate Semantics
//
// Healing verification aligns with the HTTP Build Gate API's repo+diff model:
//
//   - Initial workspace: The Build Gate validates code cloned from repo_url+ref
//     (see buildgate_executor.go). Healing mods operate on this same workspace,
//     which contains the repository at the specified ref.
//
//   - Healing modifications: Healing containers modify the workspace in-place.
//     Each healing mod's changes accumulate as diffs on top of the repo baseline.
//     Per-step diff capture (uploadHealingModDiff) records these changes for
//     multi-node rehydration scenarios.
//
//   - Re-gate verification: After healing completes, the gate re-runs against
//     the same workspace (repo_url+ref + healing modifications). Conceptually,
//     this is equivalent to calling the Build Gate HTTP API with:
//     POST /v1/buildgate/validate
//     {"repo_url": "...", "ref": "...", "diff_patch": "<accumulated-changes>"}
//     The in-process re-gate skips network round-trips since the workspace
//     already contains the modified state.
//
//   - Diff chain semantics: Workspace state at any point equals base clone plus
//     an ordered sequence of diffs. This model matches Mods multi-step execution
//     where each step's changes are captured and can be replayed for rehydration.
//
// This alignment ensures healing verification and the HTTP Build Gate API use
// consistent semantics: both validate repo_url+ref baseline plus optional changes.
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

// gateRunMetadata captures gate execution metadata and timing for stats reporting.
// It wraps gate result metadata with the duration of the gate execution to enable
// detailed observability reporting of gate performance across pre-gate and re-gate phases.
//
// This structure is used to maintain a complete history of all gate executions,
// ensuring consistency between HTTP Build Gate API behavior and Docker gate behavior.
// Both execution paths produce equivalent BuildGateStageMetadata that can be
// compared and audited.
type gateRunMetadata struct {
	// Metadata contains the full BuildGateStageMetadata from the gate execution,
	// including StaticChecks, LogFindings, LogsText, LogDigest, and Resources.
	// This is the canonical gate result produced by the node agent's GateExecutor.
	Metadata *contracts.BuildGateStageMetadata
	// DurationMs records the wall-clock duration of this gate execution in milliseconds.
	DurationMs int64
}

// executionResult wraps step.Result with additional gate run history for stats.
// This type enriches the standard execution result with gate-specific telemetry that
// tracks the initial gate attempt and any subsequent re-gate attempts after healing.
//
// The gate history (PreGate + ReGates) provides a complete audit trail of all
// gate validations performed by the node agent during the healing workflow.
// This ensures that:
//   - The node agent always re-runs the gate after healing (not relying on in-container checks)
//   - All gate results are captured for telemetry and debugging
//   - Gate behavior is consistent whether using HTTP Build Gate API or Docker gate
type executionResult struct {
	step.Result
	// PreGate captures the initial gate run metadata (if gate was executed).
	// When a build gate is configured, this field records the outcome and timing
	// of the gate check that runs before the main mod execution begins.
	// This is always populated when Gate.Enabled=true, regardless of whether
	// the gate passes or fails.
	PreGate *gateRunMetadata
	// ReGates captures re-gate attempts after healing (if healing was attempted).
	// Each entry corresponds to one re-gate run following a healing mod execution,
	// allowing telemetry to track healing efficacy across multiple retry attempts.
	// The slice length equals the number of healing retry iterations executed.
	// Combined with PreGate, this provides the full gate history for the run.
	ReGates []gateRunMetadata
}

// uploadHealingModDiff generates and uploads diff after a single healing mod execution.
// It enriches the diff summary with healing-specific metadata (mod_type, mod_index, healing_attempt)
// to distinguish healing mod diffs from main mod diffs in the database.
//
// This per-step diff capture enables multi-node rehydration where each node can reconstruct
// the workspace state at any point in the healing sequence by applying an ordered chain of diffs.
func (r *runController) uploadHealingModDiff(ctx context.Context, runID, stageID, workspace string, healResult step.Result, modIndex, healingAttempt int) {
	// Retrieve the diff generator from runtime components.
	// Since healing reuses the same runner, we need to access the diff generator.
	// The diff generator is initialized in initializeRuntime and reused across healing steps.
	diffGenerator := r.createDiffGenerator()
	if diffGenerator == nil {
		return
	}

	// Generate workspace diff for this healing mod step.
	diffBytes, err := diffGenerator.Generate(ctx, workspace)
	if err != nil {
		slog.Error("failed to generate healing mod diff", "run_id", runID, "mod_index", modIndex, "error", err)
		return
	}

	if len(diffBytes) == 0 {
		// No changes from this healing mod; skip upload.
		return
	}

	// Build diff summary with healing mod metadata for database storage.
	// The mod_type field distinguishes healing mod diffs from main mod diffs.
	// The mod_index and healing_attempt fields enable ordering and rehydration.
	summary := types.DiffSummary{
		"mod_type":        "healing",
		"mod_index":       modIndex,
		"healing_attempt": healingAttempt,
		"exit_code":       healResult.ExitCode,
		"timings": map[string]interface{}{
			"hydration_duration_ms":  healResult.Timings.HydrationDuration.Milliseconds(),
			"execution_duration_ms":  healResult.Timings.ExecutionDuration.Milliseconds(),
			"build_gate_duration_ms": healResult.Timings.BuildGateDuration.Milliseconds(),
			"diff_duration_ms":       healResult.Timings.DiffDuration.Milliseconds(),
			"total_duration_ms":      healResult.Timings.TotalDuration.Milliseconds(),
		},
	}

	// Upload diff with healing metadata to control plane.
	diffUploader, err := NewDiffUploader(r.cfg)
	if err != nil {
		slog.Error("failed to create diff uploader for healing mod", "run_id", runID, "mod_index", modIndex, "error", err)
		return
	}

	// Healing mod diffs don't have a step_index (they are intermediate diffs within a step).
	// Pass nil for step_index to indicate this is a healing diff, not a per-step diff.
	if err := diffUploader.UploadDiff(ctx, runID, stageID, diffBytes, summary, nil); err != nil {
		slog.Error("failed to upload healing mod diff", "run_id", runID, "mod_index", modIndex, "error", err)
		return
	}

	slog.Info("healing mod diff uploaded successfully", "run_id", runID, "mod_index", modIndex, "size", len(diffBytes))
}

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
//     - Upload healing mod diffs for rehydration
//     - Re-run gate after healing mods complete
//     - If gate passes, return success
//     d. If all retries exhausted, return ErrBuildGateFailed
//
// ## Repo+Diff Verification Model
//
// Healing verification uses the same repo+diff semantics as the HTTP Build Gate API:
// the workspace contains repo_url+ref plus accumulated healing modifications.
// This is semantically equivalent to POST /v1/buildgate/validate with diff_patch.
//
// ## Configuration
//
// Healing configuration is specified via build_gate_healing option in req.Options:
//   - retries (int): maximum number of healing attempts (default: 1)
//   - mods ([]map): list of healing mod specs (image, command, env)
func (r *runController) runGateWithHealing(
	ctx context.Context,
	runner step.Runner,
	req StartRunRequest,
	manifest contracts.StepManifest,
	workspace, outDir string,
	inDir *string,
	gatePhase string, // "pre" or "post"
) (*gateRunMetadata, []gateRunMetadata, error) {
	// Resolve gate spec from manifest (with backward compat for deprecated Shift field).
	gateSpec := manifest.Gate
	//lint:ignore SA1019 Backward compatibility: support deprecated Shift by mapping to Gate.
	if gateSpec == nil && manifest.Shift != nil {
		gateSpec = &contracts.StepGateSpec{
			Enabled: manifest.Shift.Enabled, //lint:ignore SA1019 compat field access
			Profile: manifest.Shift.Profile, //lint:ignore SA1019 compat field access
			Env:     manifest.Shift.Env,     //lint:ignore SA1019 compat field access
		}
	}

	// If gate is disabled or executor unavailable, return immediately.
	if runner.Gate == nil || gateSpec == nil || !gateSpec.Enabled {
		return nil, nil, nil
	}

	// Execute initial gate.
	gateStart := time.Now()
	gateMetadata, gateErr := runner.Gate.Execute(ctx, gateSpec, workspace)
	gateDuration := time.Since(gateStart)

	if gateErr != nil {
		return nil, nil, fmt.Errorf("gate execution failed: %w", gateErr)
	}

	// Capture initial gate metadata for stats.
	initialGate := &gateRunMetadata{
		Metadata:   gateMetadata,
		DurationMs: gateDuration.Milliseconds(),
	}

	// Check if gate passed.
	gatePassed := false
	if len(gateMetadata.StaticChecks) > 0 {
		gatePassed = gateMetadata.StaticChecks[0].Passed
	}

	if gatePassed {
		slog.Info("gate passed", "run_id", req.RunID, "phase", gatePhase)
		return initialGate, nil, nil
	}

	// Gate failed. Check if healing is configured.
	typedOpts := parseRunOptions(req.Options)
	if typedOpts.Healing == nil {
		// No healing configured; return the gate failure.
		slog.Info("gate failed, no healing configured", "run_id", req.RunID, "phase", gatePhase)
		return initialGate, nil, step.ErrBuildGateFailed
	}

	healingConfig := typedOpts.Healing
	if len(healingConfig.Mods) == 0 {
		slog.Warn("build_gate_healing configured but no mods provided", "run_id", req.RunID, "phase", gatePhase)
		return initialGate, nil, step.ErrBuildGateFailed
	}

	retries := healingConfig.Retries

	// Create /in directory if not already created (for build-gate.log).
	if *inDir == "" {
		tmpInDir, dirErr := os.MkdirTemp("", "ploy-mod-in-*")
		if dirErr != nil {
			slog.Error("failed to create /in directory for healing", "run_id", req.RunID, "error", dirErr)
			return initialGate, nil, step.ErrBuildGateFailed
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

	// Track Codex session state across healing loop iterations.
	// When a Codex-based healing mod writes codex-session.txt to /out, the agent
	// reads and persists this session ID to /in for subsequent attempts.
	var codexSession string
	var codexRequestedValidation bool

	// Attempt healing loop.
	// Note: This is a domain-specific healing retry loop (not a transient error retry).
	// It executes healing mods between gate validation attempts based on user-configured retries.
	// We intentionally do not use internal/workflow/backoff here because:
	//  1. This is not a retry-on-failure pattern (each iteration does useful work: running healing mods).
	//  2. The retry count is user-configured (manifest-specified retries parameter).
	//  3. No exponential backoff is needed; each healing attempt runs immediately after healing mods complete.
	for attempt := 1; attempt <= retries; attempt++ {
		slog.Info("starting healing attempt", "run_id", req.RunID, "attempt", attempt, "max_retries", retries, "phase", gatePhase)

		// Execute each healing mod in sequence using typed HealingMod structs.
		for idx, mod := range healingConfig.Mods {
			// Pass codexSession to enable CODEX_RESUME=1 injection for Codex-based healers.
			healManifest, buildErr := buildHealingManifest(req, mod, idx, codexSession)
			if buildErr != nil {
				slog.Error("failed to build healing manifest", "run_id", req.RunID, "mod_index", idx, "error", buildErr)
				return initialGate, reGates, fmt.Errorf("build healing manifest[%d]: %w", idx, buildErr)
			}

			slog.Info("executing healing mod", "run_id", req.RunID, "attempt", attempt, "mod_index", idx, "image", healManifest.Image, "phase", gatePhase)

			// Provide host workspace path for in-container build verification tools.
			if healManifest.Env == nil {
				healManifest.Env = map[string]string{}
			}
			healManifest.Env["PLOY_HOST_WORKSPACE"] = workspace
			// Inject server connection details for buildgate API access from healing containers.
			healManifest.Env["PLOY_SERVER_URL"] = r.cfg.ServerURL
			healManifest.Env["PLOY_CA_CERT_PATH"] = "/etc/ploy/certs/ca.crt"
			healManifest.Env["PLOY_CLIENT_CERT_PATH"] = "/etc/ploy/certs/client.crt"
			healManifest.Env["PLOY_CLIENT_KEY_PATH"] = "/etc/ploy/certs/client.key"
			if token := os.Getenv("PLOY_API_TOKEN"); token != "" {
				healManifest.Env["PLOY_API_TOKEN"] = token
			} else if !r.cfg.HTTP.TLS.Enabled {
				if data, err := os.ReadFile(bearerTokenPath()); err == nil {
					if token := strings.TrimSpace(string(data)); token != "" {
						healManifest.Env["PLOY_API_TOKEN"] = token
					}
				} else {
					slog.Warn("healing: failed to read bearer token for PLOY_API_TOKEN fallback", "error", err)
				}
			}

			// Mount node's TLS certificates into healing container for buildgate API access.
			if healManifest.Options == nil {
				healManifest.Options = make(map[string]any)
			}
			healManifest.Options["ploy_ca_cert_path"] = r.cfg.HTTP.TLS.CAPath
			healManifest.Options["ploy_client_cert_path"] = r.cfg.HTTP.TLS.CertPath
			healManifest.Options["ploy_client_key_path"] = r.cfg.HTTP.TLS.KeyPath

			// Run the healing mod container.
			healResult, healErr := runner.Run(ctx, step.Request{
				TicketID:  types.TicketID(req.RunID),
				Manifest:  healManifest,
				Workspace: workspace,
				OutDir:    outDir,
				InDir:     *inDir,
			})

			if healErr != nil {
				slog.Error("healing mod execution failed", "run_id", req.RunID, "mod_index", idx, "error", healErr)
				return initialGate, reGates, fmt.Errorf("healing mod[%d] failed: %w", idx, healErr)
			}

			if healResult.ExitCode != 0 {
				slog.Warn("healing mod exited with non-zero code", "run_id", req.RunID, "mod_index", idx, "exit_code", healResult.ExitCode)
				// Continue with remaining mods; we'll check gate after all mods run.
			}

			// Upload /out artifacts for this healing mod if present.
			stageID, _ := manifest.OptionString("stage_id")
			if uploadErr := r.uploadOutDir(ctx, req.RunID.String(), stageID, outDir); uploadErr != nil {
				slog.Warn("failed to upload /out for healing mod", "run_id", req.RunID, "mod_index", idx, "error", uploadErr)
			}

			// Per-step diff capture: Generate and upload diff after each healing mod step.
			r.uploadHealingModDiff(ctx, req.RunID.String(), stageID, workspace, healResult, idx, attempt)

			// Read Codex session and sentinel artifacts from /out for session propagation.
			if sessionBytes, readErr := os.ReadFile(filepath.Join(outDir, "codex-session.txt")); readErr == nil {
				if session := strings.TrimSpace(string(sessionBytes)); session != "" {
					codexSession = session
					slog.Info("healing: captured codex session from /out", "run_id", req.RunID, "mod_index", idx, "session_id", codexSession)
				}
			}

			// Check for sentinel file indicating Codex requested build validation.
			if _, statErr := os.Stat(filepath.Join(outDir, "request_build_validation")); statErr == nil {
				codexRequestedValidation = true
				slog.Info("healing: codex requested build validation", "run_id", req.RunID, "mod_index", idx)
			}
		}

		// Persist codex-session.txt to /in for subsequent healing attempts.
		if codexSession != "" && *inDir != "" {
			sessionPath := filepath.Join(*inDir, "codex-session.txt")
			if writeErr := os.WriteFile(sessionPath, []byte(codexSession), 0o644); writeErr != nil {
				slog.Warn("healing: failed to persist codex-session.txt into /in", "run_id", req.RunID, "error", writeErr)
			} else {
				slog.Info("healing: persisted codex-session.txt to /in for resume", "run_id", req.RunID, "session_id", codexSession)
			}
		}

		// Log sentinel state for observability (gate semantics unchanged).
		_ = codexRequestedValidation

		// Re-run the gate after healing mods complete.
		// CRITICAL: The node agent ALWAYS re-runs the gate via runner.Gate.Execute,
		// even if healing mods called the HTTP Build Gate API directly.
		slog.Info("re-running build gate after healing", "run_id", req.RunID, "attempt", attempt, "phase", gatePhase)

		regateStart := time.Now()
		reGateMetadata, regateErr := runner.Gate.Execute(ctx, gateSpec, workspace)
		regateDuration := time.Since(regateStart)

		if regateErr != nil {
			slog.Error("re-gate execution failed", "run_id", req.RunID, "error", regateErr)
			return initialGate, reGates, fmt.Errorf("re-gate execution failed: %w", regateErr)
		}

		// Capture re-gate metadata for stats.
		reGates = append(reGates, gateRunMetadata{
			Metadata:   reGateMetadata,
			DurationMs: regateDuration.Milliseconds(),
		})

		// Check if gate passed.
		regatePassed := false
		if len(reGateMetadata.StaticChecks) > 0 {
			regatePassed = reGateMetadata.StaticChecks[0].Passed
		}

		if regatePassed {
			slog.Info("build gate passed after healing", "run_id", req.RunID, "attempt", attempt, "phase", gatePhase)
			return initialGate, reGates, nil
		}

		// Re-gate failed; continue to next retry or exit when exhausted.
		slog.Warn("build gate still failing after healing", "run_id", req.RunID, "attempt", attempt, "phase", gatePhase)
	}

	// Retries exhausted; return the gate failure.
	slog.Error("healing retries exhausted, build gate still failing", "run_id", req.RunID, "phase", gatePhase)
	return initialGate, reGates, fmt.Errorf("%w: healing retries exhausted", step.ErrBuildGateFailed)
}

// executeWithHealing runs the main step with optional healing loop when the build gate fails.
// It handles the gate-heal-regate orchestration as specified in build_gate_healing options.
//
// ## Healing Workflow
//
//  1. Execute initial run with pre-mod build gate check
//  2. If gate fails and healing is configured:
//     a. Create /in directory and persist build-gate.log for healer inspection
//     b. Execute each healing mod in sequence (with workspace, /out, /in mounts)
//     c. Re-run build gate after healing mods complete
//     d. If gate passes, execute main mod without re-running gate or hydration
//     e. If gate fails, retry up to configured retries limit
//  3. If main mod succeeds (ExitCode == 0), run post-mod gate with healing
//  4. Return execution result with full gate history (pre-gate + re-gates)
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
// Healing verification uses the same repo+diff semantics as the HTTP Build Gate API:
//
//   - The workspace is initialized by cloning repo_url at ref (see manifest hydration).
//   - Healing mods modify the workspace in-place; changes accumulate as diffs.
//   - Re-gate validation runs against workspace = repo_url+ref + healing changes.
//   - This is semantically equivalent to HTTP Build Gate with diff_patch parameter,
//     but executed in-process to avoid network overhead for repeated validation.
//
// Healing containers can also invoke the HTTP Build Gate API directly using
// the injected PLOY_* environment variables (see PLOY_HOST_WORKSPACE,
// PLOY_SERVER_URL env vars below). The system re-runs the gate regardless of
// any in-container verification results.
//
// ## Configuration
//
// Healing configuration is specified via build_gate_healing option:
//   - retries (int): maximum number of healing attempts (default: 1)
//   - mods ([]map): list of healing mod specs (image, command, env)
//
// The healing loop injects additional environment variables into healing containers:
//   - PLOY_HOST_WORKSPACE: host filesystem path to workspace for in-container tooling
//   - PLOY_SERVER_URL, PLOY_*_CERT_PATH: server connection details for buildgate API access
//   - PLOY_REPO_URL, PLOY_BASE_REF, PLOY_TARGET_REF, PLOY_COMMIT_SHA: repo metadata
//     enabling healers to derive the same Git baseline used by the Mods run
//
// The function also mounts node TLS certificates into healing containers to enable
// authenticated API calls to the control plane for gate verification and artifact uploads.
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
	// First execution attempt (includes pre-mod gate check via Runner.Run).
	// Runner.Run internally executes the gate before the container if Gate.Enabled=true.
	result, err := runner.Run(ctx, step.Request{
		TicketID:  types.TicketID(req.RunID),
		Manifest:  manifest,
		Workspace: workspace,
		OutDir:    outDir,
		InDir:     *inDir,
	})

	// Capture pre-gate metadata for stats (if gate was executed by Runner.Run).
	var preGate *gateRunMetadata
	if result.BuildGate != nil {
		preGate = &gateRunMetadata{
			Metadata:   result.BuildGate,
			DurationMs: result.Timings.BuildGateDuration.Milliseconds(),
		}
	}

	// If execution succeeded or error is not a build gate failure, check for post-mod gate.
	if err == nil || !errors.Is(err, step.ErrBuildGateFailed) {
		// Run post-mod gate only if main mod succeeded (ExitCode == 0) and no execution error.
		// This validates the workspace after modifications using the same healing behavior
		// as pre-mod gates, keeping gate orchestration consistent.
		if err == nil && result.ExitCode == 0 {
			postGate, postReGates, postErr := r.runGateWithHealing(
				ctx, runner, req, manifest, workspace, outDir, inDir, "post",
			)
			// Build combined ReGates slice: pre-mod re-gates (empty for success path) + post-mod gates.
			var reGates []gateRunMetadata
			if postGate != nil {
				// Append initial post-gate run to history.
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

			return executionResult{
				Result:  result,
				PreGate: preGate,
				ReGates: reGates,
			}, postErr
		}
		return executionResult{Result: result, PreGate: preGate}, err
	}

	// Build gate failed during Runner.Run. Delegate healing+re-gate orchestration
	// to runGateWithHealing which handles /in directory creation, build-gate.log,
	// healing mods execution, and re-gate loops.
	//
	// Note: We pass the gate metadata from the initial failure (result.BuildGate) into
	// runHealingAfterGateFailure, which handles the healing loop and re-gates. This
	// approach preserves the existing test expectations where PreGate captures the
	// initial Runner.Run gate result (with its timing from result.Timings).

	healErr := r.runHealingAfterGateFailure(ctx, runner, req, manifest, workspace, outDir, inDir, result.BuildGate)

	// runHealingAfterGateFailure always returns a healingResult (as error) with re-gate history.
	// Extract it to populate the executionResult with gate stats.
	var hr *healingResult
	if errors.As(healErr, &hr) {
		// Healing was attempted. Check if it succeeded (MainResult != nil) or failed.
		if hr.MainResult != nil {
			// Healing succeeded and main mod ran. Start from the healed main result and
			// propagate the final pre-mod gate outcome into BuildGate when present.
			mainResult := *hr.MainResult
			reGates := hr.ReGates

			// When healing succeeds (gate eventually passes before the main mod runs),
			// FinalGate captures the last successful healing re-gate. Use this as the
			// current BuildGate so downstream stats (final_gate) reflect the healed
			// gate result even when no post-mod gate runs (e.g., main mod exits != 0).
			if hr.FinalGate != nil {
				mainResult.BuildGate = hr.FinalGate
			}

			// Run post-mod gate only if main mod succeeded (ExitCode == 0) and no error.
			// This validates the workspace after modifications, consistent with pre-mod gate behavior.
			if hr.Err == nil && mainResult.ExitCode == 0 {
				postGate, postReGates, postErr := r.runGateWithHealing(
					ctx, runner, req, manifest, workspace, outDir, inDir, "post",
				)
				// Append post-mod gate metadata to history (after pre-mod healing re-gates).
				if postGate != nil {
					reGates = append(reGates, *postGate)
				}
				reGates = append(reGates, postReGates...)

				// Update mainResult.BuildGate to reflect the final post-mod gate outcome.
				if len(postReGates) > 0 {
					mainResult.BuildGate = postReGates[len(postReGates)-1].Metadata
				} else if postGate != nil {
					mainResult.BuildGate = postGate.Metadata
				}

				return executionResult{
					Result:  mainResult,
					PreGate: preGate,
					ReGates: reGates,
				}, postErr
			}

			// Main mod succeeded but no post-gate needed (or mod had non-zero exit).
			return executionResult{
				Result:  mainResult,
				PreGate: preGate,
				ReGates: reGates,
			}, hr.Err
		}
		// Healing failed (mod execution error or retries exhausted).
		return executionResult{
			Result:  result,
			PreGate: preGate,
			ReGates: hr.ReGates,
		}, hr.Err
	}

	// errHealingNotConfigured: no healing was attempted, return original error.
	if errors.Is(healErr, errHealingNotConfigured) {
		return executionResult{Result: result, PreGate: preGate}, err
	}

	// Unexpected error path (should not happen with current implementation).
	return executionResult{Result: result, PreGate: preGate}, healErr
}

// errHealingNotConfigured indicates healing was not attempted because no healing config exists.
var errHealingNotConfigured = errors.New("healing not configured")

// healingResult wraps the outcome of a healing attempt for structured error handling.
// When healing succeeds, Err is nil and ReGates contains all re-gate metadata.
// When healing fails, Err contains the underlying error and ReGates contains partial history.
type healingResult struct {
	ReGates    []gateRunMetadata
	FinalGate  *contracts.BuildGateStageMetadata
	MainResult *step.Result // non-nil when main mod was executed after successful healing
	Err        error
}

// Error implements the error interface for healingResult.
func (hr *healingResult) Error() string {
	if hr.Err != nil {
		return hr.Err.Error()
	}
	return "healing completed"
}

// Unwrap returns the underlying error for errors.Is/As compatibility.
func (hr *healingResult) Unwrap() error {
	return hr.Err
}

// runHealingAfterGateFailure orchestrates healing mods and re-gates after an initial gate failure.
// This function is called by executeWithHealing when Runner.Run returns ErrBuildGateFailed.
//
// It mirrors the healing + re-gate loop used by runGateWithHealing, but starts from an
// already-executed (and failed) pre-mod gate. Instead of re-running the initial gate,
// it accepts the failed gate metadata and writes build-gate.log before entering
// the healing loop.
//
// Returns:
//   - errHealingNotConfigured if no healing config exists
//   - healingResult (as error) containing re-gate history and main mod result on success
//   - healingResult (as error) containing partial history and underlying error on failure
func (r *runController) runHealingAfterGateFailure(
	ctx context.Context,
	runner step.Runner,
	req StartRunRequest,
	manifest contracts.StepManifest,
	workspace, outDir string,
	inDir *string,
	failedGateMetadata *contracts.BuildGateStageMetadata,
) error {
	// Check if healing is configured.
	typedOpts := parseRunOptions(req.Options)
	if typedOpts.Healing == nil {
		return errHealingNotConfigured
	}

	healingConfig := typedOpts.Healing
	if len(healingConfig.Mods) == 0 {
		slog.Warn("build_gate_healing configured but no mods provided", "run_id", req.RunID)
		return errHealingNotConfigured
	}

	retries := healingConfig.Retries

	// Create /in directory if not already created (for build-gate.log).
	if *inDir == "" {
		tmpInDir, dirErr := os.MkdirTemp("", "ploy-mod-in-*")
		if dirErr != nil {
			slog.Error("failed to create /in directory for healing", "run_id", req.RunID, "error", dirErr)
			return &healingResult{Err: step.ErrBuildGateFailed}
		}
		*inDir = tmpInDir
		// Caller handles cleanup via defer.
	}

	// Write build-gate.log to /in for healing containers.
	// Prefer trimmed log view (LogFindings) when available so Codex and
	// other healing mods see a focused failure slice instead of the full truncated gate log.
	if failedGateMetadata != nil {
		logPayload := failedGateMetadata.LogsText
		if len(failedGateMetadata.LogFindings) > 0 {
			if trimmed := strings.TrimSpace(failedGateMetadata.LogFindings[0].Message); trimmed != "" {
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
				slog.Info("build-gate.log persisted to /in for healing", "run_id", req.RunID, "path", inLogPath)
			}
		}
	}

	// Track re-gate runs for stats.
	var reGates []gateRunMetadata

	// Track Codex session state across healing loop iterations.
	var codexSession string
	var codexRequestedValidation bool

	// Resolve gate spec from manifest (with backward compat for deprecated Shift field).
	gateSpec := manifest.Gate
	//lint:ignore SA1019 Backward compatibility: support deprecated Shift by mapping to Gate.
	if gateSpec == nil && manifest.Shift != nil {
		gateSpec = &contracts.StepGateSpec{
			Enabled: manifest.Shift.Enabled, //lint:ignore SA1019 compat field access
			Profile: manifest.Shift.Profile, //lint:ignore SA1019 compat field access
			Env:     manifest.Shift.Env,     //lint:ignore SA1019 compat field access
		}
	}

	// Healing loop: execute healing mods and re-gate after each attempt.
	// This loop mirrors the logic in runGateWithHealing but enters after an initial
	// gate failure (handled by Runner.Run).
	for attempt := 1; attempt <= retries; attempt++ {
		slog.Info("starting healing attempt", "run_id", req.RunID, "attempt", attempt, "max_retries", retries, "phase", "pre")

		// Execute each healing mod in sequence.
		for idx, mod := range healingConfig.Mods {
			healManifest, buildErr := buildHealingManifest(req, mod, idx, codexSession)
			if buildErr != nil {
				slog.Error("failed to build healing manifest", "run_id", req.RunID, "mod_index", idx, "error", buildErr)
				return &healingResult{ReGates: reGates, Err: fmt.Errorf("build healing manifest[%d]: %w", idx, buildErr)}
			}

			slog.Info("executing healing mod", "run_id", req.RunID, "attempt", attempt, "mod_index", idx, "image", healManifest.Image, "phase", "pre")

			// Provide host workspace path and server connection details for healing containers.
			if healManifest.Env == nil {
				healManifest.Env = map[string]string{}
			}
			healManifest.Env["PLOY_HOST_WORKSPACE"] = workspace
			healManifest.Env["PLOY_SERVER_URL"] = r.cfg.ServerURL
			healManifest.Env["PLOY_CA_CERT_PATH"] = "/etc/ploy/certs/ca.crt"
			healManifest.Env["PLOY_CLIENT_CERT_PATH"] = "/etc/ploy/certs/client.crt"
			healManifest.Env["PLOY_CLIENT_KEY_PATH"] = "/etc/ploy/certs/client.key"
			if token := os.Getenv("PLOY_API_TOKEN"); token != "" {
				healManifest.Env["PLOY_API_TOKEN"] = token
			} else if !r.cfg.HTTP.TLS.Enabled {
				if data, err := os.ReadFile(bearerTokenPath()); err == nil {
					if token := strings.TrimSpace(string(data)); token != "" {
						healManifest.Env["PLOY_API_TOKEN"] = token
					}
				} else {
					slog.Warn("healing: failed to read bearer token for PLOY_API_TOKEN fallback", "error", err)
				}
			}

			// Mount node's TLS certificates into healing container.
			if healManifest.Options == nil {
				healManifest.Options = make(map[string]any)
			}
			healManifest.Options["ploy_ca_cert_path"] = r.cfg.HTTP.TLS.CAPath
			healManifest.Options["ploy_client_cert_path"] = r.cfg.HTTP.TLS.CertPath
			healManifest.Options["ploy_client_key_path"] = r.cfg.HTTP.TLS.KeyPath

			// Run the healing mod container.
			healResult, healErr := runner.Run(ctx, step.Request{
				TicketID:  types.TicketID(req.RunID),
				Manifest:  healManifest,
				Workspace: workspace,
				OutDir:    outDir,
				InDir:     *inDir,
			})

			if healErr != nil {
				slog.Error("healing mod execution failed", "run_id", req.RunID, "mod_index", idx, "error", healErr)
				return &healingResult{ReGates: reGates, Err: fmt.Errorf("healing mod[%d] failed: %w", idx, healErr)}
			}

			if healResult.ExitCode != 0 {
				slog.Warn("healing mod exited with non-zero code", "run_id", req.RunID, "mod_index", idx, "exit_code", healResult.ExitCode)
			}

			// Upload /out artifacts for this healing mod.
			stageID, _ := manifest.OptionString("stage_id")
			if uploadErr := r.uploadOutDir(ctx, req.RunID.String(), stageID, outDir); uploadErr != nil {
				slog.Warn("failed to upload /out for healing mod", "run_id", req.RunID, "mod_index", idx, "error", uploadErr)
			}

			// Per-step diff capture.
			r.uploadHealingModDiff(ctx, req.RunID.String(), stageID, workspace, healResult, idx, attempt)

			// Read Codex session and sentinel artifacts from /out.
			if sessionBytes, readErr := os.ReadFile(filepath.Join(outDir, "codex-session.txt")); readErr == nil {
				if session := strings.TrimSpace(string(sessionBytes)); session != "" {
					codexSession = session
					slog.Info("healing: captured codex session from /out", "run_id", req.RunID, "mod_index", idx, "session_id", codexSession)
				}
			}

			if _, statErr := os.Stat(filepath.Join(outDir, "request_build_validation")); statErr == nil {
				codexRequestedValidation = true
				slog.Info("healing: codex requested build validation", "run_id", req.RunID, "mod_index", idx)
			}
		}

		// Persist codex-session.txt to /in for subsequent healing attempts.
		if codexSession != "" && *inDir != "" {
			sessionPath := filepath.Join(*inDir, "codex-session.txt")
			if writeErr := os.WriteFile(sessionPath, []byte(codexSession), 0o644); writeErr != nil {
				slog.Warn("healing: failed to persist codex-session.txt into /in", "run_id", req.RunID, "error", writeErr)
			} else {
				slog.Info("healing: persisted codex-session.txt to /in for resume", "run_id", req.RunID, "session_id", codexSession)
			}
		}

		_ = codexRequestedValidation // For observability; does not affect semantics.

		// Re-run the gate after healing mods complete.
		slog.Info("re-running build gate after healing", "run_id", req.RunID, "attempt", attempt, "phase", "pre")

		if runner.Gate != nil && gateSpec != nil && gateSpec.Enabled {
			regateStart := time.Now()
			reGateMetadata, regateErr := runner.Gate.Execute(ctx, gateSpec, workspace)
			regateDuration := time.Since(regateStart)

			if regateErr != nil {
				slog.Error("re-gate execution failed", "run_id", req.RunID, "error", regateErr)
				return &healingResult{ReGates: reGates, Err: fmt.Errorf("re-gate execution failed: %w", regateErr)}
			}

			// Capture re-gate metadata for stats.
			reGates = append(reGates, gateRunMetadata{
				Metadata:   reGateMetadata,
				DurationMs: regateDuration.Milliseconds(),
			})

			// Check if gate passed.
			gatePassed := false
			if len(reGateMetadata.StaticChecks) > 0 {
				gatePassed = reGateMetadata.StaticChecks[0].Passed
			}

			if gatePassed {
				slog.Info("build gate passed after healing", "run_id", req.RunID, "attempt", attempt, "phase", "pre")

				// Gate passed; proceed to main mod execution.
				// Disable the gate and hydration for the follow-up main mod run.
				manifestForMainMod := manifest
				manifestForMainMod.Gate = &contracts.StepGateSpec{Enabled: false}
				//lint:ignore SA1019 Backward compatibility: also disable deprecated Shift field.
				manifestForMainMod.Shift = nil
				if len(manifestForMainMod.Inputs) > 0 {
					inputs := make([]contracts.StepInput, len(manifestForMainMod.Inputs))
					copy(inputs, manifestForMainMod.Inputs)
					for i := range inputs {
						inputs[i].Hydration = nil
					}
					manifestForMainMod.Inputs = inputs
				}

				// Execute the main mod without re-running gate or hydration.
				mainResult, mainErr := runner.Run(ctx, step.Request{
					TicketID:  types.TicketID(req.RunID),
					Manifest:  manifestForMainMod,
					Workspace: workspace,
					OutDir:    outDir,
					InDir:     *inDir,
				})

				// Return healingResult with success (Err=mainErr which may be nil).
				return &healingResult{
					ReGates:    reGates,
					FinalGate:  reGateMetadata,
					MainResult: &mainResult,
					Err:        mainErr,
				}
			}

			// Re-gate failed; continue to next retry or exit when exhausted.
			slog.Warn("build gate still failing after healing", "run_id", req.RunID, "attempt", attempt, "phase", "pre")
		}
	}

	// Retries exhausted; return the gate failure.
	slog.Error("healing retries exhausted, build gate still failing", "run_id", req.RunID, "phase", "pre")
	return &healingResult{ReGates: reGates, Err: fmt.Errorf("%w: healing retries exhausted", step.ErrBuildGateFailed)}
}
