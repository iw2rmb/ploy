package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// healingStrategy represents a healing configuration parsed from the spec.
// After collapsing parallel healing, only the first strategy with its first
// healing mod is used per gate.
type healingStrategy struct {
	Name string           // Strategy name (e.g., "codex-ai"); may be empty when not provided.
	Mods []map[string]any // Ordered list of healing mod definitions (image, command, env, etc.).
}

// parseHealingStrategies extracts healing strategies from the build_gate_healing config.
// Returns a slice of strategies from the canonical multi-strategy (strategies[]) form.
//
// The canonical schema requires healing to be configured via:
//
//	build_gate_healing:
//	  retries: N
//	  strategies:
//	    - name: "strategy-name"
//	      mods:
//	        - image: "heal-image:latest"
//
// Legacy single-strategy form (build_gate_healing.mods[] at top level) is no longer
// supported. Callers must migrate to the strategies[] schema.
func parseHealingStrategies(healingConfig map[string]any) []healingStrategy {
	// Only accept the canonical multi-strategy form (strategies[]).
	strategiesRaw, ok := healingConfig["strategies"].([]any)
	if !ok || len(strategiesRaw) == 0 {
		// No strategies configured — healing is not enabled.
		// Note: Top-level mods[] (legacy single-strategy form) is intentionally
		// not supported. Callers must use the strategies[] schema.
		return nil
	}

	var strategies []healingStrategy
	for _, sRaw := range strategiesRaw {
		sMap, ok := sRaw.(map[string]any)
		if !ok {
			continue
		}
		// Extract strategy name (optional, defaults to empty).
		name := ""
		if n, ok := sMap["name"].(string); ok {
			name = strings.TrimSpace(n)
		}
		// Extract mods list for this strategy.
		modsRaw, ok := sMap["mods"].([]any)
		if !ok || len(modsRaw) == 0 {
			continue // Skip strategies without mods.
		}
		var mods []map[string]any
		for _, mRaw := range modsRaw {
			if m, ok := mRaw.(map[string]any); ok {
				mods = append(mods, m)
			}
		}
		if len(mods) > 0 {
			strategies = append(strategies, healingStrategy{Name: name, Mods: mods})
		}
	}
	return strategies
}

// maybeCreateHealingJobs creates healing jobs and a re-gate job when a gate job fails.
// This is called when a gate job (pre_gate, post_gate, re_gate) completes with reason="build-gate".
//
// The function:
// 1. Finds the failed gate job by step_index
// 2. Verifies it's a gate job (pre_gate, post_gate, re_gate)
// 3. Checks if healing is configured in the run spec (requires strategies[] schema)
// 4. Counts existing healing attempts to enforce retry limits
// 5. Creates healing jobs and a re-gate job at intermediate step_index values
//
// Creates parallel branches with distinct step_index windows:
//
//	pre-gate (1000) → FAIL → branch-a: heal-a-0 (1500), re-gate-a (1600)
//	                       → branch-b: heal-b-0 (1700), re-gate-b (1800)
//	→ mod-0 (2000)
//
// Note: Legacy single-strategy form (mods[] at top level) is no longer supported.
// Callers must use the canonical strategies[] schema for healing configuration.
func maybeCreateHealingJobs(
	ctx context.Context,
	st store.Store,
	run store.Run,
	runID domaintypes.RunID, // KSUID-backed string ID after run ID migration.
	failedStepIndex domaintypes.StepIndex,
	jobs []store.Job,
) error {
	// Find the failed gate job by step_index.
	var failedJob *store.Job
	for i := range jobs {
		if jobs[i].StepIndex == float64(failedStepIndex) {
			failedJob = &jobs[i]
			break
		}
	}
	if failedJob == nil {
		slog.Debug("maybeCreateHealingJobs: no job found at step_index",
			"run_id", runID,
			"step_index", failedStepIndex,
		)
		return nil
	}

	// Only create healing for gate jobs.
	modType := strings.TrimSpace(failedJob.ModType)
	if modType != "pre_gate" && modType != "post_gate" && modType != "re_gate" {
		slog.Debug("maybeCreateHealingJobs: not a gate job, skipping healing",
			"run_id", runID,
			"mod_type", modType,
		)
		return nil
	}

	// Parse run spec to get healing configuration.
	var specMap map[string]any
	if len(run.Spec) > 0 && json.Valid(run.Spec) {
		if err := json.Unmarshal(run.Spec, &specMap); err != nil {
			return fmt.Errorf("parse run spec: %w", err)
		}
	}

	// Check if healing is configured.
	healingConfig, ok := specMap["build_gate_healing"].(map[string]any)
	if !ok {
		slog.Debug("maybeCreateHealingJobs: no healing config, canceling remaining jobs",
			"run_id", runID,
		)
		if err := cancelRemainingJobsAfterFailure(ctx, st, runID, failedStepIndex, jobs); err != nil {
			slog.Error("maybeCreateHealingJobs: failed to cancel remaining jobs when no healing configured",
				"run_id", runID,
				"failed_step_index", failedStepIndex,
				"err", err,
			)
		}
		return nil
	}

	// Parse healing strategies (requires canonical strategies[] schema).
	// After collapsing parallel healing, only the first strategy and its
	// first mod are used per gate.
	strategies := parseHealingStrategies(healingConfig)
	if len(strategies) == 0 || len(strategies[0].Mods) == 0 {
		slog.Debug("maybeCreateHealingJobs: no healing strategies configured, canceling remaining jobs",
			"run_id", runID,
		)
		if err := cancelRemainingJobsAfterFailure(ctx, st, runID, failedStepIndex, jobs); err != nil {
			slog.Error("maybeCreateHealingJobs: failed to cancel remaining jobs when no healing configured",
				"run_id", runID,
				"failed_step_index", failedStepIndex,
				"err", err,
			)
		}
		return nil
	}

	// Get retry limit (default to 1 if not specified).
	retries := 1
	if r, ok := healingConfig["retries"].(float64); ok && r > 0 {
		retries = int(r)
	}

	// Determine the base gate index used to count healing attempts.
	// Healing attempts are counted per build gate (pre/post) independently.
	// For re-gate failures, associate the failure with the nearest preceding
	// pre_gate/post_gate so that all healing jobs between that gate and the
	// next non-gate/non-heal job share the same attempt counter.
	baseGateIndex := failedStepIndex
	if modType == "re_gate" {
		var (
			baseFound     bool
			baseStepIndex float64
		)
		for _, job := range jobs {
			mt := strings.TrimSpace(job.ModType)
			if mt != "pre_gate" && mt != "post_gate" {
				continue
			}
			if job.StepIndex > float64(failedStepIndex) {
				continue
			}
			if !baseFound || job.StepIndex > baseStepIndex {
				baseFound = true
				baseStepIndex = job.StepIndex
			}
		}
		if baseFound {
			baseGateIndex = domaintypes.StepIndex(baseStepIndex)
		}
	}

	windowStart := float64(baseGateIndex)

	// Find the earliest non-healing, non-gate job after the base gate.
	// This bounds the healing window for this gate so that retries are
	// counted independently for each build gate.
	var (
		windowEnd     float64
		hasWindowEnd  bool
		isGateJobType = func(t string) bool {
			return t == "pre_gate" || t == "post_gate" || t == "re_gate"
		}
	)
	for _, job := range jobs {
		if job.StepIndex <= windowStart {
			continue
		}
		jt := strings.TrimSpace(job.ModType)
		if jt == "heal" {
			continue
		}
		if isGateJobType(jt) {
			// Gate jobs (pre/post/re) live inside the healing window and
			// must not terminate it.
			continue
		}
		if !hasWindowEnd || job.StepIndex < windowEnd {
			hasWindowEnd = true
			windowEnd = job.StepIndex
		}
	}

	// Count existing healing attempts for this gate by counting "heal" jobs
	// whose step_index lies within (baseGateIndex, windowEnd).
	healingAttempts := 0
	for _, job := range jobs {
		if strings.TrimSpace(job.ModType) != "heal" {
			continue
		}
		if job.StepIndex <= windowStart {
			continue
		}
		if hasWindowEnd && job.StepIndex >= windowEnd {
			continue
		}
		healingAttempts++
	}

	// Check if retries exhausted. Each attempt creates a single heal job and a
	// single re-gate job for this gate.
	healingAttemptNumber := healingAttempts + 1 // 1-based attempt number
	if healingAttemptNumber > retries {
		slog.Info("maybeCreateHealingJobs: healing retries exhausted",
			"run_id", runID,
			"attempt", healingAttemptNumber,
			"max_retries", retries,
		)

		// When healing retries are exhausted and the gate still fails, cancel
		// all remaining non-terminal jobs for the run so the control plane
		// can derive a terminal run state and avoid leaving mods/post-gate
		// jobs stranded in created/pending state.
		if err := cancelRemainingJobsAfterFailure(ctx, st, runID, failedStepIndex, jobs); err != nil {
			slog.Error("maybeCreateHealingJobs: failed to cancel remaining jobs after exhausted healing",
				"run_id", runID,
				"failed_step_index", failedStepIndex,
				"err", err,
			)
		}
		return nil
	}

	// Find the next job after the failed gate to calculate insertion range.
	nextStepIndex := float64(failedStepIndex) + 1000 // Default gap
	for _, job := range jobs {
		if job.StepIndex > float64(failedStepIndex) {
			if job.StepIndex < nextStepIndex {
				nextStepIndex = job.StepIndex
			}
		}
	}

	// Calculate step_index allocation for a single healing path:
	//   - heal job sits midway between failed gate and next mainline job
	//   - re-gate job sits three-quarters of the way to next mainline job
	gapSize := nextStepIndex - float64(failedStepIndex)
	if gapSize <= 0 {
		gapSize = 1000
	}

	healStepIndex := float64(failedStepIndex) + gapSize*0.5
	reGateStepIndex := float64(failedStepIndex) + gapSize*0.75

	slog.Info("maybeCreateHealingJobs: creating linear healing jobs",
		"run_id", runID,
		"failed_step_index", failedStepIndex,
		"next_step_index", nextStepIndex,
		"heal_step_index", healStepIndex,
		"re_gate_step_index", reGateStepIndex,
		"attempt", healingAttemptNumber,
	)

	// Use the first strategy and its first mod as the single healing mod.
	strategy := strategies[0]
	modMap := strategy.Mods[0]

	modImage := ""
	if img, ok := modMap["image"].(string); ok {
		modImage = strings.TrimSpace(img)
	}

	branchSuffix := strategy.Name
	if branchSuffix == "" {
		branchSuffix = "branch-0"
	}

	// Create a single healing job for this attempt.
	healJobName := fmt.Sprintf("heal-%s-%d-0", branchSuffix, healingAttemptNumber)
	_, err := st.CreateJob(ctx, store.CreateJobParams{
		ID:        string(domaintypes.NewJobID()),
		RunID:     runID.String(),
		Name:      healJobName,
		ModType:   "heal",
		ModImage:  modImage,
		Status:    store.JobStatusPending,
		StepIndex: healStepIndex,
		Meta:      []byte(`{}`),
	})
	if err != nil {
		return fmt.Errorf("create healing job %s: %w", healJobName, err)
	}

	slog.Info("created healing job",
		"run_id", runID,
		"job_name", healJobName,
		"step_index", healStepIndex,
		"status", store.JobStatusPending,
		"image", modImage,
		"branch", branchSuffix,
	)

	// Create a single re-gate job for this attempt.
	reGateName := fmt.Sprintf("re-gate-%s-%d", branchSuffix, healingAttemptNumber)
	_, err = st.CreateJob(ctx, store.CreateJobParams{
		ID:        string(domaintypes.NewJobID()),
		RunID:     runID.String(),
		Name:      reGateName,
		ModType:   "re_gate",
		ModImage:  "",
		Status:    store.JobStatusCreated,
		StepIndex: reGateStepIndex,
		Meta:      []byte(`{}`),
	})
	if err != nil {
		return fmt.Errorf("create re-gate job %s: %w", reGateName, err)
	}

	slog.Info("created re-gate job",
		"run_id", runID,
		"job_name", reGateName,
		"step_index", reGateStepIndex,
		"branch", branchSuffix,
	)

	return nil
}

// cancelLoserBranches and cancelHealingPathAfterNoChange have been removed as
// part of collapsing parallel healing into a single linear healing path.

// cancelRemainingJobsAfterFailure cancels all non-terminal jobs with
// step_index greater than the failed job's step_index. This is used after the
// system determines that no further progression is possible (e.g., healing
// retries exhausted, gate failure with no healing configured, or non-gate job
// failure) to avoid leaving jobs stranded in created/pending state.
func cancelRemainingJobsAfterFailure(
	ctx context.Context,
	st store.Store,
	runID domaintypes.RunID, // KSUID-backed string ID after run ID migration.
	failedStepIndex domaintypes.StepIndex,
	jobs []store.Job,
) error {
	now := time.Now().UTC()

	for _, job := range jobs {
		if job.StepIndex <= float64(failedStepIndex) {
			continue
		}

		switch job.Status {
		case store.JobStatusSucceeded, store.JobStatusFailed, store.JobStatusCanceled, store.JobStatusSkipped:
			continue
		}

		startedAt := job.StartedAt
		var durationMs int64
		if job.StartedAt.Valid {
			durationMs = now.Sub(job.StartedAt.Time).Milliseconds()
			if durationMs < 0 {
				durationMs = 0
			}
		}

		finishedAt := pgtype.Timestamptz{
			Time:  now,
			Valid: true,
		}

		if err := st.UpdateJobStatus(ctx, store.UpdateJobStatusParams{
			ID:         job.ID,
			Status:     store.JobStatusCanceled,
			StartedAt:  startedAt,
			FinishedAt: finishedAt,
			DurationMs: durationMs,
		}); err != nil {
			return fmt.Errorf("cancel job %s: %w", job.ID, err)
		}

		slog.Info("canceled job after failure",
			"run_id", runID,
			"job_id", job.ID, // Job IDs are KSUID strings.
			"step_index", job.StepIndex,
		)
	}

	return nil
}
