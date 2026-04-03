package nodeagent

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/step"
)

// runRouterForGateFailure executes the configured build_gate.router container (when present)
// to summarize the failing gate log into gateResult.BugSummary.
//
// This runs only when healing is configured (since router is required for healing) and the
// gateResult indicates failure.
func (r *runController) runRouterForGateFailure(
	ctx context.Context,
	runner step.Runner,
	req StartRunRequest,
	typedOpts RunOptions,
	workspace string,
	gateResult *contracts.BuildGateStageMetadata,
) {
	if gateResult == nil || gateResultPassed(gateResult) {
		return
	}
	if !typedOpts.HasHealingSelector() {
		return
	}
	if typedOpts.Router == nil || typedOpts.Router.Image.IsEmpty() {
		return
	}
	gateResult.Recovery = &contracts.BuildGateRecoveryMetadata{
		LoopKind:  contracts.DefaultRecoveryLoopKind().String(),
		ErrorKind: contracts.DefaultRecoveryErrorKind().String(),
	}

	// Use nested withTempDir for router /in and /out directories.
	if err := withTempDir("ploy-gate-router-in-*", func(routerInDir string) error {
		return withTempDir("ploy-gate-router-out-*", func(routerOutDir string) error {
			return r.executeRouter(ctx, runner, req, typedOpts, workspace, gateResult, routerInDir, routerOutDir)
		})
	}); err != nil {
		slog.Warn("router setup failed", "run_id", req.RunID, "job_id", req.JobID, "error", err)
	}
}

// executeRouter runs the router container and parses its output.
func (r *runController) executeRouter(
	ctx context.Context,
	runner step.Runner,
	req StartRunRequest,
	typedOpts RunOptions,
	workspace string,
	gateResult *contracts.BuildGateStageMetadata,
	routerInDir, routerOutDir string,
) error {
	// Write /in/build-gate.log with the same trimmed preference used for healing hydration.
	logPayload := gateResult.LogsText
	if len(gateResult.LogFindings) > 0 {
		if trimmed := strings.TrimSpace(gateResult.LogFindings[0].Message); trimmed != "" {
			logPayload = trimmed
			if !strings.HasSuffix(logPayload, "\n") {
				logPayload += "\n"
			}
		}
	}
	if strings.TrimSpace(logPayload) != "" {
		if writeErr := os.WriteFile(filepath.Join(routerInDir, "build-gate.log"), []byte(logPayload), 0o644); writeErr != nil {
			slog.Warn("failed to write router /in/build-gate.log", "run_id", req.RunID, "job_id", req.JobID, "error", writeErr)
		}
	}

	// For amata-mode router: write /in/amata.yaml with deterministic overwrite.
	if writeErr := writeAmataSpecInDir(routerInDir, typedOpts.Router.Amata); writeErr != nil {
		slog.Warn("failed to write router /in/amata.yaml", "run_id", req.RunID, "job_id", req.JobID, "error", writeErr)
		return nil
	}

	stack := gateResult.DetectedStack()
	if stack == "" {
		stack = contracts.MigStackUnknown
	}

	routerManifest, buildErr := buildRouterManifest(req, *typedOpts.Router, stack, req.JobType, contracts.RecoveryLoopKindHealing.String())
	if buildErr != nil {
		slog.Warn("failed to build router manifest", "run_id", req.RunID, "job_id", req.JobID, "error", buildErr)
		return nil
	}
	r.injectHealingEnvVars(&routerManifest, workspace)
	r.mountHealingTLSCerts(&routerManifest)

	// Record the exact argv used for the router container so E2E and downstream
	// consumers can assert --set forwarding shape without re-deriving it.
	if len(routerManifest.Command) > 0 {
		gateResult.Recovery.RouterCmd = append([]string{}, routerManifest.Command...)
	}

	// Materialize Hydra resources into a staging directory for router mount planning.
	return r.withMaterializedResources(ctx, routerManifest, typedOpts.BundleMap, "ploy-router-staging-*", func(routerStagingDir string) error {
		_, runErr := runner.Run(ctx, step.Request{
			RunID:      req.RunID,
			JobID:      req.JobID,
			Manifest:   routerManifest,
			Workspace:  workspace,
			OutDir:     routerOutDir,
			InDir:      routerInDir,
			StagingDir: routerStagingDir,
		})
		if runErr != nil {
			slog.Warn("router execution failed", "run_id", req.RunID, "job_id", req.JobID, "error", runErr)
			return nil
		}

		if bugSummary := parseBugSummary(routerOutDir); bugSummary != "" {
			gateResult.BugSummary = bugSummary
			slog.Info("router produced bug_summary", "run_id", req.RunID, "job_id", req.JobID, "bug_summary", bugSummary)
		}
		parsedRecovery := parseRouterDecision(routerOutDir)
		if parsedRecovery == nil {
			return nil
		}
		if gateResult.Recovery == nil {
			gateResult.Recovery = parsedRecovery
			return nil
		}

		// Preserve pre-populated context (router_cmd) while applying parsed classifier fields.
		routerCmd := append([]string{}, gateResult.Recovery.RouterCmd...)
		*gateResult.Recovery = *parsedRecovery
		if len(routerCmd) > 0 {
			gateResult.Recovery.RouterCmd = routerCmd
		}
		return nil
	})
}
