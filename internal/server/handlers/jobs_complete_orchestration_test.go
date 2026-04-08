package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server"
	"github.com/iw2rmb/ploy/internal/store"
)

type transientGetRunStore struct {
	*jobStore
	failCount int
	err       error
	calls     int
}

func (s *transientGetRunStore) GetRun(ctx context.Context, id domaintypes.RunID) (store.Run, error) {
	s.calls++
	if s.calls <= s.failCount {
		s.jobStore.getRun.called = true
		s.jobStore.getRun.params = id.String()
		return store.Run{}, s.err
	}
	return s.jobStore.GetRun(ctx, id)
}

// ===== Side Effects & Orchestration Tests =====
// These tests verify orchestration behavior when jobs complete:
// - Event publishing to SSE hub
// - Job scheduling for next step
// - Cascade failure handling (canceling remaining jobs)

// TestCompleteJob_PublishesEvents verifies that completing a job publishes events
// when the run transitions to terminal state.
func TestCompleteJob_PublishesEvents(t *testing.T) {
	t.Parallel()

	f := newRepoScopedFixture("mig")
	now := time.Now()

	st := newJobStoreForFixture(f,
		withGetRunCreatedAt(now),
		// v1: repo-scoped progression requires all non-MR jobs to be terminal and
		// derives run_repos.status from the last job.
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
				Meta:        withNextIDMeta([]byte(`{}`), 1000),
			},
		}),
		// All repos terminal triggers run completion.
		withRunRepoStatusCounts([]store.CountRunReposByStatusRow{
			{Status: domaintypes.RunRepoStatusSuccess, Count: 1},
		}),
	)

	eventsService, _ := server.NewEventsService(server.EventsOptions{
		BufferSize:  10,
		HistorySize: 100,
	})
	handler := completeJobHandler(st, eventsService, nil)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
		"status":    "Success",
		"exit_code": 0,
		"stats":     map[string]any{"duration_ms": 500},
	}))

	assertStatus(t, rr, http.StatusNoContent)

	// Verify events were published to the hub.
	snapshot := eventsService.Hub().Snapshot(f.RunID)
	if len(snapshot) < 2 {
		t.Fatalf("expected at least 2 events (run + done), got %d", len(snapshot))
	}

	// Verify we have both a run summary event and a done event.
	foundRunEvent := false
	foundDoneEvent := false
	for _, evt := range snapshot {
		if evt.Type == domaintypes.SSEEventRun {
			foundRunEvent = true
			if !strings.Contains(string(evt.Data), "succeeded") {
				t.Errorf("expected run event data to contain 'succeeded', got: %s", string(evt.Data))
			}
		}
		if evt.Type == domaintypes.SSEEventDone {
			foundDoneEvent = true
		}
	}
	if !foundRunEvent {
		t.Error("expected to find a 'run' event in the snapshot")
	}
	if !foundDoneEvent {
		t.Error("expected to find a 'done' event in the snapshot")
	}
}

// TestCompleteJob_PromotesLinkedNextJob verifies that a successful completion
// promotes the linked successor.
func TestCompleteJob_PromotesLinkedNextJob(t *testing.T) {
	t.Parallel()

	f := newJobFixture("")
	nextJobID := domaintypes.NewJobID()
	f.Job.NextID = &nextJobID

	nextJob := store.Job{
		ID:     nextJobID,
		RunID:  f.RunID,
		Status: domaintypes.JobStatusCreated,
	}

	st := newJobStoreForFixture(f,
		withListJobsByRun([]store.Job{f.Job, nextJob}),
		withPromoteResult(nextJob),
	)

	handler := completeJobHandler(st, nil, nil)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
		"status":       "Success",
		"repo_sha_out": "0123456789abcdef0123456789abcdef01234567",
	}))

	assertStatus(t, rr, http.StatusNoContent)

	// Verify PromoteJobByIDIfUnblocked was called with the linked successor.
	assertCalled(t, "PromoteJobByIDIfUnblocked", st.promoteJobByIDIfUnblocked.called)
	if st.promoteJobByIDIfUnblocked.params != nextJobID {
		t.Fatalf("expected PromoteJobByIDIfUnblocked(%s), got %s", nextJobID, st.promoteJobByIDIfUnblocked.params)
	}
}

func TestCompleteJob_SBOMAndHookSuccessPromoteLinkedNextJob(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		jobType domaintypes.JobType
	}{
		{name: "sbom", jobType: domaintypes.JobTypeSBOM},
		{name: "hook", jobType: domaintypes.JobTypeHook},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f := newJobFixture(tc.jobType)
			nextJobID := domaintypes.NewJobID()
			f.Job.NextID = &nextJobID

			nextJob := store.Job{
				ID:     nextJobID,
				RunID:  f.RunID,
				Status: domaintypes.JobStatusCreated,
			}

			st := newJobStoreForFixture(f,
				withListJobsByRun([]store.Job{f.Job, nextJob}),
				withPromoteResult(nextJob),
			)

			handler := completeJobHandler(st, nil, nil)

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
				"status":       "Success",
				"repo_sha_out": "0123456789abcdef0123456789abcdef01234567",
			}))

			assertStatus(t, rr, http.StatusNoContent)
			assertCalled(t, "PromoteJobByIDIfUnblocked", st.promoteJobByIDIfUnblocked.called)
			if st.promoteJobByIDIfUnblocked.params != nextJobID {
				t.Fatalf("expected PromoteJobByIDIfUnblocked(%s), got %s", nextJobID, st.promoteJobByIDIfUnblocked.params)
			}
			assertNotCalled(t, "UpdateRunStatsMRURL", st.updateRunStatsMRURL.called)
		})
	}
}

func TestCompleteJob_PostAndReGatePreludeSuccessPromoteLinkedNextJob(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		jobType domaintypes.JobType
		jobName string
		spec    []byte
	}{
		{
			name:    "post_gate_sbom",
			jobType: domaintypes.JobTypeSBOM,
			jobName: "post-gate-sbom",
			spec:    []byte(`{"steps":[{"image":"mig:latest"}]}`),
		},
		{name: "regate_hook", jobType: domaintypes.JobTypeHook, jobName: "re-gate-1-hook-000"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f := newJobFixture(tc.jobType)
			f.Job.Name = tc.jobName
			nextJobID := domaintypes.NewJobID()
			f.Job.NextID = &nextJobID

			nextJob := store.Job{
				ID:     nextJobID,
				RunID:  f.RunID,
				Status: domaintypes.JobStatusCreated,
			}

			opts := []func(*jobStore){
				withListJobsByRun([]store.Job{f.Job, nextJob}),
				withPromoteResult(nextJob),
			}
			if len(tc.spec) > 0 {
				opts = append(opts, withSpec(domaintypes.NewSpecID(), tc.spec))
			}
			st := newJobStoreForFixture(f, opts...)

			handler := completeJobHandler(st, nil, nil)

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
				"status":       "Success",
				"repo_sha_out": "0123456789abcdef0123456789abcdef01234567",
			}))

			assertStatus(t, rr, http.StatusNoContent)
			assertCalled(t, "PromoteJobByIDIfUnblocked", st.promoteJobByIDIfUnblocked.called)
			if st.promoteJobByIDIfUnblocked.params != nextJobID {
				t.Fatalf("expected PromoteJobByIDIfUnblocked(%s), got %s", nextJobID, st.promoteJobByIDIfUnblocked.params)
			}
		})
	}
}

// TestCompleteJob_FailedJobDoesNotScheduleNext verifies that a failed job
// does not trigger scheduling of the next job.
func TestCompleteJob_FailedJobDoesNotScheduleNext(t *testing.T) {
	t.Parallel()

	f := newJobFixture("")
	st := newJobStoreForFixture(f)

	handler := completeJobHandler(st, nil, nil)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{"status": "Fail"}))

	assertStatus(t, rr, http.StatusNoContent)

	// Verify successor promotion was NOT called for failed jobs.
	if st.promoteJobByIDIfUnblocked.called {
		t.Fatal("did not expect PromoteJobByIDIfUnblocked to be called for failed job")
	}
}

// TestCompleteJob_NonGateFailureCancelsRemainingJobs verifies that when a non-gate
// job fails, remaining non-terminal jobs are canceled so the run can
// transition to a terminal state instead of leaving jobs stranded.
func TestCompleteJob_NonGateFailureCancelsRemainingJobs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		jobType domaintypes.JobType
	}{
		{name: "mig", jobType: domaintypes.JobTypeMig},
		{name: "sbom", jobType: domaintypes.JobTypeSBOM},
		{name: "hook", jobType: domaintypes.JobTypeHook},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f := newRepoScopedFixture(tc.jobType)
			repoID := f.Job.RepoID
			postJobID := domaintypes.NewJobID()
			f.Job.Meta = []byte(`{}`)

			// Jobs: pre-gate succeeded, non-gate failed, post-gate created.
			jobs := []store.Job{
				{
					ID:          domaintypes.NewJobID(),
					RunID:       f.RunID,
					RepoID:      repoID,
					RepoBaseRef: "main",
					Attempt:     1,
					NodeID:      &f.NodeID,
					Status:      domaintypes.JobStatusSuccess,
					JobType:     domaintypes.JobTypePreGate,
					Meta:        withNextIDMeta([]byte(`{}`), 1000),
				},
				f.Job,
				{
					ID:          postJobID,
					RunID:       f.RunID,
					RepoID:      repoID,
					RepoBaseRef: "main",
					Attempt:     1,
					Status:      domaintypes.JobStatusCreated,
					JobType:     domaintypes.JobTypePostGate,
					Meta:        withNextIDMeta([]byte(`{}`), 3000),
				},
			}
			f.Job.NextID = &postJobID
			jobs[0].NextID = &f.Job.ID
			jobs[1].NextID = &postJobID

			st := newJobStoreForFixture(f,
				withListJobsByRun(jobs),
				withRepoAttemptJobs(jobs),
			)

			handler := completeJobHandler(st, nil, nil)

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
				"status":    "Fail",
				"exit_code": 1,
			}))

			assertStatus(t, rr, http.StatusNoContent)

			assertCalled(t, "UpdateJobCompletion", st.updateJobCompletion.called)
			if st.updateJobCompletion.params.ID != jobs[1].ID {
				t.Fatalf("expected UpdateJobCompletion for %s job, got %v", tc.jobType, st.updateJobCompletion.params.ID)
			}
			if !st.updateJobStatus.called {
				t.Fatal("expected UpdateJobStatus to be called to cancel remaining jobs")
			}
			foundPostCancel := false
			for _, call := range st.updateJobStatus.calls {
				if call.ID == jobs[2].ID {
					foundPostCancel = true
					if call.Status != domaintypes.JobStatusCancelled {
						t.Fatalf("expected post-gate job to be canceled, got status %s", call.Status)
					}
				}
			}
			if !foundPostCancel {
				t.Fatal("expected post-gate job to be canceled")
			}
			if len(st.createJob.calls) > 0 {
				t.Fatalf("did not expect healing job insertion for %s failure", tc.jobType)
			}
		})
	}
}

func TestCompleteJob_PostAndReGatePreludeFailureCancelsRemainingJobs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		jobType       domaintypes.JobType
		jobName       string
		successorName string
		successorType domaintypes.JobType
	}{
		{
			name:          "post_gate_sbom_failure",
			jobType:       domaintypes.JobTypeSBOM,
			jobName:       "post-gate-sbom",
			successorName: "post-gate",
			successorType: domaintypes.JobTypePostGate,
		},
		{
			name:          "regate_hook_failure",
			jobType:       domaintypes.JobTypeHook,
			jobName:       "re-gate-1-hook-000",
			successorName: "re-gate-1",
			successorType: domaintypes.JobTypeReGate,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f := newRepoScopedFixture(tc.jobType)
			f.Job.Name = tc.jobName
			successorID := domaintypes.NewJobID()
			f.Job.NextID = &successorID

			successor := store.Job{
				ID:          successorID,
				RunID:       f.RunID,
				RepoID:      f.Job.RepoID,
				RepoBaseRef: "main",
				Attempt:     1,
				Name:        tc.successorName,
				Status:      domaintypes.JobStatusCreated,
				JobType:     tc.successorType,
				Meta:        withNextIDMeta([]byte(`{}`), 3000),
			}
			completed := f.Job
			completed.Status = domaintypes.JobStatusFail

			st := newJobStoreForFixture(f,
				withListJobsByRun([]store.Job{f.Job, successor}),
				withRepoAttemptJobs([]store.Job{completed, successor}),
			)

			handler := completeJobHandler(st, nil, nil)

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
				"status":    "Fail",
				"exit_code": 1,
			}))

			assertStatus(t, rr, http.StatusNoContent)
			if !st.updateJobStatus.called {
				t.Fatal("expected UpdateJobStatus to cancel successor")
			}
			cancelledSuccessor := false
			for _, call := range st.updateJobStatus.calls {
				if call.ID == successorID && call.Status == domaintypes.JobStatusCancelled {
					cancelledSuccessor = true
					break
				}
			}
			if !cancelledSuccessor {
				t.Fatalf("expected successor %s to be cancelled", successorID)
			}
			if len(st.createJob.calls) > 0 {
				t.Fatalf("did not expect healing insertion for %s failure", tc.jobName)
			}
		})
	}
}

// TestCompleteJob_CanceledStatus verifies that canceled status is accepted.
func TestCompleteJob_CanceledStatus(t *testing.T) {
	t.Parallel()

	f := newJobFixture("")
	st := newJobStoreForFixture(f)

	handler := completeJobHandler(st, nil, nil)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{"status": "Cancelled"}))

	assertStatus(t, rr, http.StatusNoContent)
	assertCalled(t, "UpdateJobCompletion", st.updateJobCompletion.called)
	if st.updateJobCompletion.params.Status != domaintypes.JobStatusCancelled {
		t.Fatalf("expected job status canceled, got %s", st.updateJobCompletion.params.Status)
	}
}

func TestCompleteJob_Success_DoesNotUseStepIndexScheduler(t *testing.T) {
	t.Parallel()

	f := newJobFixture("")
	nextJob := store.Job{
		ID:          domaintypes.NewJobID(),
		RunID:       f.RunID,
		RepoID:      domaintypes.NewRepoID(),
		RepoBaseRef: "main",
		Attempt:     1,
		Status:      domaintypes.JobStatusCreated,
		JobType:     domaintypes.JobTypeMig,
	}
	f.Job.RepoID = nextJob.RepoID
	f.Job.RepoBaseRef = "main"
	f.Job.Attempt = 1
	f.Job.NextID = &nextJob.ID

	st := newJobStoreForFixture(f,
		withListJobsByRun([]store.Job{f.Job, nextJob}),
		withRepoAttemptJobs([]store.Job{f.Job, nextJob}),
		withPromoteResult(nextJob),
	)

	handler := completeJobHandler(st, nil, nil)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
		"status":       "Success",
		"repo_sha_out": "0123456789abcdef0123456789abcdef01234567",
	}))

	assertStatus(t, rr, http.StatusNoContent)
	if st.scheduleNextJob.called {
		t.Fatal("expected success completion to avoid next_id scheduler path")
	}
	if !st.promoteJobByIDIfUnblocked.called {
		t.Fatal("expected linked successor promotion to be called")
	}
}

func TestCompleteJob_GateFailure_HealingInsertionRewiresNextChain(t *testing.T) {
	t.Parallel()

	gf := newGateFailureFixture(t, []byte(`{"kind":"gate","gate":{"static_checks":[{"tool":"maven","passed":false}],"recovery":{"loop_kind":"healing","error_kind":"infra","strategy_id":"infra-default"}}}`))
	st := gf.Store

	handler := completeJobHandler(st, nil, nil)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, gf.completeJobReq(map[string]any{
		"status":    "Fail",
		"exit_code": 1,
	}))

	assertStatus(t, rr, http.StatusNoContent)
	if len(st.createJob.calls) == 0 {
		t.Fatal("expected healing insertion to create follow-up jobs")
	}
	if len(st.createJob.calls) != 3 {
		t.Fatalf("expected 3 healing jobs, got %d", len(st.createJob.calls))
	}
	byName := createJobsByName(st.createJob.calls)
	reGate := byName["re-gate-1"]
	heal := byName["heal-1-0"]
	if reGate.Name != "re-gate-1" || heal.Name != "heal-1-0" {
		t.Fatalf("expected re-gate-1 and heal-1-0 jobs, got %+v", st.createJob.calls)
	}
	if heal.NextID == nil || *heal.NextID != reGate.ID {
		t.Fatalf("expected heal.NextID to point to re-gate job")
	}
	var finalSBOM *store.CreateJobParams
	for i := range st.createJob.calls {
		if st.createJob.calls[i].JobType == domaintypes.JobTypeSBOM {
			finalSBOM = &st.createJob.calls[i]
			break
		}
	}
	if finalSBOM == nil {
		t.Fatal("expected final sbom job")
	}
	if reGate.NextID == nil || *reGate.NextID != finalSBOM.ID {
		t.Fatalf("expected re-gate.NextID to point to final sbom")
	}
	if finalSBOM.NextID == nil || *finalSBOM.NextID != gf.Successor.ID {
		t.Fatalf("expected final sbom.NextID to preserve old successor %s", gf.Successor.ID)
	}
	if len(st.updateJobNextIDParams) != 1 {
		t.Fatalf("expected one next_id rewiring update, got %d", len(st.updateJobNextIDParams))
	}
	if st.updateJobNextIDParams[0].ID != gf.Job.ID || st.updateJobNextIDParams[0].NextID == nil || *st.updateJobNextIDParams[0].NextID != heal.ID {
		t.Fatalf("expected failed job rewired to heal")
	}
}

func TestCompleteJob_GateFailure_HealingInsertionRetriesRunLookup(t *testing.T) {
	t.Parallel()

	gf := newGateFailureFixture(t, []byte(`{"kind":"gate","gate":{"static_checks":[{"tool":"maven","passed":false}],"recovery":{"loop_kind":"healing","error_kind":"infra","strategy_id":"infra-default"}}}`))
	st := &transientGetRunStore{
		jobStore:  gf.Store,
		failCount: 1,
		err:       errors.New("transient get run failure"),
	}

	handler := completeJobHandler(st, nil, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, gf.completeJobReq(map[string]any{
		"status":    "Fail",
		"exit_code": 1,
	}))

	assertStatus(t, rr, http.StatusNoContent)
	if st.calls < 2 {
		t.Fatalf("expected run lookup retry after transient failure, calls=%d", st.calls)
	}
	if len(gf.Store.createJob.calls) != 3 {
		t.Fatalf("expected healing insertion to create 3 jobs after run lookup retry, got %d", len(gf.Store.createJob.calls))
	}
}

func TestCompleteJob_GateFailure_MixedClassificationInsertsHealing(t *testing.T) {
	t.Parallel()

	gf := newGateFailureFixture(t, []byte(`{"kind":"gate","gate":{"recovery":{"loop_kind":"healing","error_kind":"mixed","strategy_id":"mixed-default"}}}`))
	// Override successor meta with kind for this test.
	gf.Successor.Meta = []byte(`{"kind":"mig"}`)
	st := gf.Store

	handler := completeJobHandler(st, nil, nil)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, gf.completeJobReq(map[string]any{
		"status":    "Fail",
		"exit_code": 1,
	}))

	assertStatus(t, rr, http.StatusNoContent)
	if len(st.createJob.calls) != 3 {
		t.Fatalf("expected healing insertion to create 3 jobs, got %d", len(st.createJob.calls))
	}
	if len(st.updateJobStatus.calls) != 0 {
		t.Fatalf("expected no cancellation calls, got %d", len(st.updateJobStatus.calls))
	}
}
