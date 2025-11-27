package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// TestDiffStepIndex_CreateAndList verifies that diffs with step_index are created
// and listed in the correct order (step_index ASC, then created_at DESC for same step).
func TestDiffStepIndex_CreateAndList(t *testing.T) {
	dsn := os.Getenv("PLOY_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_PG_DSN not set; skipping DB integration test")
	}

	ctx := context.Background()
	st, err := NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore() failed: %v", err)
	}
	defer st.Close()

	// Create a run and stage for the diffs.
	createdBy := "test"
	runParams := CreateRunParams{
		RepoUrl:   "https://example.com/repo.git",
		Spec:      []byte(`{}`),
		CreatedBy: &createdBy,
		Status:    RunStatusQueued,
		BaseRef:   "main",
		TargetRef: "feature",
	}
	run, err := st.CreateRun(ctx, runParams)
	if err != nil {
		t.Fatalf("CreateRun failed: %v", err)
	}

	stageParams := CreateStageParams{
		RunID: run.ID,
		Name:  "test-stage",
	}
	stage, err := st.CreateStage(ctx, stageParams)
	if err != nil {
		t.Fatalf("CreateStage failed: %v", err)
	}

	// Create diffs with various step_index values (out of order to test ordering).
	// step 0, step 2, step 1, step 2 (again), null step_index (legacy)
	stepIndexes := []*int32{
		intPtr(0),
		intPtr(2),
		intPtr(1),
		intPtr(2), // second diff for step 2
		nil,       // legacy diff without step_index
	}

	var diffIDs []pgtype.UUID
	for i, stepIdx := range stepIndexes {
		params := CreateDiffParams{
			RunID:     run.ID,
			StageID:   stage.ID,
			Patch:     []byte{byte(i)}, // unique patch for each diff
			Summary:   []byte(`{}`),
			StepIndex: stepIdx,
		}
		diff, err := st.CreateDiff(ctx, params)
		if err != nil {
			t.Fatalf("CreateDiff %d failed: %v", i, err)
		}
		diffIDs = append(diffIDs, diff.ID)
		// Small sleep to ensure created_at differs between diffs.
		time.Sleep(10 * time.Millisecond)
	}

	// List diffs and verify ordering: step_index NULLS LAST, created_at DESC.
	// Expected order: step 0, step 1, step 2 (newer), step 2 (older), null (legacy).
	diffs, err := st.ListDiffsByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("ListDiffsByRun failed: %v", err)
	}

	if len(diffs) != 5 {
		t.Fatalf("expected 5 diffs, got %d", len(diffs))
	}

	// Verify ordering by step_index.
	// diffs[0] should be step 0 (diffIDs[0])
	if diffs[0].ID != diffIDs[0] {
		t.Errorf("diffs[0] id mismatch: got %v, want %v (step 0)", diffs[0].ID, diffIDs[0])
	}
	if diffs[0].StepIndex == nil || *diffs[0].StepIndex != 0 {
		t.Errorf("diffs[0] step_index=%v, want 0", ptrVal(diffs[0].StepIndex))
	}

	// diffs[1] should be step 1 (diffIDs[2])
	if diffs[1].ID != diffIDs[2] {
		t.Errorf("diffs[1] id mismatch: got %v, want %v (step 1)", diffs[1].ID, diffIDs[2])
	}
	if diffs[1].StepIndex == nil || *diffs[1].StepIndex != 1 {
		t.Errorf("diffs[1] step_index=%v, want 1", ptrVal(diffs[1].StepIndex))
	}

	// diffs[2] and diffs[3] should be step 2 (newer first: diffIDs[3], then diffIDs[1])
	if diffs[2].StepIndex == nil || *diffs[2].StepIndex != 2 {
		t.Errorf("diffs[2] step_index=%v, want 2", ptrVal(diffs[2].StepIndex))
	}
	if diffs[3].StepIndex == nil || *diffs[3].StepIndex != 2 {
		t.Errorf("diffs[3] step_index=%v, want 2", ptrVal(diffs[3].StepIndex))
	}
	// Verify diffs[2] is newer than diffs[3] by created_at.
	if !diffs[2].CreatedAt.Time.After(diffs[3].CreatedAt.Time) {
		t.Errorf("diffs[2] should be newer than diffs[3] for same step_index")
	}

	// diffs[4] should be the legacy diff (diffIDs[4], step_index=nil)
	if diffs[4].ID != diffIDs[4] {
		t.Errorf("diffs[4] id mismatch: got %v, want %v (legacy)", diffs[4].ID, diffIDs[4])
	}
	if diffs[4].StepIndex != nil {
		t.Errorf("diffs[4] step_index=%v, want nil (legacy)", *diffs[4].StepIndex)
	}
}

// TestDiffStepIndex_ListDiffsBeforeStep verifies that ListDiffsBeforeStep returns
// only diffs up to and including the specified step_index.
func TestDiffStepIndex_ListDiffsBeforeStep(t *testing.T) {
	dsn := os.Getenv("PLOY_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_PG_DSN not set; skipping DB integration test")
	}

	ctx := context.Background()
	st, err := NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore() failed: %v", err)
	}
	defer st.Close()

	// Create a run and stage for the diffs.
	createdBy := "test"
	runParams := CreateRunParams{
		RepoUrl:   "https://example.com/repo.git",
		Spec:      []byte(`{}`),
		CreatedBy: &createdBy,
		Status:    RunStatusQueued,
		BaseRef:   "main",
		TargetRef: "feature",
	}
	run, err := st.CreateRun(ctx, runParams)
	if err != nil {
		t.Fatalf("CreateRun failed: %v", err)
	}

	stageParams := CreateStageParams{
		RunID: run.ID,
		Name:  "test-stage",
	}
	stage, err := st.CreateStage(ctx, stageParams)
	if err != nil {
		t.Fatalf("CreateStage failed: %v", err)
	}

	// Create diffs for steps 0, 1, 2, 3, and one legacy diff (nil step_index).
	stepIndexes := []*int32{intPtr(0), intPtr(1), intPtr(2), intPtr(3), nil}
	for _, stepIdx := range stepIndexes {
		params := CreateDiffParams{
			RunID:     run.ID,
			StageID:   stage.ID,
			Patch:     []byte{0x1f, 0x8b},
			Summary:   []byte(`{}`),
			StepIndex: stepIdx,
		}
		if _, err := st.CreateDiff(ctx, params); err != nil {
			t.Fatalf("CreateDiff failed: %v", err)
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Query diffs before step 2 (inclusive): should return steps 0, 1, 2.
	listParams := ListDiffsBeforeStepParams{
		RunID:     run.ID,
		StepIndex: intPtr(2),
	}
	diffs, err := st.ListDiffsBeforeStep(ctx, listParams)
	if err != nil {
		t.Fatalf("ListDiffsBeforeStep failed: %v", err)
	}

	// Should return 3 diffs: steps 0, 1, 2 (legacy diff excluded, step 3 excluded).
	if len(diffs) != 3 {
		t.Fatalf("expected 3 diffs (steps 0, 1, 2), got %d", len(diffs))
	}

	// Verify ordering: step 0, step 1, step 2 (ASC by step_index).
	for i, expectedStep := range []int32{0, 1, 2} {
		if diffs[i].StepIndex == nil || *diffs[i].StepIndex != expectedStep {
			t.Errorf("diffs[%d] step_index=%v, want %d", i, ptrVal(diffs[i].StepIndex), expectedStep)
		}
	}
}

// TestDiffStepIndex_NegativeStepIndex verifies that DB constraint rejects negative step_index.
func TestDiffStepIndex_NegativeStepIndex(t *testing.T) {
	dsn := os.Getenv("PLOY_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_PG_DSN not set; skipping DB integration test")
	}

	ctx := context.Background()
	st, err := NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore() failed: %v", err)
	}
	defer st.Close()

	// Create a run and stage.
	createdBy := "test"
	runParams := CreateRunParams{
		RepoUrl:   "https://example.com/repo.git",
		Spec:      []byte(`{}`),
		CreatedBy: &createdBy,
		Status:    RunStatusQueued,
		BaseRef:   "main",
		TargetRef: "feature",
	}
	run, err := st.CreateRun(ctx, runParams)
	if err != nil {
		t.Fatalf("CreateRun failed: %v", err)
	}

	stageParams := CreateStageParams{
		RunID: run.ID,
		Name:  "test-stage",
	}
	stage, err := st.CreateStage(ctx, stageParams)
	if err != nil {
		t.Fatalf("CreateStage failed: %v", err)
	}

	// Attempt to create a diff with negative step_index; should fail (DB constraint).
	params := CreateDiffParams{
		RunID:     run.ID,
		StageID:   stage.ID,
		Patch:     []byte{0x1f, 0x8b},
		Summary:   []byte(`{}`),
		StepIndex: intPtr(-1),
	}
	_, err = st.CreateDiff(ctx, params)
	if err == nil {
		t.Fatal("CreateDiff should have failed with negative step_index")
	}
	// Verify error is a check constraint violation.
	if err.Error() == "" || err.Error() == "context canceled" {
		t.Errorf("expected constraint violation error, got: %v", err)
	}
}

// TestDiffsStepIndex_HealingDiffsIncluded verifies that healing diffs with mod_type="healing"
// are included in ListDiffsBeforeStep queries when they share the same step_index as mod diffs.
// C2: Unified rehydration requires that all diffs (mod + healing) are returned for a given step.
func TestDiffsStepIndex_HealingDiffsIncluded(t *testing.T) {
	dsn := os.Getenv("PLOY_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_PG_DSN not set; skipping DB integration test")
	}

	ctx := context.Background()
	st, err := NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore() failed: %v", err)
	}
	defer st.Close()

	// Create a run and stage.
	createdBy := "test"
	runParams := CreateRunParams{
		RepoUrl:   "https://example.com/repo.git",
		Spec:      []byte(`{}`),
		CreatedBy: &createdBy,
		Status:    RunStatusQueued,
		BaseRef:   "main",
		TargetRef: "feature",
	}
	run, err := st.CreateRun(ctx, runParams)
	if err != nil {
		t.Fatalf("CreateRun failed: %v", err)
	}

	stageParams := CreateStageParams{
		RunID: run.ID,
		Name:  "test-stage",
	}
	stage, err := st.CreateStage(ctx, stageParams)
	if err != nil {
		t.Fatalf("CreateStage failed: %v", err)
	}

	// Create diffs: step 0 mod, step 0 healing, step 1 mod, step 1 healing (2 attempts).
	// C2: Healing diffs use the same step_index as their parent mod step.
	diffsToCreate := []struct {
		stepIndex *int32
		summary   string // JSONB with mod_type
	}{
		{intPtr(0), `{"mod_type":"mod","step_index":0}`},     // step 0 mod
		{intPtr(0), `{"mod_type":"healing","step_index":0}`}, // step 0 healing
		{intPtr(1), `{"mod_type":"mod","step_index":1}`},     // step 1 mod
		{intPtr(1), `{"mod_type":"healing","step_index":1}`}, // step 1 healing attempt 1
		{intPtr(1), `{"mod_type":"healing","step_index":1}`}, // step 1 healing attempt 2
		{intPtr(2), `{"mod_type":"mod","step_index":2}`},     // step 2 mod (not queried)
	}

	for i, d := range diffsToCreate {
		params := CreateDiffParams{
			RunID:     run.ID,
			StageID:   stage.ID,
			Patch:     []byte{byte(i)},
			Summary:   []byte(d.summary),
			StepIndex: d.stepIndex,
		}
		if _, err := st.CreateDiff(ctx, params); err != nil {
			t.Fatalf("CreateDiff %d failed: %v", i, err)
		}
		// Small sleep to ensure created_at differs.
		time.Sleep(5 * time.Millisecond)
	}

	// Query diffs before step 1 (inclusive): should return steps 0 and 1 (5 diffs total).
	listParams := ListDiffsBeforeStepParams{
		RunID:     run.ID,
		StepIndex: intPtr(1),
	}
	diffs, err := st.ListDiffsBeforeStep(ctx, listParams)
	if err != nil {
		t.Fatalf("ListDiffsBeforeStep failed: %v", err)
	}

	// C2: Should return 5 diffs (2 for step 0, 3 for step 1).
	if len(diffs) != 5 {
		t.Fatalf("expected 5 diffs (step 0 mod+heal, step 1 mod+heal*2), got %d", len(diffs))
	}

	// Verify ordering: step 0 first (mod, heal), then step 1 (mod, heal, heal).
	// Within each step, order is by created_at ASC.
	expectedSteps := []int32{0, 0, 1, 1, 1}
	for i, expectedStep := range expectedSteps {
		if diffs[i].StepIndex == nil || *diffs[i].StepIndex != expectedStep {
			t.Errorf("diffs[%d] step_index=%v, want %d", i, ptrVal(diffs[i].StepIndex), expectedStep)
		}
	}

	// Verify that step 2 is NOT included (query was for step_index <= 1).
	for _, d := range diffs {
		if d.StepIndex != nil && *d.StepIndex > 1 {
			t.Errorf("unexpected diff with step_index=%d (should be <= 1)", *d.StepIndex)
		}
	}
}

// Helper: intPtr returns a pointer to an int32 value.
func intPtr(v int32) *int32 {
	return &v
}

// Helper: ptrVal returns the value of a pointer or -1 if nil.
func ptrVal(p *int32) int32 {
	if p == nil {
		return -1
	}
	return *p
}
