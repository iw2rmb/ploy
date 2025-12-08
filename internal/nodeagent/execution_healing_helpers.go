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
// E3: For multi-branch healing strategies, the diff summary includes branch_id extracted from
// the job name. This enables branch-local workspace isolation during rehydration — each branch
// only sees mainline diffs plus its own branch's diffs.
//
// This per-step diff capture enables multi-node rehydration where each node can reconstruct
// the workspace state at any point in the healing sequence by applying an ordered chain of diffs.
func (r *runController) uploadHealingModDiff(ctx context.Context, runID types.RunID, jobID types.JobID, jobName, workspace string, healResult step.Result, modIndex, healingAttempt, stepIndex int) {
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
	// E3: branch_id enables branch-local workspace isolation for multi-strategy healing.
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

	// E3: Add branch_id for multi-branch healing isolation.
	// For multi-branch jobs (e.g., "heal-branch-a-1-0"), this enables rehydration to filter
	// diffs by branch, ensuring each branch workspace is isolated from others.
	if branchID := ExtractBranchFromJobName(jobName); branchID != "" {
		summary["branch_id"] = branchID
	}

	// Upload diff with healing metadata to control plane.
	diffUploader, err := NewDiffUploader(r.cfg)
	if err != nil {
		slog.Error("failed to create diff uploader for healing mod", "run_id", runID, "mod_index", modIndex, "error", err)
		return
	}

	// Upload diff to job-scoped endpoint. Step ordering is determined by the job's step_index.
	if err := diffUploader.UploadDiff(ctx, runID, jobID, diffBytes, summary); err != nil {
		slog.Error("failed to upload healing mod diff", "run_id", runID, "job_id", jobID, "mod_index", modIndex, "step_index", stepIndex, "error", err)
		return
	}

	slog.Info("healing mod diff uploaded successfully", "run_id", runID, "job_id", jobID, "mod_index", modIndex, "step_index", stepIndex, "size", len(diffBytes))
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
