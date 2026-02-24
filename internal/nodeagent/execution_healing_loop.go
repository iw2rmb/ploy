// execution_healing_loop.go contains the retry-based healing loop that
// alternates between healing mig execution and gate re-validation.
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

type healingLoopInput struct {
	runner      step.Runner
	req         StartRunRequest
	workspace   string
	outDir      string
	inDir       string
	gatePhase   string
	gateSpec    *contracts.StepGateSpec
	initialGate *gateRunMetadata
	typedOpts   RunOptions
	healingCfg  *HealingConfig
	retries     int
	logPayload  string
}

func (r *runController) runHealingLoop(ctx context.Context, in healingLoopInput) ([]gateRunMetadata, string, error) {
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
	// It executes a single healing mig between gate validation attempts based on user-configured retries.
	// We intentionally do not use internal/workflow/backoff here because:
	//  1. This is not a retry-on-failure pattern (each iteration does useful work: running a healing mig).
	//  2. The retry count is user-configured (manifest-specified retries parameter).
	//  3. No exponential backoff is needed; each healing attempt runs immediately after the healing mig completes.
	// healingLogBuf accumulates the /in/healing-log.md content across iterations.
	var healingLogBuf strings.Builder

	logPayload := in.logPayload
	for attempt := 1; attempt <= in.retries; attempt++ {
		slog.Info("starting healing attempt", "run_id", in.req.RunID, "attempt", attempt, "max_retries", in.retries, "phase", in.gatePhase)

		// Stack-aware image selection uses the stack derived from the gate metadata
		// that triggered this healing attempt.
		stackForAttempt := contracts.ModStackUnknown
		switch {
		case attempt == 1 && in.initialGate != nil && in.initialGate.Metadata != nil:
			stackForAttempt = in.initialGate.Metadata.DetectedStack()
		case len(reGates) > 0 && reGates[len(reGates)-1].Metadata != nil:
			stackForAttempt = reGates[len(reGates)-1].Metadata.DetectedStack()
		}
		if stackForAttempt == "" {
			stackForAttempt = contracts.ModStackUnknown
		}

		// --- Per-iteration artifact: /in/build-gate-iteration-N.log ---
		// Write the current gate failure log as a per-iteration snapshot.
		persistBuildGateIterationLog(in.inDir, logPayload, attempt, in.req.RunID)

		// --- Per-iteration router execution (bug_summary) ---
		// Run router before healing so we capture a one-line summary for the current
		// failing build-gate.log (which may change across re-gate attempts).
		bugSummary := ""
		if in.typedOpts.Router != nil && !in.typedOpts.Router.Image.IsEmpty() {
			slog.Info("running router for bug_summary", "run_id", in.req.RunID, "attempt", attempt, "phase", in.gatePhase)

			func() {
				routerOutDir, routerDirErr := os.MkdirTemp("", "ploy-router-out-*")
				if routerDirErr != nil {
					slog.Error("failed to create router /out directory", "run_id", in.req.RunID, "attempt", attempt, "error", routerDirErr)
					return
				}
				defer os.RemoveAll(routerOutDir)

				routerManifest, routerBuildErr := buildRouterManifest(in.req, *in.typedOpts.Router, stackForAttempt)
				if routerBuildErr != nil {
					slog.Error("failed to build router manifest", "run_id", in.req.RunID, "attempt", attempt, "error", routerBuildErr)
					return
				}

				r.injectHealingEnvVars(&routerManifest, in.workspace)
				r.mountHealingTLSCerts(&routerManifest)

				_, routerErr := in.runner.Run(ctx, step.Request{
					RunID:     in.req.RunID,
					Manifest:  routerManifest,
					Workspace: in.workspace,
					OutDir:    routerOutDir,
					InDir:     in.inDir,
				})
				if routerErr != nil {
					slog.Warn("router execution failed", "run_id", in.req.RunID, "attempt", attempt, "error", routerErr)
					return
				}

				bugSummary = parseBugSummary(routerOutDir)
				if bugSummary == "" {
					return
				}

				// Attach bug_summary to the initial gate metadata for downstream stats.
				// Inline healing does not have a separate gate job, but this preserves
				// the first failing gate's bug_summary in run stats.
				if attempt == 1 && in.initialGate != nil && in.initialGate.Metadata != nil {
					in.initialGate.Metadata.BugSummary = bugSummary
				}
				slog.Info("router produced bug_summary", "run_id", in.req.RunID, "attempt", attempt, "bug_summary", bugSummary)
			}()
		}

		// Capture workspace status before running the healing mig so we can detect
		// whether this healing attempt produced any net changes.
		preStatus, preStatusErr := workspaceStatus(ctx, in.workspace)
		if preStatusErr != nil {
			slog.Warn("healing: failed to compute workspace status before healing; assuming changes may occur",
				"run_id", in.req.RunID,
				"attempt", attempt,
				"phase", in.gatePhase,
				"error", preStatusErr,
			)
		}

		// Build healing manifest from the single configured healing mig.
		mig := in.healingCfg.Mod
		const healingIndex = 0

		// Pass healingSession through so agent-specific session env (for example,
		// CODEX_RESUME=1 for Codex-based healers) can be injected by
		// buildHealingManifest when appropriate.
		// Stack is unknown during inline healing loops since the gate result
		// that detected the stack is the one that just failed. Use stackForAttempt
		// (derived from that gate metadata) for deterministic stack-aware image selection.
		healManifest, buildErr := buildHealingManifest(in.req, mig, healingIndex, healingSession, stackForAttempt)
		if buildErr != nil {
			slog.Error("failed to build healing manifest", "run_id", in.req.RunID, "healing_index", healingIndex, "error", buildErr)
			return reGates, lastActionSummary, fmt.Errorf("build healing manifest[%d]: %w", healingIndex, buildErr)
		}

		slog.Info("executing healing mig", "run_id", in.req.RunID, "attempt", attempt, "healing_index", healingIndex, "image", healManifest.Image, "phase", in.gatePhase)

		// Inject healing-specific environment variables and TLS certificate mounts.
		// Uses shared helpers (injectHealingEnvVars, mountHealingTLSCerts) to ensure
		// consistent configuration between inline healing (runGateWithHealing) and
		// discrete healing jobs (executeHealingJob).
		r.injectHealingEnvVars(&healManifest, in.workspace)
		r.mountHealingTLSCerts(&healManifest)

		// Run the healing mig container.
		// Pass RunID directly for consistent labeling and telemetry.
		healResult, healErr := in.runner.Run(ctx, step.Request{
			RunID:     in.req.RunID,
			Manifest:  healManifest,
			Workspace: in.workspace,
			OutDir:    in.outDir,
			InDir:     in.inDir,
		})

		if healErr != nil {
			slog.Error("healing mig execution failed", "run_id", in.req.RunID, "healing_index", healingIndex, "error", healErr)
			return reGates, lastActionSummary, fmt.Errorf("healing mig[%d] failed: %w", healingIndex, healErr)
		}

		if healResult.ExitCode != 0 {
			slog.Warn("healing mig exited with non-zero code", "run_id", in.req.RunID, "healing_index", healingIndex, "exit_code", healResult.ExitCode)
			// Continue; re-gate will still run so callers see gate status.
		}

		// Upload /out artifacts for this healing mig if present.
		if uploadErr := r.uploadOutDir(ctx, in.req.RunID, in.req.JobID, in.outDir); uploadErr != nil {
			slog.Warn("failed to upload /out for healing mig", "run_id", in.req.RunID, "job_id", in.req.JobID, "healing_index", healingIndex, "error", uploadErr)
		}

		// Read session artifacts from /out for propagation across retries.
		if session := readHealingSessionFromOutDir(in.outDir, in.req.RunID, healingIndex); session != "" {
			healingSession = session
		}

		// Persist codex-session.txt to /in for subsequent healing attempts.
		// The filename is part of the current session contract; callers inside
		// containers remain free to interpret it as needed.
		persistHealingSessionToInDir(in.inDir, healingSession, in.req.RunID)

		// --- Capture action_summary from healing container ---
		actionSummary := parseActionSummary(in.outDir)
		if actionSummary != "" {
			lastActionSummary = actionSummary
			slog.Info("healing: captured action_summary", "run_id", in.req.RunID, "attempt", attempt, "action_summary", actionSummary)
		}

		// --- Per-iteration artifact: /in/healing-iteration-N.log ---
		// Copy healing agent output (codex.log or equivalent) to /in for history.
		persistHealingIterationLog(in.inDir, in.outDir, attempt, in.req.RunID)

		// --- Append iteration block to /in/healing-log.md ---
		appendHealingLogEntry(&healingLogBuf, attempt, bugSummary, actionSummary)

		// Write healing-log.md after each iteration so it's available even if we bail out early.
		persistHealingLog(in.inDir, &healingLogBuf, in.req.RunID)

		// Capture workspace status after the healing mig completes and compare with
		// the pre-healing status. If both are available and identical, then this
		// healing attempt produced no net workspace changes and there is no point
		// in re-running the gate. Treat this as a terminal build gate failure.
		postStatus, postStatusErr := workspaceStatus(ctx, in.workspace)
		if postStatusErr != nil {
			slog.Warn("healing: failed to compute workspace status after healing; proceeding with re-gate",
				"run_id", in.req.RunID,
				"attempt", attempt,
				"phase", in.gatePhase,
				"error", postStatusErr,
			)
		}

		if preStatusErr == nil && postStatusErr == nil && preStatus == postStatus {
			slog.Warn("healing: no workspace changes detected after healing attempt; skipping re-gate",
				"run_id", in.req.RunID,
				"attempt", attempt,
				"phase", in.gatePhase,
			)
			// Retries are effectively exhausted because healing cannot make further progress.
			return reGates, lastActionSummary, fmt.Errorf("%w: healing produced no workspace changes", step.ErrBuildGateFailed)
		}

		// Re-run the gate after the healing mig completes.
		// The node agent ALWAYS re-runs the gate via runner.Gate.Execute to verify
		// that healing modifications have resolved the validation failure.
		slog.Info("re-running build gate after healing", "run_id", in.req.RunID, "attempt", attempt, "phase", in.gatePhase)

		// Clone gateSpec for re-gate execution. DiffPatch is intentionally left empty:
		// gate validation runs directly against the mutated workspace, and discrete
		// healing jobs publish baseline-based diffs for rehydration.
		regateSpec := &contracts.StepGateSpec{
			Enabled:        in.gateSpec.Enabled,
			Env:            in.gateSpec.Env,
			ImageOverrides: in.gateSpec.ImageOverrides,
			RepoURL:        in.gateSpec.RepoURL,
			Ref:            in.gateSpec.Ref,
			StackGate:      in.gateSpec.StackGate,
		}

		regateStart := time.Now()
		reGateMetadata, regateErr := in.runner.Gate.Execute(ctx, regateSpec, in.workspace)
		regateDuration := time.Since(regateStart)

		if regateErr != nil {
			slog.Error("re-gate execution failed", "run_id", in.req.RunID, "error", regateErr)
			return reGates, lastActionSummary, fmt.Errorf("re-gate execution failed: %w", regateErr)
		}

		// Capture re-gate metadata for stats.
		reGates = append(reGates, gateRunMetadata{
			Metadata:   reGateMetadata,
			DurationMs: regateDuration.Milliseconds(),
		})

		// Check if gate passed using shared helper.
		if gateResultPassed(reGateMetadata) {
			slog.Info("build gate passed after healing", "run_id", in.req.RunID, "attempt", attempt, "phase", in.gatePhase)
			return reGates, lastActionSummary, nil
		}

		// Re-gate failed; update logPayload and /in/build-gate.log for the next iteration.
		slog.Warn("build gate still failing after healing", "run_id", in.req.RunID, "attempt", attempt, "phase", in.gatePhase)

		// Refresh logPayload from the re-gate result so the next iteration's
		// build-gate-iteration-N.log and healing-log.md reflect the latest failure.
		logPayload = gateLogPayloadFromMetadata(reGateMetadata)
		persistBuildGateLog(in.inDir, logPayload, in.req.RunID)
	}

	// Retries exhausted; return the gate failure.
	slog.Error("healing retries exhausted, build gate still failing", "run_id", in.req.RunID, "phase", in.gatePhase)
	return reGates, lastActionSummary, fmt.Errorf("%w: healing retries exhausted", step.ErrBuildGateFailed)
}
