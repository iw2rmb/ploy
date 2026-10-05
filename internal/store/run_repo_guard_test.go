package store

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/jackc/pgx/v5"
)

func TestCreateWaveWithRuns_RepoGuardIgnoresBranchAndMig(t *testing.T) {
	for _, status := range []types.RunStatus{
		types.RunStatusQueued, types.RunStatusRunning, types.RunStatusSuccess,
		types.RunStatusFail, types.RunStatusCancelled,
	} {
		t.Run(string(status), func(t *testing.T) {
			ctx, db := newTestStore(t)
			fx := newV1Fixture(t, ctx, db, "https://gitlab.example.com/acme/service", "main", []byte(`{}`))
			if err := db.UpdateRunStatus(ctx, UpdateRunStatusParams{ID: fx.Run.ID, Status: status}); err != nil {
				t.Fatal(err)
			}
			migID := types.NewMigID()
			if _, err := db.CreateMig(ctx, CreateMigParams{ID: migID, Name: migID.String(), SpecID: &fx.Spec.ID}); err != nil {
				t.Fatal(err)
			}
			repo, err := db.CreateMigRepo(ctx, CreateMigRepoParams{
				ID: types.NewMigRepoID(), MigID: migID, Url: "https://gitlab.example.com/acme/service", BaseRef: "feature/other",
			})
			if err != nil {
				t.Fatal(err)
			}
			params := guardedWaveForTest(fx, repo.RepoID, "feature/other")
			params.Wave.MigID = migID
			_, runs, err := db.CreateWaveWithRuns(ctx, params)
			if status == types.RunStatusQueued || status == types.RunStatusRunning {
				// A different branch and mig still return the active repository run.
				assertActiveRepoRunForTest(t, err, fx.Run)
				if _, err := db.GetWave(ctx, params.Wave.ID); !errors.Is(err, pgx.ErrNoRows) {
					t.Fatalf("rejected wave persisted: %v", err)
				}
			} else if err != nil || len(runs) != 1 {
				// A terminal run permits the next migration.
				t.Fatalf("terminal run blocked launch: runs=%d err=%v", len(runs), err)
			}
		})
	}
}

func TestCreateWaveWithRuns_RepoConflictRollsBackWholeWave(t *testing.T) {
	ctx, db := newTestStore(t)
	fx := newV1Fixture(t, ctx, db, "https://gitlab.example.com/acme/service", "main", []byte(`{}`))
	repo, err := db.CreateMigRepo(ctx, CreateMigRepoParams{
		ID: types.NewMigRepoID(), MigID: fx.Mig.ID, Url: "https://gitlab.example.com/other/service", BaseRef: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	params := guardedWaveForTest(fx, repo.RepoID, "main")
	params.Runs = append(params.Runs, guardedWaveForTest(fx, fx.Run.RepoID, "feature").Runs[0])
	_, _, err = db.CreateWaveWithRuns(ctx, params)
	// A conflict on the second repo also removes the first repo's run and jobs.
	assertActiveRepoRunForTest(t, err, fx.Run)
	if _, err := db.GetWave(ctx, params.Wave.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("rejected wave persisted: %v", err)
	}
	for _, plan := range params.Runs {
		if _, err := db.GetRun(ctx, plan.ID); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("rejected run persisted: %v", err)
		}
		if jobs, err := db.ListJobsByRun(ctx, plan.ID); err != nil || len(jobs) != 0 {
			t.Fatalf("rejected run jobs=%d err=%v", len(jobs), err)
		}
	}
	params.Runs = params.Runs[:1]
	// The same repo name in another namespace can run independently.
	if _, _, err := db.CreateWaveWithRuns(ctx, params); err != nil {
		t.Fatalf("another namespace blocked: %v", err)
	}
}

func TestCreateWaveWithRuns_DuplicateRepoInWaveIsRejected(t *testing.T) {
	ctx, db := newTestStore(t)
	fx := newV1Fixture(t, ctx, db, "https://gitlab.example.com/acme/service", "main", []byte(`{}`))
	if err := db.CancelRun(ctx, fx.Run.ID); err != nil {
		t.Fatal(err)
	}
	params := guardedWaveForTest(fx, fx.Run.RepoID, "main")
	params.Runs = append(params.Runs, guardedWaveForTest(fx, fx.Run.RepoID, "feature").Runs[0])
	_, _, err := db.CreateWaveWithRuns(ctx, params)
	// Two branches in one wave cannot create two active runs for the same repo.
	assertActiveRepoRunForTest(t, err, Run{ID: params.Runs[0].ID, RepoID: fx.Run.RepoID})
	if _, err := db.GetWave(ctx, params.Wave.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("duplicate-repo wave persisted: %v", err)
	}
}

func TestRestartRun_RepoConflictPreservesTerminalAttempt(t *testing.T) {
	ctx, db := newTestStore(t)
	fx := newV1Fixture(t, ctx, db, "https://gitlab.example.com/acme/service", "main", []byte(`{}`))
	if err := db.CancelRun(ctx, fx.Run.ID); err != nil {
		t.Fatal(err)
	}
	_, current, err := db.CreateWaveWithRuns(ctx, guardedWaveForTest(fx, fx.Run.RepoID, "feature"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.RestartRun(ctx, RestartRunParams{RunID: fx.Run.ID, ExpectedAttempt: fx.Run.Attempt, Jobs: plannedJobsForStoreTest()})
	// Restart cannot bypass another branch's active migration.
	assertActiveRepoRunForTest(t, err, current[0].Run)
	unchanged, err := db.GetRun(ctx, fx.Run.ID)
	if err != nil || unchanged.Attempt != fx.Run.Attempt || unchanged.Status != types.RunStatusCancelled {
		t.Fatalf("rejected restart changed run: %+v err=%v", unchanged, err)
	}
	if jobs, err := db.ListJobsByRun(ctx, fx.Run.ID); err != nil || len(jobs) != 0 {
		t.Fatalf("rejected restart jobs=%d err=%v", len(jobs), err)
	}
}

func TestRepoGuard_ConcurrentLaunchesAndRestartsCreateOneActiveRun(t *testing.T) {
	for _, mode := range []string{"launch-launch", "launch-restart", "restart-restart"} {
		t.Run(mode, func(t *testing.T) {
			ctx, db := newTestStore(t)
			ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			fx := newV1Fixture(t, ctx, db, "https://gitlab.example.com/acme/service", "main", []byte(`{}`))
			if err := db.CancelRun(ctx, fx.Run.ID); err != nil {
				t.Fatal(err)
			}
			other := newV1Fixture(t, ctx, db, "https://gitlab.example.com/acme/service", "feature", []byte(`{}`))
			if err := db.CancelRun(ctx, other.Run.ID); err != nil {
				t.Fatal(err)
			}
			secondStore, err := NewStore(ctx, os.Getenv("PLOY_TEST_DB_DSN"))
			if err != nil {
				t.Fatal(err)
			}
			defer secondStore.Close()
			stores := []Store{db, secondStore}
			fixtures := []v1Fixture{fx, other}
			start := make(chan struct{})
			type result struct {
				run Run
				err error
			}
			results := make(chan result, 2)
			for i := range stores {
				go func(i int) {
					<-start
					fixture := fixtures[i]
					if mode == "restart-restart" || (mode == "launch-restart" && i == 1) {
						run, err := stores[i].RestartRun(ctx, RestartRunParams{
							RunID: fixture.Run.ID, ExpectedAttempt: fixture.Run.Attempt, Jobs: plannedJobsForStoreTest(),
						})
						results <- result{run: run, err: err}
					} else {
						_, runs, err := stores[i].CreateWaveWithRuns(ctx, guardedWaveForTest(fixture, fixture.Run.RepoID, fixture.Run.RepoBaseRef))
						var run Run
						if len(runs) != 0 {
							run = runs[0].Run
						}
						results <- result{run: run, err: err}
					}
				}(i)
			}
			close(start)
			first, second := <-results, <-results
			if first.err != nil {
				first, second = second, first
			}
			// Separate store connections must agree on one winner and its Run ID.
			if first.err != nil || first.run.ID.IsZero() {
				t.Fatalf("no winning run: %v / %v", first.err, second.err)
			}
			assertActiveRepoRunForTest(t, second.err, first.run)
			var count int
			if err := db.Pool().QueryRow(ctx, `SELECT count(*) FROM runs WHERE repo_id = $1 AND status IN ('Queued', 'Running')`, fx.Run.RepoID).Scan(&count); err != nil || count != 1 {
				t.Fatalf("active runs=%d err=%v, want 1", count, err)
			}
			jobs, err := db.ListJobsByRunAttempt(ctx, ListJobsByRunAttemptParams{RunID: first.run.ID, Attempt: first.run.Attempt})
			if err != nil || len(jobs) != 2 {
				t.Fatalf("winner jobs=%d err=%v, want one complete chain", len(jobs), err)
			}
		})
	}
}

func guardedWaveForTest(fx v1Fixture, repoID types.RepoID, ref string) CreateWaveWithRunsParams {
	return CreateWaveWithRunsParams{
		Wave: CreateWaveParams{ID: types.NewWaveID(), MigID: fx.Mig.ID, SpecID: fx.Spec.ID},
		Runs: []RunPlan{{
			ID: types.NewRunID(), RepoID: repoID, RepoBaseRef: ref,
			SourceCommitSha: testSHA, RepoSha0: testSHA, Jobs: plannedJobsForStoreTest(),
		}},
	}
}

func assertActiveRepoRunForTest(t *testing.T, err error, run Run) {
	t.Helper()
	var conflict *ActiveRepoRunError
	if !errors.As(err, &conflict) || conflict.RunID != run.ID || conflict.RepoID != run.RepoID {
		t.Fatalf("conflict=%v, want active run %s for repo %s", err, run.ID, run.RepoID)
	}
}
