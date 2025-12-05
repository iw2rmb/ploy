package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// =============================================================================
// E5 — Parallel Healing Tests and Guardrails
// =============================================================================
//
// This file contains integration-style tests that validate end-to-end behavior
// for the parallel healing (multi-branch) healing pipeline per ROADMAP.md E5.
//
// Test coverage includes:
//   - Multi-strategy spec parsing and branch creation
//   - Branch execution isolation (distinct step_index windows)
//   - Winner selection when one branch succeeds
//   - Failure modes when all branches fail
//   - Run completion status derivation for multi-branch scenarios

// -----------------------------------------------------------------------------
// TestParallelHealing_AllBranchesFail_TicketFails verifies that when all healing
// branches fail (no re-gate succeeds), the run terminates with failed status.
//
// Scenario:
//   - pre-gate fails → two parallel branches created (codex, patcher)
//   - both branches: heal job succeeds, re-gate fails
//   - no winner exists → remaining jobs canceled → run status = failed
//
// This is the specific test name mentioned in ROADMAP.md E5.
// -----------------------------------------------------------------------------
func TestParallelHealing_AllBranchesFail_TicketFails(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	runUUID := uuid.New()
	runID := pgtype.UUID{Bytes: runUUID, Valid: true}

	// Multi-strategy spec: two branches, each with one healing mod.
	// When both re-gates fail, no winner exists and the run should fail.
	spec := map[string]any{
		"build_gate_healing": map[string]any{
			"retries": float64(1), // Only one attempt per branch.
			"strategies": []any{
				map[string]any{
					"name": "codex",
					"mods": []any{map[string]any{"image": "mods-codex:latest"}},
				},
				map[string]any{
					"name": "patcher",
					"mods": []any{map[string]any{"image": "mods-patcher:latest"}},
				},
			},
		},
	}
	specBytes, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("failed to marshal spec: %v", err)
	}

	// Jobs layout: pre-gate failed → both branches healed but re-gates failed.
	// After both re-gates fail, mod-0 should be canceled.
	jobs := []store.Job{
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     runID,
			Name:      "pre-gate",
			Status:    store.JobStatusFailed, // Initial gate failure triggers healing.
			ModType:   "pre_gate",
			StepIndex: 1000,
			Meta:      []byte(`{}`),
		},
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     runID,
			Name:      "heal-codex-1-0",
			Status:    store.JobStatusSucceeded, // Branch A heal succeeded.
			ModType:   "heal",
			StepIndex: 1333,
			Meta:      []byte(`{}`),
		},
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     runID,
			Name:      "re-gate-codex-1",
			Status:    store.JobStatusFailed, // Branch A re-gate FAILED.
			ModType:   "re_gate",
			StepIndex: 1444,
			Meta:      []byte(`{}`),
		},
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     runID,
			Name:      "heal-patcher-1-0",
			Status:    store.JobStatusSucceeded, // Branch B heal succeeded.
			ModType:   "heal",
			StepIndex: 1555,
			Meta:      []byte(`{}`),
		},
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     runID,
			Name:      "re-gate-patcher-1",
			Status:    store.JobStatusFailed, // Branch B re-gate FAILED.
			ModType:   "re_gate",
			StepIndex: 1666,
			Meta:      []byte(`{}`),
		},
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     runID,
			Name:      "mod-0",
			Status:    store.JobStatusCanceled, // Canceled after all healing exhausted.
			ModType:   "mod",
			StepIndex: 2000,
			Meta:      []byte(`{}`),
		},
	}

	run := store.Run{
		ID:   runID,
		Spec: specBytes,
	}

	st := &mockStore{
		listJobsByRunResult: jobs,
	}

	// Call maybeCompleteMultiStepRun to verify run completes with failed status.
	// All branches failed (both re-gates failed) → no winner → run fails.
	if err := maybeCompleteMultiStepRun(ctx, st, nil, run, runID); err != nil {
		t.Fatalf("maybeCompleteMultiStepRun returned error: %v", err)
	}

	// Verify: run should complete with "failed" status.
	// Failure is expected because:
	//   1. Both healing branches' re-gates failed.
	//   2. No winner was selected; mod-0 was canceled.
	//   3. The final gate status is "failed" (last re-gate in each branch failed).
	if !st.updateRunCompletionCalled {
		t.Fatalf("expected UpdateRunCompletion to be called")
	}
	if st.updateRunCompletionParams.Status != store.RunStatusFailed {
		t.Fatalf("expected run status=failed when all branches fail, got %s",
			st.updateRunCompletionParams.Status)
	}
}

// -----------------------------------------------------------------------------
// TestParallelHealing_OneBranchWins_RunSucceeds verifies that when one branch
// succeeds (re-gate passes), the run completes successfully despite other
// branches failing.
//
// Scenario:
//   - pre-gate fails → two parallel branches (codex, patcher)
//   - codex branch: heal succeeds, re-gate SUCCEEDS (winner)
//   - patcher branch: heal running → skipped by winner selection
//   - mod-0 executes and succeeds
//   - run status = succeeded
//
// -----------------------------------------------------------------------------
func TestParallelHealing_OneBranchWins_RunSucceeds(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	runUUID := uuid.New()
	runID := pgtype.UUID{Bytes: runUUID, Valid: true}

	// Jobs layout:
	//   pre-gate (1000) → FAILED
	//   codex branch: heal (1333) → SUCCEEDED, re-gate (1444) → SUCCEEDED (winner)
	//   patcher branch: heal (1555) → SKIPPED, re-gate (1666) → SKIPPED
	//   mod-0 (2000) → SUCCEEDED
	jobs := []store.Job{
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     runID,
			Name:      "pre-gate",
			Status:    store.JobStatusFailed,
			ModType:   "pre_gate",
			StepIndex: 1000,
			Meta:      []byte(`{}`),
		},
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     runID,
			Name:      "heal-codex-1-0",
			Status:    store.JobStatusSucceeded, // Winner branch heal.
			ModType:   "heal",
			StepIndex: 1333,
			Meta:      []byte(`{}`),
		},
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     runID,
			Name:      "re-gate-codex-1",
			Status:    store.JobStatusSucceeded, // Winner re-gate PASSED.
			ModType:   "re_gate",
			StepIndex: 1444,
			Meta:      []byte(`{}`),
		},
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     runID,
			Name:      "heal-patcher-1-0",
			Status:    store.JobStatusSkipped, // Loser branch skipped.
			ModType:   "heal",
			StepIndex: 1555,
			Meta:      []byte(`{}`),
		},
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     runID,
			Name:      "re-gate-patcher-1",
			Status:    store.JobStatusSkipped, // Loser branch skipped.
			ModType:   "re_gate",
			StepIndex: 1666,
			Meta:      []byte(`{}`),
		},
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     runID,
			Name:      "mod-0",
			Status:    store.JobStatusSucceeded, // Main mod succeeded after healing.
			ModType:   "mod",
			StepIndex: 2000,
			Meta:      []byte(`{}`),
		},
	}

	run := store.Run{
		ID:   runID,
		Spec: []byte(`{}`),
	}

	st := &mockStore{
		listJobsByRunResult: jobs,
	}

	if err := maybeCompleteMultiStepRun(ctx, st, nil, run, runID); err != nil {
		t.Fatalf("maybeCompleteMultiStepRun returned error: %v", err)
	}

	// Verify: run should complete with "succeeded" status.
	// Success expected because:
	//   1. One re-gate (codex) passed → healing was successful.
	//   2. Loser branch (patcher) was skipped (not failed).
	//   3. mod-0 completed successfully.
	if !st.updateRunCompletionCalled {
		t.Fatalf("expected UpdateRunCompletion to be called")
	}
	if st.updateRunCompletionParams.Status != store.RunStatusSucceeded {
		t.Fatalf("expected run status=succeeded when winner branch exists, got %s",
			st.updateRunCompletionParams.Status)
	}
}

// -----------------------------------------------------------------------------
// TestParallelHealing_BranchCreation_DistinctWindows verifies that when a gate
// fails with multi-strategy spec, parallel branches are created with distinct
// step_index windows that don't overlap.
//
// This tests the branch planner's step_index allocation algorithm (E2).
// -----------------------------------------------------------------------------
func TestParallelHealing_BranchCreation_DistinctWindows(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	runUUID := uuid.New()
	runID := pgtype.UUID{Bytes: runUUID, Valid: true}

	// Three strategies with varying numbers of mods to test window allocation.
	spec := map[string]any{
		"build_gate_healing": map[string]any{
			"retries": float64(2),
			"strategies": []any{
				map[string]any{
					"name": "fast",
					"mods": []any{
						map[string]any{"image": "fast-fix:latest"},
					},
				},
				map[string]any{
					"name": "thorough",
					"mods": []any{
						map[string]any{"image": "analyze:latest"},
						map[string]any{"image": "fix:latest"},
					},
				},
				map[string]any{
					"name": "ai",
					"mods": []any{
						map[string]any{"image": "mods-codex:latest"},
					},
				},
			},
		},
	}
	specBytes, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("failed to marshal spec: %v", err)
	}

	// Initial state: pre-gate failed, mod-0 waiting at step_index 2000.
	jobs := []store.Job{
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     runID,
			Name:      "pre-gate",
			Status:    store.JobStatusFailed,
			ModType:   "pre_gate",
			StepIndex: 1000,
			Meta:      []byte(`{}`),
		},
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     runID,
			Name:      "mod-0",
			Status:    store.JobStatusCreated,
			ModType:   "mod",
			StepIndex: 2000,
			Meta:      []byte(`{}`),
		},
	}

	run := store.Run{
		ID:   runID,
		Spec: specBytes,
	}

	st := &mockStore{
		listJobsByRunResult: jobs,
	}

	// Trigger healing job creation.
	if err := maybeCreateHealingJobs(ctx, st, run, runID, domaintypes.StepIndex(1000), jobs); err != nil {
		t.Fatalf("maybeCreateHealingJobs returned error: %v", err)
	}

	// Expected jobs created:
	//   Branch "fast": heal-fast-1-0 (pending), re-gate-fast-1 (created)
	//   Branch "thorough": heal-thorough-1-0 (pending), heal-thorough-1-1 (created), re-gate-thorough-1 (created)
	//   Branch "ai": heal-ai-1-0 (pending), re-gate-ai-1 (created)
	// Total: 2 + 3 + 2 = 7 jobs.
	expectedJobCount := 7
	if st.createJobCallCount != expectedJobCount {
		t.Fatalf("expected %d CreateJob calls, got %d", expectedJobCount, st.createJobCallCount)
	}

	// Build a map of created jobs by name for verification.
	jobsByName := make(map[string]store.CreateJobParams)
	for _, p := range st.createJobParams {
		jobsByName[p.Name] = p
	}

	// Verify distinct step_index windows for each branch.
	// Branch windows must not overlap, and all jobs must be between 1000 and 2000.
	branchJobs := map[string][]store.CreateJobParams{
		"fast":     {},
		"thorough": {},
		"ai":       {},
	}

	for name, p := range jobsByName {
		// Verify all jobs are in the healing window (1000 < step_index < 2000).
		if p.StepIndex <= 1000 || p.StepIndex >= 2000 {
			t.Fatalf("job %s step_index=%f should be between 1000 and 2000", name, p.StepIndex)
		}

		// Categorize jobs by branch.
		for branch := range branchJobs {
			if containsSubstring(name, branch) {
				branchJobs[branch] = append(branchJobs[branch], p)
			}
		}
	}

	// Verify each branch has correct job count and status.
	fastJobs := branchJobs["fast"]
	if len(fastJobs) != 2 { // 1 heal + 1 re-gate
		t.Fatalf("expected 2 fast branch jobs, got %d", len(fastJobs))
	}

	thoroughJobs := branchJobs["thorough"]
	if len(thoroughJobs) != 3 { // 2 heal + 1 re-gate
		t.Fatalf("expected 3 thorough branch jobs, got %d", len(thoroughJobs))
	}

	aiJobs := branchJobs["ai"]
	if len(aiJobs) != 2 { // 1 heal + 1 re-gate
		t.Fatalf("expected 2 ai branch jobs, got %d", len(aiJobs))
	}

	// Verify branch windows don't overlap by checking max(branch_i) < min(branch_j) for i < j.
	// The branch planner allocates windows in order: fast, thorough, ai.
	var fastMax, thoroughMin, thoroughMax, aiMin float64
	for _, p := range fastJobs {
		if p.StepIndex > fastMax {
			fastMax = p.StepIndex
		}
	}
	thoroughMin = 3000 // Initialize high.
	for _, p := range thoroughJobs {
		if p.StepIndex < thoroughMin {
			thoroughMin = p.StepIndex
		}
		if p.StepIndex > thoroughMax {
			thoroughMax = p.StepIndex
		}
	}
	aiMin = 3000 // Initialize high.
	for _, p := range aiJobs {
		if p.StepIndex < aiMin {
			aiMin = p.StepIndex
		}
	}

	// Verify non-overlapping windows.
	if fastMax >= thoroughMin {
		t.Fatalf("fast branch window (max=%f) overlaps with thorough branch (min=%f)", fastMax, thoroughMin)
	}
	if thoroughMax >= aiMin {
		t.Fatalf("thorough branch window (max=%f) overlaps with ai branch (min=%f)", thoroughMax, aiMin)
	}

	// Verify first heal job of each branch is pending (parallel start).
	for _, name := range []string{"heal-fast-1-0", "heal-thorough-1-0", "heal-ai-1-0"} {
		if p, ok := jobsByName[name]; ok {
			if p.Status != store.JobStatusPending {
				t.Fatalf("expected %s to be pending (branch first job), got %s", name, p.Status)
			}
		} else {
			t.Fatalf("expected job %s to be created", name)
		}
	}
}

// -----------------------------------------------------------------------------
// TestParallelHealing_WinnerSelection_CancelsLoserJobs verifies that when a
// re-gate succeeds (winner selected), all other branch jobs in non-terminal
// state are skipped.
//
// This tests the winner selection and loser teardown logic (E4).
// -----------------------------------------------------------------------------
func TestParallelHealing_WinnerSelection_CancelsLoserJobs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	runUUID := uuid.New()
	runID := pgtype.UUID{Bytes: runUUID, Valid: true}

	// Scenario: Three branches, middle branch (thorough) wins first.
	// Winner: re-gate-thorough-1 (step_index 1500)
	// Losers to skip:
	//   - heal-fast-1-0 (running)
	//   - re-gate-fast-1 (created)
	//   - heal-ai-1-0 (pending)
	//   - re-gate-ai-1 (created)
	winnerJobID := uuid.New()
	loserFastHealID := uuid.New()
	loserFastReGateID := uuid.New()
	loserAiHealID := uuid.New()
	loserAiReGateID := uuid.New()
	mainlineModID := uuid.New()

	jobs := []store.Job{
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     runID,
			Name:      "pre-gate",
			Status:    store.JobStatusFailed,
			ModType:   "pre_gate",
			StepIndex: 1000,
		},
		// Fast branch (jobs in progress when winner selected).
		{
			ID:        pgtype.UUID{Bytes: loserFastHealID, Valid: true},
			RunID:     runID,
			Name:      "heal-fast-1-0",
			Status:    store.JobStatusRunning, // Should be skipped.
			ModType:   "heal",
			StepIndex: 1200,
		},
		{
			ID:        pgtype.UUID{Bytes: loserFastReGateID, Valid: true},
			RunID:     runID,
			Name:      "re-gate-fast-1",
			Status:    store.JobStatusCreated, // Should be skipped.
			ModType:   "re_gate",
			StepIndex: 1300,
		},
		// Thorough branch (winner).
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     runID,
			Name:      "heal-thorough-1-0",
			Status:    store.JobStatusSucceeded, // Winner heal completed.
			ModType:   "heal",
			StepIndex: 1400,
		},
		{
			ID:        pgtype.UUID{Bytes: winnerJobID, Valid: true},
			RunID:     runID,
			Name:      "re-gate-thorough-1",
			Status:    store.JobStatusSucceeded, // WINNER re-gate.
			ModType:   "re_gate",
			StepIndex: 1500,
		},
		// AI branch (not yet started when winner selected).
		{
			ID:        pgtype.UUID{Bytes: loserAiHealID, Valid: true},
			RunID:     runID,
			Name:      "heal-ai-1-0",
			Status:    store.JobStatusPending, // Should be skipped.
			ModType:   "heal",
			StepIndex: 1600,
		},
		{
			ID:        pgtype.UUID{Bytes: loserAiReGateID, Valid: true},
			RunID:     runID,
			Name:      "re-gate-ai-1",
			Status:    store.JobStatusCreated, // Should be skipped.
			ModType:   "re_gate",
			StepIndex: 1700,
		},
		// Mainline job (should NOT be affected).
		{
			ID:        pgtype.UUID{Bytes: mainlineModID, Valid: true},
			RunID:     runID,
			Name:      "mod-0",
			Status:    store.JobStatusCreated,
			ModType:   "mod",
			StepIndex: 2000,
		},
	}

	winnerJob := jobs[4] // re-gate-thorough-1

	st := &mockStore{
		listJobsByRunResult: jobs,
	}

	if err := cancelLoserBranches(ctx, st, runID, winnerJob, jobs); err != nil {
		t.Fatalf("cancelLoserBranches returned error: %v", err)
	}

	// Verify: loser branch jobs should be marked as skipped.
	// Expected skipped jobs: 4 (fast heal, fast re-gate, ai heal, ai re-gate).
	if len(st.updateJobStatusCalls) != 4 {
		t.Fatalf("expected 4 UpdateJobStatus calls for loser jobs, got %d", len(st.updateJobStatusCalls))
	}

	// Collect skipped job IDs.
	skippedIDs := make(map[uuid.UUID]bool)
	for _, call := range st.updateJobStatusCalls {
		if call.Status != store.JobStatusSkipped {
			t.Fatalf("expected skipped status for loser job, got %s", call.Status)
		}
		skippedIDs[uuid.UUID(call.ID.Bytes)] = true
	}

	// Verify correct loser jobs were skipped.
	expectedSkipped := []uuid.UUID{loserFastHealID, loserFastReGateID, loserAiHealID, loserAiReGateID}
	for _, id := range expectedSkipped {
		if !skippedIDs[id] {
			t.Fatalf("expected job %s to be skipped", id)
		}
	}

	// Verify mainline mod was NOT skipped.
	if skippedIDs[mainlineModID] {
		t.Fatalf("mainline mod-0 should NOT be skipped")
	}
}

// -----------------------------------------------------------------------------
// TestParallelHealing_RetriesExhausted_AllBranchesCanceled verifies that when
// healing retries are exhausted across all branches, remaining jobs are canceled.
//
// Scenario:
//   - pre-gate fails → branches created (attempt 1)
//   - all re-gates fail → retries exhausted
//   - mod-0 canceled → run fails
//
// -----------------------------------------------------------------------------
func TestParallelHealing_RetriesExhausted_AllBranchesCanceled(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	runUUID := uuid.New()
	runID := pgtype.UUID{Bytes: runUUID, Valid: true}

	// Spec with retries=1 (only one healing attempt per branch).
	spec := map[string]any{
		"build_gate_healing": map[string]any{
			"retries": float64(1),
			"strategies": []any{
				map[string]any{
					"name": "branch-a",
					"mods": []any{map[string]any{"image": "heal-a:latest"}},
				},
				map[string]any{
					"name": "branch-b",
					"mods": []any{map[string]any{"image": "heal-b:latest"}},
				},
			},
		},
	}
	specBytes, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("failed to marshal spec: %v", err)
	}

	modJobID := uuid.New()

	// Jobs after attempt 1 completes: both re-gates failed.
	jobs := []store.Job{
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     runID,
			Name:      "pre-gate",
			Status:    store.JobStatusFailed,
			ModType:   "pre_gate",
			StepIndex: 1000,
			Meta:      []byte(`{}`),
		},
		// Branch A attempt 1.
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     runID,
			Name:      "heal-branch-a-1-0",
			Status:    store.JobStatusSucceeded,
			ModType:   "heal",
			StepIndex: 1200,
			Meta:      []byte(`{}`),
		},
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     runID,
			Name:      "re-gate-branch-a-1",
			Status:    store.JobStatusFailed, // Re-gate failed.
			ModType:   "re_gate",
			StepIndex: 1300,
			Meta:      []byte(`{}`),
		},
		// Branch B attempt 1.
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     runID,
			Name:      "heal-branch-b-1-0",
			Status:    store.JobStatusSucceeded,
			ModType:   "heal",
			StepIndex: 1500,
			Meta:      []byte(`{}`),
		},
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     runID,
			Name:      "re-gate-branch-b-1",
			Status:    store.JobStatusFailed, // Re-gate failed.
			ModType:   "re_gate",
			StepIndex: 1600,
			Meta:      []byte(`{}`),
		},
		// Mainline mod still waiting.
		{
			ID:        pgtype.UUID{Bytes: modJobID, Valid: true},
			RunID:     runID,
			Name:      "mod-0",
			Status:    store.JobStatusCreated, // Should be canceled.
			ModType:   "mod",
			StepIndex: 2000,
			Meta:      []byte(`{}`),
		},
	}

	run := store.Run{
		ID:   runID,
		Spec: specBytes,
	}

	st := &mockStore{
		listJobsByRunResult: jobs,
	}

	// Simulate the last re-gate failing and triggering retry check.
	// failedStepIndex=1600 (re-gate-branch-b-1).
	if err := maybeCreateHealingJobs(ctx, st, run, runID, domaintypes.StepIndex(1600), jobs); err != nil {
		t.Fatalf("maybeCreateHealingJobs returned error: %v", err)
	}

	// Verify: no new healing jobs created (retries exhausted).
	if st.createJobCallCount != 0 {
		t.Fatalf("expected 0 CreateJob calls (retries exhausted), got %d", st.createJobCallCount)
	}

	// Verify: mod-0 was canceled.
	if !st.updateJobStatusCalled {
		t.Fatal("expected UpdateJobStatus to be called to cancel remaining jobs")
	}

	var modCanceled bool
	for _, call := range st.updateJobStatusCalls {
		if uuid.UUID(call.ID.Bytes) == modJobID && call.Status == store.JobStatusCanceled {
			modCanceled = true
		}
	}
	if !modCanceled {
		t.Fatal("expected mod-0 to be canceled after retries exhausted")
	}
}

// -----------------------------------------------------------------------------
// TestParallelHealing_MixedBranchOutcomes verifies correct behavior when branches
// have different outcomes (some succeed, some fail, some still running).
//
// Scenario:
//   - Branch A: re-gate succeeds (winner)
//   - Branch B: re-gate fails (loser - terminal)
//   - Branch C: heal running (loser - should be skipped)
//
// -----------------------------------------------------------------------------
func TestParallelHealing_MixedBranchOutcomes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	runUUID := uuid.New()
	runID := pgtype.UUID{Bytes: runUUID, Valid: true}

	winnerJobID := uuid.New()
	loserFailedReGateID := uuid.New()
	loserRunningHealID := uuid.New()
	loserCreatedReGateID := uuid.New()

	jobs := []store.Job{
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     runID,
			Name:      "pre-gate",
			Status:    store.JobStatusFailed,
			ModType:   "pre_gate",
			StepIndex: 1000,
		},
		// Branch A: winner.
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     runID,
			Name:      "heal-a-1-0",
			Status:    store.JobStatusSucceeded,
			ModType:   "heal",
			StepIndex: 1200,
		},
		{
			ID:        pgtype.UUID{Bytes: winnerJobID, Valid: true},
			RunID:     runID,
			Name:      "re-gate-a-1",
			Status:    store.JobStatusSucceeded, // WINNER.
			ModType:   "re_gate",
			StepIndex: 1300,
		},
		// Branch B: already failed (should NOT be re-skipped).
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     runID,
			Name:      "heal-b-1-0",
			Status:    store.JobStatusSucceeded,
			ModType:   "heal",
			StepIndex: 1400,
		},
		{
			ID:        pgtype.UUID{Bytes: loserFailedReGateID, Valid: true},
			RunID:     runID,
			Name:      "re-gate-b-1",
			Status:    store.JobStatusFailed, // Already terminal.
			ModType:   "re_gate",
			StepIndex: 1500,
		},
		// Branch C: still running (should be skipped).
		{
			ID:        pgtype.UUID{Bytes: loserRunningHealID, Valid: true},
			RunID:     runID,
			Name:      "heal-c-1-0",
			Status:    store.JobStatusRunning, // Should be skipped.
			ModType:   "heal",
			StepIndex: 1600,
		},
		{
			ID:        pgtype.UUID{Bytes: loserCreatedReGateID, Valid: true},
			RunID:     runID,
			Name:      "re-gate-c-1",
			Status:    store.JobStatusCreated, // Should be skipped.
			ModType:   "re_gate",
			StepIndex: 1700,
		},
		// Mainline.
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     runID,
			Name:      "mod-0",
			Status:    store.JobStatusCreated,
			ModType:   "mod",
			StepIndex: 2000,
		},
	}

	winnerJob := jobs[2] // re-gate-a-1

	st := &mockStore{
		listJobsByRunResult: jobs,
	}

	if err := cancelLoserBranches(ctx, st, runID, winnerJob, jobs); err != nil {
		t.Fatalf("cancelLoserBranches returned error: %v", err)
	}

	// Verify: only non-terminal loser jobs should be skipped (Branch C jobs).
	// Branch B's re-gate is already failed (terminal), so it should NOT be updated.
	if len(st.updateJobStatusCalls) != 2 {
		t.Fatalf("expected 2 UpdateJobStatus calls (Branch C jobs), got %d", len(st.updateJobStatusCalls))
	}

	skippedIDs := make(map[uuid.UUID]bool)
	for _, call := range st.updateJobStatusCalls {
		skippedIDs[uuid.UUID(call.ID.Bytes)] = true
	}

	// Verify Branch C jobs were skipped.
	if !skippedIDs[loserRunningHealID] {
		t.Fatal("expected heal-c-1-0 (running) to be skipped")
	}
	if !skippedIDs[loserCreatedReGateID] {
		t.Fatal("expected re-gate-c-1 (created) to be skipped")
	}

	// Verify Branch B's failed re-gate was NOT updated (already terminal).
	if skippedIDs[loserFailedReGateID] {
		t.Fatal("re-gate-b-1 (already failed) should NOT be updated")
	}
}

// containsSubstring is a helper to check if s contains substr (case-insensitive).
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && findSubstringCI(s, substr)
}

// findSubstringCI performs case-insensitive substring search.
func findSubstringCI(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			c1, c2 := s[i+j], substr[j]
			// Normalize to lowercase.
			if c1 >= 'A' && c1 <= 'Z' {
				c1 += 'a' - 'A'
			}
			if c2 >= 'A' && c2 <= 'Z' {
				c2 += 'a' - 'A'
			}
			if c1 != c2 {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
