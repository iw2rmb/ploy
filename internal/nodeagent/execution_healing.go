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
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
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
// C2: Healing diffs are tagged with the same step_index as their parent mod step, enabling
// unified rehydration that includes both mod and healing diffs. The mod_type="healing" field
// distinguishes healing diffs from regular mod diffs when filtering is needed.
//
// This per-step diff capture enables multi-node rehydration where each node can reconstruct
// the workspace state at any point in the healing sequence by applying an ordered chain of diffs.
func (r *runController) uploadHealingModDiff(ctx context.Context, runID, stageID, workspace string, healResult step.Result, modIndex, healingAttempt, stepIndex int) {
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
	// C2: step_index + mod_type enable unified rehydration across mod and healing diffs.
	// - step_index: Same as parent step, so rehydration queries include healing diffs.
	// - mod_type: "healing" distinguishes from regular "mod" diffs for filtering.
	// - mod_index: Index of healing mod within the healing config (for ordering within step).
	// - healing_attempt: Retry iteration (1-based) for debugging and telemetry.
	summary := types.DiffSummary{
		"step_index":      stepIndex, // C2: Tag healing diff with parent step's index.
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

	// C2: Pass step_index so healing diffs are included in rehydration queries.
	// Healing diffs use the same step_index as their parent mod step, enabling
	// unified rehydration that fetches all diffs (mod + healing) up to step k.
	stepIdx := int32(stepIndex)
	if err := diffUploader.UploadDiff(ctx, runID, stageID, diffBytes, summary, &stepIdx); err != nil {
		slog.Error("failed to upload healing mod diff", "run_id", runID, "mod_index", modIndex, "step_index", stepIndex, "error", err)
		return
	}

	slog.Info("healing mod diff uploaded successfully", "run_id", runID, "mod_index", modIndex, "step_index", stepIndex, "size", len(diffBytes))
}

// computeGzippedDiff generates a unified git diff of workspace changes and compresses it.
// Used by HTTP-based re-gates to send accumulated healing changes to remote Build Gate workers.
//
// The diff captures all changes relative to the initial repo_url+ref clone (HEAD), enabling
// remote workers to reconstruct workspace state by cloning repo_url+ref and applying the patch.
//
// Returns:
//   - Gzipped diff bytes (ready for BuildGateValidateRequest.DiffPatch).
//   - nil if workspace has no changes or diff generation fails (logs warning but continues).
func computeGzippedDiff(ctx context.Context, workspace string) []byte {
	// Generate unified diff of all workspace changes relative to HEAD.
	cmd := exec.CommandContext(ctx, "git", "diff", "HEAD")
	cmd.Dir = workspace

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Log warning but don't fail re-gate — HTTP executor will use repo_url+ref baseline.
		slog.Warn("failed to generate diff for HTTP re-gate",
			"workspace", workspace,
			"error", err,
			"stderr", strings.TrimSpace(stderr.String()),
		)
		return nil
	}

	diffBytes := stdout.Bytes()
	if len(diffBytes) == 0 {
		// No changes in workspace; HTTP gate will validate baseline.
		return nil
	}

	// Gzip the diff for efficient transmission.
	var gzBuf bytes.Buffer
	gzWriter := gzip.NewWriter(&gzBuf)
	if _, err := gzWriter.Write(diffBytes); err != nil {
		slog.Warn("failed to gzip diff for HTTP re-gate", "workspace", workspace, "error", err)
		return nil
	}
	if err := gzWriter.Close(); err != nil {
		slog.Warn("failed to close gzip writer for HTTP re-gate", "workspace", workspace, "error", err)
		return nil
	}

	slog.Debug("computed gzipped diff for HTTP re-gate",
		"workspace", workspace,
		"raw_size", len(diffBytes),
		"gzipped_size", gzBuf.Len(),
	)

	return gzBuf.Bytes()
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
//   - stepIndex: 0-based step number for tagging healing diffs (C2: stage+diff model).
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
	stepIndex int, // C2: step number for healing diff tagging
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
			// C2: Pass stepIndex so healing diffs are included in rehydration queries.
			r.uploadHealingModDiff(ctx, req.RunID.String(), stageID, workspace, healResult, idx, attempt, stepIndex)

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
		//
		// For HTTP-based gates (PLOY_BUILDGATE_MODE=remote-http), compute the accumulated
		// workspace diff and pass it via DiffPatch. This enables remote Build Gate workers
		// to reconstruct workspace state by cloning repo_url+ref and applying the patch,
		// decoupling the healing node from the gate node.
		slog.Info("re-running build gate after healing", "run_id", req.RunID, "attempt", attempt, "phase", gatePhase)

		// Clone gateSpec and populate DiffPatch for HTTP re-gates.
		// DiffPatch enables remote gate workers to validate accumulated healing changes
		// without requiring direct workspace access. Docker-based gates ignore DiffPatch
		// since they access the workspace directly.
		regateSpec := &contracts.StepGateSpec{
			Enabled:   gateSpec.Enabled,
			Profile:   gateSpec.Profile,
			Env:       gateSpec.Env,
			RepoURL:   gateSpec.RepoURL,
			Ref:       gateSpec.Ref,
			DiffPatch: computeGzippedDiff(ctx, workspace),
		}

		regateStart := time.Now()
		reGateMetadata, regateErr := runner.Gate.Execute(ctx, regateSpec, workspace)
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
// ## Execution Flow (Phase G: Pre-run Gate Only)
//
// Per ROADMAP.md Phase G, executeWithHealing disables per-step pre-gate in Runner.Run calls:
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
	// Phase G: Run pre-mod gate via runGateWithHealing (not via Runner.Run).
	// This centralizes all gate execution in runGateWithHealing, ensuring gate failures
	// are handled uniformly with healing support. Runner.Run is reserved for container execution.
	// C2: Pass stepIndex so healing diffs are tagged with the correct step.
	preGate, preReGates, preGateErr := r.runGateWithHealing(
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
			Result:  result,
			PreGate: preGate,
			ReGates: reGates,
		}, preGateErr
	}

	// Pre-mod gate passed. Clone manifest for main mod execution with gate disabled.
	// Per ROADMAP.md Phase G: Set Gate.Enabled=false and clear deprecated Shift and
	// Inputs[i].Hydration entries so Runner.Run performs container execution only.
	manifestForMainMod := manifest
	manifestForMainMod.Gate = &contracts.StepGateSpec{Enabled: false}
	//lint:ignore SA1019 Backward compatibility: also disable deprecated Shift field.
	manifestForMainMod.Shift = nil

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
	result, err := runner.Run(ctx, step.Request{
		TicketID:  types.TicketID(req.RunID),
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
			Result:  result,
			PreGate: preGate,
			ReGates: reGates,
		}, err
	}

	// Run post-mod gate only if main mod succeeded (ExitCode == 0).
	// This validates the workspace after modifications using the same healing behavior
	// as pre-mod gates, keeping gate orchestration consistent.
	// C2: Pass stepIndex so post-gate healing diffs are tagged with the correct step.
	if result.ExitCode == 0 {
		postGate, postReGates, postErr := r.runGateWithHealing(
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

		return executionResult{
			Result:  result,
			PreGate: preGate,
			ReGates: reGates,
		}, postErr
	}

	// Main mod exited with non-zero code; no post-gate runs.
	return executionResult{
		Result:  result,
		PreGate: preGate,
		ReGates: reGates,
	}, nil
}
