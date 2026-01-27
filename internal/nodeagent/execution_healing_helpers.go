package nodeagent

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
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
// providing a canonical record of BuildGateStageMetadata for telemetry, debugging,
// and audit across pre-gate and re-gate phases.
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
	// ActionSummary holds the last action_summary produced by the healing container
	// during the inline gate-heal-regate loop. Populated by runGateWithHealing when
	// the healer writes a valid action_summary to /out/codex-last.txt.
	ActionSummary string
}

// uploadHealingJobDiff generates and uploads a diff for a discrete healing job by
// comparing the pre-healing baseline snapshot with the post-healing workspace.
//
// Discrete healing jobs publish diffs as mod_type=DiffModTypeMod so that subsequent
// steps rehydrate the healed workspace from the diff chain. Using
// GenerateBetween(baseDir, workspace) ensures that:
//   - Untracked files created by the healer are included in the diff
//   - The diff captures the full delta from the baseline (base+prior-diffs)
//     to the healed workspace, matching repo+diff semantics.
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

	// If no baseline snapshot is available, skip diff upload rather than
	// falling back to legacy HEAD-based generation. Healing job diffs must
	// use baseline-aware GenerateBetween semantics.
	if strings.TrimSpace(baseDir) == "" {
		return
	}

	// Generate diff between baseline snapshot and healed workspace so untracked
	// files are included in the patch (git diff --no-index semantics).
	diffBytes, err := diffGenerator.GenerateBetween(ctx, baseDir, workspace)
	if err != nil {
		slog.Error("failed to generate healing job diff", "run_id", runID, "job_id", jobID, "step_index", stepIndex, "error", err)
		return
	}

	if len(diffBytes) == 0 {
		// No changes between baseline and healed workspace; skip upload.
		slog.Info("no diff to upload for healing job (no changes between baseline and workspace)", "run_id", runID, "job_id", jobID, "step_index", stepIndex)
		return
	}

	// Build diff summary with step metadata for database storage.
	// Discrete healing jobs publish mod_type=DiffModTypeMod so their diffs participate in
	// the rehydration chain (healing diffs are not intermediate states here).
	// Uses typed builder to eliminate map[string]any construction.
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

	// Ensure uploaders are initialized (lazy init for backward compatibility with tests).
	// In production, uploaders are pre-initialized at agent startup.
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

// workspaceStatus returns the output of `git status --porcelain` for the given
// workspace. An empty string indicates a clean working tree relative to the
// current HEAD. When git status fails (e.g., workspace is not a git repo),
// the error is returned so callers can decide how to proceed.
func workspaceStatus(ctx context.Context, workspace string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	cmd.Dir = workspace

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git status --porcelain failed: %w (stderr=%s)", err, strings.TrimSpace(stderr.String()))
	}

	return stdout.String(), nil
}
