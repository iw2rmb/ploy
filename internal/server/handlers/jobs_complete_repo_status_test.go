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

// TestCompleteJob_RepoTerminalStatus verifies that run_repos.status is updated
// correctly when the last job in a repo attempt completes.
func TestCompleteJob_RepoTerminalStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		jobStatus        domaintypes.JobStatus
		reqBody          map[string]any
		wantRepoStatus   domaintypes.RunRepoStatus
		wantRunRepoCount domaintypes.RunRepoStatus
		wantRunFinished  bool
	}{
		{
			name:             "success",
			jobStatus:        domaintypes.JobStatusSuccess,
			reqBody:          map[string]any{"status": "Success", "exit_code": 0, "repo_sha_out": "0123456789abcdef0123456789abcdef01234567"},
			wantRepoStatus:   domaintypes.RunRepoStatusSuccess,
			wantRunRepoCount: domaintypes.RunRepoStatusSuccess,
			wantRunFinished:  true,
		},
		{
			name:             "fail",
			jobStatus:        domaintypes.JobStatusFail,
			reqBody:          map[string]any{"status": "Fail", "exit_code": 1},
			wantRepoStatus:   domaintypes.RunRepoStatusFail,
			wantRunRepoCount: domaintypes.RunRepoStatusFail,
			wantRunFinished:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newRepoScopedFixture("mig")

			st := newJobStoreForFixture(f,
				withRepoAttemptJobs([]store.Job{
					{
						ID:          f.JobID,
						RunID:       f.RunID,
						RepoID:      f.Job.RepoID,
						RepoBaseRef: "main",
						Attempt:     1,
						Name:        "mig-0",
						Status:      tt.jobStatus,
						JobType:     "mig",
						Meta:        withNextIDMeta([]byte(`{}`), 2000),
					},
				}),
				withRunRepoStatusCounts([]store.CountRunReposByStatusRow{
					{Status: tt.wantRunRepoCount, Count: 1},
				}),
			)

			handler := completeJobHandler(st, nil, nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, f.completeJobReq(tt.reqBody))

			assertStatus(t, rr, http.StatusNoContent)

			assertCalled(t, "ListJobsByRunRepoAttempt", st.listJobsByRunRepoAttempt.called)
			assertCalled(t, "UpdateRunRepoStatus", st.updateRunRepoStatus.called)
			if len(st.updateRunRepoStatus.calls) == 0 {
				t.Fatal("expected at least one UpdateRunRepoStatus call")
			}
			lastRepoUpdate := st.updateRunRepoStatus.calls[len(st.updateRunRepoStatus.calls)-1]
			if lastRepoUpdate.Status != tt.wantRepoStatus {
				t.Errorf("expected repo status %s, got %s", tt.wantRepoStatus, lastRepoUpdate.Status)
			}

			if tt.wantRunFinished {
				assertCalled(t, "UpdateRunStatus", st.updateRunStatus.called)
				if st.updateRunStatus.params.Status != domaintypes.RunStatusFinished {
					t.Errorf("expected run status Finished, got %s", st.updateRunStatus.params.Status)
				}
			}
		})
	}
}

// TestCompleteJob_RepoNotTerminalWhileJobsInProgress verifies that run_repos.status
// is NOT updated when there are still non-terminal jobs for the repo attempt.
func TestCompleteJob_RepoNotTerminalWhileJobsInProgress(t *testing.T) {
	t.Parallel()

	f := newRepoScopedFixture("pre_gate")
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

	st := newJobStoreForFixture(f,
		withListJobsByRun([]store.Job{f.Job, job2}),
		withPromoteResult(job2),
		withRepoAttemptJobs([]store.Job{
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

	// Verify repo status was NOT updated (jobs still in progress).
	if st.updateRunRepoStatus.called {
		t.Error("did not expect UpdateRunRepoStatus to be called while jobs still in progress")
	}

	// Verify run status was NOT updated to Finished.
	if st.updateRunStatus.called {
		t.Error("did not expect UpdateRunStatus to be called while repo not terminal")
	}

	// Verify linked successor was promoted.
	assertCalled(t, "PromoteJobByIDIfUnblocked", st.promoteJobByIDIfUnblocked.called)
}

// TestCompleteJob_RepoStatusUsesLastJobStatus verifies that when all jobs are
// terminal, run_repos.status is derived from the terminal status of the last job
// (highest next_id), ignoring earlier failures.
func TestCompleteJob_RepoStatusUsesLastJobStatus(t *testing.T) {
	t.Parallel()

	f := newRepoScopedFixture("post_gate")
	f.Job.Name = "post-gate"

	// Complete the last job (post-gate) successfully. Earlier gate failure exists.
	st := newJobStoreForFixture(f,
		withRepoAttemptJobs([]store.Job{
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
		}),
		withRunRepoStatusCounts([]store.CountRunReposByStatusRow{
			{Status: domaintypes.RunRepoStatusSuccess, Count: 1},
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

	assertCalled(t, "UpdateRunRepoStatus", st.updateRunRepoStatus.called)
	lastRepoUpdate := st.updateRunRepoStatus.calls[len(st.updateRunRepoStatus.calls)-1]
	if lastRepoUpdate.Status != domaintypes.RunRepoStatusSuccess {
		t.Errorf("expected repo status Success, got %s", lastRepoUpdate.Status)
	}
}

// TestCompleteJob_MRJobDoesNotAffectRepoStatus verifies that MR jobs (job_type='mr')
// do NOT trigger repo status updates. MR jobs are auxiliary post-run jobs.
func TestCompleteJob_MRJobDoesNotAffectRepoStatus(t *testing.T) {
	t.Parallel()

	f := newRepoScopedFixture("mr")
	f.Job.Name = "mr-0"

	// MR job (auxiliary, should not affect repo/run status).
	st := newJobStoreForFixture(f, withRunStatus(domaintypes.RunStatusFinished))

	handler := completeJobHandler(st, nil, nil)

	req := f.completeJobReq(map[string]any{
		"status":    "Success",
		"exit_code": 0,
	})

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusNoContent)

	// Verify ListJobsByRunRepoAttempt was NOT called for MR jobs.
	if st.listJobsByRunRepoAttempt.called {
		t.Error("did not expect ListJobsByRunRepoAttempt to be called for MR job")
	}

	// Verify repo status was NOT updated.
	if st.updateRunRepoStatus.called {
		t.Error("did not expect UpdateRunRepoStatus to be called for MR job")
	}

	// Verify run status was NOT updated (already Finished, MR doesn't change it).
	if st.updateRunStatus.called {
		t.Error("did not expect UpdateRunStatus to be called for MR job")
	}
}

// TestCompleteJob_MultiRepoRunFinishesWhenAllReposTerminal verifies that runs.status
// becomes Finished only when ALL repos reach terminal state, not just one.
func TestCompleteJob_MultiRepoRunFinishesWhenAllReposTerminal(t *testing.T) {
	t.Parallel()

	f := newRepoScopedFixture("mig")
	// repoIDB is another repo in the run, still Running (not explicitly used but modeled in countRunReposByStatus.val).
	_ = domaintypes.NewRepoID() // repoIDB - unused but conceptually part of the multi-repo scenario

	// Job for repo A completing (repo B still has work).
	st := newJobStoreForFixture(f,
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
	if !st.updateRunRepoStatus.called {
		t.Fatal("expected UpdateRunRepoStatus to be called for repo A")
	}

	// Verify run status was NOT updated to Finished (repo B still in progress).
	if st.updateRunStatus.called {
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
			f := newJobFixture("mig")
			st := &jobStore{}
			handler := completeJobHandler(st, nil, nil)

			req := f.completeJobReq(map[string]any{"status": v0status})
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			assertStatus(t, rr, http.StatusBadRequest)
			if !strings.Contains(rr.Body.String(), "status") {
				t.Errorf("expected error message to mention status, got: %s", rr.Body.String())
			}
			if st.updateJobCompletion.called {
				t.Fatal("did not expect UpdateJobCompletion to be called for v0 status")
			}
		})
	}
}

