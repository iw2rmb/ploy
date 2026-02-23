package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/events"
	"github.com/iw2rmb/ploy/internal/store"
)

// ===== Side Effects & Orchestration Tests =====
// These tests verify orchestration behavior when jobs complete:
// - Event publishing to SSE hub
// - Job scheduling for next step
// - Cascade failure handling (canceling remaining jobs)

// TestCompleteJob_PublishesEvents verifies that completing a job publishes events
// when the run transitions to terminal state.
func TestCompleteJob_PublishesEvents(t *testing.T) {
	t.Parallel()

	f := newJobFixture("mod", 1000)
	now := time.Now()

	repoID := domaintypes.NewModRepoID()
	f.Job.RepoID = repoID
	f.Job.RepoBaseRef = "main"
	f.Job.Attempt = 1

	st := &mockStore{
		getRunResult: store.Run{
			ID:        f.RunID,
			Status:    store.RunStatusStarted,
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
				Name:        "mod-0",
				Status:      store.JobStatusSuccess,
				ModType:     "mod",
				StepIndex:   1000,
			},
		},
		// All repos terminal triggers run completion.
		countRunReposByStatusResult: []store.CountRunReposByStatusRow{
			{Status: store.RunRepoStatusSuccess, Count: 1},
		},
	}

	eventsService, _ := events.New(events.Options{
		BufferSize:  10,
		HistorySize: 100,
	})
	handler := completeJobHandler(st, eventsService)

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

// TestCompleteJob_SchedulesNextJob verifies that a successful job completion
// triggers scheduling of the next job.
func TestCompleteJob_SchedulesNextJob(t *testing.T) {
	t.Parallel()

	f := newJobFixture("", 1000)
	nextJobID := domaintypes.NewJobID()

	nextJob := store.Job{
		ID:        nextJobID,
		RunID:     f.RunID,
		Status:    store.JobStatusCreated,
		StepIndex: 2000,
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     f.RunID,
			Status: store.RunStatusStarted,
		},
		getJobResult:          f.Job,
		listJobsByRunResult:   []store.Job{f.Job, nextJob},
		scheduleNextJobResult: nextJob,
	}

	handler := completeJobHandler(st, nil)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{"status": "Success"}))

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify ScheduleNextJob was called.
	if !st.scheduleNextJobCalled {
		t.Fatal("expected ScheduleNextJob to be called")
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
			Status: store.RunStatusStarted,
		},
		getJobResult:        f.Job,
		listJobsByRunResult: []store.Job{f.Job},
	}

	handler := completeJobHandler(st, nil)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{"status": "Fail"}))

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify ScheduleNextJob was NOT called for failed jobs.
	if st.scheduleNextJobCalled {
		t.Fatal("did not expect ScheduleNextJob to be called for failed job")
	}
}

// TestCompleteJob_ModFailureCancelsRemainingJobs verifies that when a non-gate
// mod job fails, remaining non-terminal jobs are canceled so the run can
// transition to a terminal state instead of leaving jobs stranded.
func TestCompleteJob_ModFailureCancelsRemainingJobs(t *testing.T) {
	t.Parallel()

	f := newJobFixture(domaintypes.ModTypeMod.String(), 2000)
	repoID := domaintypes.NewModRepoID()
	postJobID := domaintypes.NewJobID()

	f.Job.RepoID = repoID
	f.Job.RepoBaseRef = "main"
	f.Job.Attempt = 1
	f.Job.Meta = []byte(`{}`)

	// Jobs: pre-gate succeeded, mod failed, post-gate created.
	jobs := []store.Job{
		{
			ID:          domaintypes.NewJobID(),
			RunID:       f.RunID,
			RepoID:      repoID,
			RepoBaseRef: "main",
			Attempt:     1,
			NodeID:      &f.NodeID,
			Status:      store.JobStatusSuccess,
			ModType:     domaintypes.ModTypePreGate.String(),
			StepIndex:   1000,
			Meta:        []byte(`{}`),
		},
		f.Job,
		{
			ID:          postJobID,
			RunID:       f.RunID,
			RepoID:      repoID,
			RepoBaseRef: "main",
			Attempt:     1,
			Status:      store.JobStatusCreated,
			ModType:     domaintypes.ModTypePostGate.String(),
			StepIndex:   3000,
			Meta:        []byte(`{}`),
		},
	}

	st := &mockStore{
		getRunResult: store.Run{
			ID:     f.RunID,
			Status: store.RunStatusStarted,
		},
		getJobResult:                   jobs[1], // mod job
		listJobsByRunResult:            jobs,
		listJobsByRunRepoAttemptResult: jobs,
	}

	handler := completeJobHandler(st, nil)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{
		"status":    "Fail",
		"exit_code": 1,
	}))

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify UpdateJobCompletion was called for the mod job.
	if !st.updateJobCompletionCalled {
		t.Fatal("expected UpdateJobCompletion to be called")
	}
	if st.updateJobCompletionParams.ID != jobs[1].ID {
		t.Fatalf("expected UpdateJobCompletion for mod job, got %v", st.updateJobCompletionParams.ID)
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
			if call.Status != store.JobStatusCancelled {
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
			Status: store.RunStatusStarted,
		},
		getJobResult:        f.Job,
		listJobsByRunResult: []store.Job{f.Job},
	}

	handler := completeJobHandler(st, nil)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{"status": "Cancelled"}))

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}
	if !st.updateJobCompletionCalled {
		t.Fatal("expected UpdateJobCompletion to be called")
	}
	if st.updateJobCompletionParams.Status != store.JobStatusCancelled {
		t.Fatalf("expected job status canceled, got %s", st.updateJobCompletionParams.Status)
	}
}

func TestCompleteJob_Success_DoesNotUseStepIndexScheduler(t *testing.T) {
	t.Parallel()

	f := newJobFixture("", 1000)
	nextJob := store.Job{
		ID:          domaintypes.NewJobID(),
		RunID:       f.RunID,
		RepoID:      domaintypes.NewModRepoID(),
		RepoBaseRef: "main",
		Attempt:     1,
		Status:      store.JobStatusCreated,
		ModType:     domaintypes.ModTypeMod.String(),
		StepIndex:   2000,
	}
	f.Job.RepoID = nextJob.RepoID
	f.Job.RepoBaseRef = "main"
	f.Job.Attempt = 1

	st := &mockStore{
		getRunResult: store.Run{
			ID:     f.RunID,
			Status: store.RunStatusStarted,
		},
		getJobResult:                   f.Job,
		listJobsByRunResult:            []store.Job{f.Job, nextJob},
		listJobsByRunRepoAttemptResult: []store.Job{f.Job, nextJob},
		scheduleNextJobResult:          nextJob,
	}

	handler := completeJobHandler(st, nil)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, f.completeJobReq(map[string]any{"status": "Success"}))

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", rr.Code, rr.Body.String())
	}
	if st.scheduleNextJobCalled {
		t.Fatal("expected success completion to avoid step_index scheduler path")
	}
}

func TestCompleteJob_GateFailure_HealingInsertionDoesNotUseMidpointStepIndices(t *testing.T) {
	t.Parallel()

	f := newJobFixture(domaintypes.ModTypePreGate.String(), 1000)
	repoID := domaintypes.NewModRepoID()
	specID := domaintypes.NewSpecID()
	f.Job.RepoID = repoID
	f.Job.RepoBaseRef = "main"
	f.Job.Attempt = 1
	f.Job.Meta = []byte(`{}`)

	specBytes, err := json.Marshal(map[string]any{
		"steps": []any{
			map[string]any{"image": "mods-orw:latest"},
		},
		"build_gate": map[string]any{
			"healing": map[string]any{
				"retries": 1,
				"image":   "mods-codex:latest",
			},
			"router": map[string]any{
				"image": "mods-router:latest",
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
		Name:        "mod-0",
		Status:      store.JobStatusCreated,
		ModType:     domaintypes.ModTypeMod.String(),
		StepIndex:   2000,
		Meta:        []byte(`{}`),
	}

	jobs := []store.Job{f.Job, successor}
	st := &mockStore{
		getRunResult: store.Run{
			ID:     f.RunID,
			SpecID: specID,
			Status: store.RunStatusStarted,
		},
		getJobResult:                   f.Job,
		getSpecResult:                  store.Spec{ID: specID, Spec: specBytes},
		listJobsByRunResult:            jobs,
		listJobsByRunRepoAttemptResult: jobs,
	}

	handler := completeJobHandler(st, nil)

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
	for _, created := range st.createJobParams {
		if created.StepIndex > f.Job.StepIndex && created.StepIndex < successor.StepIndex {
			t.Fatalf("expected no midpoint insertion step_index between %v and %v, got %v for %s",
				f.Job.StepIndex, successor.StepIndex, created.StepIndex, created.Name)
		}
	}
}
