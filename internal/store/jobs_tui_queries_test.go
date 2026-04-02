package store

import (
	"context"
	"os"
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/segmentio/ksuid"
)

// TestListJobsForTUI covers ordering, run_id filtering, and total counting via CountJobsForTUI.
// Skipped unless PLOY_TEST_DB_DSN is set.
func TestListJobsForTUI(t *testing.T) {
	dsn := os.Getenv("PLOY_TEST_DB_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_DB_DSN not set; skipping store integration test")
	}

	ctx := context.Background()
	db, err := NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore() failed: %v", err)
	}
	defer db.Close()
	cleanTestTables(t, ctx, db)

	// Two separate runs so we can test filtered vs unfiltered results.
	fxA := newV1Fixture(t, ctx, db, "https://github.com/test/tui-a", "main", "feat-a", []byte(`{"type":"test"}`))
	fxB := newV1Fixture(t, ctx, db, "https://github.com/test/tui-b", "main", "feat-b", []byte(`{"type":"test"}`))

	createJob := func(fx v1Fixture, name string, id types.JobID) types.JobID {
		t.Helper()
		_, err := db.CreateJob(ctx, CreateJobParams{
			ID:          id,
			RunID:       fx.Run.ID,
			RepoID:      fx.MigRepo.RepoID,
			RepoBaseRef: fx.RunRepo.RepoBaseRef,
			Attempt:     fx.RunRepo.Attempt,
			Name:        name,
			Status:      types.JobStatusQueued,
			JobType:     "mig",
			JobImage:    "",
			NextID:      nil,
			Meta:        []byte(`{}`),
			RepoShaIn:   "0000000000000000000000000000000000000000",
		})
		if err != nil {
			t.Fatalf("CreateJob(%s) failed: %v", name, err)
		}
		return id
	}

	// Pre-generate IDs with distinct timestamps so lexicographic (DESC) ordering is deterministic.
	// idA1 < idA2 < idB1 by KSUID string comparison (oldest to newest).
	mustKSUID := func(s string) types.JobID {
		k, err := ksuid.Parse(s)
		if err != nil {
			t.Fatalf("ksuid.Parse(%q): %v", s, err)
		}
		return types.JobID(k.String())
	}
	idA1 := mustKSUID("1VlnyEyNOAvwaZh3KZJkgl61t4x")
	idA2 := mustKSUID("1VlnyOsy3wff4GTBrpOGU9GfYZH")
	idB1 := mustKSUID("1VlnyXwq5jBYFLpwR2ZoF4BiNxr")

	// Pre-delete hard-coded IDs in case a previous test run left them behind.
	for _, id := range []types.JobID{idA1, idA2, idB1} {
		_, _ = db.Pool().Exec(ctx, "DELETE FROM jobs WHERE id = $1", id)
	}
	t.Cleanup(func() {
		for _, id := range []types.JobID{idA1, idA2, idB1} {
			_, _ = db.Pool().Exec(ctx, "DELETE FROM jobs WHERE id = $1", id)
		}
	})

	jobA1 := createJob(fxA, "job-a1", idA1)
	jobA2 := createJob(fxA, "job-a2", idA2)
	jobB1 := createJob(fxB, "job-b1", idB1)

	t.Run("NewestToOldestOrdering", func(t *testing.T) {
		rows, err := db.ListJobsForTUI(ctx, ListJobsForTUIParams{
			Limit:  100,
			Offset: 0,
			RunID:  nil,
		})
		if err != nil {
			t.Fatalf("ListJobsForTUI() failed: %v", err)
		}

		// Find positions of our three jobs among all rows.
		pos := map[types.JobID]int{}
		for i, r := range rows {
			pos[r.JobID] = i
		}
		for _, id := range []types.JobID{jobA1, jobA2, jobB1} {
			if _, ok := pos[id]; !ok {
				t.Fatalf("job %s not found in ListJobsForTUI results", id)
			}
		}
		// Newer jobs (higher KSUID) must appear before older ones.
		if pos[jobA2] >= pos[jobA1] {
			t.Errorf("expected jobA2 (newer) before jobA1 (older), got positions %d vs %d", pos[jobA2], pos[jobA1])
		}
		if pos[jobB1] >= pos[jobA1] {
			t.Errorf("expected jobB1 (newer) before jobA1 (older), got positions %d vs %d", pos[jobB1], pos[jobA1])
		}
	})

	t.Run("RunIDFilteredResults", func(t *testing.T) {
		runIDA := fxA.Run.ID
		rows, err := db.ListJobsForTUI(ctx, ListJobsForTUIParams{
			Limit:  100,
			Offset: 0,
			RunID:  &runIDA,
		})
		if err != nil {
			t.Fatalf("ListJobsForTUI(run_id=A) failed: %v", err)
		}

		for _, r := range rows {
			if r.RunID != runIDA {
				t.Errorf("expected all rows to have run_id=%s, got %s", runIDA, r.RunID)
			}
		}

		// Our two A jobs must be present.
		found := map[types.JobID]bool{}
		for _, r := range rows {
			found[r.JobID] = true
		}
		for _, id := range []types.JobID{jobA1, jobA2} {
			if !found[id] {
				t.Errorf("expected job %s in filtered results", id)
			}
		}
		if found[jobB1] {
			t.Errorf("job %s from run B should not appear when filtering by run A", jobB1)
		}
	})

	t.Run("UnfilteredIncludesAllRuns", func(t *testing.T) {
		rows, err := db.ListJobsForTUI(ctx, ListJobsForTUIParams{
			Limit:  100,
			Offset: 0,
			RunID:  nil,
		})
		if err != nil {
			t.Fatalf("ListJobsForTUI(unfiltered) failed: %v", err)
		}

		found := map[types.JobID]bool{}
		for _, r := range rows {
			found[r.JobID] = true
		}
		for _, id := range []types.JobID{jobA1, jobA2, jobB1} {
			if !found[id] {
				t.Errorf("expected job %s in unfiltered results", id)
			}
		}
	})

	t.Run("CountJobsForTUI_Unfiltered", func(t *testing.T) {
		count, err := db.CountJobsForTUI(ctx, nil)
		if err != nil {
			t.Fatalf("CountJobsForTUI(nil) failed: %v", err)
		}
		if count < 3 {
			t.Errorf("expected at least 3 jobs, got %d", count)
		}
	})

	t.Run("CountJobsForTUI_FilteredByRun", func(t *testing.T) {
		runIDA := fxA.Run.ID

		countA, err := db.CountJobsForTUI(ctx, &runIDA)
		if err != nil {
			t.Fatalf("CountJobsForTUI(runA) failed: %v", err)
		}
		if countA < 2 {
			t.Errorf("expected at least 2 jobs for run A, got %d", countA)
		}

		runIDB := fxB.Run.ID
		countB, err := db.CountJobsForTUI(ctx, &runIDB)
		if err != nil {
			t.Fatalf("CountJobsForTUI(runB) failed: %v", err)
		}
		if countB < 1 {
			t.Errorf("expected at least 1 job for run B, got %d", countB)
		}

		countAll, err := db.CountJobsForTUI(ctx, nil)
		if err != nil {
			t.Fatalf("CountJobsForTUI(nil) for sum check failed: %v", err)
		}
		if countAll < countA+countB {
			t.Errorf("unfiltered count %d should be >= sum of per-run counts %d+%d=%d", countAll, countA, countB, countA+countB)
		}
	})
}
