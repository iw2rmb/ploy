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

// TestMaybeCreateHealingJobs_SecondAttemptUsesModType verifies that healing
// retries are counted using the jobs.mod_type column so that subsequent
// attempts receive incremented attempt numbers (heal-2-0, re-gate-2, ...),
// avoiding duplicate job names on re-gate failure.
func TestMaybeCreateHealingJobs_SecondAttemptUsesModType(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	runUUID := uuid.New()
	runID := pgtype.UUID{Bytes: runUUID, Valid: true}

	// build_gate_healing with a single healing mod and retries=3
	spec := map[string]any{
		"build_gate_healing": map[string]any{
			"retries": float64(3),
			"mods": []any{
				map[string]any{"image": "heal:latest"},
			},
		},
	}
	specBytes, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("failed to marshal spec: %v", err)
	}

	// Jobs for a run where:
	// - pre-gate (1000) failed previously
	// - heal-1-0 (1333.33) succeeded
	// - re-gate-1 (1666.66) has just failed (failedStepIndex)
	// - mod-0/post-gate are still created
	reGateStepIndex := 1666.6666666666665
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
			Name:      "heal-1-0",
			Status:    store.JobStatusSucceeded,
			ModType:   "heal",
			StepIndex: 1333.3333333333333,
			Meta:      []byte(`{}`),
		},
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     runID,
			Name:      "re-gate-1",
			Status:    store.JobStatusFailed,
			ModType:   "re_gate",
			StepIndex: reGateStepIndex,
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
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     runID,
			Name:      "post-gate",
			Status:    store.JobStatusCreated,
			ModType:   "post_gate",
			StepIndex: 3000,
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

	if err := maybeCreateHealingJobs(ctx, st, run, runID, domaintypes.StepIndex(reGateStepIndex), jobs); err != nil {
		t.Fatalf("maybeCreateHealingJobs returned error: %v", err)
	}

	// With one existing "heal" job and one healing mod configured, the second
	// invocation should use attempt=2 and create:
	//   - heal-2-0 (pending)
	//   - re-gate-2 (created)
	if st.createJobCallCount != 2 {
		t.Fatalf("expected 2 CreateJob calls (heal + re-gate), got %d", st.createJobCallCount)
	}
	if len(st.createJobParams) != 2 {
		t.Fatalf("expected 2 CreateJobParams entries, got %d", len(st.createJobParams))
	}

	healJob := st.createJobParams[0]
	if healJob.Name != "heal-2-0" {
		t.Fatalf("expected first healing job name heal-2-0, got %q", healJob.Name)
	}
	if healJob.ModType != "heal" {
		t.Fatalf("expected heal job ModType=heal, got %q", healJob.ModType)
	}

	reGateJob := st.createJobParams[1]
	if reGateJob.Name != "re-gate-2" {
		t.Fatalf("expected re-gate job name re-gate-2, got %q", reGateJob.Name)
	}
	if reGateJob.ModType != "re_gate" {
		t.Fatalf("expected re-gate job ModType=re_gate, got %q", reGateJob.ModType)
	}
}

// TestMaybeCreateHealingJobs_MultiBranchStrategies verifies that the branch planner
// creates parallel healing branches from multi-strategy specs with distinct step_index windows.
func TestMaybeCreateHealingJobs_MultiBranchStrategies(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	runUUID := uuid.New()
	runID := pgtype.UUID{Bytes: runUUID, Valid: true}

	// Multi-strategy spec with two named branches, each with one healing mod.
	spec := map[string]any{
		"build_gate_healing": map[string]any{
			"retries": float64(2),
			"strategies": []any{
				map[string]any{
					"name": "codex-ai",
					"mods": []any{
						map[string]any{"image": "mods-codex:latest"},
					},
				},
				map[string]any{
					"name": "static-patch",
					"mods": []any{
						map[string]any{"image": "mods-patcher:latest"},
					},
				},
			},
		},
	}
	specBytes, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("failed to marshal spec: %v", err)
	}

	// Jobs for a fresh run where pre-gate just failed.
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

	if err := maybeCreateHealingJobs(ctx, st, run, runID, domaintypes.StepIndex(1000), jobs); err != nil {
		t.Fatalf("maybeCreateHealingJobs returned error: %v", err)
	}

	// With 2 strategies, each with 1 mod, we expect:
	// - Branch "codex-ai": heal-codex-ai-1-0 (pending), re-gate-codex-ai-1 (created)
	// - Branch "static-patch": heal-static-patch-1-0 (pending), re-gate-static-patch-1 (created)
	// Total: 4 jobs created.
	if st.createJobCallCount != 4 {
		t.Fatalf("expected 4 CreateJob calls (2 branches × (1 heal + 1 re-gate)), got %d", st.createJobCallCount)
	}

	// Verify job names and step_index windows are distinct.
	jobsByName := make(map[string]store.CreateJobParams)
	for _, p := range st.createJobParams {
		jobsByName[p.Name] = p
	}

	// Branch "codex-ai" jobs.
	codexHeal, ok := jobsByName["heal-codex-ai-1-0"]
	if !ok {
		t.Fatalf("expected heal-codex-ai-1-0 job to be created")
	}
	if codexHeal.Status != store.JobStatusPending {
		t.Fatalf("expected heal-codex-ai-1-0 to be pending (first job of branch), got %s", codexHeal.Status)
	}
	if codexHeal.ModImage != "mods-codex:latest" {
		t.Fatalf("expected heal-codex-ai-1-0 image=mods-codex:latest, got %q", codexHeal.ModImage)
	}

	codexReGate, ok := jobsByName["re-gate-codex-ai-1"]
	if !ok {
		t.Fatalf("expected re-gate-codex-ai-1 job to be created")
	}
	if codexReGate.Status != store.JobStatusCreated {
		t.Fatalf("expected re-gate-codex-ai-1 to be created, got %s", codexReGate.Status)
	}

	// Branch "static-patch" jobs.
	patchHeal, ok := jobsByName["heal-static-patch-1-0"]
	if !ok {
		t.Fatalf("expected heal-static-patch-1-0 job to be created")
	}
	if patchHeal.Status != store.JobStatusPending {
		t.Fatalf("expected heal-static-patch-1-0 to be pending (first job of branch), got %s", patchHeal.Status)
	}
	if patchHeal.ModImage != "mods-patcher:latest" {
		t.Fatalf("expected heal-static-patch-1-0 image=mods-patcher:latest, got %q", patchHeal.ModImage)
	}

	patchReGate, ok := jobsByName["re-gate-static-patch-1"]
	if !ok {
		t.Fatalf("expected re-gate-static-patch-1 job to be created")
	}
	if patchReGate.Status != store.JobStatusCreated {
		t.Fatalf("expected re-gate-static-patch-1 to be created, got %s", patchReGate.Status)
	}

	// Verify distinct step_index windows (branches must not overlap).
	// Window 1 (codex-ai): step_index values in (1000, 1333.33...)
	// Window 2 (static-patch): step_index values in (1333.33..., 1666.66...)
	if codexHeal.StepIndex >= patchHeal.StepIndex {
		t.Fatalf("codex-ai branch should have lower step_index than static-patch branch: codex=%f, patch=%f",
			codexHeal.StepIndex, patchHeal.StepIndex)
	}
	if codexReGate.StepIndex >= patchHeal.StepIndex {
		t.Fatalf("codex-ai re-gate should have lower step_index than static-patch heal: re-gate=%f, patch-heal=%f",
			codexReGate.StepIndex, patchHeal.StepIndex)
	}

	// Verify all jobs are between failedStepIndex (1000) and nextStepIndex (2000).
	for name, p := range jobsByName {
		if p.StepIndex <= 1000 || p.StepIndex >= 2000 {
			t.Fatalf("job %s step_index=%f should be between 1000 and 2000", name, p.StepIndex)
		}
	}
}

// TestMaybeCreateHealingJobs_LegacySingleStrategy verifies backward compatibility
// with legacy specs that use build_gate_healing.mods[] instead of strategies[].
func TestMaybeCreateHealingJobs_LegacySingleStrategy(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	runUUID := uuid.New()
	runID := pgtype.UUID{Bytes: runUUID, Valid: true}

	// Legacy single-strategy spec (mods at top level, no strategies array).
	spec := map[string]any{
		"build_gate_healing": map[string]any{
			"retries": float64(2),
			"mods": []any{
				map[string]any{"image": "heal-a:latest"},
				map[string]any{"image": "heal-b:latest"},
			},
		},
	}
	specBytes, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("failed to marshal spec: %v", err)
	}

	// Jobs for a fresh run where pre-gate just failed.
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

	if err := maybeCreateHealingJobs(ctx, st, run, runID, domaintypes.StepIndex(1000), jobs); err != nil {
		t.Fatalf("maybeCreateHealingJobs returned error: %v", err)
	}

	// Legacy behavior: 2 healing mods + 1 re-gate = 3 jobs.
	if st.createJobCallCount != 3 {
		t.Fatalf("expected 3 CreateJob calls (2 heal + 1 re-gate), got %d", st.createJobCallCount)
	}

	// Verify legacy job naming (heal-1-0, heal-1-1, re-gate-1).
	expectedNames := []string{"heal-1-0", "heal-1-1", "re-gate-1"}
	for i, expected := range expectedNames {
		if st.createJobParams[i].Name != expected {
			t.Fatalf("expected job[%d] name=%q, got %q", i, expected, st.createJobParams[i].Name)
		}
	}

	// First healing job is pending, others are created.
	if st.createJobParams[0].Status != store.JobStatusPending {
		t.Fatalf("expected heal-1-0 to be pending, got %s", st.createJobParams[0].Status)
	}
	if st.createJobParams[1].Status != store.JobStatusCreated {
		t.Fatalf("expected heal-1-1 to be created, got %s", st.createJobParams[1].Status)
	}
	if st.createJobParams[2].Status != store.JobStatusCreated {
		t.Fatalf("expected re-gate-1 to be created, got %s", st.createJobParams[2].Status)
	}

	// Verify step_index order (sequential, not branched).
	for i := 1; i < len(st.createJobParams); i++ {
		if st.createJobParams[i].StepIndex <= st.createJobParams[i-1].StepIndex {
			t.Fatalf("step_index should increase: job[%d]=%f, job[%d]=%f",
				i-1, st.createJobParams[i-1].StepIndex,
				i, st.createJobParams[i].StepIndex)
		}
	}
}

// TestMaybeCreateHealingJobs_MultiBranchWithMultipleMods verifies multi-branch creation
// when each strategy has multiple healing mods.
func TestMaybeCreateHealingJobs_MultiBranchWithMultipleMods(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	runUUID := uuid.New()
	runID := pgtype.UUID{Bytes: runUUID, Valid: true}

	// Multi-strategy spec: branch A has 2 mods, branch B has 1 mod.
	spec := map[string]any{
		"build_gate_healing": map[string]any{
			"retries": float64(1),
			"strategies": []any{
				map[string]any{
					"name": "branch-a",
					"mods": []any{
						map[string]any{"image": "heal-a-0:latest"},
						map[string]any{"image": "heal-a-1:latest"},
					},
				},
				map[string]any{
					"name": "branch-b",
					"mods": []any{
						map[string]any{"image": "heal-b-0:latest"},
					},
				},
			},
		},
	}
	specBytes, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("failed to marshal spec: %v", err)
	}

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

	if err := maybeCreateHealingJobs(ctx, st, run, runID, domaintypes.StepIndex(1000), jobs); err != nil {
		t.Fatalf("maybeCreateHealingJobs returned error: %v", err)
	}

	// Branch A: 2 heal + 1 re-gate = 3 jobs
	// Branch B: 1 heal + 1 re-gate = 2 jobs
	// Total: 5 jobs.
	if st.createJobCallCount != 5 {
		t.Fatalf("expected 5 CreateJob calls, got %d", st.createJobCallCount)
	}

	jobsByName := make(map[string]store.CreateJobParams)
	for _, p := range st.createJobParams {
		jobsByName[p.Name] = p
	}

	// Verify branch A jobs.
	healA0, ok := jobsByName["heal-branch-a-1-0"]
	if !ok {
		t.Fatalf("expected heal-branch-a-1-0 job")
	}
	if healA0.Status != store.JobStatusPending {
		t.Fatalf("expected heal-branch-a-1-0 to be pending, got %s", healA0.Status)
	}

	healA1, ok := jobsByName["heal-branch-a-1-1"]
	if !ok {
		t.Fatalf("expected heal-branch-a-1-1 job")
	}
	if healA1.Status != store.JobStatusCreated {
		t.Fatalf("expected heal-branch-a-1-1 to be created, got %s", healA1.Status)
	}

	reGateA, ok := jobsByName["re-gate-branch-a-1"]
	if !ok {
		t.Fatalf("expected re-gate-branch-a-1 job")
	}

	// Verify branch B jobs.
	healB0, ok := jobsByName["heal-branch-b-1-0"]
	if !ok {
		t.Fatalf("expected heal-branch-b-1-0 job")
	}
	if healB0.Status != store.JobStatusPending {
		t.Fatalf("expected heal-branch-b-1-0 to be pending, got %s", healB0.Status)
	}

	reGateB, ok := jobsByName["re-gate-branch-b-1"]
	if !ok {
		t.Fatalf("expected re-gate-branch-b-1 job")
	}

	// Verify branch A step_index < branch B step_index (windows don't overlap).
	if reGateA.StepIndex >= healB0.StepIndex {
		t.Fatalf("branch-a re-gate (%f) should be before branch-b heal (%f)",
			reGateA.StepIndex, healB0.StepIndex)
	}

	// Verify step order within branch A.
	if healA0.StepIndex >= healA1.StepIndex {
		t.Fatalf("heal-branch-a-1-0 (%f) should be before heal-branch-a-1-1 (%f)",
			healA0.StepIndex, healA1.StepIndex)
	}
	if healA1.StepIndex >= reGateA.StepIndex {
		t.Fatalf("heal-branch-a-1-1 (%f) should be before re-gate-branch-a-1 (%f)",
			healA1.StepIndex, reGateA.StepIndex)
	}

	// Verify step order within branch B.
	if healB0.StepIndex >= reGateB.StepIndex {
		t.Fatalf("heal-branch-b-1-0 (%f) should be before re-gate-branch-b-1 (%f)",
			healB0.StepIndex, reGateB.StepIndex)
	}
}

// TestCancelLoserBranches_WinnerSelectsAndCancelsLosers verifies that when a re-gate
// succeeds, all other parallel branch jobs (heal, re_gate) in the healing window are canceled.
func TestCancelLoserBranches_WinnerSelectsAndCancelsLosers(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	runUUID := uuid.New()
	runID := pgtype.UUID{Bytes: runUUID, Valid: true}

	// Scenario: Two-branch healing where branch "codex-ai" re-gate succeeds first.
	// Branch "static-patch" jobs should be canceled.
	//
	// Jobs layout:
	//   pre-gate (1000) → FAILED
	//   heal-codex-ai-1-0 (1333) → SUCCEEDED
	//   re-gate-codex-ai-1 (1444) → SUCCEEDED (winner)
	//   heal-static-patch-1-0 (1555) → RUNNING (should be canceled)
	//   re-gate-static-patch-1 (1666) → CREATED (should be canceled)
	//   mod-0 (2000) → CREATED (mainline, should NOT be canceled)
	winnerJobID := uuid.New()
	loserHealID := uuid.New()
	loserReGateID := uuid.New()
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
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     runID,
			Name:      "heal-codex-ai-1-0",
			Status:    store.JobStatusSucceeded,
			ModType:   "heal",
			StepIndex: 1333,
		},
		{
			ID:        pgtype.UUID{Bytes: winnerJobID, Valid: true},
			RunID:     runID,
			Name:      "re-gate-codex-ai-1",
			Status:    store.JobStatusSucceeded,
			ModType:   "re_gate",
			StepIndex: 1444,
		},
		{
			ID:        pgtype.UUID{Bytes: loserHealID, Valid: true},
			RunID:     runID,
			Name:      "heal-static-patch-1-0",
			Status:    store.JobStatusRunning,
			ModType:   "heal",
			StepIndex: 1555,
		},
		{
			ID:        pgtype.UUID{Bytes: loserReGateID, Valid: true},
			RunID:     runID,
			Name:      "re-gate-static-patch-1",
			Status:    store.JobStatusCreated,
			ModType:   "re_gate",
			StepIndex: 1666,
		},
		{
			ID:        pgtype.UUID{Bytes: mainlineModID, Valid: true},
			RunID:     runID,
			Name:      "mod-0",
			Status:    store.JobStatusCreated,
			ModType:   "mod",
			StepIndex: 2000,
		},
	}

	winnerJob := jobs[2] // re-gate-codex-ai-1

	st := &mockStore{
		listJobsByRunResult: jobs,
	}

	if err := cancelLoserBranches(ctx, st, runID, winnerJob, jobs); err != nil {
		t.Fatalf("cancelLoserBranches returned error: %v", err)
	}

	// Verify UpdateJobStatus was called for the loser jobs (heal and re-gate of static-patch).
	if len(st.updateJobStatusCalls) != 2 {
		t.Fatalf("expected 2 UpdateJobStatus calls for loser jobs, got %d", len(st.updateJobStatusCalls))
	}

	// Collect canceled job IDs.
	canceledIDs := make(map[uuid.UUID]bool)
	for _, call := range st.updateJobStatusCalls {
		if call.Status != store.JobStatusCanceled {
			t.Fatalf("expected canceled status, got %s", call.Status)
		}
		canceledIDs[uuid.UUID(call.ID.Bytes)] = true
	}

	// Verify the loser heal job was canceled.
	if !canceledIDs[loserHealID] {
		t.Fatalf("expected heal-static-patch-1-0 to be canceled")
	}

	// Verify the loser re-gate job was canceled.
	if !canceledIDs[loserReGateID] {
		t.Fatalf("expected re-gate-static-patch-1 to be canceled")
	}

	// Verify the mainline mod-0 was NOT canceled.
	if canceledIDs[mainlineModID] {
		t.Fatalf("mainline mod-0 should NOT be canceled")
	}
}

// TestCancelLoserBranches_NoLosersWhenSingleBranch verifies that winner selection
// with a single branch (legacy behavior) doesn't cancel anything extra.
func TestCancelLoserBranches_NoLosersWhenSingleBranch(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	runUUID := uuid.New()
	runID := pgtype.UUID{Bytes: runUUID, Valid: true}

	// Single-branch legacy healing: only one re-gate, so no losers to cancel.
	winnerJobID := uuid.New()
	jobs := []store.Job{
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     runID,
			Name:      "pre-gate",
			Status:    store.JobStatusFailed,
			ModType:   "pre_gate",
			StepIndex: 1000,
		},
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     runID,
			Name:      "heal-1-0",
			Status:    store.JobStatusSucceeded,
			ModType:   "heal",
			StepIndex: 1333,
		},
		{
			ID:        pgtype.UUID{Bytes: winnerJobID, Valid: true},
			RunID:     runID,
			Name:      "re-gate-1",
			Status:    store.JobStatusSucceeded,
			ModType:   "re_gate",
			StepIndex: 1666,
		},
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     runID,
			Name:      "mod-0",
			Status:    store.JobStatusCreated,
			ModType:   "mod",
			StepIndex: 2000,
		},
	}

	winnerJob := jobs[2] // re-gate-1

	st := &mockStore{
		listJobsByRunResult: jobs,
	}

	if err := cancelLoserBranches(ctx, st, runID, winnerJob, jobs); err != nil {
		t.Fatalf("cancelLoserBranches returned error: %v", err)
	}

	// No jobs should be canceled (heal-1-0 is already succeeded, winner is the only re-gate).
	if len(st.updateJobStatusCalls) != 0 {
		t.Fatalf("expected 0 UpdateJobStatus calls (no losers), got %d", len(st.updateJobStatusCalls))
	}
}

// TestCancelLoserBranches_SkipsTerminalJobs verifies that jobs already in terminal
// state (succeeded, failed, canceled, skipped) are not re-canceled.
func TestCancelLoserBranches_SkipsTerminalJobs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	runUUID := uuid.New()
	runID := pgtype.UUID{Bytes: runUUID, Valid: true}

	winnerJobID := uuid.New()
	alreadyFailedID := uuid.New()
	pendingLoserID := uuid.New()

	jobs := []store.Job{
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     runID,
			Name:      "pre-gate",
			Status:    store.JobStatusFailed,
			ModType:   "pre_gate",
			StepIndex: 1000,
		},
		{
			ID:        pgtype.UUID{Bytes: winnerJobID, Valid: true},
			RunID:     runID,
			Name:      "re-gate-branch-a-1",
			Status:    store.JobStatusSucceeded,
			ModType:   "re_gate",
			StepIndex: 1400,
		},
		{
			// This loser re-gate already failed (maybe timeout); should not be re-canceled.
			ID:        pgtype.UUID{Bytes: alreadyFailedID, Valid: true},
			RunID:     runID,
			Name:      "re-gate-branch-b-1",
			Status:    store.JobStatusFailed,
			ModType:   "re_gate",
			StepIndex: 1600,
		},
		{
			// This loser heal is still pending; should be canceled.
			ID:        pgtype.UUID{Bytes: pendingLoserID, Valid: true},
			RunID:     runID,
			Name:      "heal-branch-c-1-0",
			Status:    store.JobStatusPending,
			ModType:   "heal",
			StepIndex: 1700,
		},
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     runID,
			Name:      "mod-0",
			Status:    store.JobStatusCreated,
			ModType:   "mod",
			StepIndex: 2000,
		},
	}

	winnerJob := jobs[1]

	st := &mockStore{
		listJobsByRunResult: jobs,
	}

	if err := cancelLoserBranches(ctx, st, runID, winnerJob, jobs); err != nil {
		t.Fatalf("cancelLoserBranches returned error: %v", err)
	}

	// Only the pending loser (heal-branch-c-1-0) should be canceled.
	// The already-failed re-gate-branch-b-1 should be skipped.
	if len(st.updateJobStatusCalls) != 1 {
		t.Fatalf("expected 1 UpdateJobStatus call, got %d", len(st.updateJobStatusCalls))
	}

	if uuid.UUID(st.updateJobStatusCalls[0].ID.Bytes) != pendingLoserID {
		t.Fatalf("expected heal-branch-c-1-0 to be canceled, got job %v",
			uuid.UUID(st.updateJobStatusCalls[0].ID.Bytes))
	}
}

// TestCancelLoserBranches_AllBranchesFail verifies that when all branches fail
// (no winner), the run eventually fails via maybeCompleteMultiStepRun.
// This test ensures cancelLoserBranches is NOT called on failure (only on success).
func TestCancelLoserBranches_AllBranchesFail_RunFailsCorrectly(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	runUUID := uuid.New()
	runID := pgtype.UUID{Bytes: runUUID, Valid: true}

	// Both branches failed; no winner selection happens.
	// The run should eventually fail via maybeCompleteMultiStepRun.
	jobs := []store.Job{
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     runID,
			Name:      "pre-gate",
			Status:    store.JobStatusFailed,
			ModType:   "pre_gate",
			StepIndex: 1000,
		},
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     runID,
			Name:      "heal-branch-a-1-0",
			Status:    store.JobStatusSucceeded,
			ModType:   "heal",
			StepIndex: 1333,
		},
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     runID,
			Name:      "re-gate-branch-a-1",
			Status:    store.JobStatusFailed, // Branch A re-gate failed
			ModType:   "re_gate",
			StepIndex: 1444,
		},
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     runID,
			Name:      "heal-branch-b-1-0",
			Status:    store.JobStatusSucceeded,
			ModType:   "heal",
			StepIndex: 1555,
		},
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     runID,
			Name:      "re-gate-branch-b-1",
			Status:    store.JobStatusFailed, // Branch B re-gate also failed
			ModType:   "re_gate",
			StepIndex: 1666,
		},
		{
			ID:        pgtype.UUID{Bytes: uuid.New(), Valid: true},
			RunID:     runID,
			Name:      "mod-0",
			Status:    store.JobStatusCanceled, // Canceled after healing exhausted
			ModType:   "mod",
			StepIndex: 2000,
		},
	}

	run := store.Run{
		ID:   runID,
		Spec: []byte(`{}`),
	}

	st := &mockStore{
		listJobsByRunResult: jobs,
	}

	// Call maybeCompleteMultiStepRun to verify run completes as failed.
	if err := maybeCompleteMultiStepRun(ctx, st, nil, run, runID); err != nil {
		t.Fatalf("maybeCompleteMultiStepRun returned error: %v", err)
	}

	// Run should be completed with failed status (last gate failed).
	if !st.updateRunCompletionCalled {
		t.Fatalf("expected UpdateRunCompletion to be called")
	}
	if st.updateRunCompletionParams.Status != store.RunStatusFailed {
		t.Fatalf("expected run status=failed, got %s", st.updateRunCompletionParams.Status)
	}
}

// TestParseHealingStrategies verifies strategy parsing for both legacy and multi-strategy forms.
func TestParseHealingStrategies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		config           map[string]any
		wantStrategies   int
		wantFirstName    string
		wantFirstModsLen int
	}{
		{
			name: "legacy mods form",
			config: map[string]any{
				"mods": []any{
					map[string]any{"image": "heal:latest"},
					map[string]any{"image": "heal2:latest"},
				},
			},
			wantStrategies:   1,
			wantFirstName:    "", // Unnamed strategy for legacy form.
			wantFirstModsLen: 2,
		},
		{
			name: "multi-strategy form",
			config: map[string]any{
				"strategies": []any{
					map[string]any{
						"name": "codex",
						"mods": []any{map[string]any{"image": "codex:latest"}},
					},
					map[string]any{
						"name": "patch",
						"mods": []any{map[string]any{"image": "patch:latest"}},
					},
				},
			},
			wantStrategies:   2,
			wantFirstName:    "codex",
			wantFirstModsLen: 1,
		},
		{
			name: "strategies takes precedence over mods",
			config: map[string]any{
				"mods": []any{map[string]any{"image": "legacy:latest"}},
				"strategies": []any{
					map[string]any{
						"name": "winner",
						"mods": []any{map[string]any{"image": "winner:latest"}},
					},
				},
			},
			wantStrategies:   1,
			wantFirstName:    "winner",
			wantFirstModsLen: 1,
		},
		{
			name:           "empty strategies",
			config:         map[string]any{"strategies": []any{}},
			wantStrategies: 0,
		},
		{
			name:           "empty mods",
			config:         map[string]any{"mods": []any{}},
			wantStrategies: 0,
		},
		{
			name: "strategy without name uses empty string",
			config: map[string]any{
				"strategies": []any{
					map[string]any{
						"mods": []any{map[string]any{"image": "anon:latest"}},
					},
				},
			},
			wantStrategies:   1,
			wantFirstName:    "",
			wantFirstModsLen: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			strategies := parseHealingStrategies(tc.config)
			if len(strategies) != tc.wantStrategies {
				t.Fatalf("expected %d strategies, got %d", tc.wantStrategies, len(strategies))
			}

			if tc.wantStrategies > 0 {
				if strategies[0].Name != tc.wantFirstName {
					t.Fatalf("expected first strategy name=%q, got %q", tc.wantFirstName, strategies[0].Name)
				}
				if len(strategies[0].Mods) != tc.wantFirstModsLen {
					t.Fatalf("expected first strategy mods len=%d, got %d", tc.wantFirstModsLen, len(strategies[0].Mods))
				}
			}
		})
	}
}
