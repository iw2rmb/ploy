package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// TestLabSmoke is a minimal smoke test that simulates server + node workflow:
// 1. Create run (queued status).
// 2. Simulate node operations: append logs and diffs as if a node is executing the run.
// 3. Assert that logs and diff rows are stored in the database.
//
// This test requires a test database accessible via PLOY_TEST_DB_DSN.
func TestLabSmoke(t *testing.T) {
	skipDBIntegrationUnderCoverage(t)

	dsn := os.Getenv("PLOY_TEST_DB_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_DB_DSN not set; skipping lab smoke test")
	}

	ctx := context.Background()
	db, err := store.NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore() failed: %v", err)
	}
	defer db.Close()

	// Step 1: Create v1 entities: spec → mig → mig_repo → run → run_repo.
	createdBy := "smoke-test"
	specJSON := []byte(`{"type":"smoke-test","description":"Lab smoke test"}`)
	specID := domaintypes.NewSpecID()
	spec, err := db.CreateSpec(ctx, store.CreateSpecParams{
		ID:        specID,
		Name:      "smoke-test",
		Spec:      specJSON,
		CreatedBy: &createdBy,
	})
	if err != nil {
		t.Fatalf("CreateSpec() failed: %v", err)
	}

	migID := domaintypes.NewMigID()
	_, err = db.CreateMig(ctx, store.CreateMigParams{
		ID:        migID,
		Name:      "smoke-test-" + migID.String(),
		SpecID:    &spec.ID,
		CreatedBy: &createdBy,
	})
	if err != nil {
		t.Fatalf("CreateMig() failed: %v", err)
	}

	repoURL := "https://github.com/octocat/Hello-World"
	baseRef := "main"
	targetRef := "feature/smoke-test"

	migRepoID := domaintypes.NewMigRepoID()
	migRepo, err := db.CreateMigRepo(ctx, store.CreateMigRepoParams{
		ID:        migRepoID,
		MigID:     migID,
		Url:       repoURL,
		BaseRef:   baseRef,
		TargetRef: targetRef,
	})
	if err != nil {
		t.Fatalf("CreateMigRepo() failed: %v", err)
	}

	runID := domaintypes.NewRunID()
	run, err := db.CreateRun(ctx, store.CreateRunParams{
		ID:        runID,
		MigID:     migID,
		SpecID:    spec.ID,
		CreatedBy: &createdBy,
	})
	if err != nil {
		t.Fatalf("CreateRun() failed: %v", err)
	}
	t.Logf("Created run: id=%v, mig_id=%s, spec_id=%s, status=%s", run.ID, run.MigID, run.SpecID, run.Status)

	runRepo, err := db.CreateRunRepo(ctx, store.CreateRunRepoParams{
		MigID:         migID,
		RunID:         run.ID,
		RepoID:        migRepo.RepoID,
		RepoBaseRef:   migRepo.BaseRef,
		RepoTargetRef: migRepo.TargetRef,
	})
	if err != nil {
		t.Fatalf("CreateRunRepo() failed: %v", err)
	}

	// Step 4: Simulate node operations - Create a job for the run.
	job, err := db.CreateJob(ctx, store.CreateJobParams{
		ID:          domaintypes.NewJobID(),
		RunID:       run.ID,
		RepoID:      runRepo.RepoID,
		RepoBaseRef: runRepo.RepoBaseRef,
		Attempt:     runRepo.Attempt,
		Name:        "build",
		Status:      domaintypes.JobStatusRunning,
		JobType:     domaintypes.JobTypeMig,
		JobImage:    "",
		NextID:      nil,
		Meta:        []byte(`{"type":"build","tool":"make"}`),
	})
	if err != nil {
		t.Fatalf("CreateJob() failed: %v", err)
	}
	t.Logf("Created job: id=%v, run_id=%v, name=%s", job.ID, job.RunID, job.Name)

	// Step 5: Simulate node appends - Create logs (simulating log streaming from node).
	logData := []byte("INFO: Starting smoke test run\nINFO: Cloning repository\nINFO: Running build job\n")
	log, err := db.CreateLog(ctx, store.CreateLogParams{
		RunID:    run.ID,
		JobID:    &job.ID,
		ChunkNo:  0,
		DataSize: int64(len(logData)),
	})
	if err != nil {
		t.Fatalf("CreateLog() failed: %v", err)
	}
	t.Logf("Created log: id=%d, run_id=%v, chunk_no=%d, data_size=%d", log.ID, log.RunID, log.ChunkNo, log.DataSize)

	// Create a second log chunk to simulate continued streaming.
	log2Data := []byte("INFO: Build completed successfully\nINFO: Running tests\nINFO: All tests passed\n")
	log2, err := db.CreateLog(ctx, store.CreateLogParams{
		RunID:    run.ID,
		JobID:    &job.ID,
		ChunkNo:  1,
		DataSize: int64(len(log2Data)),
	})
	if err != nil {
		t.Fatalf("CreateLog() #2 failed: %v", err)
	}
	t.Logf("Created log #2: id=%d, chunk_no=%d", log2.ID, log2.ChunkNo)

	// Step 6: Simulate node appends - Create diff (simulating diff upload from node).
	diffPatch := []byte(`diff --git a/README.md b/README.md
index 1234567..abcdefg 100644
--- a/README.md
+++ b/README.md
@@ -1,3 +1,4 @@
 # Hello World

 This is a smoke test repository.
+Applied modifications via Ploy smoke test.
`)
	diffSummary := []byte(`{"files_changed":1,"insertions":1,"deletions":0}`)
	diff, err := db.CreateDiff(ctx, store.CreateDiffParams{
		RunID:     run.ID,
		JobID:     &job.ID,
		PatchSize: int64(len(diffPatch)),
		Summary:   diffSummary,
	})
	if err != nil {
		t.Fatalf("CreateDiff() failed: %v", err)
	}
	t.Logf("Created diff: id=%v, run_id=%v, job_id=%v, patch_size=%d", diff.ID, diff.RunID, diff.JobID, diff.PatchSize)

	// Step 7: Assert logs are stored - Verify logs can be listed by run.
	logs, err := db.ListLogsByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("ListLogsByRun() failed: %v", err)
	}
	if len(logs) != 2 {
		t.Errorf("Expected 2 logs, got %d", len(logs))
	}
	// Verify logs are ordered by chunk_no.
	if len(logs) >= 2 {
		if logs[0].ChunkNo != 0 || logs[1].ChunkNo != 1 {
			t.Errorf("Logs not ordered correctly: chunk_no[0]=%d, chunk_no[1]=%d", logs[0].ChunkNo, logs[1].ChunkNo)
		}
		if logs[0].DataSize != int64(len(logData)) {
			t.Errorf("Log chunk 0 data_size mismatch")
		}
		if logs[1].DataSize != int64(len(log2Data)) {
			t.Errorf("Log chunk 1 data_size mismatch")
		}
	}
	t.Logf("✓ Verified %d log chunks stored correctly", len(logs))

	// Step 8: Assert diffs are stored - Verify diffs can be listed by run.
	diffs, err := db.ListDiffsByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("ListDiffsByRun() failed: %v", err)
	}
	if len(diffs) != 1 {
		t.Errorf("Expected 1 diff, got %d", len(diffs))
	}
	if len(diffs) >= 1 {
		if diffs[0].ID.Bytes != diff.ID.Bytes {
			t.Errorf("Diff ID mismatch: expected %v, got %v", diff.ID, diffs[0].ID)
		}
		if diffs[0].RunID != run.ID {
			t.Errorf("Diff run_id mismatch: expected %v, got %v", run.ID, diffs[0].RunID)
		}
		if diffs[0].JobID == nil || *diffs[0].JobID != job.ID {
			t.Errorf("Diff job_id mismatch: expected %v, got %v", job.ID, diffs[0].JobID)
		}
		if diffs[0].PatchSize != int64(len(diffPatch)) {
			t.Errorf("Diff patch_size mismatch")
		}
		if !jsonEqual(diffs[0].Summary, diffSummary) {
			t.Errorf("Diff summary mismatch: expected %s, got %s", string(diffSummary), string(diffs[0].Summary))
		}
	}
	t.Logf("✓ Verified %d diff row(s) stored correctly", len(diffs))

	// Step 9: Verify the created diff can be resolved by ID from run diffs.
	var fetchedDiff *store.Diff
	for i := range diffs {
		if diffs[i].ID.Bytes == diff.ID.Bytes {
			fetchedDiff = &diffs[i]
			break
		}
	}
	if fetchedDiff == nil {
		t.Fatalf("Diff %v not found in ListDiffsByRun() result", diff.ID)
	}
	if fetchedDiff.ID.Bytes != diff.ID.Bytes {
		t.Errorf("Fetched diff ID mismatch: expected %v, got %v", diff.ID, fetchedDiff.ID)
	}
	if fetchedDiff.RunID != run.ID {
		t.Errorf("Fetched diff run_id mismatch: expected %v, got %v", run.ID, fetchedDiff.RunID)
	}
	t.Logf("✓ Individual diff retrieval successful")

	// Step 10: Create an event to simulate node status updates.
	now := time.Now().UTC()
	eventParams := store.CreateEventParams{
		RunID: run.ID,
		Time: pgtype.Timestamptz{
			Time:  now,
			Valid: true,
		},
		Level:   "info",
		Message: "Run completed successfully in smoke test",
		Meta:    []byte(`{"source":"lab-smoke","status":"completed"}`),
	}
	event, err := db.CreateEvent(ctx, eventParams)
	if err != nil {
		t.Fatalf("CreateEvent() failed: %v", err)
	}
	t.Logf("Created event: id=%d, level=%s, message=%s", event.ID, event.Level, event.Message)

	// Verify event was stored.
	events, err := db.ListEventsByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("ListEventsByRun() failed: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(events))
	}
	if len(events) >= 1 && events[0].ID != event.ID {
		t.Errorf("Event ID mismatch: expected %d, got %d", event.ID, events[0].ID)
	}
	t.Logf("✓ Event stored correctly")

	t.Log("✓ Lab smoke test completed successfully: logs and diffs are stored")
}

func jsonEqual(a, b []byte) bool {
	var objA any
	var objB any
	if err := json.Unmarshal(a, &objA); err != nil {
		return bytes.Equal(a, b)
	}
	if err := json.Unmarshal(b, &objB); err != nil {
		return bytes.Equal(a, b)
	}
	return reflect.DeepEqual(objA, objB)
}
