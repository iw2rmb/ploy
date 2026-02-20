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

	f := newJobFixture("mod", 2000)
	f.Job.RepoID = domaintypes.NewModRepoID()
	f.Job.RepoBaseRef = "main"
	f.Job.Attempt = 1

	// Single job per repo, completing it should mark repo as terminal.
	st := &mockStore{
		getRunResult: store.Run{
			ID:     f.RunID,
			Status: store.RunStatusStarted,
		},
		getJobResult:        f.Job,
		listJobsByRunResult: []store.Job{f.Job},
		// All jobs (1 total) are now Success after completion.
		listJobsByRunRepoAttemptResult: []store.Job{
			{
				ID:          f.JobID,
				RunID:       f.RunID,
				RepoID:      f.Job.RepoID,
				RepoBaseRef: "main",
				Attempt:     1,
				Name:        "mod-0",
				Status:      store.JobStatusSuccess,
				ModType:     "mod",
				StepIndex:   2000,
			},
		},
		// All repos terminal (1 Success), so run becomes Finished.
		countRunReposByStatusResult: []store.CountRunReposByStatusRow{
			{Status: store.RunRepoStatusSuccess, Count: 1},
		},
	}

	handler := completeJobHandler(st, nil)

	req := f.completeJobReq(map[string]any{
		"status":    "Success",
		"exit_code": 0,
	})

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify ListJobsByRunRepoAttempt was called to check repo terminal state.
	if !st.listJobsByRunRepoAttemptCalled {
		t.Fatal("expected ListJobsByRunRepoAttempt to be called")
	}
	if st.listJobsByRunRepoAttemptParams.RunID != f.RunID {
		t.Errorf("expected run_id %s, got %s", f.RunID, st.listJobsByRunRepoAttemptParams.RunID)
	}
	if st.listJobsByRunRepoAttemptParams.RepoID != f.Job.RepoID {
		t.Errorf("expected repo_id %s, got %s", f.Job.RepoID, st.listJobsByRunRepoAttemptParams.RepoID)
	}

	// Verify UpdateRunRepoStatus was called to update repo to Success.
	if !st.updateRunRepoStatusCalled {
		t.Fatal("expected UpdateRunRepoStatus to be called")
	}
	if len(st.updateRunRepoStatusParams) == 0 {
		t.Fatal("expected at least one UpdateRunRepoStatus call")
	}
	lastRepoUpdate := st.updateRunRepoStatusParams[len(st.updateRunRepoStatusParams)-1]
	if lastRepoUpdate.Status != store.RunRepoStatusSuccess {
		t.Errorf("expected repo status Success, got %s", lastRepoUpdate.Status)
	}

	// Verify UpdateRunStatus was called to set run to Finished.
	if !st.updateRunStatusCalled {
		t.Fatal("expected UpdateRunStatus to be called")
	}
	if st.updateRunStatusParams.Status != store.RunStatusFinished {
		t.Errorf("expected run status Finished, got %s", st.updateRunStatusParams.Status)
	}
}

// TestCompleteJob_RepoStatusFail verifies that run_repos.status is updated
// to Fail when a job in the repo attempt fails.
func TestCompleteJob_RepoStatusFail(t *testing.T) {
	t.Parallel()

	f := newJobFixture("mod", 2000)
	f.Job.RepoID = domaintypes.NewModRepoID()
	f.Job.RepoBaseRef = "main"
	f.Job.Attempt = 1

	// Job that will fail.
	st := &mockStore{
		getRunResult: store.Run{
			ID:     f.RunID,
			Status: store.RunStatusStarted,
		},
		getJobResult:        f.Job,
		listJobsByRunResult: []store.Job{f.Job},
		listJobsByRunRepoAttemptResult: []store.Job{
			{
				ID:          f.JobID,
				RunID:       f.RunID,
				RepoID:      f.Job.RepoID,
				RepoBaseRef: "main",
				Attempt:     1,
				Name:        "mod-0",
				Status:      store.JobStatusFail,
				ModType:     "mod",
				StepIndex:   2000,
			},
		},
		// All repos terminal.
		countRunReposByStatusResult: []store.CountRunReposByStatusRow{
			{Status: store.RunRepoStatusFail, Count: 1},
		},
	}

	handler := completeJobHandler(st, nil)

	req := f.completeJobReq(map[string]any{
		"status":    "Fail",
		"exit_code": 1,
	})

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify repo status was updated to Fail.
	if !st.updateRunRepoStatusCalled {
		t.Fatal("expected UpdateRunRepoStatus to be called")
	}
	if len(st.updateRunRepoStatusParams) == 0 {
		t.Fatal("expected at least one UpdateRunRepoStatus call")
	}
	lastRepoUpdate := st.updateRunRepoStatusParams[len(st.updateRunRepoStatusParams)-1]
	if lastRepoUpdate.Status != store.RunRepoStatusFail {
		t.Errorf("expected repo status Fail, got %s", lastRepoUpdate.Status)
	}
}

// TestCompleteJob_RepoNotTerminalWhileJobsInProgress verifies that run_repos.status
// is NOT updated when there are still non-terminal jobs for the repo attempt.
func TestCompleteJob_RepoNotTerminalWhileJobsInProgress(t *testing.T) {
	t.Parallel()

	f := newJobFixture("pre_gate", 1000)
	f.Job.RepoID = domaintypes.NewModRepoID()
	f.Job.RepoBaseRef = "main"
	f.Job.Attempt = 1
	f.Job.Name = "pre-gate"

	// Two jobs: first completes, second is still Created.
	nextJobID := domaintypes.NewJobID()
	job2 := store.Job{
		ID:          nextJobID,
		RunID:       f.RunID,
		RepoID:      f.Job.RepoID,
		RepoBaseRef: "main",
		Attempt:     1,
		Name:        "mod-0",
		Status:      store.JobStatusCreated,
		ModType:     "mod",
		StepIndex:   2000,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     f.RunID,
			Status: store.RunStatusStarted,
		},
		getJobResult:          f.Job,
		listJobsByRunResult:   []store.Job{f.Job, job2},
		scheduleNextJobResult: job2,
		listJobsByRunRepoAttemptResult: []store.Job{
			{
				ID:          f.JobID,
				RunID:       f.RunID,
				RepoID:      f.Job.RepoID,
				RepoBaseRef: "main",
				Attempt:     1,
				Name:        "pre-gate",
				Status:      store.JobStatusSuccess,
				ModType:     "pre_gate",
				StepIndex:   1000,
			},
			{
				ID:          nextJobID,
				RunID:       f.RunID,
				RepoID:      f.Job.RepoID,
				RepoBaseRef: "main",
				Attempt:     1,
				Name:        "mod-0",
				Status:      store.JobStatusCreated,
				ModType:     "mod",
				StepIndex:   2000,
			},
		},
	}

	handler := completeJobHandler(st, nil)

	req := f.completeJobReq(map[string]any{
		"status":    "Success",
		"exit_code": 0,
	})

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify repo status was NOT updated (jobs still in progress).
	if st.updateRunRepoStatusCalled {
		t.Error("did not expect UpdateRunRepoStatus to be called while jobs still in progress")
	}

	// Verify run status was NOT updated to Finished.
	if st.updateRunStatusCalled {
		t.Error("did not expect UpdateRunStatus to be called while repo not terminal")
	}

	// Verify next job was scheduled.
	if !st.scheduleNextJobCalled {
		t.Fatal("expected ScheduleNextJob to be called")
	}
}

// TestCompleteJob_RepoStatusUsesLastJobStatus verifies that when all jobs are
// terminal, run_repos.status is derived from the terminal status of the last job
// (highest step_index), ignoring earlier failures.
func TestCompleteJob_RepoStatusUsesLastJobStatus(t *testing.T) {
	t.Parallel()

	f := newJobFixture("post_gate", 3000)
	f.Job.RepoID = domaintypes.NewModRepoID()
	f.Job.RepoBaseRef = "main"
	f.Job.Attempt = 1
	f.Job.Name = "post-gate"

	// Complete the last job (post-gate) successfully. Earlier gate failure exists.
	st := &mockStore{
		getRunResult: store.Run{
			ID:     f.RunID,
			Status: store.RunStatusStarted,
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
				Status:      store.JobStatusFail,
				ModType:     "pre_gate",
				StepIndex:   1000,
			},
			{
				ID:          domaintypes.NewJobID(),
				RunID:       f.RunID,
				RepoID:      f.Job.RepoID,
				RepoBaseRef: "main",
				Attempt:     1,
				Name:        "heal-1-0",
				Status:      store.JobStatusSuccess,
				ModType:     "heal",
				StepIndex:   1500,
			},
			{
				ID:          domaintypes.NewJobID(),
				RunID:       f.RunID,
				RepoID:      f.Job.RepoID,
				RepoBaseRef: "main",
				Attempt:     1,
				Name:        "re-gate-1",
				Status:      store.JobStatusSuccess,
				ModType:     "re_gate",
				StepIndex:   1750,
			},
			{
				ID:          domaintypes.NewJobID(),
				RunID:       f.RunID,
				RepoID:      f.Job.RepoID,
				RepoBaseRef: "main",
				Attempt:     1,
				Name:        "mod-0",
				Status:      store.JobStatusSuccess,
				ModType:     "mod",
				StepIndex:   2000,
			},
			// Last job: post-gate succeeded.
			{
				ID:          f.JobID,
				RunID:       f.RunID,
				RepoID:      f.Job.RepoID,
				RepoBaseRef: "main",
				Attempt:     1,
				Name:        "post-gate",
				Status:      store.JobStatusSuccess,
				ModType:     "post_gate",
				StepIndex:   3000,
			},
		},
		countRunReposByStatusResult: []store.CountRunReposByStatusRow{
			{Status: store.RunRepoStatusSuccess, Count: 1},
		},
	}

	handler := completeJobHandler(st, nil)

	req := f.completeJobReq(map[string]any{
		"status":    "Success",
		"exit_code": 0,
	})

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}

	if !st.updateRunRepoStatusCalled {
		t.Fatal("expected UpdateRunRepoStatus to be called")
	}
	lastRepoUpdate := st.updateRunRepoStatusParams[len(st.updateRunRepoStatusParams)-1]
	if lastRepoUpdate.Status != store.RunRepoStatusSuccess {
		t.Errorf("expected repo status Success, got %s", lastRepoUpdate.Status)
	}
}

// TestCompleteJob_MRJobDoesNotAffectRepoStatus verifies that MR jobs (mod_type='mr')
// do NOT trigger repo status updates. MR jobs are auxiliary post-run jobs.
func TestCompleteJob_MRJobDoesNotAffectRepoStatus(t *testing.T) {
	t.Parallel()

	f := newJobFixture("mr", 9000)
	f.Job.RepoID = domaintypes.NewModRepoID()
	f.Job.RepoBaseRef = "main"
	f.Job.Attempt = 1
	f.Job.Name = "mr-0"

	// MR job (auxiliary, should not affect repo/run status).
	st := &mockStore{
		getRunResult: store.Run{
			ID:     f.RunID,
			Status: store.RunStatusFinished, // MR jobs run after run is Finished.
		},
		getJobResult:        f.Job,
		listJobsByRunResult: []store.Job{f.Job},
	}

	handler := completeJobHandler(st, nil)

	req := f.completeJobReq(map[string]any{
		"status":    "Success",
		"exit_code": 0,
	})

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}

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

	f := newJobFixture("mod", 2000)
	f.Job.RepoID = domaintypes.NewModRepoID()
	f.Job.RepoBaseRef = "main"
	f.Job.Attempt = 1
	// repoIDB is another repo in the run, still Running (not explicitly used but modeled in countRunReposByStatusResult).
	_ = domaintypes.NewModRepoID() // repoIDB - unused but conceptually part of the multi-repo scenario

	// Job for repo A completing (repo B still has work).
	st := &mockStore{
		getRunResult: store.Run{
			ID:     f.RunID,
			Status: store.RunStatusStarted,
		},
		getJobResult:        f.Job,
		listJobsByRunResult: []store.Job{f.Job},
		// Repo A is now terminal (all jobs Success).
		listJobsByRunRepoAttemptResult: []store.Job{
			{
				ID:          f.JobID,
				RunID:       f.RunID,
				RepoID:      f.Job.RepoID,
				RepoBaseRef: "main",
				Attempt:     1,
				Name:        "mod-0",
				Status:      store.JobStatusSuccess,
				ModType:     "mod",
				StepIndex:   2000,
			},
		},
		// But repo B is still Running, so run should NOT become Finished.
		countRunReposByStatusResult: []store.CountRunReposByStatusRow{
			{Status: store.RunRepoStatusSuccess, Count: 1}, // Repo A
			{Status: store.RunRepoStatusRunning, Count: 1}, // Repo B still running
		},
	}

	handler := completeJobHandler(st, nil)

	req := f.completeJobReq(map[string]any{
		"status":    "Success",
		"exit_code": 0,
	})

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify repo A status was updated to Success.
	if !st.updateRunRepoStatusCalled {
		t.Fatal("expected UpdateRunRepoStatus to be called for repo A")
	}

	// Verify run status was NOT updated to Finished (repo B still in progress).
	if st.updateRunStatusCalled {
		t.Error("did not expect UpdateRunStatus to be called when not all repos are terminal")
	}
}

// ===== v0 Status String Rejection Tests =====
// v1 API uses capitalized status strings: Success, Fail, Cancelled.
// v0 status strings (succeeded, failed, canceled) must be rejected with 400.

// TestCompleteJob_RejectsV0StatusSucceeded verifies that v0 "succeeded" is rejected.
func TestCompleteJob_RejectsV0StatusSucceeded(t *testing.T) {
	t.Parallel()

	f := newJobFixture("mod", 1000)
	st := &mockStore{}
	handler := completeJobHandler(st, nil)

	// v0 status "succeeded" should be rejected in favor of v1 "Success".
	req := f.completeJobReq(map[string]any{"status": "succeeded"})

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for v0 'succeeded', got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "status") {
		t.Errorf("expected error message to mention status, got: %s", rr.Body.String())
	}
	if st.updateJobCompletionCalled {
		t.Fatal("did not expect UpdateJobCompletion to be called for v0 status")
	}
}

// TestCompleteJob_RejectsV0StatusFailed verifies that v0 "failed" is rejected.
func TestCompleteJob_RejectsV0StatusFailed(t *testing.T) {
	t.Parallel()

	f := newJobFixture("mod", 1000)
	st := &mockStore{}
	handler := completeJobHandler(st, nil)

	// v0 status "failed" should be rejected in favor of v1 "Fail".
	req := f.completeJobReq(map[string]any{"status": "failed"})

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for v0 'failed', got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "status") {
		t.Errorf("expected error message to mention status, got: %s", rr.Body.String())
	}
	if st.updateJobCompletionCalled {
		t.Fatal("did not expect UpdateJobCompletion to be called for v0 status")
	}
}

// TestCompleteJob_RejectsV0StatusCanceled verifies that v0 "canceled" (single 'l') is rejected.
func TestCompleteJob_RejectsV0StatusCanceled(t *testing.T) {
	t.Parallel()

	f := newJobFixture("mod", 1000)
	st := &mockStore{}
	handler := completeJobHandler(st, nil)

	// v0 status "canceled" (American spelling) should be rejected in favor of v1 "Cancelled".
	req := f.completeJobReq(map[string]any{"status": "canceled"})

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for v0 'canceled', got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "status") {
		t.Errorf("expected error message to mention status, got: %s", rr.Body.String())
	}
	if st.updateJobCompletionCalled {
		t.Fatal("did not expect UpdateJobCompletion to be called for v0 status")
	}
}
