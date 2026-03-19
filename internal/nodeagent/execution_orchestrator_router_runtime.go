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

	// Create temp /in and /out for router.
	routerInDir, err := os.MkdirTemp("", "ploy-gate-router-in-*")
	if err != nil {
		slog.Warn("failed to create router /in directory", "run_id", req.RunID, "job_id", req.JobID, "error", err)
		return
	}
	defer os.RemoveAll(routerInDir)

	routerOutDir, err := os.MkdirTemp("", "ploy-gate-router-out-*")
	if err != nil {
		slog.Warn("failed to create router /out directory", "run_id", req.RunID, "job_id", req.JobID, "error", err)
		return
	}
	defer os.RemoveAll(routerOutDir)

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
	if typedOpts.Router.Amata != nil && strings.TrimSpace(typedOpts.Router.Amata.Spec) != "" {
		amataPath := filepath.Join(routerInDir, "amata.yaml")
		if writeErr := os.WriteFile(amataPath, []byte(typedOpts.Router.Amata.Spec), 0o644); writeErr != nil {
			slog.Warn("failed to write router /in/amata.yaml", "run_id", req.RunID, "job_id", req.JobID, "error", writeErr)
			return
		}
	}

	stack := gateResult.DetectedStack()
	if stack == "" {
		stack = contracts.ModStackUnknown
	}

	routerManifest, buildErr := buildRouterManifest(req, *typedOpts.Router, stack, req.JobType, contracts.RecoveryLoopKindHealing.String())
	if buildErr != nil {
		slog.Warn("failed to build router manifest", "run_id", req.RunID, "job_id", req.JobID, "error", buildErr)
		return
	}
	r.injectHealingEnvVars(&routerManifest, workspace)
	r.mountHealingTLSCerts(&routerManifest)

	// Materialize any router tmp files into a staging directory.
	// The staging dir is removed deterministically when runRouterForGateFailure returns.
	var routerTmpStagingDir string
	if len(routerManifest.TmpDir) > 0 {
		dir, err := os.MkdirTemp("", "ploy-router-tmpfiles-*")
		if err != nil {
			slog.Warn("failed to create router tmp staging dir", "run_id", req.RunID, "job_id", req.JobID, "error", err)
			return
		}
		defer func() {
			if rmErr := os.RemoveAll(dir); rmErr != nil {
				slog.Warn("failed to remove router tmp staging dir", "path", dir, "error", rmErr)
			}
		}()
		if err := materializeTmpFiles(routerManifest.TmpDir, dir); err != nil {
			slog.Warn("failed to materialize router tmp files", "run_id", req.RunID, "job_id", req.JobID, "error", err)
			return
		}
		routerTmpStagingDir = dir
	}

	_, runErr := runner.Run(ctx, step.Request{
		RunID:         req.RunID,
		JobID:         req.JobID,
		Manifest:      routerManifest,
		Workspace:     workspace,
		OutDir:        routerOutDir,
		InDir:         routerInDir,
		TmpStagingDir: routerTmpStagingDir,
	})
	if runErr != nil {
		slog.Warn("router execution failed", "run_id", req.RunID, "job_id", req.JobID, "error", runErr)
		return
	}

	if bugSummary := parseBugSummary(routerOutDir); bugSummary != "" {
		gateResult.BugSummary = bugSummary
		slog.Info("router produced bug_summary", "run_id", req.RunID, "job_id", req.JobID, "bug_summary", bugSummary)
	}
	gateResult.Recovery = parseRouterDecision(routerOutDir)
}
