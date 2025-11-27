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
//  3. Return execution result with full gate history (pre-gate + re-gates)
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
// Healing containers can also invoke the HTTP Build Gate API directly via
// buildgate-validate (see PLOY_HOST_WORKSPACE, PLOY_SERVER_URL env vars below).
// The system re-runs the gate regardless of in-container verification results.
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
	// First execution attempt (includes pre-mod gate check).
	result, err := runner.Run(ctx, step.Request{
		TicketID:  types.TicketID(req.RunID),
		Manifest:  manifest,
		Workspace: workspace,
		OutDir:    outDir,
		InDir:     *inDir,
	})

	// Capture pre-gate metadata for stats (if gate was executed).
	var preGate *gateRunMetadata
	if result.BuildGate != nil {
		preGate = &gateRunMetadata{
			Metadata:   result.BuildGate,
			DurationMs: result.Timings.BuildGateDuration.Milliseconds(),
		}
	}

	// If execution succeeded or error is not a build gate failure, return immediately.
	if err == nil || !errors.Is(err, step.ErrBuildGateFailed) {
		return executionResult{Result: result, PreGate: preGate}, err
	}

	// Build gate failed. Check if healing is configured.
	// Parse typed healing config from raw options to avoid map[string]any casts.
	typedOpts := parseRunOptions(req.Options)
	if typedOpts.Healing == nil {
		// No healing configured; return the gate failure.
		return executionResult{Result: result, PreGate: preGate}, err
	}

	healingConfig := typedOpts.Healing

	// Validate healing configuration.
	if len(healingConfig.Mods) == 0 {
		slog.Warn("build_gate_healing configured but no mods provided", "run_id", req.RunID)
		return executionResult{Result: result, PreGate: preGate}, err
	}

	retries := healingConfig.Retries

	// Create /in directory if not already created (for build-gate.log).
	if *inDir == "" {
		tmpInDir, dirErr := os.MkdirTemp("", "ploy-mod-in-*")
		if dirErr != nil {
			slog.Error("failed to create /in directory for healing", "run_id", req.RunID, "error", dirErr)
			return executionResult{Result: result, PreGate: preGate}, err
		}
		*inDir = tmpInDir
		// Caller handles cleanup via defer.

		// Write build-gate.log to /in for healing containers.
		if result.BuildGate != nil && result.BuildGate.LogsText != "" {
			inLogPath := filepath.Join(*inDir, "build-gate.log")
			if writeErr := os.WriteFile(inLogPath, []byte(result.BuildGate.LogsText), 0o644); writeErr != nil {
				slog.Warn("failed to write /in/build-gate.log", "run_id", req.RunID, "error", writeErr)
			} else {
				slog.Info("build-gate.log persisted to /in for healing", "run_id", req.RunID, "path", inLogPath)
			}
		}
	}

	// Track re-gate runs for stats.
	var reGates []gateRunMetadata

	// Track Codex session state across healing loop iterations.
	// When a Codex-based healing mod writes codex-session.txt to /out, the agent
	// reads and persists this session ID to /in for subsequent attempts. This enables
	// Codex to resume conversations instead of starting fresh on each retry.
	// The sentinel state (request_build_validation) is tracked for observability but
	// does not affect gate semantics: the agent always re-runs the gate after healing.
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
		slog.Info("starting healing attempt", "run_id", req.RunID, "attempt", attempt, "max_retries", retries)

		// Execute each healing mod in sequence using typed HealingMod structs.
		for idx, mod := range healingConfig.Mods {
			// Pass codexSession to enable CODEX_RESUME=1 injection for Codex-based healers.
			// On the first attempt codexSession is empty; subsequent attempts may have
			// a session ID from the previous healing mod run.
			healManifest, buildErr := buildHealingManifest(req, mod, idx, codexSession)
			if buildErr != nil {
				slog.Error("failed to build healing manifest", "run_id", req.RunID, "mod_index", idx, "error", buildErr)
				return executionResult{Result: result, PreGate: preGate, ReGates: reGates}, fmt.Errorf("build healing manifest[%d]: %w", idx, buildErr)
			}

			slog.Info("executing healing mod", "run_id", req.RunID, "attempt", attempt, "mod_index", idx, "image", healManifest.Image)

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
				return executionResult{Result: result, PreGate: preGate, ReGates: reGates}, fmt.Errorf("healing mod[%d] failed: %w", idx, healErr)
			}

			if healResult.ExitCode != 0 {
				slog.Warn("healing mod exited with non-zero code", "run_id", req.RunID, "mod_index", idx, "exit_code", healResult.ExitCode)
				// Continue with remaining mods; we'll check gate after all mods run.
			}

			// Upload /out artifacts for this healing mod if present.
			// Use centralized options accessor for stage_id when re-gating.
			stageID, _ := manifest.OptionString("stage_id")
			if uploadErr := r.uploadOutDir(ctx, req.RunID.String(), stageID, outDir); uploadErr != nil {
				slog.Warn("failed to upload /out for healing mod", "run_id", req.RunID, "mod_index", idx, "error", uploadErr)
			}

			// Per-step diff capture: Generate and upload diff after each healing mod step.
			// This enables rehydration of workspaces from base + ordered diff chain.
			// Each healing mod diff is tagged with mod_type and mod_index to distinguish
			// from the main mod diff captured later in execution_orchestrator.go.
			r.uploadHealingModDiff(ctx, req.RunID.String(), stageID, workspace, healResult, idx, attempt)

			// Read Codex session and sentinel artifacts from /out for session propagation.
			// These files enable resume mode for Codex-based healers and observability
			// for whether Codex requested build validation.
			if sessionBytes, readErr := os.ReadFile(filepath.Join(outDir, "codex-session.txt")); readErr == nil {
				if session := strings.TrimSpace(string(sessionBytes)); session != "" {
					codexSession = session
					slog.Info("healing: captured codex session from /out", "run_id", req.RunID, "mod_index", idx, "session_id", codexSession)
				}
			}

			// Check for sentinel file indicating Codex requested build validation.
			// This is tracked for observability but does not affect gate semantics.
			if _, statErr := os.Stat(filepath.Join(outDir, "request_build_validation")); statErr == nil {
				codexRequestedValidation = true
				slog.Info("healing: codex requested build validation", "run_id", req.RunID, "mod_index", idx)
			}
		}

		// Persist codex-session.txt to /in for subsequent healing attempts.
		// This allows Codex-based healers to resume from the previous conversation
		// instead of starting fresh. The /in directory is read-only inside containers,
		// so we write from the host side only.
		if codexSession != "" && *inDir != "" {
			sessionPath := filepath.Join(*inDir, "codex-session.txt")
			if writeErr := os.WriteFile(sessionPath, []byte(codexSession), 0o644); writeErr != nil {
				slog.Warn("healing: failed to persist codex-session.txt into /in", "run_id", req.RunID, "error", writeErr)
			} else {
				slog.Info("healing: persisted codex-session.txt to /in for resume", "run_id", req.RunID, "session_id", codexSession)
			}
		}

		// Log sentinel state for observability (gate semantics unchanged).
		_ = codexRequestedValidation // Use variable to silence unused warning in non-logging builds.

		// Re-run the gate after healing mods complete.
		// CRITICAL: The node agent ALWAYS re-runs the gate via runner.Gate.Execute,
		// even if healing mods called the HTTP Build Gate API directly. This ensures:
		//   1. Consistent gate semantics between HTTP API and Docker gate execution
		//   2. Canonical gate results are produced by the node agent (not in-container scripts)
		//   3. Full gate history is captured in ReGates for telemetry and auditing
		//
		// This verification uses the same repo+diff semantics as the HTTP Build Gate API:
		// the workspace now contains repo_url+ref plus accumulated healing modifications.
		// Conceptually equivalent to: POST /v1/buildgate/validate with diff_patch.
		slog.Info("re-running build gate after healing", "run_id", req.RunID, "attempt", attempt)

		gateSpec := manifest.Gate
		//lint:ignore SA1019 Backward compatibility: support deprecated Shift by mapping to Gate.
		if gateSpec == nil && manifest.Shift != nil {
			gateSpec = &contracts.StepGateSpec{
				Enabled: manifest.Shift.Enabled, //lint:ignore SA1019 compat field access
				Profile: manifest.Shift.Profile, //lint:ignore SA1019 compat field access
				Env:     manifest.Shift.Env,     //lint:ignore SA1019 compat field access
			}
		}

		if runner.Gate != nil && gateSpec != nil && gateSpec.Enabled {
			regateStart := time.Now()
			gateMetadata, gateErr := runner.Gate.Execute(ctx, gateSpec, workspace)
			regateDuration := time.Since(regateStart)

			if gateErr != nil {
				slog.Error("re-gate execution failed", "run_id", req.RunID, "error", gateErr)
				return executionResult{Result: result, PreGate: preGate, ReGates: reGates}, fmt.Errorf("re-gate execution failed: %w", gateErr)
			}

			result.BuildGate = gateMetadata

			// Capture re-gate metadata for stats.
			reGates = append(reGates, gateRunMetadata{
				Metadata:   gateMetadata,
				DurationMs: regateDuration.Milliseconds(),
			})

			// Check if gate passed.
			gatePassed := false
			if len(gateMetadata.StaticChecks) > 0 {
				gatePassed = gateMetadata.StaticChecks[0].Passed
			}

			if gatePassed {
				slog.Info("build gate passed after healing", "run_id", req.RunID, "attempt", attempt)
				// Gate passed; proceed to main mod execution.
				// Disable the gate and hydration for the follow-up main mod run to
				// avoid cloning the repository a second time in the same workspace.
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
				// Return with all gate history.
				return executionResult{Result: mainResult, PreGate: preGate, ReGates: reGates}, mainErr
			}

			// Re-gate failed; continue to next retry or exit when exhausted.
			slog.Warn("build gate still failing after healing", "run_id", req.RunID, "attempt", attempt)
		}
	}

	// Retries exhausted; return the gate failure.
	slog.Error("healing retries exhausted, build gate still failing", "run_id", req.RunID)
	return executionResult{Result: result, PreGate: preGate, ReGates: reGates}, fmt.Errorf("%w: healing retries exhausted", step.ErrBuildGateFailed)
}
