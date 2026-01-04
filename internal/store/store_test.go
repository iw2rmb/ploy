package store

import (
	"context"
	"os"
	"testing"
)

// TestNewStore verifies that Store creation works with a valid DSN.
// This test is skipped if PLOY_TEST_PG_DSN is not set, following the pattern
// of integration tests that require external dependencies.
func TestNewStore(t *testing.T) {
	dsn := os.Getenv("PLOY_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_PG_DSN not set; skipping store initialization test")
	}

	ctx := context.Background()
	store, err := NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore() failed: %v", err)
	}
	defer store.Close()

	// Connectivity verified by running migrations in NewStore; no cluster table anymore.
}

// TestNewStore_InvalidDSN verifies that Store creation fails gracefully with an invalid DSN.
func TestNewStore_InvalidDSN(t *testing.T) {
	ctx := context.Background()
	_, err := NewStore(ctx, "invalid-dsn")
	if err == nil {
		t.Fatal("NewStore() should have failed with invalid DSN")
	}
}

// TestCreateRun_WithAndWithoutName verifies that runs can be created with or without
// an optional batch name, and that the name round-trips correctly through the database.
// This test covers the optional batch naming feature per ROADMAP.md line 24.
func TestCreateRun_WithAndWithoutName(t *testing.T) {
	dsn := os.Getenv("PLOY_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_PG_DSN not set; skipping store integration test")
	}

	ctx := context.Background()
	db, err := NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore() failed: %v", err)
	}
	defer db.Close()

	// Subtest: Create a run without a name (nil).
	t.Run("without_name", func(t *testing.T) {
		run, err := db.CreateRun(ctx, CreateRunParams{
			Name:      nil, // No batch name.
			RepoUrl:   "https://github.com/test/no-name",
			Spec:      []byte(`{"type":"unnamed-run"}`),
			Status:    RunStatusStarted,
			BaseRef:   "main",
			TargetRef: "feature/unnamed",
		})
		if err != nil {
			t.Fatalf("CreateRun() failed: %v", err)
		}

		// Verify name is nil.
		if run.Name != nil {
			t.Errorf("expected run.Name to be nil, got %q", *run.Name)
		}

		// Fetch and verify round-trip.
		fetched, err := db.GetRun(ctx, run.ID)
		if err != nil {
			t.Fatalf("GetRun() failed: %v", err)
		}
		if fetched.Name != nil {
			t.Errorf("expected fetched run.Name to be nil, got %q", *fetched.Name)
		}
	})

	// Subtest: Create a run with a batch name.
	t.Run("with_name", func(t *testing.T) {
		batchName := "my-batch-2024-12-06"
		run, err := db.CreateRun(ctx, CreateRunParams{
			Name:      &batchName,
			RepoUrl:   "https://github.com/test/with-name",
			Spec:      []byte(`{"type":"named-run"}`),
			Status:    RunStatusStarted,
			BaseRef:   "main",
			TargetRef: "feature/named",
		})
		if err != nil {
			t.Fatalf("CreateRun() failed: %v", err)
		}

		// Verify name is set.
		if run.Name == nil {
			t.Fatal("expected run.Name to be set, got nil")
		}
		if *run.Name != batchName {
			t.Errorf("expected run.Name = %q, got %q", batchName, *run.Name)
		}

		// Fetch and verify round-trip.
		fetched, err := db.GetRun(ctx, run.ID)
		if err != nil {
			t.Fatalf("GetRun() failed: %v", err)
		}
		if fetched.Name == nil {
			t.Fatal("expected fetched run.Name to be set, got nil")
		}
		if *fetched.Name != batchName {
			t.Errorf("expected fetched run.Name = %q, got %q", batchName, *fetched.Name)
		}
	})

	// Subtest: List runs includes name field correctly.
	t.Run("list_includes_name", func(t *testing.T) {
		runs, err := db.ListRuns(ctx, ListRunsParams{Limit: 10, Offset: 0})
		if err != nil {
			t.Fatalf("ListRuns() failed: %v", err)
		}

		// Find at least one named and one unnamed run from previous subtests.
		var foundNamed, foundUnnamed bool
		for _, r := range runs {
			if r.Name != nil && *r.Name == "my-batch-2024-12-06" {
				foundNamed = true
			}
			if r.Name == nil && r.RepoUrl == "https://github.com/test/no-name" {
				foundUnnamed = true
			}
		}

		if !foundNamed {
			t.Error("expected to find a named run in ListRuns output")
		}
		if !foundUnnamed {
			t.Error("expected to find an unnamed run in ListRuns output")
		}
	})
}

// TestRunRepo_CRUDAndStateTransitions verifies the RunRepo CRUD operations and
// state-transition helpers. Per ROADMAP.md line 49, this test covers:
// - CreateRunRepo: attaches a repo to a run with 'pending' status.
// - ListRunReposByRun: lists all repos for a given run.
// - UpdateRunRepoStatus: transitions status and auto-sets timing fields.
// - CountRunReposByStatus: aggregates counts by status for batch-level derivation.
// - IncrementRunRepoAttempt: resets state for retry with incremented attempt.
// - UpdateRunRepoError: sets last_error on failure.
// - DeleteRunRepo: removes a repo entry.
func TestRunRepo_CRUDAndStateTransitions(t *testing.T) {
	dsn := os.Getenv("PLOY_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_PG_DSN not set; skipping store integration test")
	}

	ctx := context.Background()
	db, err := NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore() failed: %v", err)
	}
	defer db.Close()

	// Create a parent run to attach run_repos to.
	batchName := "test-batch-runrepo"
	parentRun, err := db.CreateRun(ctx, CreateRunParams{
		Name:      &batchName,
		RepoUrl:   "https://github.com/test/batch-parent",
		Spec:      []byte(`{"type":"batch"}`),
		Status:    RunStatusStarted,
		BaseRef:   "main",
		TargetRef: "feature/batch",
	})
	if err != nil {
		t.Fatalf("CreateRun() for parent batch failed: %v", err)
	}

	// Subtest: CreateRunRepo creates a repo entry with 'pending' status.
	t.Run("create_run_repo", func(t *testing.T) {
		repo, err := db.CreateRunRepo(ctx, CreateRunRepoParams{
			RunID:     parentRun.ID,
			RepoUrl:   "https://github.com/org/repo-a",
			BaseRef:   "main",
			TargetRef: "feature/a",
		})
		if err != nil {
			t.Fatalf("CreateRunRepo() failed: %v", err)
		}

		// Verify defaults: status='pending', attempt=1.
		if repo.Status != RunRepoStatusQueued {
			t.Errorf("expected status=%q, got %q", RunRepoStatusQueued, repo.Status)
		}
		if repo.Attempt != 1 {
			t.Errorf("expected attempt=1, got %d", repo.Attempt)
		}
		if repo.LastError != nil {
			t.Errorf("expected last_error=nil, got %q", *repo.LastError)
		}

		// Fetch via GetRunRepo and verify round-trip.
		fetched, err := db.GetRunRepo(ctx, repo.ID)
		if err != nil {
			t.Fatalf("GetRunRepo() failed: %v", err)
		}
		if fetched.RepoUrl != repo.RepoUrl {
			t.Errorf("expected repo_url=%q, got %q", repo.RepoUrl, fetched.RepoUrl)
		}
	})

	// Subtest: ListRunReposByRun returns repos ordered by created_at.
	t.Run("list_run_repos_by_run", func(t *testing.T) {
		// Add a second repo.
		_, err := db.CreateRunRepo(ctx, CreateRunRepoParams{
			RunID:     parentRun.ID,
			RepoUrl:   "https://github.com/org/repo-b",
			BaseRef:   "main",
			TargetRef: "feature/b",
		})
		if err != nil {
			t.Fatalf("CreateRunRepo() for repo-b failed: %v", err)
		}

		repos, err := db.ListRunReposByRun(ctx, parentRun.ID)
		if err != nil {
			t.Fatalf("ListRunReposByRun() failed: %v", err)
		}

		// Expect at least 2 repos.
		if len(repos) < 2 {
			t.Errorf("expected at least 2 repos, got %d", len(repos))
		}

		// Verify ordering: first repo should be repo-a (created earlier).
		if len(repos) >= 2 && repos[0].RepoUrl != "https://github.com/org/repo-a" {
			t.Errorf("expected first repo to be repo-a, got %q", repos[0].RepoUrl)
		}
	})

	// Subtest: UpdateRunRepoStatus transitions status and sets timing fields.
	t.Run("status_transitions_pending_to_running_to_succeeded", func(t *testing.T) {
		// Create a fresh repo for isolated testing.
		repo, err := db.CreateRunRepo(ctx, CreateRunRepoParams{
			RunID:     parentRun.ID,
			RepoUrl:   "https://github.com/org/repo-transition",
			BaseRef:   "main",
			TargetRef: "feature/transition",
		})
		if err != nil {
			t.Fatalf("CreateRunRepo() failed: %v", err)
		}

		// Transition: pending → running.
		err = db.UpdateRunRepoStatus(ctx, UpdateRunRepoStatusParams{
			ID:     repo.ID,
			Status: RunRepoStatusRunning,
		})
		if err != nil {
			t.Fatalf("UpdateRunRepoStatus(running) failed: %v", err)
		}

		// Verify started_at is set.
		running, err := db.GetRunRepo(ctx, repo.ID)
		if err != nil {
			t.Fatalf("GetRunRepo() failed: %v", err)
		}
		if running.Status != RunRepoStatusRunning {
			t.Errorf("expected status=%q, got %q", RunRepoStatusRunning, running.Status)
		}
		if !running.StartedAt.Valid {
			t.Error("expected started_at to be set after transition to running")
		}
		if running.FinishedAt.Valid {
			t.Error("expected finished_at to be unset while running")
		}

		// Transition: running → succeeded.
		err = db.UpdateRunRepoStatus(ctx, UpdateRunRepoStatusParams{
			ID:     repo.ID,
			Status: RunRepoStatusSuccess,
		})
		if err != nil {
			t.Fatalf("UpdateRunRepoStatus(succeeded) failed: %v", err)
		}

		// Verify finished_at is set.
		succeeded, err := db.GetRunRepo(ctx, repo.ID)
		if err != nil {
			t.Fatalf("GetRunRepo() failed: %v", err)
		}
		if succeeded.Status != RunRepoStatusSuccess {
			t.Errorf("expected status=%q, got %q", RunRepoStatusSuccess, succeeded.Status)
		}
		if !succeeded.FinishedAt.Valid {
			t.Error("expected finished_at to be set after terminal status")
		}
		// started_at should remain unchanged.
		if !succeeded.StartedAt.Valid {
			t.Error("expected started_at to remain set")
		}
	})

	// Subtest: CountRunReposByStatus aggregates counts correctly.
	t.Run("count_run_repos_by_status", func(t *testing.T) {
		counts, err := db.CountRunReposByStatus(ctx, parentRun.ID)
		if err != nil {
			t.Fatalf("CountRunReposByStatus() failed: %v", err)
		}

		// Build a map for easy lookup.
		countMap := make(map[RunRepoStatus]int32)
		for _, c := range counts {
			countMap[c.Status] = c.Count
		}

		// We should have multiple repos in various states by now.
		// At minimum: pending repos (repo-a, repo-b), succeeded (repo-transition).
		totalCount := int32(0)
		for _, c := range countMap {
			totalCount += c
		}
		if totalCount < 3 {
			t.Errorf("expected at least 3 repos in aggregate, got %d", totalCount)
		}

		// Verify succeeded count is at least 1.
		if countMap[RunRepoStatusSuccess] < 1 {
			t.Errorf("expected at least 1 succeeded repo, got %d", countMap[RunRepoStatusSuccess])
		}
	})

	// Subtest: UpdateRunRepoError sets last_error field.
	t.Run("update_run_repo_error", func(t *testing.T) {
		// Create a repo for error testing.
		repo, err := db.CreateRunRepo(ctx, CreateRunRepoParams{
			RunID:     parentRun.ID,
			RepoUrl:   "https://github.com/org/repo-error",
			BaseRef:   "main",
			TargetRef: "feature/error",
		})
		if err != nil {
			t.Fatalf("CreateRunRepo() failed: %v", err)
		}

		// Transition to failed and set error.
		errMsg := "build failed: exit code 1"
		err = db.UpdateRunRepoError(ctx, UpdateRunRepoErrorParams{
			ID:        repo.ID,
			LastError: &errMsg,
		})
		if err != nil {
			t.Fatalf("UpdateRunRepoError() failed: %v", err)
		}

		err = db.UpdateRunRepoStatus(ctx, UpdateRunRepoStatusParams{
			ID:     repo.ID,
			Status: RunRepoStatusFail,
		})
		if err != nil {
			t.Fatalf("UpdateRunRepoStatus(failed) failed: %v", err)
		}

		// Verify last_error and status.
		failed, err := db.GetRunRepo(ctx, repo.ID)
		if err != nil {
			t.Fatalf("GetRunRepo() failed: %v", err)
		}
		if failed.LastError == nil || *failed.LastError != errMsg {
			t.Errorf("expected last_error=%q, got %v", errMsg, failed.LastError)
		}
		if failed.Status != RunRepoStatusFail {
			t.Errorf("expected status=%q, got %q", RunRepoStatusFail, failed.Status)
		}
	})

	// Subtest: IncrementRunRepoAttempt resets status and increments attempt.
	t.Run("increment_run_repo_attempt", func(t *testing.T) {
		// Create a repo and transition to failed.
		repo, err := db.CreateRunRepo(ctx, CreateRunRepoParams{
			RunID:     parentRun.ID,
			RepoUrl:   "https://github.com/org/repo-retry",
			BaseRef:   "main",
			TargetRef: "feature/retry",
		})
		if err != nil {
			t.Fatalf("CreateRunRepo() failed: %v", err)
		}

		// Simulate a failed run.
		_ = db.UpdateRunRepoStatus(ctx, UpdateRunRepoStatusParams{ID: repo.ID, Status: RunRepoStatusRunning})
		errMsg := "transient failure"
		_ = db.UpdateRunRepoError(ctx, UpdateRunRepoErrorParams{ID: repo.ID, LastError: &errMsg})
		_ = db.UpdateRunRepoStatus(ctx, UpdateRunRepoStatusParams{ID: repo.ID, Status: RunRepoStatusFail})

		// Verify current state before retry.
		beforeRetry, _ := db.GetRunRepo(ctx, repo.ID)
		if beforeRetry.Attempt != 1 {
			t.Errorf("expected attempt=1 before retry, got %d", beforeRetry.Attempt)
		}

		// Retry: increment attempt.
		err = db.IncrementRunRepoAttempt(ctx, repo.ID)
		if err != nil {
			t.Fatalf("IncrementRunRepoAttempt() failed: %v", err)
		}

		// Verify state after retry.
		afterRetry, err := db.GetRunRepo(ctx, repo.ID)
		if err != nil {
			t.Fatalf("GetRunRepo() failed: %v", err)
		}
		if afterRetry.Attempt != 2 {
			t.Errorf("expected attempt=2 after retry, got %d", afterRetry.Attempt)
		}
		if afterRetry.Status != RunRepoStatusQueued {
			t.Errorf("expected status=%q after retry, got %q", RunRepoStatusQueued, afterRetry.Status)
		}
		if afterRetry.LastError != nil {
			t.Errorf("expected last_error=nil after retry, got %q", *afterRetry.LastError)
		}
		if afterRetry.StartedAt.Valid {
			t.Error("expected started_at to be cleared after retry")
		}
		if afterRetry.FinishedAt.Valid {
			t.Error("expected finished_at to be cleared after retry")
		}
	})

	// Subtest: DeleteRunRepo removes the repo entry.
	t.Run("delete_run_repo", func(t *testing.T) {
		repo, err := db.CreateRunRepo(ctx, CreateRunRepoParams{
			RunID:     parentRun.ID,
			RepoUrl:   "https://github.com/org/repo-delete",
			BaseRef:   "main",
			TargetRef: "feature/delete",
		})
		if err != nil {
			t.Fatalf("CreateRunRepo() failed: %v", err)
		}

		// Delete the repo.
		err = db.DeleteRunRepo(ctx, repo.ID)
		if err != nil {
			t.Fatalf("DeleteRunRepo() failed: %v", err)
		}

		// Verify it's gone.
		_, err = db.GetRunRepo(ctx, repo.ID)
		if err == nil {
			t.Error("expected GetRunRepo() to fail for deleted repo")
		}
	})
}
