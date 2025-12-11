package nodeagent

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"

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
func (r *runController) uploadHealingModDiff(ctx context.Context, runID types.RunID, jobID types.JobID, jobName, workspace string, healResult step.Result, modIndex, healingAttempt, stepIndex int) {
	// Retrieve the diff generator from runtime components.
	diffGenerator := r.createDiffGenerator()
	if diffGenerator == nil {
		return
	}

	// Healing mods run inline against the same workspace; there is no separate
	// baseline directory available here. To keep semantics consistent with the
	// rest of the system and avoid legacy HEAD-based diffs, healing mod diffs
	// are currently disabled. When a baseline snapshot is introduced for inline
	// healing, this function must be updated to use GenerateBetween.
	_ = diffGenerator
	slog.Warn("uploadHealingModDiff: baseline-less healing mod diff generation disabled (no Generate fallback)", "run_id", runID, "job_id", jobID, "mod_index", modIndex, "step_index", stepIndex)
}

// uploadHealingJobDiff generates and uploads a diff for a discrete healing job by
// comparing the pre-healing baseline snapshot with the post-healing workspace.
//
// Unlike uploadHealingModDiff (which tags diffs as mod_type="healing" for inline
// gate healing), discrete healing jobs must publish diffs as mod_type="mod" so
// that subsequent re-gate steps rehydrate the healed workspace from the diff
// chain. Using GenerateBetween(baseDir, workspace) ensures that:
//   - Untracked files created by the healer are included in the diff
//   - The diff captures the full delta from the baseline (base+prior-diffs)
//     to the healed workspace, matching repo+diff semantics.
//
// When baseDir is empty or the diff generator is nil, this helper falls back to
// the standard per-step diff behavior (uploadDiffForStep) so healing still
// produces diagnostics even if baseline snapshots are unavailable.
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
	// Discrete healing jobs publish mod_type="mod" so their diffs participate in
	// the rehydration chain (healing diffs are not intermediate states here).
	summary := types.DiffSummary{
		"step_index": stepIndex,
		"mod_type":   "mod",
		"exit_code":  result.ExitCode,
		"timings": map[string]interface{}{
			"hydration_duration_ms":  result.Timings.HydrationDuration.Milliseconds(),
			"execution_duration_ms":  result.Timings.ExecutionDuration.Milliseconds(),
			"build_gate_duration_ms": result.Timings.BuildGateDuration.Milliseconds(),
			"diff_duration_ms":       result.Timings.DiffDuration.Milliseconds(),
			"total_duration_ms":      result.Timings.TotalDuration.Milliseconds(),
		},
	}

	diffUploader, err := NewDiffUploader(r.cfg)
	if err != nil {
		slog.Error("failed to create diff uploader for healing job", "run_id", runID, "job_id", jobID, "step_index", stepIndex, "error", err)
		return
	}

	if err := diffUploader.UploadDiff(ctx, runID, jobID, diffBytes, summary); err != nil {
		slog.Error("failed to upload healing job diff", "run_id", runID, "job_id", jobID, "step_index", stepIndex, "error", err)
		return
	}

	slog.Info("healing job diff uploaded successfully", "run_id", runID, "job_id", jobID, "step_index", stepIndex, "size", len(diffBytes))
}

// computeGzippedDiff generates a unified git diff of workspace changes and compresses it.
// It captures accumulated healing changes so gate re-execution can reason about the same
// repo+diff workspace model used by the Docker-based gate executor.
//
// The diff captures all changes relative to the initial repo_url+ref clone (HEAD), enabling
// future distributed gate workers to reconstruct workspace state by cloning repo_url+ref
// and applying the patch when needed.
//
// Returns:
//   - Gzipped diff bytes (suitable for repo+diff-style consumers).
//   - nil if workspace has no changes or diff generation fails (logs warning but continues).
func computeGzippedDiff(ctx context.Context, workspace string) []byte {
	// Generate unified diff of all workspace changes relative to HEAD.
	cmd := exec.CommandContext(ctx, "git", "diff", "HEAD")
	cmd.Dir = workspace

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Log warning but don't fail re-gate — the gate executor will fall back to validating
		// the repo_url+ref baseline without an explicit diff patch.
		slog.Warn("failed to generate diff for gate re-execution",
			"workspace", workspace,
			"error", err,
			"stderr", strings.TrimSpace(stderr.String()),
		)
		return nil
	}

	diffBytes := stdout.Bytes()
	if len(diffBytes) == 0 {
		// No changes in workspace; gate executor will validate baseline.
		return nil
	}

	// Gzip the diff for efficient transmission.
	var gzBuf bytes.Buffer
	gzWriter := gzip.NewWriter(&gzBuf)
	if _, err := gzWriter.Write(diffBytes); err != nil {
		slog.Warn("failed to gzip diff for gate re-execution", "workspace", workspace, "error", err)
		return nil
	}
	if err := gzWriter.Close(); err != nil {
		slog.Warn("failed to close gzip writer for gate re-execution", "workspace", workspace, "error", err)
		return nil
	}

	slog.Debug("computed gzipped diff for gate re-execution",
		"workspace", workspace,
		"raw_size", len(diffBytes),
		"gzipped_size", gzBuf.Len(),
	)

	return gzBuf.Bytes()
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
