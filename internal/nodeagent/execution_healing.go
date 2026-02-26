// execution_healing.go implements gate-heal-regate orchestration.
// After healing migs complete, the node agent always re-runs the gate to verify the fix.
//
// File layout:
//   - execution_healing.go      — orchestration (this file): runGateWithHealing, executeWithHealing, types, env/TLS injection
//   - execution_healing_loop.go — healing retry loop: healingLoopInput, runHealingLoop
//   - execution_healing_io.go   — I/O helpers: parsers, file persistence
package nodeagent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/step"
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
	ctx = withGateExecutionLabels(ctx, req)

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
		// No healing configured (or healing configured without a mig); return the gate failure.
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
// Flow: pre-gate → main mig (gate disabled) → post-gate (if ExitCode==0).
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
	// Phase G: Run pre-mig gate via runGateWithHealing (not via Runner.Run).
	// This centralizes all gate execution in runGateWithHealing, ensuring gate failures
	// are handled uniformly with healing support. Runner.Run is reserved for container execution.
	// Pass stepIndex so gate history and stats remain aligned with the Mods step index.
	preGate, preReGates, preActionSummary, preGateErr := r.runGateWithHealing(
		ctx, runner, req, manifest, workspace, outDir, inDir, "pre", stepIndex,
	)

	// Build the initial ReGates slice from any pre-mig healing attempts.
	var reGates []gateRunMetadata
	reGates = append(reGates, preReGates...)

	// If pre-mig gate failed (with or without healing), return the failure.
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

	// Pre-mig gate passed. Clone manifest for main mig execution with gate disabled.
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

	// Execute main mig container via Runner.Run. Gate is disabled, so this call
	// will not produce ErrBuildGateFailed — it only runs the container.
	// Pass RunID directly for consistent labeling and telemetry.
	result, err := runner.Run(ctx, step.Request{
		RunID:     req.RunID,
		JobID:     req.JobID,
		Manifest:  manifestForMainMod,
		Workspace: workspace,
		OutDir:    outDir,
		InDir:     *inDir,
	})

	// Propagate the final pre-mig gate result into result.BuildGate for downstream stats.
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

	// Run post-mig gate only if main mig succeeded (ExitCode == 0).
	// This validates the workspace after modifications using the same healing behavior
	// as pre-mig gates, keeping gate orchestration consistent.
	// Pass stepIndex so post-mig gate history and stats remain aligned with the Mods step index.
	if result.ExitCode == 0 {
		postGate, postReGates, postActionSummary, postErr := r.runGateWithHealing(
			ctx, runner, req, manifest, workspace, outDir, inDir, "post", stepIndex,
		)
		// Append initial post-gate run to history.
		if postGate != nil {
			reGates = append(reGates, *postGate)
		}
		// Append any post-mig healing re-gates to history.
		reGates = append(reGates, postReGates...)

		// Update result.BuildGate to reflect the final post-mig gate outcome.
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

	// Main mig exited with non-zero code; no post-gate runs.
	return executionResult{
		Result:        result,
		PreGate:       preGate,
		ReGates:       reGates,
		ActionSummary: preActionSummary,
	}, nil
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
