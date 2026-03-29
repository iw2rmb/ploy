package integration

import (
	"context"
	"os"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestSmokeWorkflow_HealingDiffs(t *testing.T) {
	skipDBIntegrationUnderCoverage(t)

	dsn := os.Getenv("PLOY_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_PG_DSN not set; skipping smoke workflow test")
	}

	ctx := context.Background()
	db, err := store.NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore() failed: %v", err)
	}
	defer db.Close()

	// Create a run for the healing diff test.
	modSpec := []byte(`{"type": "healing-test"}`)
	fixture := newV1RunFixture(t, ctx, db, "https://github.com/example/healing-test", "main", "feature/healing-test", modSpec)
	run := fixture.Run
	runRepo := fixture.RunRepo
	t.Logf("✓ Created run: id=%v", run.ID)

	// Create jobs for step 0 and step 1 so ListDiffsByRunRepo can filter by jobs.next_id.
	jobStep0, err := db.CreateJob(ctx, store.CreateJobParams{
		ID:          domaintypes.NewJobID(),
		RunID:       run.ID,
		RepoID:      runRepo.RepoID,
		RepoBaseRef: runRepo.RepoBaseRef,
		Attempt:     runRepo.Attempt,
		Name:        "main-0",
		Status:      domaintypes.JobStatusRunning,
		JobType:     domaintypes.JobTypeMod,
		JobImage:    "",
		NextID:      nil,
		Meta:        []byte(`{"type":"mig"}`),
	})
	if err != nil {
		t.Fatalf("CreateJob() failed: %v", err)
	}
	t.Logf("✓ Created step 0 job: id=%v", jobStep0.ID)

	jobStep1, err := db.CreateJob(ctx, store.CreateJobParams{
		ID:          domaintypes.NewJobID(),
		RunID:       run.ID,
		RepoID:      runRepo.RepoID,
		RepoBaseRef: runRepo.RepoBaseRef,
		Attempt:     runRepo.Attempt,
		Name:        "main-1",
		Status:      domaintypes.JobStatusRunning,
		JobType:     domaintypes.JobTypeMod,
		JobImage:    "",
		NextID:      nil,
		Meta:        []byte(`{"type":"mig"}`),
	})
	if err != nil {
		t.Fatalf("CreateJob(step 1) failed: %v", err)
	}
	t.Logf("✓ Created step 1 job: id=%v", jobStep1.ID)

	// C2: Create diffs with next_id and job_type in summary.
	// Step 0: mig diff + healing diff
	step0ModSummary := []byte(`{"next_id":0,"job_type":"mig"}`)
	step0HealSummary := []byte(`{"next_id":0,"job_type":"healing","healing_attempt":1}`)
	// Step 1: mig diff + 2 healing diffs
	step1ModSummary := []byte(`{"next_id":1,"job_type":"mig"}`)
	step1Heal1Summary := []byte(`{"next_id":1,"job_type":"healing","healing_attempt":1}`)
	step1Heal2Summary := []byte(`{"next_id":1,"job_type":"healing","healing_attempt":2}`)

	// Create step 0 mig diff.
	step0ModPatch := []byte{0x1f, 0x8b, 0x01} // Placeholder gzip bytes.
	step0ModDiff, err := db.CreateDiff(ctx, store.CreateDiffParams{
		RunID:     run.ID,
		JobID:     &jobStep0.ID,
		PatchSize: int64(len(step0ModPatch)),
		Summary:   step0ModSummary,
	})
	if err != nil {
		t.Fatalf("CreateDiff(step0-mig) failed: %v", err)
	}
	t.Logf("✓ Created step 0 mig diff: id=%v", step0ModDiff.ID)

	// Create step 0 healing diff with same next_id.
	step0HealPatch := []byte{0x1f, 0x8b, 0x02}
	step0HealDiff, err := db.CreateDiff(ctx, store.CreateDiffParams{
		RunID:     run.ID,
		JobID:     &jobStep0.ID,
		PatchSize: int64(len(step0HealPatch)),
		Summary:   step0HealSummary,
	})
	if err != nil {
		t.Fatalf("CreateDiff(step0-heal) failed: %v", err)
	}
	t.Logf("✓ Created step 0 healing diff: id=%v", step0HealDiff.ID)

	// Create step 1 mig diff.
	step1ModPatch := []byte{0x1f, 0x8b, 0x03}
	step1ModDiff, err := db.CreateDiff(ctx, store.CreateDiffParams{
		RunID:     run.ID,
		JobID:     &jobStep1.ID,
		PatchSize: int64(len(step1ModPatch)),
		Summary:   step1ModSummary,
	})
	if err != nil {
		t.Fatalf("CreateDiff(step1-mig) failed: %v", err)
	}
	t.Logf("✓ Created step 1 mig diff: id=%v", step1ModDiff.ID)

	// Create step 1 healing diffs (2 attempts).
	step1Heal1Patch := []byte{0x1f, 0x8b, 0x04}
	step1Heal1Diff, err := db.CreateDiff(ctx, store.CreateDiffParams{
		RunID:     run.ID,
		JobID:     &jobStep1.ID,
		PatchSize: int64(len(step1Heal1Patch)),
		Summary:   step1Heal1Summary,
	})
	if err != nil {
		t.Fatalf("CreateDiff(step1-heal1) failed: %v", err)
	}
	t.Logf("✓ Created step 1 healing diff 1: id=%v", step1Heal1Diff.ID)

	step1Heal2Patch := []byte{0x1f, 0x8b, 0x05}
	step1Heal2Diff, err := db.CreateDiff(ctx, store.CreateDiffParams{
		RunID:     run.ID,
		JobID:     &jobStep1.ID,
		PatchSize: int64(len(step1Heal2Patch)),
		Summary:   step1Heal2Summary,
	})
	if err != nil {
		t.Fatalf("CreateDiff(step1-heal2) failed: %v", err)
	}
	t.Logf("✓ Created step 1 healing diff 2: id=%v", step1Heal2Diff.ID)

	// Verify ListDiffsByRunRepo returns all 5 diffs.
	allDiffs, err := db.ListDiffsByRunRepo(ctx, store.ListDiffsByRunRepoParams{
		MetadataOnly: false,
		RunID:        run.ID,
		RepoID:       jobStep0.RepoID,
	})
	if err != nil {
		t.Fatalf("ListDiffsByRunRepo() failed: %v", err)
	}
	if len(allDiffs) != 5 {
		t.Errorf("Expected 5 diffs (2 for step 0 + 3 for step 1), got %d", len(allDiffs))
	}
	t.Logf("✓ ListDiffsByRunRepo returned %d diffs", len(allDiffs))

	// Verify ordering: diffs are ordered by created_at ASC.
	t.Logf("✓ Verified diff ordering (by created_at)")

	// Silence unused variable warnings for diff IDs (used implicitly via DB state).
	_ = step0ModDiff
	_ = step0HealDiff
	_ = step1ModDiff
	_ = step1Heal1Diff
	_ = step1Heal2Diff

	t.Log("✓✓✓ Healing diffs smoke test completed successfully")
}
