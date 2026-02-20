// execution_healing.go implements gate-heal-regate orchestration.
// After healing mods complete, the node agent always re-runs the gate to verify the fix.
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

	types "github.com/iw2rmb/ploy/internal/domain/types"
	gitpkg "github.com/iw2rmb/ploy/internal/nodeagent/git"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/runtime/step"
)

// runGateWithHealing executes the build gate with optional healing loop when validation fails.
// Returns initialGate metadata, re-gate history, last action summary, and error.
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
	_ = stepIndex

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

	if err := ensureHealingInDir(inDir, req.RunID); err != nil {
		return initialGate, nil, "", step.ErrBuildGateFailed
	}

	logPayload := gateLogPayloadFromMetadata(gateMetadata)
	persistBuildGateLog(*inDir, logPayload, req.RunID)

	reGates, lastActionSummary, loopErr := r.runHealingLoop(ctx, healingLoopInput{
		runner:      runner,
		req:         req,
		workspace:   workspace,
		outDir:      outDir,
		inDir:       *inDir,
		gatePhase:   gatePhase,
		gateSpec:    gateSpec,
		initialGate: initialGate,
		typedOpts:   typedOpts,
		healingCfg:  healingConfig,
		retries:     retries,
		logPayload:  logPayload,
	})
	return initialGate, reGates, lastActionSummary, loopErr
}

// executeWithHealing runs the main step with optional healing loop when the build gate fails.
// Flow: pre-gate → main mod (gate disabled) → post-gate (if ExitCode==0).
// Returns execution result with full gate history.
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

// --- Merged from execution_healing_helpers.go ---

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
	stepIndex types.StepIndex,
) {
	if diffGenerator == nil {
		return
	}
	if strings.TrimSpace(baseDir) == "" {
		return
	}

	diffBytes, err := diffGenerator.GenerateBetween(ctx, baseDir, workspace)
	if err != nil {
		slog.Error("failed to generate healing job diff", "run_id", runID, "job_id", jobID, "step_index", stepIndex, "error", err)
		return
	}

	if len(diffBytes) == 0 {
		slog.Info("no diff to upload for healing job (no changes between baseline and workspace)", "run_id", runID, "job_id", jobID, "step_index", stepIndex)
		return
	}

	summary := types.NewDiffSummaryBuilder().
		StepIndex(stepIndex).
		ModType(DiffModTypeMod.String()).
		ExitCode(result.ExitCode).
		Timings(
			time.Duration(result.Timings.HydrationDuration).Milliseconds(),
			time.Duration(result.Timings.ExecutionDuration).Milliseconds(),
			time.Duration(result.Timings.DiffDuration).Milliseconds(),
			time.Duration(result.Timings.TotalDuration).Milliseconds(),
		).
		MustBuild()

	if err := r.ensureUploaders(); err != nil {
		slog.Error("failed to initialize uploaders", "run_id", runID, "job_id", jobID, "step_index", stepIndex, "error", err)
		return
	}

	if err := r.diffUploader.UploadDiff(ctx, runID, jobID, diffBytes, summary); err != nil {
		slog.Error("failed to upload healing job diff", "run_id", runID, "job_id", jobID, "step_index", stepIndex, "error", err)
		return
	}

	slog.Info("healing job diff uploaded successfully", "run_id", runID, "job_id", jobID, "step_index", stepIndex, "size", len(diffBytes))
}

// gateRunMetadata captures gate execution metadata and timing for stats reporting.
type gateRunMetadata struct {
	Metadata   *contracts.BuildGateStageMetadata
	DurationMs int64
}

// executionResult wraps step.Result with additional gate run history for stats.
type executionResult struct {
	step.Result
	PreGate       *gateRunMetadata
	ReGates       []gateRunMetadata
	ActionSummary string
}

// --- Merged from execution_healing_indir.go ---

func ensureHealingInDir(inDir *string, runID types.RunID) error {
	if *inDir != "" {
		return nil
	}
	tmpInDir, dirErr := os.MkdirTemp("", "ploy-mod-in-*")
	if dirErr != nil {
		slog.Error("failed to create /in directory for healing", "run_id", runID, "error", dirErr)
		return dirErr
	}
	*inDir = tmpInDir
	return nil
}

func gateLogPayloadFromMetadata(gateMetadata *contracts.BuildGateStageMetadata) string {
	if gateMetadata == nil {
		return ""
	}
	logPayload := gateMetadata.LogsText
	if len(gateMetadata.LogFindings) > 0 {
		if trimmed := strings.TrimSpace(gateMetadata.LogFindings[0].Message); trimmed != "" {
			logPayload = trimmed
			if !strings.HasSuffix(logPayload, "\n") {
				logPayload += "\n"
			}
		}
	}
	return logPayload
}

// persistBuildGateLog writes logPayload to /in/build-gate.log.
func persistBuildGateLog(inDir, logPayload string, runID types.RunID) {
	if logPayload == "" || inDir == "" {
		return
	}
	inLogPath := filepath.Join(inDir, "build-gate.log")
	if writeErr := os.WriteFile(inLogPath, []byte(logPayload), 0o644); writeErr != nil {
		slog.Warn("failed to write /in/build-gate.log", "run_id", runID, "error", writeErr)
	}
}

func persistBuildGateIterationLog(inDir, logPayload string, attempt int, runID types.RunID) {
	if logPayload == "" || inDir == "" {
		return
	}
	iterGateLogPath := filepath.Join(inDir, fmt.Sprintf("build-gate-iteration-%d.log", attempt))
	if writeErr := os.WriteFile(iterGateLogPath, []byte(logPayload), 0o644); writeErr != nil {
		slog.Warn("failed to write build-gate-iteration log", "run_id", runID, "attempt", attempt, "error", writeErr)
	}
}

func persistHealingIterationLog(inDir, outDir string, attempt int, runID types.RunID) {
	if inDir == "" {
		return
	}
	healingIterLogPath := filepath.Join(inDir, fmt.Sprintf("healing-iteration-%d.log", attempt))
	if codexLog, readErr := os.ReadFile(filepath.Join(outDir, "codex.log")); readErr == nil {
		if writeErr := os.WriteFile(healingIterLogPath, codexLog, 0o644); writeErr != nil {
			slog.Warn("failed to write healing-iteration log", "run_id", runID, "attempt", attempt, "error", writeErr)
		}
	}
}

func appendHealingLogEntry(buf *strings.Builder, attempt int, bugSummary, actionSummary string) {
	if attempt == 1 {
		buf.WriteString("# Healing Log\n\n")
	}
	fmt.Fprintf(buf, "## Iteration %d\n\n", attempt)
	if bugSummary != "" {
		fmt.Fprintf(buf, "- Bug Summary: %s\n", bugSummary)
	} else {
		buf.WriteString("- Bug Summary: N/A\n")
	}
	fmt.Fprintf(buf, "  Build Log: /in/build-gate-iteration-%d.log\n", attempt)
	if actionSummary != "" {
		fmt.Fprintf(buf, "- Healing Attempt: %s\n", actionSummary)
	} else {
		buf.WriteString("- Healing Attempt: N/A\n")
	}
	fmt.Fprintf(buf, "  Agent Log: /in/healing-iteration-%d.log\n\n", attempt)
}

func persistHealingLog(inDir string, buf *strings.Builder, runID types.RunID) {
	if inDir == "" {
		return
	}
	healingLogPath := filepath.Join(inDir, "healing-log.md")
	if writeErr := os.WriteFile(healingLogPath, []byte(buf.String()), 0o644); writeErr != nil {
		slog.Warn("failed to write healing-log.md", "run_id", runID, "error", writeErr)
	}
}

// --- Merged from execution_healing_session.go ---

func readHealingSessionFromOutDir(outDir string, runID types.RunID, healingIndex int) string {
	sessionBytes, readErr := os.ReadFile(filepath.Join(outDir, "codex-session.txt"))
	if readErr != nil {
		return ""
	}
	session := strings.TrimSpace(string(sessionBytes))
	if session == "" {
		return ""
	}
	slog.Info("healing: captured session from /out", "run_id", runID, "healing_index", healingIndex, "session_id", session)
	return session
}

func persistHealingSessionToInDir(inDir, session string, runID types.RunID) {
	if session == "" || inDir == "" {
		return
	}
	sessionPath := filepath.Join(inDir, "codex-session.txt")
	if writeErr := os.WriteFile(sessionPath, []byte(session), 0o644); writeErr != nil {
		slog.Warn("healing: failed to persist codex-session.txt into /in", "run_id", runID, "error", writeErr)
	}
}

// --- Healing injection helpers ---

// injectHealingEnvVars adds healing-specific environment variables to the manifest.
func (r *runController) injectHealingEnvVars(manifest *contracts.StepManifest, workspace string) {
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
}

// mountHealingTLSCerts configures TLS certificate paths in manifest options.
func (r *runController) mountHealingTLSCerts(manifest *contracts.StepManifest) {
	if manifest.Options == nil {
		manifest.Options = make(map[string]any)
	}
	manifest.Options["ploy_ca_cert_path"] = r.cfg.HTTP.TLS.CAPath
	manifest.Options["ploy_client_cert_path"] = r.cfg.HTTP.TLS.CertPath
	manifest.Options["ploy_client_key_path"] = r.cfg.HTTP.TLS.KeyPath
}
