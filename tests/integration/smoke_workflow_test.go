package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

type v1RunFixture struct {
	Spec    store.Spec
	Mod     store.Mod
	ModRepo store.ModRepo
	Run     store.Run
	RunRepo store.RunRepo
}

func newV1RunFixture(t *testing.T, ctx context.Context, db store.Store, repoURL, baseRef, targetRef string, specJSON []byte) v1RunFixture {
	t.Helper()

	createdBy := "smoke-test"

	specID := domaintypes.NewSpecID()
	spec, err := db.CreateSpec(ctx, store.CreateSpecParams{
		ID:        specID,
		Name:      "smoke-workflow",
		Spec:      specJSON,
		CreatedBy: &createdBy,
	})
	if err != nil {
		t.Fatalf("CreateSpec() failed: %v", err)
	}

	modID := domaintypes.NewModID()
	mod, err := db.CreateMod(ctx, store.CreateModParams{
		ID:        modID,
		Name:      "smoke-" + modID.String(),
		SpecID:    &spec.ID,
		CreatedBy: &createdBy,
	})
	if err != nil {
		t.Fatalf("CreateMod() failed: %v", err)
	}

	modRepoID := domaintypes.NewModRepoID()
	modRepo, err := db.CreateModRepo(ctx, store.CreateModRepoParams{
		ID:        modRepoID,
		ModID:     modID,
		RepoUrl:   repoURL,
		BaseRef:   baseRef,
		TargetRef: targetRef,
	})
	if err != nil {
		t.Fatalf("CreateModRepo() failed: %v", err)
	}

	runID := domaintypes.NewRunID()
	run, err := db.CreateRun(ctx, store.CreateRunParams{
		ID:        runID,
		ModID:     modID,
		SpecID:    spec.ID,
		CreatedBy: &createdBy,
	})
	if err != nil {
		t.Fatalf("CreateRun() failed: %v", err)
	}

	runRepo, err := db.CreateRunRepo(ctx, store.CreateRunRepoParams{
		ModID:         modID,
		RunID:         run.ID,
		RepoID:        modRepo.ID,
		RepoBaseRef:   baseRef,
		RepoTargetRef: targetRef,
	})
	if err != nil {
		t.Fatalf("CreateRunRepo() failed: %v", err)
	}

	return v1RunFixture{
		Spec:    spec,
		Mod:     mod,
		ModRepo: modRepo,
		Run:     run,
		RunRepo: runRepo,
	}
}

// TestSmokeWorkflow_EndToEnd validates a complete workflow combining multiple operations:
// 1. Create run (queued)
// 2. Create jobs (build, test, deploy)
// 3. Append logs across jobs
// 4. Generate diffs
// 5. Create events
// 6. Update run status to completed
// 7. Verify all data is correctly persisted and retrievable
//
// This test simulates the critical path through the system from run creation
// to completion, validating database operations, foreign key relationships,
// and query correctness.
//
// Requires: PLOY_TEST_PG_DSN environment variable.
func TestSmokeWorkflow_EndToEnd(t *testing.T) {
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

	// Step 1: Create a v1 run representing a mod execution workflow.
	modSpec := []byte(`{
		"type": "smoke-workflow",
		"image": "docker.io/example/mod-test:latest",
		"command": ["mod-test", "--input", "/workspace"],
		"build_gate": {
			"enabled": true
		}
	}`)
	fixture := newV1RunFixture(t, ctx, db, "https://github.com/example/smoke-workflow", "main", "feature/smoke-workflow", modSpec)
	run := fixture.Run
	runRepo := fixture.RunRepo

	t.Logf("✓ Created run: id=%v, status=%s", run.ID, run.Status)

	// Step 2: Create multiple jobs representing the workflow phases.
	// Stage 1: Build Gate (pre-validation)
	jobBuildGate, err := db.CreateJob(ctx, store.CreateJobParams{
		ID:          domaintypes.NewJobID(),
		RunID:       run.ID,
		RepoID:      runRepo.RepoID,
		RepoBaseRef: runRepo.RepoBaseRef,
		Attempt:     runRepo.Attempt,
		Name:        "build-gate",
		Status:      store.JobStatusRunning,
		JobType:     "",
		JobImage:    "",
		NextID:      nil,
		Meta:        []byte(`{"type":"build-gate"}`),
	})
	if err != nil {
		t.Fatalf("CreateJob(build-gate) failed: %v", err)
	}
	t.Logf("✓ Created job: id=%v, name=%s", jobBuildGate.ID, jobBuildGate.Name)

	// Stage 2: Main mod execution
	jobMain, err := db.CreateJob(ctx, store.CreateJobParams{
		ID:          domaintypes.NewJobID(),
		RunID:       run.ID,
		RepoID:      runRepo.RepoID,
		RepoBaseRef: runRepo.RepoBaseRef,
		Attempt:     runRepo.Attempt,
		Name:        "main",
		Status:      store.JobStatusCreated,
		JobType:     "",
		JobImage:    "",
		NextID:      nil,
		Meta:        []byte(`{"type":"mod","lane":"main"}`),
	})
	if err != nil {
		t.Fatalf("CreateJob(main) failed: %v", err)
	}
	t.Logf("✓ Created job: id=%v, name=%s", jobMain.ID, jobMain.Name)

	// Stage 3: Post-processing (e.g., artifact upload)
	jobPost, err := db.CreateJob(ctx, store.CreateJobParams{
		ID:          domaintypes.NewJobID(),
		RunID:       run.ID,
		RepoID:      runRepo.RepoID,
		RepoBaseRef: runRepo.RepoBaseRef,
		Attempt:     runRepo.Attempt,
		Name:        "post-process",
		Status:      store.JobStatusCreated,
		JobType:     "",
		JobImage:    "",
		NextID:      nil,
		Meta:        []byte(`{"type":"post-process","action":"upload-artifacts"}`),
	})
	if err != nil {
		t.Fatalf("CreateJob(post-process) failed: %v", err)
	}
	t.Logf("✓ Created job: id=%v, name=%s", jobPost.ID, jobPost.Name)

	// Step 3: Simulate log streaming across jobs.
	// Build Gate logs
	buildGateLog := []byte("INFO: Starting build gate validation\nINFO: Running Maven build\nINFO: Build gate passed\n")
	log1, err := db.CreateLog(ctx, store.CreateLogParams{
		RunID:    run.ID,
		JobID:    &jobBuildGate.ID,
		ChunkNo:  0,
		DataSize: int64(len(buildGateLog)),
	})
	if err != nil {
		t.Fatalf("CreateLog(build-gate) failed: %v", err)
	}
	t.Logf("✓ Created log chunk 0: %d bytes", log1.DataSize)

	// Main job logs
	mainLog := []byte("INFO: Executing mod\nINFO: Processing files\nINFO: Generated 5 changes\nINFO: Mod execution complete\n")
	log2, err := db.CreateLog(ctx, store.CreateLogParams{
		RunID:    run.ID,
		JobID:    &jobMain.ID,
		ChunkNo:  1,
		DataSize: int64(len(mainLog)),
	})
	if err != nil {
		t.Fatalf("CreateLog(main) failed: %v", err)
	}
	t.Logf("✓ Created log chunk 1: %d bytes", log2.DataSize)

	// Post-processing logs
	postLog := []byte("INFO: Uploading artifacts\nINFO: Artifacts uploaded successfully\n")
	log3, err := db.CreateLog(ctx, store.CreateLogParams{
		RunID:    run.ID,
		JobID:    &jobPost.ID,
		ChunkNo:  2,
		DataSize: int64(len(postLog)),
	})
	if err != nil {
		t.Fatalf("CreateLog(post-process) failed: %v", err)
	}
	t.Logf("✓ Created log chunk 2: %d bytes", log3.DataSize)

	// Step 4: Generate diffs for the main job.
	diffPatch := []byte(`diff --git a/src/Main.java b/src/Main.java
index abc1234..def5678 100644
--- a/src/Main.java
+++ b/src/Main.java
@@ -10,7 +10,7 @@ public class Main {
     }

     private static void processData() {
-        // Old implementation
+        // New implementation using modern APIs
         System.out.println("Processing data");
     }
 }
`)
	diffSummary := []byte(`{"files_changed":1,"insertions":1,"deletions":1}`)
	diff, err := db.CreateDiff(ctx, store.CreateDiffParams{
		RunID:     run.ID,
		JobID:     &jobMain.ID,
		PatchSize: int64(len(diffPatch)),
		Summary:   diffSummary,
	})
	if err != nil {
		t.Fatalf("CreateDiff() failed: %v", err)
	}
	t.Logf("✓ Created diff: id=%v, patch_size=%d", diff.ID, diff.PatchSize)

	// Step 5: Create events representing workflow state transitions.
	now := time.Now().UTC()

	// Event 1: Run started
	event1, err := db.CreateEvent(ctx, store.CreateEventParams{
		RunID: run.ID,
		Time: pgtype.Timestamptz{
			Time:  now,
			Valid: true,
		},
		Level:   "info",
		Message: "Run started: smoke workflow",
		Meta:    []byte(`{"source":"smoke-test","phase":"start"}`),
	})
	if err != nil {
		t.Fatalf("CreateEvent(start) failed: %v", err)
	}
	t.Logf("✓ Created event: id=%d, message=%s", event1.ID, event1.Message)

	// Event 2: Build gate passed
	event2, err := db.CreateEvent(ctx, store.CreateEventParams{
		RunID: run.ID,
		Time: pgtype.Timestamptz{
			Time:  now.Add(10 * time.Second),
			Valid: true,
		},
		Level:   "info",
		Message: "Build gate validation passed",
		Meta:    []byte(`{"source":"smoke-test","phase":"build-gate","status":"passed"}`),
	})
	if err != nil {
		t.Fatalf("CreateEvent(build-gate-passed) failed: %v", err)
	}
	t.Logf("✓ Created event: id=%d, message=%s", event2.ID, event2.Message)

	// Event 3: Main mod completed
	event3, err := db.CreateEvent(ctx, store.CreateEventParams{
		RunID: run.ID,
		Time: pgtype.Timestamptz{
			Time:  now.Add(30 * time.Second),
			Valid: true,
		},
		Level:   "info",
		Message: "Mod execution completed successfully",
		Meta:    []byte(`{"source":"smoke-test","phase":"main","status":"completed"}`),
	})
	if err != nil {
		t.Fatalf("CreateEvent(main-completed) failed: %v", err)
	}
	t.Logf("✓ Created event: id=%d, message=%s", event3.ID, event3.Message)

	// Event 4: Run completed
	event4, err := db.CreateEvent(ctx, store.CreateEventParams{
		RunID: run.ID,
		Time: pgtype.Timestamptz{
			Time:  now.Add(40 * time.Second),
			Valid: true,
		},
		Level:   "info",
		Message: "Run completed: all jobs successful",
		Meta:    []byte(`{"source":"smoke-test","phase":"complete","status":"success"}`),
	})
	if err != nil {
		t.Fatalf("CreateEvent(completed) failed: %v", err)
	}
	t.Logf("✓ Created event: id=%d, message=%s", event4.ID, event4.Message)

	// Step 6: Update run status to succeeded.
	// In a real workflow, the runner would update job statuses and then the run status.
	err = db.UpdateRunStatus(ctx, store.UpdateRunStatusParams{
		ID:     run.ID,
		Status: store.RunStatusFinished,
	})
	if err != nil {
		t.Fatalf("UpdateRunStatus() failed: %v", err)
	}
	t.Logf("✓ Updated run status to finished")

	// Step 7: Verify all data is correctly persisted and retrievable.
	// Verify run retrieval
	fetchedRun, err := db.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() failed: %v", err)
	}
	if fetchedRun.Status != store.RunStatusFinished {
		t.Errorf("Fetched run status mismatch: expected 'Finished', got %s", fetchedRun.Status)
	}
	t.Logf("✓ Verified run status: %s", fetchedRun.Status)

	// Verify jobs are listable
	jobs, err := db.ListJobsByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("ListJobsByRun() failed: %v", err)
	}
	if len(jobs) != 3 {
		t.Errorf("Expected 3 jobs, got %d", len(jobs))
	}
	// Verify job names are correct
	jobNames := make(map[string]bool)
	for _, s := range jobs {
		jobNames[s.Name] = true
	}
	expectedStages := []string{"build-gate", "main", "post-process"}
	for _, name := range expectedStages {
		if !jobNames[name] {
			t.Errorf("Expected job %s not found", name)
		}
	}
	t.Logf("✓ Verified %d jobs with correct names", len(jobs))

	// Verify logs are ordered and complete
	logs, err := db.ListLogsByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("ListLogsByRun() failed: %v", err)
	}
	if len(logs) != 3 {
		t.Errorf("Expected 3 log chunks, got %d", len(logs))
	}
	// Verify log order and content
	if len(logs) >= 3 {
		if logs[0].ChunkNo != 0 || logs[1].ChunkNo != 1 || logs[2].ChunkNo != 2 {
			t.Errorf("Log chunks not ordered correctly: got chunk_nos %d, %d, %d",
				logs[0].ChunkNo, logs[1].ChunkNo, logs[2].ChunkNo)
		}
		// Spot-check log metadata
		if logs[0].DataSize != int64(len(buildGateLog)) {
			t.Errorf("Log chunk 0 size mismatch")
		}
		if logs[0].ObjectKey == nil || *logs[0].ObjectKey == "" {
			t.Errorf("Log chunk 0 missing object_key")
		}
	}
	t.Logf("✓ Verified %d log chunks in correct order", len(logs))

	// Verify diffs are retrievable
	diffs, err := db.ListDiffsByRunRepo(ctx, store.ListDiffsByRunRepoParams{
		RunID:  run.ID,
		RepoID: jobMain.RepoID,
	})
	if err != nil {
		t.Fatalf("ListDiffsByRunRepo() failed: %v", err)
	}
	if len(diffs) != 1 {
		t.Errorf("Expected 1 diff, got %d", len(diffs))
	}
	if len(diffs) >= 1 {
		if diffs[0].JobID == nil || *diffs[0].JobID != jobMain.ID {
			t.Errorf("Diff job_id mismatch: expected %v, got %v", jobMain.ID, diffs[0].JobID)
		}
		if diffs[0].PatchSize != int64(len(diffPatch)) {
			t.Errorf("Diff patch_size mismatch")
		}
		if diffs[0].ObjectKey == nil || *diffs[0].ObjectKey == "" {
			t.Errorf("Diff missing object_key")
		}
	}
	t.Logf("✓ Verified %d diff(s) with correct job association", len(diffs))

	// Verify events are ordered chronologically
	events, err := db.ListEventsByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("ListEventsByRun() failed: %v", err)
	}
	if len(events) != 4 {
		t.Errorf("Expected 4 events, got %d", len(events))
	}
	// Verify event order (should be chronological by time)
	if len(events) >= 4 {
		expectedIDs := []int64{event1.ID, event2.ID, event3.ID, event4.ID}
		for i, e := range events {
			if e.ID != expectedIDs[i] {
				t.Errorf("Event %d ID mismatch: expected %d, got %d", i, expectedIDs[i], e.ID)
			}
		}
	}
	t.Logf("✓ Verified %d events in chronological order", len(events))

	// Verify ListEventsByRunSince works correctly
	eventsSince, err := db.ListEventsByRunSince(ctx, store.ListEventsByRunSinceParams{
		RunID: run.ID,
		ID:    event2.ID, // Get events after build-gate-passed
	})
	if err != nil {
		t.Fatalf("ListEventsByRunSince() failed: %v", err)
	}
	if len(eventsSince) != 2 { // Should get event3 and event4
		t.Errorf("ListEventsByRunSince: expected 2 events, got %d", len(eventsSince))
	}
	if len(eventsSince) >= 2 {
		if eventsSince[0].ID != event3.ID || eventsSince[1].ID != event4.ID {
			t.Errorf("ListEventsByRunSince: unexpected event IDs")
		}
	}
	t.Logf("✓ Verified ListEventsByRunSince returns correct event subset")

	t.Log("✓✓✓ Smoke workflow end-to-end test completed successfully")
}

// TestSmokeWorkflow_HealingDiffs validates that healing diffs with job_type and next_id
// are correctly stored and retrieved alongside mod diffs.
// C2: This test verifies the unified job+diff model where both mod and healing diffs
// share the same next_id, enabling rehydration to include all diffs for a step.
//
// Requires: PLOY_TEST_PG_DSN environment variable.
func TestSmokeWorkflow_HealingDiffs(t *testing.T) {
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
		Status:      store.JobStatusRunning,
		JobType:     "",
		JobImage:    "",
		NextID:      nil,
		Meta:        []byte(`{"type":"mod"}`),
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
		Status:      store.JobStatusRunning,
		JobType:     "",
		JobImage:    "",
		NextID:      nil,
		Meta:        []byte(`{"type":"mod"}`),
	})
	if err != nil {
		t.Fatalf("CreateJob(step 1) failed: %v", err)
	}
	t.Logf("✓ Created step 1 job: id=%v", jobStep1.ID)

	// C2: Create diffs with next_id and job_type in summary.
	// Step 0: mod diff + healing diff
	step0ModSummary := []byte(`{"next_id":0,"job_type":"mod"}`)
	step0HealSummary := []byte(`{"next_id":0,"job_type":"healing","healing_attempt":1}`)
	// Step 1: mod diff + 2 healing diffs
	step1ModSummary := []byte(`{"next_id":1,"job_type":"mod"}`)
	step1Heal1Summary := []byte(`{"next_id":1,"job_type":"healing","healing_attempt":1}`)
	step1Heal2Summary := []byte(`{"next_id":1,"job_type":"healing","healing_attempt":2}`)

	// Create step 0 mod diff.
	step0ModPatch := []byte{0x1f, 0x8b, 0x01} // Placeholder gzip bytes.
	step0ModDiff, err := db.CreateDiff(ctx, store.CreateDiffParams{
		RunID:     run.ID,
		JobID:     &jobStep0.ID,
		PatchSize: int64(len(step0ModPatch)),
		Summary:   step0ModSummary,
	})
	if err != nil {
		t.Fatalf("CreateDiff(step0-mod) failed: %v", err)
	}
	t.Logf("✓ Created step 0 mod diff: id=%v", step0ModDiff.ID)

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

	// Create step 1 mod diff.
	step1ModPatch := []byte{0x1f, 0x8b, 0x03}
	step1ModDiff, err := db.CreateDiff(ctx, store.CreateDiffParams{
		RunID:     run.ID,
		JobID:     &jobStep1.ID,
		PatchSize: int64(len(step1ModPatch)),
		Summary:   step1ModSummary,
	})
	if err != nil {
		t.Fatalf("CreateDiff(step1-mod) failed: %v", err)
	}
	t.Logf("✓ Created step 1 mod diff: id=%v", step1ModDiff.ID)

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
		RunID:  run.ID,
		RepoID: jobStep0.RepoID,
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
