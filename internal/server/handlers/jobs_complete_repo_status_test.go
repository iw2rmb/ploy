package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// ===== Repo-Scoped Status Progression Tests =====
// These tests verify v1 repo-scoped progression behavior.
// When a job completes:
// - run_repos.status is updated when all jobs for the repo attempt are terminal
// - runs.status becomes Finished when all repos are terminal

// TestCompleteJob_RepoStatusUpdatedOnLastJob verifies that run_repos.status is updated
// to Success when the last job in a repo attempt completes successfully.
func TestCompleteJob_RepoStatusUpdatedOnLastJob(t *testing.T) {
	t.Parallel()

	f := newRepoScopedFixture("mig", 2000)

	// Single job per repo, completing it should mark repo as terminal.
	st := newMockStoreForJob(f,
		// All jobs (1 total) are now Success after completion.
		withRepoAttemptJobs([]store.Job{
			{
				ID:          f.JobID,
				RunID:       f.RunID,
				RepoID:      f.Job.RepoID,
				RepoBaseRef: "main",
				Attempt:     1,
				Name:        "mig-0",
				Status:      domaintypes.JobStatusSuccess,
				JobType:     "mig",
				Meta:        withNextIDMeta([]byte(`{}`), 2000),
			},
		}),
		// All repos terminal (1 Success), so run becomes Finished.
		withRunRepoStatusCounts([]store.CountRunReposByStatusRow{
			{Status: domaintypes.RunRepoStatusSuccess, Count: 1},
		}),
	)

	handler := completeJobHandler(st, nil, nil)

	req := f.completeJobReq(map[string]any{
		"status":       "Success",
		"exit_code":    0,
		"repo_sha_out": "0123456789abcdef0123456789abcdef01234567",
	})

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusNoContent)

	// Verify ListJobsByRunRepoAttempt was called to check repo terminal state.
	assertCalled(t, "ListJobsByRunRepoAttempt", st.listJobsByRunRepoAttemptCalled)
	if st.listJobsByRunRepoAttemptParams.RunID != f.RunID {
		t.Errorf("expected run_id %s, got %s", f.RunID, st.listJobsByRunRepoAttemptParams.RunID)
	}
	if st.listJobsByRunRepoAttemptParams.RepoID != f.Job.RepoID {
		t.Errorf("expected repo_id %s, got %s", f.Job.RepoID, st.listJobsByRunRepoAttemptParams.RepoID)
	}

	// Verify UpdateRunRepoStatus was called to update repo to Success.
	assertCalled(t, "UpdateRunRepoStatus", st.updateRunRepoStatusCalled)
	if len(st.updateRunRepoStatusParams) == 0 {
		t.Fatal("expected at least one UpdateRunRepoStatus call")
	}
	lastRepoUpdate := st.updateRunRepoStatusParams[len(st.updateRunRepoStatusParams)-1]
	if lastRepoUpdate.Status != domaintypes.RunRepoStatusSuccess {
		t.Errorf("expected repo status Success, got %s", lastRepoUpdate.Status)
	}

	// Verify UpdateRunStatus was called to set run to Finished.
	assertCalled(t, "UpdateRunStatus", st.updateRunStatusCalled)
	if st.updateRunStatusParams.Status != domaintypes.RunStatusFinished {
		t.Errorf("expected run status Finished, got %s", st.updateRunStatusParams.Status)
	}
}

// TestCompleteJob_RepoStatusFail verifies that run_repos.status is updated
// to Fail when a job in the repo attempt fails.
func TestCompleteJob_RepoStatusFail(t *testing.T) {
	t.Parallel()

	f := newRepoScopedFixture("mig", 2000)

	// Job that will fail.
	st := newMockStoreForJob(f,
		withRepoAttemptJobs([]store.Job{
			{
				ID:          f.JobID,
				RunID:       f.RunID,
				RepoID:      f.Job.RepoID,
				RepoBaseRef: "main",
				Attempt:     1,
				Name:        "mig-0",
				Status:      domaintypes.JobStatusFail,
				JobType:     "mig",
				Meta:        withNextIDMeta([]byte(`{}`), 2000),
			},
		}),
		// All repos terminal.
		withRunRepoStatusCounts([]store.CountRunReposByStatusRow{
			{Status: domaintypes.RunRepoStatusFail, Count: 1},
		}),
	)

	handler := completeJobHandler(st, nil, nil)

	req := f.completeJobReq(map[string]any{
		"status":    "Fail",
		"exit_code": 1,
	})

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusNoContent)

	// Verify repo status was updated to Fail.
	assertCalled(t, "UpdateRunRepoStatus", st.updateRunRepoStatusCalled)
	if len(st.updateRunRepoStatusParams) == 0 {
		t.Fatal("expected at least one UpdateRunRepoStatus call")
	}
	lastRepoUpdate := st.updateRunRepoStatusParams[len(st.updateRunRepoStatusParams)-1]
	if lastRepoUpdate.Status != domaintypes.RunRepoStatusFail {
		t.Errorf("expected repo status Fail, got %s", lastRepoUpdate.Status)
	}
}

// TestCompleteJob_RepoNotTerminalWhileJobsInProgress verifies that run_repos.status
// is NOT updated when there are still non-terminal jobs for the repo attempt.
func TestCompleteJob_RepoNotTerminalWhileJobsInProgress(t *testing.T) {
	t.Parallel()

	f := newRepoScopedFixture("pre_gate", 1000)
	f.Job.Name = "pre-gate"

	// Two jobs: first completes, second is still Created.
	nextJobID := domaintypes.NewJobID()
	f.Job.NextID = &nextJobID
	job2 := store.Job{
		ID:          nextJobID,
		RunID:       f.RunID,
		RepoID:      f.Job.RepoID,
		RepoBaseRef: "main",
		Attempt:     1,
		Name:        "mig-0",
		Status:      domaintypes.JobStatusCreated,
		JobType:     "mig",
		Meta:        withNextIDMeta([]byte(`{}`), 2000),
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     f.RunID,
			Status: domaintypes.RunStatusStarted,
		},
		getJobResult:                    f.Job,
		listJobsByRunResult:             []store.Job{f.Job, job2},
		promoteJobByIDIfUnblockedResult: job2,
		listJobsByRunRepoAttemptResult: []store.Job{
			{
				ID:          f.JobID,
				RunID:       f.RunID,
				RepoID:      f.Job.RepoID,
				RepoBaseRef: "main",
				Attempt:     1,
				Name:        "pre-gate",
				Status:      domaintypes.JobStatusSuccess,
				JobType:     "pre_gate",
				Meta:        withNextIDMeta([]byte(`{}`), 1000),
			},
			{
				ID:          nextJobID,
				RunID:       f.RunID,
				RepoID:      f.Job.RepoID,
				RepoBaseRef: "main",
				Attempt:     1,
				Name:        "mig-0",
				Status:      domaintypes.JobStatusCreated,
				JobType:     "mig",
				Meta:        withNextIDMeta([]byte(`{}`), 2000),
			},
		},
	}

	handler := completeJobHandler(st, nil, nil)

	req := f.completeJobReq(map[string]any{
		"status":       "Success",
		"exit_code":    0,
		"repo_sha_out": "0123456789abcdef0123456789abcdef01234567",
	})

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusNoContent)

	// Verify repo status was NOT updated (jobs still in progress).
	if st.updateRunRepoStatusCalled {
		t.Error("did not expect UpdateRunRepoStatus to be called while jobs still in progress")
	}

	// Verify run status was NOT updated to Finished.
	if st.updateRunStatusCalled {
		t.Error("did not expect UpdateRunStatus to be called while repo not terminal")
	}

	// Verify linked successor was promoted.
	assertCalled(t, "PromoteJobByIDIfUnblocked", st.promoteJobByIDIfUnblockedCalled)
}

// TestCompleteJob_RepoStatusUsesLastJobStatus verifies that when all jobs are
// terminal, run_repos.status is derived from the terminal status of the last job
// (highest next_id), ignoring earlier failures.
func TestCompleteJob_RepoStatusUsesLastJobStatus(t *testing.T) {
	t.Parallel()

	f := newRepoScopedFixture("post_gate", 3000)
	f.Job.Name = "post-gate"

	// Complete the last job (post-gate) successfully. Earlier gate failure exists.
	st := &mockStore{
		getRunResult: store.Run{
			ID:     f.RunID,
			Status: domaintypes.RunStatusStarted,
		},
		getJobResult:        f.Job,
		listJobsByRunResult: []store.Job{f.Job},
		listJobsByRunRepoAttemptResult: []store.Job{
			// Earlier pre-gate failure (healed later).
			{
				ID:          domaintypes.NewJobID(),
				RunID:       f.RunID,
				RepoID:      f.Job.RepoID,
				RepoBaseRef: "main",
				Attempt:     1,
				Name:        "pre-gate",
				Status:      domaintypes.JobStatusFail,
				JobType:     "pre_gate",
				Meta:        withNextIDMeta([]byte(`{}`), 1000),
			},
			{
				ID:          domaintypes.NewJobID(),
				RunID:       f.RunID,
				RepoID:      f.Job.RepoID,
				RepoBaseRef: "main",
				Attempt:     1,
				Name:        "heal-1-0",
				Status:      domaintypes.JobStatusSuccess,
				JobType:     "heal",
				Meta:        withNextIDMeta([]byte(`{}`), 1500),
			},
			{
				ID:          domaintypes.NewJobID(),
				RunID:       f.RunID,
				RepoID:      f.Job.RepoID,
				RepoBaseRef: "main",
				Attempt:     1,
				Name:        "re-gate-1",
				Status:      domaintypes.JobStatusSuccess,
				JobType:     "re_gate",
				Meta:        withNextIDMeta([]byte(`{}`), 1750),
			},
			{
				ID:          domaintypes.NewJobID(),
				RunID:       f.RunID,
				RepoID:      f.Job.RepoID,
				RepoBaseRef: "main",
				Attempt:     1,
				Name:        "mig-0",
				Status:      domaintypes.JobStatusSuccess,
				JobType:     "mig",
				Meta:        withNextIDMeta([]byte(`{}`), 2000),
			},
			// Last job: post-gate succeeded.
			{
				ID:          f.JobID,
				RunID:       f.RunID,
				RepoID:      f.Job.RepoID,
				RepoBaseRef: "main",
				Attempt:     1,
				Name:        "post-gate",
				Status:      domaintypes.JobStatusSuccess,
				JobType:     "post_gate",
				Meta:        withNextIDMeta([]byte(`{}`), 3000),
			},
		},
		countRunReposByStatusResult: []store.CountRunReposByStatusRow{
			{Status: domaintypes.RunRepoStatusSuccess, Count: 1},
		},
	}

	handler := completeJobHandler(st, nil, nil)

	req := f.completeJobReq(map[string]any{
		"status":    "Success",
		"exit_code": 0,
	})

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusNoContent)

	assertCalled(t, "UpdateRunRepoStatus", st.updateRunRepoStatusCalled)
	lastRepoUpdate := st.updateRunRepoStatusParams[len(st.updateRunRepoStatusParams)-1]
	if lastRepoUpdate.Status != domaintypes.RunRepoStatusSuccess {
		t.Errorf("expected repo status Success, got %s", lastRepoUpdate.Status)
	}
}

// TestCompleteJob_MRJobDoesNotAffectRepoStatus verifies that MR jobs (job_type='mr')
// do NOT trigger repo status updates. MR jobs are auxiliary post-run jobs.
func TestCompleteJob_MRJobDoesNotAffectRepoStatus(t *testing.T) {
	t.Parallel()

	f := newRepoScopedFixture("mr", 9000)
	f.Job.Name = "mr-0"

	// MR job (auxiliary, should not affect repo/run status).
	st := newMockStoreForJob(f, withRunStatus(domaintypes.RunStatusFinished))

	handler := completeJobHandler(st, nil, nil)

	req := f.completeJobReq(map[string]any{
		"status":    "Success",
		"exit_code": 0,
	})

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusNoContent)

	// Verify ListJobsByRunRepoAttempt was NOT called for MR jobs.
	if st.listJobsByRunRepoAttemptCalled {
		t.Error("did not expect ListJobsByRunRepoAttempt to be called for MR job")
	}

	// Verify repo status was NOT updated.
	if st.updateRunRepoStatusCalled {
		t.Error("did not expect UpdateRunRepoStatus to be called for MR job")
	}

	// Verify run status was NOT updated (already Finished, MR doesn't change it).
	if st.updateRunStatusCalled {
		t.Error("did not expect UpdateRunStatus to be called for MR job")
	}
}

// TestCompleteJob_MultiRepoRunFinishesWhenAllReposTerminal verifies that runs.status
// becomes Finished only when ALL repos reach terminal state, not just one.
func TestCompleteJob_MultiRepoRunFinishesWhenAllReposTerminal(t *testing.T) {
	t.Parallel()

	f := newRepoScopedFixture("mig", 2000)
	// repoIDB is another repo in the run, still Running (not explicitly used but modeled in countRunReposByStatusResult).
	_ = domaintypes.NewRepoID() // repoIDB - unused but conceptually part of the multi-repo scenario

	// Job for repo A completing (repo B still has work).
	st := newMockStoreForJob(f,
		// Repo A is now terminal (all jobs Success).
		withRepoAttemptJobs([]store.Job{
			{
				ID:          f.JobID,
				RunID:       f.RunID,
				RepoID:      f.Job.RepoID,
				RepoBaseRef: "main",
				Attempt:     1,
				Name:        "mig-0",
				Status:      domaintypes.JobStatusSuccess,
				JobType:     "mig",
				Meta:        withNextIDMeta([]byte(`{}`), 2000),
			},
		}),
		// But repo B is still Running, so run should NOT become Finished.
		withRunRepoStatusCounts([]store.CountRunReposByStatusRow{
			{Status: domaintypes.RunRepoStatusSuccess, Count: 1}, // Repo A
			{Status: domaintypes.RunRepoStatusRunning, Count: 1}, // Repo B still running
		}),
	)

	handler := completeJobHandler(st, nil, nil)

	req := f.completeJobReq(map[string]any{
		"status":    "Success",
		"exit_code": 0,
	})

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusNoContent)

	// Verify repo A status was updated to Success.
	if !st.updateRunRepoStatusCalled {
		t.Fatal("expected UpdateRunRepoStatus to be called for repo A")
	}

	// Verify run status was NOT updated to Finished (repo B still in progress).
	if st.updateRunStatusCalled {
		t.Error("did not expect UpdateRunStatus to be called when not all repos are terminal")
	}
}

// TestCompleteJob_RejectsV0Status verifies that v0 status strings are rejected.
// v1 API uses capitalized status strings: Success, Fail, Cancelled.
func TestCompleteJob_RejectsV0Status(t *testing.T) {
	t.Parallel()

	for _, v0status := range []string{"succeeded", "failed", "canceled"} {
		t.Run(v0status, func(t *testing.T) {
			t.Parallel()
			f := newJobFixture("mig", 1000)
			st := &mockStore{}
			handler := completeJobHandler(st, nil, nil)

			req := f.completeJobReq(map[string]any{"status": v0status})
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			assertStatus(t, rr, http.StatusBadRequest)
			if !strings.Contains(rr.Body.String(), "status") {
				t.Errorf("expected error message to mention status, got: %s", rr.Body.String())
			}
			if st.updateJobCompletionCalled {
				t.Fatal("did not expect UpdateJobCompletion to be called for v0 status")
			}
		})
	}
}

