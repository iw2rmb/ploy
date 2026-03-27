package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	bsmock "github.com/iw2rmb/ploy/internal/blobstore/mock"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/lifecycle"
)

type transientGetRunStore struct {
	*mockStore
	failCount int
	err       error
	calls     int
}

func (s *transientGetRunStore) GetRun(ctx context.Context, id domaintypes.RunID) (store.Run, error) {
	s.calls++
	if s.calls <= s.failCount {
		s.mockStore.getRunCalled = true
		s.mockStore.getRunParams = id.String()
		return store.Run{}, s.err
	}
	return s.mockStore.GetRun(ctx, id)
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

	f := newJobFixture("mig", 1000)
	now := time.Now()

	repoID := domaintypes.NewRepoID()
	f.Job.RepoID = repoID
	f.Job.RepoBaseRef = "main"
	f.Job.Attempt = 1

	st := &mockStore{
		getRunResult: store.Run{
			ID:        f.RunID,
			Status:    domaintypes.RunStatusStarted,
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
		getJobResult:        f.Job,
		listJobsByRunResult: []store.Job{f.Job},
		// v1: repo-scoped progression requires all non-MR jobs to be terminal and
		// derives run_repos.status from the last job.
		listJobsByRunRepoAttemptResult: []store.Job{
			{
				ID:          f.JobID,
				RunID:       f.RunID,
				RepoID:      repoID,
				RepoBaseRef: "main",
				Attempt:     1,
				Name:        "mig-0",
				Status:      domaintypes.JobStatusSuccess,
				JobType:     "mig",
				Meta:        withNextIDMeta([]byte(`{}`), 1000),
			},
		},
		// All repos terminal triggers run completion.
		countRunReposByStatusResult: []store.CountRunReposByStatusRow{
			{Status: domaintypes.RunRepoStatusSuccess, Count: 1},
		},
	}

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

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}

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

	f := newJobFixture("", 1000)
	nextJobID := domaintypes.NewJobID()
	f.Job.NextID = &nextJobID

	nextJob := store.Job{
		ID:     nextJobID,
		RunID:  f.RunID,
		Status: domaintypes.JobStatusCreated,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     f.RunID,
			Status: domaintypes.RunStatusStarted,
		},
		getJobResult:                    f.Job,
		listJobsByRunResult:             []store.Job{f.Job, nextJob},
		promoteJobByIDIfUnblockedResult: nextJob,
	}

	handler := completeJobHandler(st, nil, nil)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
		"status":       "Success",
		"repo_sha_out": "0123456789abcdef0123456789abcdef01234567",
	}))

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify PromoteJobByIDIfUnblocked was called with the linked successor.
	if !st.promoteJobByIDIfUnblockedCalled {
		t.Fatal("expected PromoteJobByIDIfUnblocked to be called")
	}
	if st.promoteJobByIDIfUnblockedParam != nextJobID {
		t.Fatalf("expected PromoteJobByIDIfUnblocked(%s), got %s", nextJobID, st.promoteJobByIDIfUnblockedParam)
	}
}

// TestCompleteJob_FailedJobDoesNotScheduleNext verifies that a failed job
// does not trigger scheduling of the next job.
func TestCompleteJob_FailedJobDoesNotScheduleNext(t *testing.T) {
	t.Parallel()

	f := newJobFixture("", 1000)

	st := &mockStore{
		getRunResult: store.Run{
			ID:     f.RunID,
			Status: domaintypes.RunStatusStarted,
		},
		getJobResult:        f.Job,
		listJobsByRunResult: []store.Job{f.Job},
	}

	handler := completeJobHandler(st, nil, nil)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{"status": "Fail"}))

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify successor promotion was NOT called for failed jobs.
	if st.promoteJobByIDIfUnblockedCalled {
		t.Fatal("did not expect PromoteJobByIDIfUnblocked to be called for failed job")
	}
}

// TestCompleteJob_ModFailureCancelsRemainingJobs verifies that when a non-gate
// mig job fails, remaining non-terminal jobs are canceled so the run can
// transition to a terminal state instead of leaving jobs stranded.
func TestCompleteJob_ModFailureCancelsRemainingJobs(t *testing.T) {
	t.Parallel()

	f := newJobFixture(domaintypes.JobTypeMod, 2000)
	repoID := domaintypes.NewRepoID()
	postJobID := domaintypes.NewJobID()

	f.Job.RepoID = repoID
	f.Job.RepoBaseRef = "main"
	f.Job.Attempt = 1
	f.Job.Meta = []byte(`{"kind":"mig"}`)

	// Jobs: pre-gate succeeded, mig failed, post-gate created.
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

	st := &mockStore{
		getRunResult: store.Run{
			ID:     f.RunID,
			Status: domaintypes.RunStatusStarted,
		},
		getJobResult:                   jobs[1], // mig job
		listJobsByRunResult:            jobs,
		listJobsByRunRepoAttemptResult: jobs,
	}

	handler := completeJobHandler(st, nil, nil)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
		"status":    "Fail",
		"exit_code": 1,
	}))

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify UpdateJobCompletion was called for the mig job.
	if !st.updateJobCompletionCalled {
		t.Fatal("expected UpdateJobCompletion to be called")
	}
	if st.updateJobCompletionParams.ID != jobs[1].ID {
		t.Fatalf("expected UpdateJobCompletion for mig job, got %v", st.updateJobCompletionParams.ID)
	}

	// Verify UpdateJobStatus was called to cancel the post-gate job.
	if !st.updateJobStatusCalled {
		t.Fatal("expected UpdateJobStatus to be called to cancel remaining jobs")
	}
	if len(st.updateJobStatusCalls) == 0 {
		t.Fatal("expected at least one UpdateJobStatus call")
	}
	foundPostCancel := false
	for _, call := range st.updateJobStatusCalls {
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
}

// TestCompleteJob_CanceledStatus verifies that canceled status is accepted.
func TestCompleteJob_CanceledStatus(t *testing.T) {
	t.Parallel()

	f := newJobFixture("", 1000)

	st := &mockStore{
		getRunResult: store.Run{
			ID:     f.RunID,
			Status: domaintypes.RunStatusStarted,
		},
		getJobResult:        f.Job,
		listJobsByRunResult: []store.Job{f.Job},
	}

	handler := completeJobHandler(st, nil, nil)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{"status": "Cancelled"}))

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}
	if !st.updateJobCompletionCalled {
		t.Fatal("expected UpdateJobCompletion to be called")
	}
	if st.updateJobCompletionParams.Status != domaintypes.JobStatusCancelled {
		t.Fatalf("expected job status canceled, got %s", st.updateJobCompletionParams.Status)
	}
}

func TestCompleteJob_Success_DoesNotUseStepIndexScheduler(t *testing.T) {
	t.Parallel()

	f := newJobFixture("", 1000)
	nextJob := store.Job{
		ID:          domaintypes.NewJobID(),
		RunID:       f.RunID,
		RepoID:      domaintypes.NewRepoID(),
		RepoBaseRef: "main",
		Attempt:     1,
		Status:      domaintypes.JobStatusCreated,
		JobType:     domaintypes.JobTypeMod,
	}
	f.Job.RepoID = nextJob.RepoID
	f.Job.RepoBaseRef = "main"
	f.Job.Attempt = 1
	f.Job.NextID = &nextJob.ID

	st := &mockStore{
		getRunResult: store.Run{
			ID:     f.RunID,
			Status: domaintypes.RunStatusStarted,
		},
		getJobResult:                    f.Job,
		listJobsByRunResult:             []store.Job{f.Job, nextJob},
		listJobsByRunRepoAttemptResult:  []store.Job{f.Job, nextJob},
		promoteJobByIDIfUnblockedResult: nextJob,
	}

	handler := completeJobHandler(st, nil, nil)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
		"status":       "Success",
		"repo_sha_out": "0123456789abcdef0123456789abcdef01234567",
	}))

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}
	if st.scheduleNextJobCalled {
		t.Fatal("expected success completion to avoid next_id scheduler path")
	}
	if !st.promoteJobByIDIfUnblockedCalled {
		t.Fatal("expected linked successor promotion to be called")
	}
}

func TestCompleteJob_GateFailure_HealingInsertionRewiresNextChain(t *testing.T) {
	t.Parallel()

	f := newJobFixture(domaintypes.JobTypePreGate, 1000)
	repoID := domaintypes.NewRepoID()
	specID := domaintypes.NewSpecID()
	f.Job.RepoID = repoID
	f.Job.RepoBaseRef = "main"
	f.Job.Attempt = 1
	f.Job.Meta = []byte(`{"kind":"gate","gate":{"static_checks":[{"tool":"maven","passed":false}],"recovery":{"loop_kind":"healing","error_kind":"infra","strategy_id":"infra-default"}}}`)

	specBytes, err := json.Marshal(map[string]any{
		"steps": []any{
			map[string]any{"image": "migs-orw:latest"},
		},
		"build_gate": map[string]any{
			"healing": map[string]any{
				"by_error_kind": map[string]any{
					"infra": map[string]any{
						"retries": 1,
						"image":   "migs-codex:latest",
					},
				},
			},
			"router": map[string]any{
				"image": "migs-router:latest",
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal spec: %v", err)
	}

	successor := store.Job{
		ID:          domaintypes.NewJobID(),
		RunID:       f.RunID,
		RepoID:      repoID,
		RepoBaseRef: "main",
		Attempt:     1,
		Name:        "mig-0",
		Status:      domaintypes.JobStatusCreated,
		JobType:     domaintypes.JobTypeMod,
		Meta:        []byte(`{}`),
	}
	f.Job.NextID = &successor.ID

	jobs := []store.Job{f.Job, successor}
	st := &mockStore{
		getRunResult: store.Run{
			ID:     f.RunID,
			SpecID: specID,
			Status: domaintypes.RunStatusStarted,
		},
		getJobResult:                   f.Job,
		getSpecResult:                  store.Spec{ID: specID, Spec: specBytes},
		listJobsByRunResult:            jobs,
		listJobsByRunRepoAttemptResult: jobs,
	}

	handler := completeJobHandler(st, nil, nil)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
		"status":    "Fail",
		"exit_code": 1,
	}))

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}
	if st.createJobCallCount == 0 {
		t.Fatal("expected healing insertion to create follow-up jobs")
	}
	if st.createJobCallCount != 2 {
		t.Fatalf("expected 2 healing jobs, got %d", st.createJobCallCount)
	}
	reGate := st.createJobParams[0]
	heal := st.createJobParams[1]
	if reGate.Name != "re-gate-1" {
		t.Fatalf("expected first created healing job to be re-gate-1, got %q", reGate.Name)
	}
	if heal.Name != "heal-1-0" {
		t.Fatalf("expected second created healing job to be heal-1-0, got %q", heal.Name)
	}
	if heal.NextID == nil || *heal.NextID != reGate.ID {
		t.Fatalf("expected heal.NextID to point to re-gate job")
	}
	if reGate.NextID == nil || *reGate.NextID != successor.ID {
		t.Fatalf("expected re-gate.NextID to preserve old successor %s", successor.ID)
	}
	if len(st.updateJobNextIDParams) != 1 {
		t.Fatalf("expected one next_id rewiring update, got %d", len(st.updateJobNextIDParams))
	}
	if st.updateJobNextIDParams[0].ID != f.Job.ID || st.updateJobNextIDParams[0].NextID == nil || *st.updateJobNextIDParams[0].NextID != heal.ID {
		t.Fatalf("expected failed job rewired to heal")
	}
}

func TestCompleteJob_GateFailure_HealingInsertionRetriesRunLookup(t *testing.T) {
	t.Parallel()

	f := newJobFixture(domaintypes.JobTypePreGate, 1000)
	repoID := domaintypes.NewRepoID()
	specID := domaintypes.NewSpecID()
	f.Job.RepoID = repoID
	f.Job.RepoBaseRef = "main"
	f.Job.Attempt = 1
	f.Job.Meta = []byte(`{"kind":"gate","gate":{"static_checks":[{"tool":"maven","passed":false}],"recovery":{"loop_kind":"healing","error_kind":"infra","strategy_id":"infra-default"}}}`)

	specBytes, err := json.Marshal(map[string]any{
		"steps": []any{
			map[string]any{"image": "migs-orw:latest"},
		},
		"build_gate": map[string]any{
			"healing": map[string]any{
				"by_error_kind": map[string]any{
					"infra": map[string]any{
						"retries": 1,
						"image":   "migs-codex:latest",
					},
				},
			},
			"router": map[string]any{
				"image": "migs-router:latest",
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal spec: %v", err)
	}

	successor := store.Job{
		ID:          domaintypes.NewJobID(),
		RunID:       f.RunID,
		RepoID:      repoID,
		RepoBaseRef: "main",
		Attempt:     1,
		Name:        "mig-0",
		Status:      domaintypes.JobStatusCreated,
		JobType:     domaintypes.JobTypeMod,
		Meta:        []byte(`{}`),
	}
	f.Job.NextID = &successor.ID

	base := &mockStore{
		getRunResult: store.Run{
			ID:     f.RunID,
			SpecID: specID,
			Status: domaintypes.RunStatusStarted,
		},
		getJobResult:                   f.Job,
		getSpecResult:                  store.Spec{ID: specID, Spec: specBytes},
		listJobsByRunResult:            []store.Job{f.Job, successor},
		listJobsByRunRepoAttemptResult: []store.Job{f.Job, successor},
	}
	st := &transientGetRunStore{
		mockStore: base,
		failCount: 1,
		err:       errors.New("transient get run failure"),
	}

	handler := completeJobHandler(st, nil, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
		"status":    "Fail",
		"exit_code": 1,
	}))

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}
	if st.calls < 2 {
		t.Fatalf("expected run lookup retry after transient failure, calls=%d", st.calls)
	}
	if base.createJobCallCount != 2 {
		t.Fatalf("expected healing insertion to create 2 jobs after run lookup retry, got %d", base.createJobCallCount)
	}
}

func TestCompleteJob_GateFailure_MixedClassificationCancelsRemaining(t *testing.T) {
	t.Parallel()

	f := newJobFixture(domaintypes.JobTypePreGate, 1000)
	repoID := domaintypes.NewRepoID()
	specID := domaintypes.NewSpecID()
	f.Job.RepoID = repoID
	f.Job.RepoBaseRef = "main"
	f.Job.Attempt = 1
	f.Job.Meta = []byte(`{"kind":"gate","gate":{"recovery":{"loop_kind":"healing","error_kind":"mixed","strategy_id":"mixed-default"}}}`)

	specBytes, err := json.Marshal(map[string]any{
		"steps": []any{
			map[string]any{"image": "migs-orw:latest"},
		},
		"build_gate": map[string]any{
			"healing": map[string]any{
				"by_error_kind": map[string]any{
					"infra": map[string]any{
						"retries": 1,
						"image":   "migs-codex:latest",
					},
				},
			},
			"router": map[string]any{
				"image": "migs-router:latest",
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal spec: %v", err)
	}

	successor := store.Job{
		ID:          domaintypes.NewJobID(),
		RunID:       f.RunID,
		RepoID:      repoID,
		RepoBaseRef: "main",
		Attempt:     1,
		Name:        "mig-0",
		Status:      domaintypes.JobStatusCreated,
		JobType:     domaintypes.JobTypeMod,
		Meta:        []byte(`{"kind":"mig"}`),
	}
	f.Job.NextID = &successor.ID

	jobs := []store.Job{f.Job, successor}
	st := &mockStore{
		getRunResult: store.Run{
			ID:     f.RunID,
			SpecID: specID,
			Status: domaintypes.RunStatusStarted,
		},
		getJobResult:                   f.Job,
		getSpecResult:                  store.Spec{ID: specID, Spec: specBytes},
		listJobsByRunResult:            jobs,
		listJobsByRunRepoAttemptResult: jobs,
	}

	handler := completeJobHandler(st, nil, nil)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
		"status":    "Fail",
		"exit_code": 1,
	}))

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}
	if st.createJobCallCount != 0 {
		t.Fatalf("expected no healing insertion jobs, got %d", st.createJobCallCount)
	}
	if len(st.updateJobStatusCalls) != 1 {
		t.Fatalf("expected one cancellation call, got %d", len(st.updateJobStatusCalls))
	}
	if st.updateJobStatusCalls[0].ID != successor.ID || st.updateJobStatusCalls[0].Status != domaintypes.JobStatusCancelled {
		t.Fatalf("unexpected cancellation call: %+v", st.updateJobStatusCalls[0])
	}
}

// ===== Repo-Scoped Status Progression Tests =====
// These tests verify v1 repo-scoped progression behavior.
// When a job completes:
// - run_repos.status is updated when all jobs for the repo attempt are terminal
// - runs.status becomes Finished when all repos are terminal

// TestCompleteJob_RepoStatusUpdatedOnLastJob verifies that run_repos.status is updated
// to Success when the last job in a repo attempt completes successfully.
func TestCompleteJob_RepoStatusUpdatedOnLastJob(t *testing.T) {
	t.Parallel()

	f := newJobFixture("mig", 2000)
	f.Job.RepoID = domaintypes.NewRepoID()
	f.Job.RepoBaseRef = "main"
	f.Job.Attempt = 1

	// Single job per repo, completing it should mark repo as terminal.
	st := &mockStore{
		getRunResult: store.Run{
			ID:     f.RunID,
			Status: domaintypes.RunStatusStarted,
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
				Name:        "mig-0",
				Status:      domaintypes.JobStatusSuccess,
				JobType:     "mig",
				Meta:        withNextIDMeta([]byte(`{}`), 2000),
			},
		},
		// All repos terminal (1 Success), so run becomes Finished.
		countRunReposByStatusResult: []store.CountRunReposByStatusRow{
			{Status: domaintypes.RunRepoStatusSuccess, Count: 1},
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
	if lastRepoUpdate.Status != domaintypes.RunRepoStatusSuccess {
		t.Errorf("expected repo status Success, got %s", lastRepoUpdate.Status)
	}

	// Verify UpdateRunStatus was called to set run to Finished.
	if !st.updateRunStatusCalled {
		t.Fatal("expected UpdateRunStatus to be called")
	}
	if st.updateRunStatusParams.Status != domaintypes.RunStatusFinished {
		t.Errorf("expected run status Finished, got %s", st.updateRunStatusParams.Status)
	}
}

// TestCompleteJob_RepoStatusFail verifies that run_repos.status is updated
// to Fail when a job in the repo attempt fails.
func TestCompleteJob_RepoStatusFail(t *testing.T) {
	t.Parallel()

	f := newJobFixture("mig", 2000)
	f.Job.RepoID = domaintypes.NewRepoID()
	f.Job.RepoBaseRef = "main"
	f.Job.Attempt = 1

	// Job that will fail.
	st := &mockStore{
		getRunResult: store.Run{
			ID:     f.RunID,
			Status: domaintypes.RunStatusStarted,
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
				Name:        "mig-0",
				Status:      domaintypes.JobStatusFail,
				JobType:     "mig",
				Meta:        withNextIDMeta([]byte(`{}`), 2000),
			},
		},
		// All repos terminal.
		countRunReposByStatusResult: []store.CountRunReposByStatusRow{
			{Status: domaintypes.RunRepoStatusFail, Count: 1},
		},
	}

	handler := completeJobHandler(st, nil, nil)

	req := f.completeJobReq(map[string]any{
		"status":    "Fail",
		"exit_code": 1,
	})

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusNoContent)

	// Verify repo status was updated to Fail.
	if !st.updateRunRepoStatusCalled {
		t.Fatal("expected UpdateRunRepoStatus to be called")
	}
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

	f := newJobFixture("pre_gate", 1000)
	f.Job.RepoID = domaintypes.NewRepoID()
	f.Job.RepoBaseRef = "main"
	f.Job.Attempt = 1
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
	if !st.promoteJobByIDIfUnblockedCalled {
		t.Fatal("expected PromoteJobByIDIfUnblocked to be called")
	}
}

// TestCompleteJob_RepoStatusUsesLastJobStatus verifies that when all jobs are
// terminal, run_repos.status is derived from the terminal status of the last job
// (highest next_id), ignoring earlier failures.
func TestCompleteJob_RepoStatusUsesLastJobStatus(t *testing.T) {
	t.Parallel()

	f := newJobFixture("post_gate", 3000)
	f.Job.RepoID = domaintypes.NewRepoID()
	f.Job.RepoBaseRef = "main"
	f.Job.Attempt = 1
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

	if !st.updateRunRepoStatusCalled {
		t.Fatal("expected UpdateRunRepoStatus to be called")
	}
	lastRepoUpdate := st.updateRunRepoStatusParams[len(st.updateRunRepoStatusParams)-1]
	if lastRepoUpdate.Status != domaintypes.RunRepoStatusSuccess {
		t.Errorf("expected repo status Success, got %s", lastRepoUpdate.Status)
	}
}

// TestCompleteJob_MRJobDoesNotAffectRepoStatus verifies that MR jobs (job_type='mr')
// do NOT trigger repo status updates. MR jobs are auxiliary post-run jobs.
func TestCompleteJob_MRJobDoesNotAffectRepoStatus(t *testing.T) {
	t.Parallel()

	f := newJobFixture("mr", 9000)
	f.Job.RepoID = domaintypes.NewRepoID()
	f.Job.RepoBaseRef = "main"
	f.Job.Attempt = 1
	f.Job.Name = "mr-0"

	// MR job (auxiliary, should not affect repo/run status).
	st := &mockStore{
		getRunResult: store.Run{
			ID:     f.RunID,
			Status: domaintypes.RunStatusFinished, // MR jobs run after run is Finished.
		},
		getJobResult:        f.Job,
		listJobsByRunResult: []store.Job{f.Job},
	}

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

	f := newJobFixture("mig", 2000)
	f.Job.RepoID = domaintypes.NewRepoID()
	f.Job.RepoBaseRef = "main"
	f.Job.Attempt = 1
	// repoIDB is another repo in the run, still Running (not explicitly used but modeled in countRunReposByStatusResult).
	_ = domaintypes.NewRepoID() // repoIDB - unused but conceptually part of the multi-repo scenario

	// Job for repo A completing (repo B still has work).
	st := &mockStore{
		getRunResult: store.Run{
			ID:     f.RunID,
			Status: domaintypes.RunStatusStarted,
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
				Name:        "mig-0",
				Status:      domaintypes.JobStatusSuccess,
				JobType:     "mig",
				Meta:        withNextIDMeta([]byte(`{}`), 2000),
			},
		},
		// But repo B is still Running, so run should NOT become Finished.
		countRunReposByStatusResult: []store.CountRunReposByStatusRow{
			{Status: domaintypes.RunRepoStatusSuccess, Count: 1}, // Repo A
			{Status: domaintypes.RunRepoStatusRunning, Count: 1}, // Repo B still running
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

	f := newJobFixture("mig", 1000)
	st := &mockStore{}
	handler := completeJobHandler(st, nil, nil)

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

	f := newJobFixture("mig", 1000)
	st := &mockStore{}
	handler := completeJobHandler(st, nil, nil)

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

	f := newJobFixture("mig", 1000)
	st := &mockStore{}
	handler := completeJobHandler(st, nil, nil)

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

// ===== Recovery Flow Tests =====

func TestCompleteJob_ReGateSuccessPromotesValidatedCandidate(t *testing.T) {
	t.Parallel()

	f := newJobFixture(domaintypes.JobTypeReGate, 1000)
	f.Job.RepoID = domaintypes.NewRepoID()
	f.Job.RepoBaseRef = "main"
	f.Job.Attempt = 1
	f.Job.Meta = []byte(`{"kind":"gate","recovery":{"loop_kind":"healing","error_kind":"infra","candidate_schema_id":"gate_profile_v1","candidate_artifact_path":"/out/gate-profile-candidate.json","candidate_validation_status":"valid","candidate_gate_profile":{"schema_version":1,"repo_id":"repo_1","runner_mode":"simple","stack":{"language":"go","tool":"go"},"targets":{"active":"build","build":{"status":"passed","command":"go test ./...","env":{},"failure_code":null},"unit":{"status":"not_attempted","env":{}},"all_tests":{"status":"not_attempted","env":{}}},"orchestration":{"pre":[],"post":[]}},"candidate_promoted":false}}`)

	st := &mockStore{
		getRunResult: store.Run{ID: f.RunID, Status: domaintypes.RunStatusStarted},
		getJobResult: f.Job,
		resolveStackRowByLangToolResult: store.ResolveStackRowByLangToolRow{
			ID: 7, Lang: "go", Tool: "go", Release: "",
		},
	}

	handler := completeJobHandler(st, nil, nil, bsmock.New())
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
		"status": "Success",
		"stats": map[string]any{
			"job_meta": map[string]any{
				"kind": "gate",
				"gate": map[string]any{
					"static_checks": []any{map[string]any{"tool": "maven", "passed": true}},
				},
			},
		},
	}))

	assertStatus(t, rr, http.StatusNoContent)
	if !st.upsertExactGateProfileCalled {
		t.Fatal("expected UpsertExactGateProfile to be called")
	}
	if !st.upsertGateJobProfileLinkCalled {
		t.Fatal("expected UpsertGateJobProfileLink to be called")
	}
	if !st.updateJobMetaCalled {
		t.Fatal("expected UpdateJobMeta to be called")
	}
	meta, err := contracts.UnmarshalJobMeta(st.updateJobMetaParams.Meta)
	if err != nil {
		t.Fatalf("unmarshal promoted meta: %v", err)
	}
	if meta.Recovery == nil || meta.Recovery.CandidatePromoted == nil || !*meta.Recovery.CandidatePromoted {
		t.Fatalf("candidate_promoted = %#v, want true", meta.Recovery)
	}
}

func TestCompleteJob_ReGateCompletionMergesExistingRecoveryMetadata(t *testing.T) {
	t.Parallel()

	f := newJobFixture(domaintypes.JobTypeReGate, 1000)
	f.Job.RepoID = domaintypes.NewRepoID()
	f.Job.RepoBaseRef = "main"
	f.Job.Attempt = 1
	f.Job.Meta = []byte(`{"kind":"gate","recovery":{"loop_kind":"healing","error_kind":"infra","candidate_schema_id":"gate_profile_v1","candidate_artifact_path":"/out/gate-profile-candidate.json","candidate_validation_status":"valid","candidate_gate_profile":{"schema_version":1,"repo_id":"repo_1","runner_mode":"simple","stack":{"language":"go","tool":"go"},"targets":{"active":"build","build":{"status":"passed","command":"go test ./...","env":{},"failure_code":null},"unit":{"status":"not_attempted","env":{}},"all_tests":{"status":"not_attempted","env":{}}},"orchestration":{"pre":[],"post":[]}},"candidate_promoted":false}}`)

	st := &mockStore{
		getRunResult: store.Run{ID: f.RunID, Status: domaintypes.RunStatusStarted},
		getJobResult: f.Job,
	}

	handler := completeJobHandler(st, nil, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
		"status": "Success",
		"stats": map[string]any{
			"job_meta": map[string]any{
				"kind": "gate",
				"gate": map[string]any{
					"static_checks": []any{map[string]any{"tool": "maven", "passed": true}},
				},
			},
		},
	}))

	assertStatus(t, rr, http.StatusNoContent)
	if !st.updateJobCompletionWithMetaCalled {
		t.Fatal("expected UpdateJobCompletionWithMeta to be called")
	}

	meta, err := contracts.UnmarshalJobMeta(st.updateJobCompletionWithMetaParams.Meta)
	if err != nil {
		t.Fatalf("unmarshal persisted meta: %v", err)
	}
	if meta.Recovery == nil {
		t.Fatal("expected merged job-level recovery metadata")
	}
	if got, want := meta.Recovery.CandidateValidationStatus, contracts.RecoveryCandidateStatusValid; got != want {
		t.Fatalf("candidate_validation_status = %q, want %q", got, want)
	}
	if meta.Recovery.CandidatePromoted == nil || *meta.Recovery.CandidatePromoted {
		t.Fatalf("candidate_promoted = %#v, want false before promotion write", meta.Recovery.CandidatePromoted)
	}
}

func TestCompleteJob_ReGateFailureDoesNotPromoteCandidate(t *testing.T) {
	t.Parallel()

	f := newJobFixture(domaintypes.JobTypeReGate, 1000)
	f.Job.RepoID = domaintypes.NewRepoID()
	f.Job.RepoBaseRef = "main"
	f.Job.Attempt = 1
	f.Job.Meta = []byte(`{"kind":"gate","recovery":{"loop_kind":"healing","error_kind":"infra","candidate_schema_id":"gate_profile_v1","candidate_artifact_path":"/out/gate-profile-candidate.json","candidate_validation_status":"valid","candidate_gate_profile":{"schema_version":1,"repo_id":"repo_1","runner_mode":"simple","stack":{"language":"go","tool":"go"},"targets":{"active":"build","build":{"status":"passed","command":"go test ./...","env":{},"failure_code":null},"unit":{"status":"not_attempted","env":{}},"all_tests":{"status":"not_attempted","env":{}}},"orchestration":{"pre":[],"post":[]}}}}`)

	st := &mockStore{
		getRunResult: store.Run{ID: f.RunID, Status: domaintypes.RunStatusStarted},
		getJobResult: f.Job,
	}

	handler := completeJobHandler(st, nil, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{"status": "Fail"}))

	assertStatus(t, rr, http.StatusNoContent)
	if st.upsertExactGateProfileCalled {
		t.Fatal("did not expect gate profile persistence on failed re-gate")
	}
}

func TestCompleteJob_HealSuccessRefreshesNextReGateCandidate(t *testing.T) {
	t.Parallel()

	f := newJobFixture(domaintypes.JobTypeHeal, 1000)
	f.Job.RepoID = domaintypes.NewRepoID()
	f.Job.RepoBaseRef = "main"
	f.Job.Attempt = 1
	reGateID := domaintypes.NewJobID()
	f.Job.NextID = &reGateID
	f.Job.Meta = []byte(`{"kind":"mig","recovery":{"loop_kind":"healing","error_kind":"infra","strategy_id":"infra-default","expectations":{"artifacts":[{"path":"/out/gate-profile-candidate.json","schema":"gate_profile_v1"}]}}}`)

	failedGateID := domaintypes.NewJobID()
	failedGate := store.Job{
		ID: failedGateID, RunID: f.RunID, RepoID: f.Job.RepoID,
		RepoBaseRef: f.Job.RepoBaseRef, Attempt: f.Job.Attempt,
		Name: "pre-gate", Status: domaintypes.JobStatusFail,
		JobType: domaintypes.JobTypePreGate, NextID: &f.Job.ID,
		Meta: []byte(`{"kind":"gate","gate":{"static_checks":[{"language":"java","tool":"maven","passed":true}],"recovery":{"loop_kind":"healing","error_kind":"infra","strategy_id":"infra-default"}}}`),
	}
	reGate := store.Job{
		ID: reGateID, RunID: f.RunID, RepoID: f.Job.RepoID,
		RepoBaseRef: f.Job.RepoBaseRef, Attempt: f.Job.Attempt,
		Name: "re-gate-1", Status: domaintypes.JobStatusCreated,
		JobType: domaintypes.JobTypeReGate,
		Meta:    []byte(`{"kind":"gate","recovery":{"loop_kind":"healing","error_kind":"infra","strategy_id":"infra-default","candidate_schema_id":"gate_profile_v1","candidate_artifact_path":"/out/gate-profile-candidate.json","candidate_validation_status":"missing"}}`),
	}

	st := &mockStore{
		getRunResult:  store.Run{ID: f.RunID, Status: domaintypes.RunStatusStarted},
		getJobResult:  f.Job,
		getJobResults: map[domaintypes.JobID]store.Job{reGateID: reGate},
		listJobsByRunRepoAttemptResult: []store.Job{failedGate, f.Job, reGate},
		listArtifactBundlesMetaByRunAndJobResult: []store.ArtifactBundle{
			{RunID: f.RunID, JobID: &f.Job.ID, ObjectKey: strPtr("artifacts/run/" + f.RunID.String() + "/bundle/heal.tar.gz")},
		},
	}
	if _, stack, _ := lifecycle.ResolveGateRecoveryContext(failedGate); stack == contracts.ModStackUnknown {
		t.Fatal("expected failed gate metadata to expose detected stack")
	}

	bs := bsmock.New()
	candidateJSON := []byte(`{
  "schema_version":1,
  "repo_id":"` + f.Job.RepoID.String() + `",
  "runner_mode":"simple",
  "stack":{"language":"java","tool":"maven"},
  "targets":{
    "active":"build",
    "build":{"status":"passed","command":"mvn test","env":{},"failure_code":null},
    "unit":{"status":"not_attempted","env":{}},
    "all_tests":{"status":"not_attempted","env":{}}
  },
  "orchestration":{"pre":[],"post":[]},
  "tactics_used":["unit_test_focused_profile"],
  "attempts":[],
  "evidence":{"log_refs":["/in/build-gate.log"],"diagnostics":[]},
  "repro_check":{"status":"failed","details":"not run"},
  "prompt_delta_suggestion":{"status":"none","summary":"","candidate_lines":[]}
}`)
	bundle := mustTarGzPayload(t, map[string][]byte{"out/gate-profile-candidate.json": candidateJSON})
	if _, err := bs.Put(context.Background(), "artifacts/run/"+f.RunID.String()+"/bundle/heal.tar.gz", "application/gzip", bundle); err != nil {
		t.Fatalf("put blob: %v", err)
	}
	bp := blobpersist.New(st, bs)

	handler := completeJobHandler(st, nil, bp)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
		"status":       "Success",
		"repo_sha_out": "0123456789abcdef0123456789abcdef01234567",
	}))

	assertStatus(t, rr, http.StatusNoContent)
	if !st.updateJobMetaCalled {
		t.Fatal("expected UpdateJobMeta to be called for next re-gate")
	}
	if st.updateJobMetaParams.ID != reGateID {
		t.Fatalf("updated meta job_id = %s, want %s", st.updateJobMetaParams.ID, reGateID)
	}
	meta, err := contracts.UnmarshalJobMeta(st.updateJobMetaParams.Meta)
	if err != nil {
		t.Fatalf("unmarshal updated re-gate meta: %v", err)
	}
	if meta.Recovery == nil {
		t.Fatal("expected recovery metadata")
	}
	if got, want := meta.Recovery.CandidateValidationStatus, contracts.RecoveryCandidateStatusValid; got != want {
		t.Fatalf("candidate_validation_status = %q, want %q (error=%q)", got, want, meta.Recovery.CandidateValidationError)
	}
	if len(meta.Recovery.CandidateGateProfile) == 0 {
		t.Fatal("expected candidate_gate_profile payload")
	}
}

func TestCompleteJob_HealSuccessRefreshesNextReGateCandidateMissing(t *testing.T) {
	t.Parallel()

	f := newJobFixture(domaintypes.JobTypeHeal, 1000)
	f.Job.RepoID = domaintypes.NewRepoID()
	f.Job.RepoBaseRef = "main"
	f.Job.Attempt = 1
	reGateID := domaintypes.NewJobID()
	f.Job.NextID = &reGateID

	failedGateID := domaintypes.NewJobID()
	failedGate := store.Job{
		ID: failedGateID, RunID: f.RunID, RepoID: f.Job.RepoID,
		RepoBaseRef: f.Job.RepoBaseRef, Attempt: f.Job.Attempt,
		Name: "pre-gate", Status: domaintypes.JobStatusFail,
		JobType: domaintypes.JobTypePreGate, NextID: &f.Job.ID,
		Meta: []byte(`{"kind":"gate","gate":{"static_checks":[{"language":"java","tool":"maven","passed":true}],"recovery":{"loop_kind":"healing","error_kind":"infra","strategy_id":"infra-default"}}}`),
	}
	reGate := store.Job{
		ID: reGateID, RunID: f.RunID, RepoID: f.Job.RepoID,
		RepoBaseRef: f.Job.RepoBaseRef, Attempt: f.Job.Attempt,
		Name: "re-gate-1", Status: domaintypes.JobStatusCreated,
		JobType: domaintypes.JobTypeReGate,
		Meta:    []byte(`{"kind":"gate","recovery":{"loop_kind":"healing","error_kind":"infra","strategy_id":"infra-default","candidate_schema_id":"gate_profile_v1","candidate_artifact_path":"/out/gate-profile-candidate.json","candidate_validation_status":"missing"}}`),
	}

	st := &mockStore{
		getRunResult:                   store.Run{ID: f.RunID, Status: domaintypes.RunStatusStarted},
		getJobResult:                   f.Job,
		getJobResults:                  map[domaintypes.JobID]store.Job{reGateID: reGate},
		listJobsByRunRepoAttemptResult: []store.Job{failedGate, f.Job, reGate},
	}
	bp := blobpersist.New(st, bsmock.New())

	handler := completeJobHandler(st, nil, bp)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
		"status":       "Success",
		"repo_sha_out": "0123456789abcdef0123456789abcdef01234567",
	}))

	assertStatus(t, rr, http.StatusNoContent)
	if !st.updateJobMetaCalled {
		t.Fatal("expected UpdateJobMeta to be called for next re-gate")
	}
	meta, err := contracts.UnmarshalJobMeta(st.updateJobMetaParams.Meta)
	if err != nil {
		t.Fatalf("unmarshal updated re-gate meta: %v", err)
	}
	if meta.Recovery == nil {
		t.Fatal("expected recovery metadata")
	}
	if got, want := meta.Recovery.CandidateValidationStatus, contracts.RecoveryCandidateStatusMissing; got != want {
		t.Fatalf("candidate_validation_status = %q, want %q", got, want)
	}
	if meta.Recovery.CandidateValidationError == "" {
		t.Fatal("expected candidate_validation_error for missing artifact")
	}
}
