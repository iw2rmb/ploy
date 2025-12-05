package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// healingStrategy represents a named healing strategy (branch) parsed from the spec.
// A strategy contains a name and a list of healing mods to execute sequentially.
type healingStrategy struct {
	Name string           // Strategy name (e.g., "codex-ai", "static-patch"); empty for legacy single-strategy.
	Mods []map[string]any // Ordered list of healing mod definitions (image, command, env, etc.).
}

// parseHealingStrategies extracts healing strategies from the build_gate_healing config.
// Returns a slice of strategies that supports both legacy single-strategy (mods[]) and
// multi-strategy (strategies[]) forms.
//
// The function handles three cases:
//  1. Legacy form: build_gate_healing.mods[] — maps to a single unnamed strategy.
//  2. Multi-strategy form: build_gate_healing.strategies[] — each entry becomes a strategy.
//  3. Both present: strategies[] takes precedence (as documented in mod.example.yaml).
func parseHealingStrategies(healingConfig map[string]any) []healingStrategy {
	// Check for multi-strategy form first (takes precedence per docs).
	if strategiesRaw, ok := healingConfig["strategies"].([]any); ok && len(strategiesRaw) > 0 {
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

	// Fallback to legacy single-strategy form (mods[] at top level).
	if modsRaw, ok := healingConfig["mods"].([]any); ok && len(modsRaw) > 0 {
		var mods []map[string]any
		for _, mRaw := range modsRaw {
			if m, ok := mRaw.(map[string]any); ok {
				mods = append(mods, m)
			}
		}
		if len(mods) > 0 {
			// Return a single unnamed strategy for backward compatibility.
			return []healingStrategy{{Name: "", Mods: mods}}
		}
	}

	return nil
}

// maybeCreateHealingJobs creates healing jobs and a re-gate job when a gate job fails.
// This is called when a gate job (pre_gate, post_gate, re_gate) completes with reason="build-gate".
//
// The function:
// 1. Finds the failed gate job by step_index
// 2. Verifies it's a gate job (pre_gate, post_gate, re_gate)
// 3. Checks if healing is configured in the run spec
// 4. Counts existing healing attempts to enforce retry limits
// 5. Creates healing jobs and a re-gate job at intermediate step_index values
//
// For multi-strategy specs, creates parallel branches with distinct step_index windows:
//
//	pre-gate (1000) → FAIL → branch-a: heal-a-0 (1500), re-gate-a (1600)
//	                       → branch-b: heal-b-0 (1700), re-gate-b (1800)
//	→ mod-0 (2000)
//
// For legacy single-strategy specs, maintains existing behavior:
//
//	pre-gate (1000) → FAIL → healing-0 (1100) → healing-1 (1200) → re-gate (1300) → mod-0 (2000)
func maybeCreateHealingJobs(
	ctx context.Context,
	st store.Store,
	run store.Run,
	runID pgtype.UUID,
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

	// Parse healing strategies (supports both legacy mods[] and multi-strategy forms).
	strategies := parseHealingStrategies(healingConfig)
	if len(strategies) == 0 {
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

	// Count total mods across all strategies for attempt calculation.
	// For multi-strategy, we count all mods from all branches.
	totalModsAcrossStrategies := 0
	for _, s := range strategies {
		totalModsAcrossStrategies += len(s.Mods)
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

	// Check if retries exhausted.
	// For multi-strategy, each attempt creates jobs for all strategies in parallel,
	// so we divide by totalModsAcrossStrategies to get the attempt number.
	healingAttemptNumber := healingAttempts/totalModsAcrossStrategies + 1 // 1-based attempt number
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

	// Calculate step_index allocation based on strategy count.
	// Total gap is divided into windows per strategy, plus a buffer before nextStepIndex.
	// Each branch gets its own contiguous step_index window for its heal jobs + re-gate.
	gapSize := nextStepIndex - float64(failedStepIndex)
	numStrategies := len(strategies)

	// For multi-strategy, allocate distinct windows per branch (e.g., 1500-1600, 1700-1800).
	// For single-strategy (legacy), use the existing behavior with evenly distributed indices.
	if numStrategies > 1 {
		// Multi-strategy branch planner: allocate non-overlapping step_index windows.
		// Window size = gap / (numStrategies + 1) to leave buffer before nextStepIndex.
		windowSize := gapSize / float64(numStrategies+1)

		slog.Info("maybeCreateHealingJobs: creating multi-branch healing jobs",
			"run_id", runID,
			"failed_step_index", failedStepIndex,
			"next_step_index", nextStepIndex,
			"num_strategies", numStrategies,
			"window_size", windowSize,
			"attempt", healingAttemptNumber,
		)

		// Create jobs for each strategy branch in parallel (all first jobs are pending).
		// Each branch gets a distinct step_index window.
		for branchIdx, strategy := range strategies {
			// Branch window starts at: failedStepIndex + windowSize * (branchIdx + 1)
			// This places branch 0 at (failedStepIndex + windowSize), branch 1 at (failedStepIndex + 2*windowSize), etc.
			branchWindowStart := float64(failedStepIndex) + windowSize*float64(branchIdx+1)

			// Within the branch, distribute mods + re-gate evenly.
			// branchIncrement = windowSize / (len(mods) + 2) to fit mods and re-gate with buffer.
			modsCount := len(strategy.Mods)
			branchIncrement := windowSize / float64(modsCount+2)

			// Derive branch name suffix for job naming.
			branchSuffix := strategy.Name
			if branchSuffix == "" {
				branchSuffix = fmt.Sprintf("branch-%d", branchIdx)
			}

			// Create healing jobs for this branch.
			// First job of each branch is 'pending' (parallel execution across branches).
			for modIdx, modMap := range strategy.Mods {
				modImage := ""
				if img, ok := modMap["image"].(string); ok {
					modImage = strings.TrimSpace(img)
				}

				// step_index for this mod within the branch window.
				healStepIndex := branchWindowStart + branchIncrement*float64(modIdx+1)

				// First job of each branch is pending (parallel branches execute concurrently).
				jobStatus := store.JobStatusCreated
				if modIdx == 0 {
					jobStatus = store.JobStatusPending
				}

				// Job name includes attempt, branch name, and mod index.
				jobName := fmt.Sprintf("heal-%s-%d-%d", branchSuffix, healingAttemptNumber, modIdx)
				_, err := st.CreateJob(ctx, store.CreateJobParams{
					RunID:     runID,
					Name:      jobName,
					ModType:   "heal",
					ModImage:  modImage,
					Status:    jobStatus,
					StepIndex: healStepIndex,
					Meta:      []byte(`{}`),
				})
				if err != nil {
					return fmt.Errorf("create healing job %s: %w", jobName, err)
				}

				slog.Info("created healing job",
					"run_id", runID,
					"job_name", jobName,
					"step_index", healStepIndex,
					"status", jobStatus,
					"image", modImage,
					"branch", branchSuffix,
				)
			}

			// Create re-gate job for this branch (after its healing mods).
			reGateStepIndex := branchWindowStart + branchIncrement*float64(modsCount+1)
			reGateName := fmt.Sprintf("re-gate-%s-%d", branchSuffix, healingAttemptNumber)
			_, err := st.CreateJob(ctx, store.CreateJobParams{
				RunID:     runID,
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
		}

		return nil
	}

	// Single-strategy (legacy) behavior: create healing jobs sequentially with one re-gate.
	// This preserves backward compatibility for specs with only mods[] and no strategies[].
	strategy := strategies[0]
	healingMods := strategy.Mods
	healingCount := len(healingMods)
	stepIncrement := gapSize / float64(healingCount+2) // +2 for re-gate and buffer

	slog.Info("maybeCreateHealingJobs: creating healing jobs",
		"run_id", runID,
		"failed_step_index", failedStepIndex,
		"next_step_index", nextStepIndex,
		"healing_count", healingCount,
		"attempt", healingAttemptNumber,
	)

	// Create healing jobs.
	// Server-driven scheduling: first healing job is 'pending' (runs immediately),
	// subsequent jobs are 'created' (wait for server to schedule after prior completes).
	for i, modMap := range healingMods {
		// Extract healing mod image.
		modImage := ""
		if img, ok := modMap["image"].(string); ok {
			modImage = strings.TrimSpace(img)
		}

		// Calculate step_index for this healing job.
		healStepIndex := float64(failedStepIndex) + stepIncrement*float64(i+1)

		// First healing job is pending (ready to claim), others are created.
		jobStatus := store.JobStatusCreated
		if i == 0 {
			jobStatus = store.JobStatusPending
		}

		// Create the healing job.
		jobName := fmt.Sprintf("heal-%d-%d", healingAttemptNumber, i)
		_, err := st.CreateJob(ctx, store.CreateJobParams{
			RunID:     runID,
			Name:      jobName,
			ModType:   "heal",
			ModImage:  modImage,
			Status:    jobStatus,
			StepIndex: healStepIndex,
			Meta:      []byte(`{}`),
		})
		if err != nil {
			return fmt.Errorf("create healing job %s: %w", jobName, err)
		}

		slog.Info("created healing job",
			"run_id", runID,
			"job_name", jobName,
			"step_index", healStepIndex,
			"status", jobStatus,
			"image", modImage,
		)
	}

	// Create re-gate job after healing jobs - starts as 'created'.
	reGateStepIndex := float64(failedStepIndex) + stepIncrement*float64(healingCount+1)
	reGateName := fmt.Sprintf("re-gate-%d", healingAttemptNumber)
	_, err := st.CreateJob(ctx, store.CreateJobParams{
		RunID:     runID,
		Name:      reGateName,
		ModType:   "re_gate",
		ModImage:  "",
		Status:    store.JobStatusCreated,
		StepIndex: reGateStepIndex,
		Meta:      []byte(`{}`),
	})
	if err != nil {
		return fmt.Errorf("create re-gate job: %w", err)
	}

	slog.Info("created re-gate job",
		"run_id", runID,
		"job_name", reGateName,
		"step_index", reGateStepIndex,
	)

	return nil
}

// cancelRemainingJobsAfterFailure cancels all non-terminal jobs with
// step_index greater than the failed job's step_index. This is used after the
// system determines that no further progression is possible (e.g., healing
// retries exhausted, gate failure with no healing configured, or non-gate job
// failure) to avoid leaving jobs stranded in created/pending state.
func cancelRemainingJobsAfterFailure(
	ctx context.Context,
	st store.Store,
	runID pgtype.UUID,
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
			return fmt.Errorf("cancel job %s: %w", uuid.UUID(job.ID.Bytes).String(), err)
		}

		slog.Info("canceled job after failure",
			"run_id", runID,
			"job_id", uuid.UUID(job.ID.Bytes).String(),
			"step_index", job.StepIndex,
		)
	}

	return nil
}
