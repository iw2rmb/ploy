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

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server"
	"github.com/iw2rmb/ploy/internal/store"
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

	assertStatus(t, rr, http.StatusNoContent)

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

	assertStatus(t, rr, http.StatusNoContent)

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

	assertStatus(t, rr, http.StatusNoContent)

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

	assertStatus(t, rr, http.StatusNoContent)
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

	assertStatus(t, rr, http.StatusNoContent)
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

	assertStatus(t, rr, http.StatusNoContent)
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

	assertStatus(t, rr, http.StatusNoContent)
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

	assertStatus(t, rr, http.StatusNoContent)
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

