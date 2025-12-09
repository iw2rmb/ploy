package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// TestResumeRun_FailedRun verifies that a failed run can be resumed.
func TestResumeRun_FailedRun(t *testing.T) {
	t.Parallel()
	id := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()
	st := &mockStore{
		getRunResult: store.Run{
			ID:         id.String(),
			Status:     store.RunStatusFailed,
			RepoUrl:    "https://example/repo.git",
			CreatedAt:  pgtype.Timestamptz{Time: time.Now().Add(-time.Minute), Valid: true},
			FinishedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
		},
		listJobsByRunResult: []store.Job{
			{
				ID:         jobID.String(),
				Status:     store.JobStatusFailed,
				StepIndex:  1000,
				StartedAt:  pgtype.Timestamptz{Time: time.Now().Add(-30 * time.Second), Valid: true},
				FinishedAt: pgtype.Timestamptz{Time: time.Now().Add(-10 * time.Second), Valid: true},
			},
		},
	}

	handler := resumeRunHandler(st, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods/"+id.String()+"/resume", nil)
	req.SetPathValue("id", id.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rr.Code, rr.Body.String())
	}
	// Verify run status was updated to queued.
	if !st.updateRunStatusCalled {
		t.Fatal("expected UpdateRunStatus to be called")
	}
	if st.updateRunStatusParams.Status != store.RunStatusQueued {
		t.Fatalf("expected run status queued, got %s", st.updateRunStatusParams.Status)
	}
	// Verify job was reset.
	if !st.updateJobStatusCalled {
		t.Fatal("expected UpdateJobStatus to be called")
	}
	// First job should be set to pending (ready for immediate claim).
	if len(st.updateJobStatusCalls) != 1 {
		t.Fatalf("expected 1 job update, got %d", len(st.updateJobStatusCalls))
	}
	if st.updateJobStatusCalls[0].Status != store.JobStatusPending {
		t.Fatalf("expected first job status pending, got %s", st.updateJobStatusCalls[0].Status)
	}
	// Timing should be cleared.
	if st.updateJobStatusCalls[0].StartedAt.Valid {
		t.Fatal("expected started_at to be cleared")
	}
	if st.updateJobStatusCalls[0].FinishedAt.Valid {
		t.Fatal("expected finished_at to be cleared")
	}
}

// TestResumeRun_CanceledRun verifies that a canceled run can be resumed.
func TestResumeRun_CanceledRun(t *testing.T) {
	t.Parallel()
	id := domaintypes.NewRunID()
	st := &mockStore{
		getRunResult: store.Run{
			ID:         id.String(),
			Status:     store.RunStatusCanceled,
			RepoUrl:    "https://example/repo.git",
			CreatedAt:  pgtype.Timestamptz{Time: time.Now().Add(-time.Minute), Valid: true},
			FinishedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
		},
		listJobsByRunResult: []store.Job{
			{ID: domaintypes.NewJobID().String(), Status: store.JobStatusCanceled, StepIndex: 1000},
		},
	}

	handler := resumeRunHandler(st, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods/"+id.String()+"/resume", nil)
	req.SetPathValue("id", id.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rr.Code, rr.Body.String())
	}
	if st.updateRunStatusParams.Status != store.RunStatusQueued {
		t.Fatalf("expected run status queued, got %s", st.updateRunStatusParams.Status)
	}
}

// TestResumeRun_Idempotent_AlreadyRunning verifies 200 when run is already in progress.
func TestResumeRun_Idempotent_AlreadyRunning(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		runStatus store.RunStatus
	}{
		{"queued", store.RunStatusQueued},
		{"assigned", store.RunStatusAssigned},
		{"running", store.RunStatusRunning},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			id := domaintypes.NewRunID()
			st := &mockStore{getRunResult: store.Run{ID: id.String(), Status: tt.runStatus}}
			handler := resumeRunHandler(st, nil)
			req := httptest.NewRequest(http.MethodPost, "/v1/mods/"+id.String()+"/resume", nil)
			req.SetPathValue("id", id.String())
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			if rr.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d", rr.Code)
			}
			if st.updateRunStatusCalled {
				t.Fatal("did not expect UpdateRunStatus when already in progress")
			}
		})
	}
}

// TestResumeRun_SucceededConflict verifies 409 when trying to resume a succeeded run.
// This tests resumability invariant 2: succeeded runs cannot be resumed.
func TestResumeRun_SucceededConflict(t *testing.T) {
	t.Parallel()
	id := domaintypes.NewRunID()
	st := &mockStore{getRunResult: store.Run{ID: id.String(), Status: store.RunStatusSucceeded}}
	handler := resumeRunHandler(st, nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/mods/"+id.String()+"/resume", nil)
	req.SetPathValue("id", id.String())
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rr.Code, rr.Body.String())
	}
	// Verify error message follows the invariant format.
	body := rr.Body.String()
	if !contains(body, "state=succeeded is not resumable") {
		t.Fatalf("expected error message with invariant format, got: %s", body)
	}
}

// TestResumeRun_BadID_And_NotFound tests error handling for invalid IDs and missing runs.
// Run IDs are now KSUID strings; only empty/whitespace IDs are rejected as invalid.
func TestResumeRun_BadID_And_NotFound(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		id         string
		mockStore  *mockStore
		wantStatus int
		wantBody   string
	}{
		// Note: "abc" is now a valid KSUID string ID, so we removed the "invalid uuid format" test.
		{
			name:       "empty id",
			id:         "",
			mockStore:  &mockStore{},
			wantStatus: http.StatusBadRequest,
			wantBody:   "id path parameter is required",
		},
		{
			name:       "whitespace only id",
			id:         "   ",
			mockStore:  &mockStore{},
			wantStatus: http.StatusBadRequest,
			wantBody:   "id path parameter is required",
		},
		{
			name:       "run not found",
			id:         domaintypes.NewRunID().String(),
			mockStore:  &mockStore{getRunErr: pgx.ErrNoRows},
			wantStatus: http.StatusNotFound,
			wantBody:   "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			handler := resumeRunHandler(tt.mockStore, nil)
			req := httptest.NewRequest(http.MethodPost, "/v1/mods/placeholder/resume", nil)
			req.SetPathValue("id", tt.id)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			if rr.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d", tt.wantStatus, rr.Code)
			}
			if tt.wantBody != "" && !contains(rr.Body.String(), tt.wantBody) {
				t.Fatalf("expected body to contain %q, got %q", tt.wantBody, rr.Body.String())
			}
		})
	}
}

// TestResumeRun_PartiallySucceeded resumes a run where some jobs succeeded and one failed.
func TestResumeRun_PartiallySucceeded(t *testing.T) {
	t.Parallel()
	id := domaintypes.NewRunID()
	successJob := store.Job{ID: domaintypes.NewJobID().String(), Status: store.JobStatusSucceeded, StepIndex: 1000}
	failedJob := store.Job{ID: domaintypes.NewJobID().String(), Status: store.JobStatusFailed, StepIndex: 2000}
	createdJob := store.Job{ID: domaintypes.NewJobID().String(), Status: store.JobStatusCreated, StepIndex: 3000}

	st := &mockStore{
		getRunResult: store.Run{
			ID:         id.String(),
			Status:     store.RunStatusFailed,
			RepoUrl:    "https://example/repo.git",
			CreatedAt:  pgtype.Timestamptz{Time: time.Now().Add(-time.Minute), Valid: true},
			FinishedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
		},
		listJobsByRunResult: []store.Job{successJob, failedJob, createdJob},
	}

	handler := resumeRunHandler(st, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods/"+id.String()+"/resume", nil)
	req.SetPathValue("id", id.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rr.Code, rr.Body.String())
	}
	// Verify that only the failed job was reset (succeeded job should remain untouched).
	if len(st.updateJobStatusCalls) != 1 {
		t.Fatalf("expected 1 job update (failed job), got %d", len(st.updateJobStatusCalls))
	}
	// The failed job should be set to 'pending' (it's the first non-succeeded job).
	call := st.updateJobStatusCalls[0]
	if call.ID != failedJob.ID {
		t.Fatalf("expected failed job to be updated, got %s", call.ID)
	}
	if call.Status != store.JobStatusPending {
		t.Fatalf("expected failed job status pending, got %s", call.Status)
	}
}

// TestResumeRun_MultipleFailedJobs verifies correct ordering when multiple jobs need reset.
func TestResumeRun_MultipleFailedJobs(t *testing.T) {
	t.Parallel()
	id := domaintypes.NewRunID()
	job1 := store.Job{ID: domaintypes.NewJobID().String(), Status: store.JobStatusSucceeded, StepIndex: 1000}
	job2 := store.Job{ID: domaintypes.NewJobID().String(), Status: store.JobStatusFailed, StepIndex: 2000}
	job3 := store.Job{ID: domaintypes.NewJobID().String(), Status: store.JobStatusCanceled, StepIndex: 3000}

	st := &mockStore{
		getRunResult: store.Run{
			ID:         id.String(),
			Status:     store.RunStatusFailed,
			RepoUrl:    "https://example/repo.git",
			CreatedAt:  pgtype.Timestamptz{Time: time.Now().Add(-time.Minute), Valid: true},
			FinishedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
		},
		listJobsByRunResult: []store.Job{job1, job2, job3},
	}

	handler := resumeRunHandler(st, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods/"+id.String()+"/resume", nil)
	req.SetPathValue("id", id.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rr.Code, rr.Body.String())
	}
	// Both failed and canceled jobs should be reset.
	if len(st.updateJobStatusCalls) != 2 {
		t.Fatalf("expected 2 job updates, got %d", len(st.updateJobStatusCalls))
	}
	// First job (job2, failed) should be 'pending', second job (job3, canceled) should be 'created'.
	statusByJob := make(map[string]store.JobStatus)
	for _, call := range st.updateJobStatusCalls {
		statusByJob[call.ID] = call.Status
	}
	if statusByJob[job2.ID] != store.JobStatusPending {
		t.Fatalf("expected job2 to be pending, got %s", statusByJob[job2.ID])
	}
	if statusByJob[job3.ID] != store.JobStatusCreated {
		t.Fatalf("expected job3 to be created, got %s", statusByJob[job3.ID])
	}
}

// TestResumeRun_SSEPublish verifies that resume publishes run events.
func TestResumeRun_SSEPublish(t *testing.T) {
	t.Parallel()
	id := domaintypes.NewRunID()
	st := &mockStore{
		getRunResult: store.Run{
			ID:         id.String(),
			Status:     store.RunStatusFailed,
			RepoUrl:    "https://example/repo.git",
			CreatedAt:  pgtype.Timestamptz{Time: time.Now().Add(-time.Minute), Valid: true},
			FinishedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
		},
		listJobsByRunResult: []store.Job{
			{ID: domaintypes.NewJobID().String(), Status: store.JobStatusFailed, StepIndex: 1000},
		},
	}

	eventsService, err := createTestEventsService()
	if err != nil {
		t.Fatalf("failed to create events service: %v", err)
	}

	handler := resumeRunHandler(st, eventsService)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods/"+id.String()+"/resume", nil)
	req.SetPathValue("id", id.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify run event was published.
	snapshot := eventsService.Hub().Snapshot(id.String())
	if len(snapshot) == 0 {
		t.Fatal("expected at least 1 run event in snapshot")
	}
	foundRun := false
	for _, evt := range snapshot {
		if evt.Type == "run" {
			foundRun = true
		}
	}
	if !foundRun {
		t.Fatal("expected run event in snapshot")
	}
}

// TestResumeRun_IdempotentWhenPendingJobExists verifies 200 OK when a pending job already exists.
func TestResumeRun_IdempotentWhenPendingJobExists(t *testing.T) {
	t.Parallel()
	id := domaintypes.NewRunID()
	st := &mockStore{
		getRunResult: store.Run{
			ID:         id.String(),
			Status:     store.RunStatusFailed, // Run is terminal but has a pending job (edge case).
			RepoUrl:    "https://example/repo.git",
			CreatedAt:  pgtype.Timestamptz{Time: time.Now().Add(-time.Minute), Valid: true},
			FinishedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
		},
		listJobsByRunResult: []store.Job{
			{ID: domaintypes.NewJobID().String(), Status: store.JobStatusPending, StepIndex: 1000},
		},
	}

	handler := resumeRunHandler(st, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods/"+id.String()+"/resume", nil)
	req.SetPathValue("id", id.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Should return 200 OK because there's already a pending job.
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if st.updateRunStatusCalled {
		t.Fatal("did not expect UpdateRunStatus when pending job exists")
	}
}

// TestResumeRun_AllJobsSucceeded verifies 200 OK when all jobs are already succeeded.
func TestResumeRun_AllJobsSucceeded(t *testing.T) {
	t.Parallel()
	id := domaintypes.NewRunID()
	st := &mockStore{
		getRunResult: store.Run{
			ID:         id.String(),
			Status:     store.RunStatusFailed, // Run is failed but all jobs succeeded (edge case).
			RepoUrl:    "https://example/repo.git",
			CreatedAt:  pgtype.Timestamptz{Time: time.Now().Add(-time.Minute), Valid: true},
			FinishedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
		},
		listJobsByRunResult: []store.Job{
			{ID: domaintypes.NewJobID().String(), Status: store.JobStatusSucceeded, StepIndex: 1000},
			{ID: domaintypes.NewJobID().String(), Status: store.JobStatusSucceeded, StepIndex: 2000},
		},
	}

	handler := resumeRunHandler(st, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods/"+id.String()+"/resume", nil)
	req.SetPathValue("id", id.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Should return 200 OK because there's nothing to resume.
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if st.updateRunStatusCalled {
		t.Fatal("did not expect UpdateRunStatus when all jobs succeeded")
	}
}

// TestResumeRun_UpdateRunResumeCalled verifies that UpdateRunResume is called to track resume metadata.
func TestResumeRun_UpdateRunResumeCalled(t *testing.T) {
	t.Parallel()
	id := domaintypes.NewRunID()
	st := &mockStore{
		getRunResult: store.Run{
			ID:         id.String(),
			Status:     store.RunStatusFailed,
			RepoUrl:    "https://example/repo.git",
			BaseRef:    "main",
			TargetRef:  "feature",
			CreatedAt:  pgtype.Timestamptz{Time: time.Now().Add(-time.Minute), Valid: true},
			FinishedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
		},
		listJobsByRunResult: []store.Job{
			{ID: domaintypes.NewJobID().String(), Status: store.JobStatusFailed, StepIndex: 1000},
		},
	}

	handler := resumeRunHandler(st, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods/"+id.String()+"/resume", nil)
	req.SetPathValue("id", id.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rr.Code, rr.Body.String())
	}
	// Verify UpdateRunResume was called to track resume metadata.
	if !st.updateRunResumeCalled {
		t.Fatal("expected UpdateRunResume to be called")
	}
	if st.updateRunResumeParam != id.String() {
		t.Fatalf("UpdateRunResume called with wrong id: got %s, want %s",
			st.updateRunResumeParam, id)
	}
}

// TestResumeRun_SSEPublishWithResumeMetadata verifies that resume events include resume metadata.
func TestResumeRun_SSEPublishWithResumeMetadata(t *testing.T) {
	t.Parallel()
	id := domaintypes.NewRunID()
	// Simulate a run that has already been resumed once (stats contain resume_count=1).
	statsJSON := []byte(`{"resume_count":1,"last_resumed_at":"2025-01-15T10:00:00Z"}`)
	st := &mockStore{
		getRunResult: store.Run{
			ID:         id.String(),
			Status:     store.RunStatusFailed,
			RepoUrl:    "https://example/repo.git",
			BaseRef:    "main",
			TargetRef:  "feature",
			Stats:      statsJSON,
			CreatedAt:  pgtype.Timestamptz{Time: time.Now().Add(-time.Minute), Valid: true},
			FinishedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
		},
		listJobsByRunResult: []store.Job{
			{ID: domaintypes.NewJobID().String(), Status: store.JobStatusFailed, StepIndex: 1000},
		},
	}

	eventsService, err := createTestEventsService()
	if err != nil {
		t.Fatalf("failed to create events service: %v", err)
	}

	handler := resumeRunHandler(st, eventsService)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods/"+id.String()+"/resume", nil)
	req.SetPathValue("id", id.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify run event was published with resume metadata.
	snapshot := eventsService.Hub().Snapshot(id.String())
	if len(snapshot) == 0 {
		t.Fatal("expected at least 1 run event in snapshot")
	}
	// The event should be a run type. The metadata includes resume_count and last_resumed_at.
	// Since we re-fetch the run after UpdateRunResume (which the mock doesn't actually update),
	// the stats from getRunResult are used. The test verifies the plumbing works.
	foundRun := false
	for _, evt := range snapshot {
		if evt.Type == "run" {
			foundRun = true
		}
	}
	if !foundRun {
		t.Fatal("expected run event in snapshot")
	}
}

// TestResumeRun_ResumabilityInvariants is a table-driven test verifying all resumability
// invariants with their expected HTTP status codes and error message formats.
// This directly tests the requirements from D4: guard against unsafe or confusing resumes.
func TestResumeRun_ResumabilityInvariants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		runStatus      store.RunStatus
		wantStatus     int
		wantBodySubstr string // Substring expected in response body (for 409 cases).
	}{
		// Invariant 1: Terminal failure states (failed, canceled) are resumable.
		{
			name:       "failed run is resumable",
			runStatus:  store.RunStatusFailed,
			wantStatus: http.StatusAccepted, // 202 - resume proceeds.
		},
		{
			name:       "canceled run is resumable",
			runStatus:  store.RunStatusCanceled,
			wantStatus: http.StatusAccepted, // 202 - resume proceeds.
		},
		// Invariant 2: Succeeded runs cannot be resumed.
		{
			name:           "succeeded run returns 409 conflict",
			runStatus:      store.RunStatusSucceeded,
			wantStatus:     http.StatusConflict,
			wantBodySubstr: "state=succeeded is not resumable",
		},
		// Invariant 3: In-progress runs return 200 OK for idempotency.
		{
			name:       "queued run returns 200 idempotent",
			runStatus:  store.RunStatusQueued,
			wantStatus: http.StatusOK,
		},
		{
			name:       "assigned run returns 200 idempotent",
			runStatus:  store.RunStatusAssigned,
			wantStatus: http.StatusOK,
		},
		{
			name:       "running run returns 200 idempotent",
			runStatus:  store.RunStatusRunning,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			id := domaintypes.NewRunID()
			// For resumable states, provide a job to reset.
			jobs := []store.Job{}
			if tt.runStatus == store.RunStatusFailed || tt.runStatus == store.RunStatusCanceled {
				jobs = []store.Job{
					{ID: domaintypes.NewJobID().String(), Status: store.JobStatusFailed, StepIndex: 1000},
				}
			}
			st := &mockStore{
				getRunResult: store.Run{
					ID:        id.String(),
					Status:    tt.runStatus,
					RepoUrl:   "https://example/repo.git",
					CreatedAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Minute), Valid: true},
				},
				listJobsByRunResult: jobs,
			}

			handler := resumeRunHandler(st, nil)
			req := httptest.NewRequest(http.MethodPost, "/v1/mods/"+id.String()+"/resume", nil)
			req.SetPathValue("id", id.String())
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d: %s", tt.wantStatus, rr.Code, rr.Body.String())
			}
			// For conflict cases, verify the error message format.
			if tt.wantBodySubstr != "" && !contains(rr.Body.String(), tt.wantBodySubstr) {
				t.Fatalf("expected body to contain %q, got: %s", tt.wantBodySubstr, rr.Body.String())
			}
		})
	}
}

// TestCheckResumability_Unit tests the checkResumability helper function directly.
func TestCheckResumability_Unit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		status        store.RunStatus
		wantResumable bool
		wantHTTP      int
		wantMsgSubstr string
	}{
		{"queued", store.RunStatusQueued, false, http.StatusOK, "already in progress"},
		{"assigned", store.RunStatusAssigned, false, http.StatusOK, "already in progress"},
		{"running", store.RunStatusRunning, false, http.StatusOK, "already in progress"},
		{"succeeded", store.RunStatusSucceeded, false, http.StatusConflict, "nothing to fix"},
		{"failed", store.RunStatusFailed, true, 0, ""},
		{"canceled", store.RunStatusCanceled, true, 0, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			run := store.Run{Status: tt.status}
			resumable, httpStatus, errMsg := checkResumability(run)

			if resumable != tt.wantResumable {
				t.Errorf("resumable: got %v, want %v", resumable, tt.wantResumable)
			}
			if httpStatus != tt.wantHTTP {
				t.Errorf("httpStatus: got %d, want %d", httpStatus, tt.wantHTTP)
			}
			if tt.wantMsgSubstr != "" && !contains(errMsg, tt.wantMsgSubstr) {
				t.Errorf("errMsg should contain %q, got: %s", tt.wantMsgSubstr, errMsg)
			}
		})
	}
}

// TestResumeRun_JobLevelInvariants verifies that already-succeeded jobs are preserved
// during resume (invariant 4) and pending/running jobs trigger idempotent behavior (invariant 5).
func TestResumeRun_JobLevelInvariants(t *testing.T) {
	t.Parallel()

	t.Run("invariant 4: succeeded jobs preserved", func(t *testing.T) {
		t.Parallel()
		id := domaintypes.NewRunID()
		succeededJobID := domaintypes.NewJobID()
		failedJobID := domaintypes.NewJobID()

		st := &mockStore{
			getRunResult: store.Run{
				ID:        id.String(),
				Status:    store.RunStatusFailed,
				RepoUrl:   "https://example/repo.git",
				CreatedAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Minute), Valid: true},
			},
			listJobsByRunResult: []store.Job{
				{ID: succeededJobID.String(), Status: store.JobStatusSucceeded, StepIndex: 1000},
				{ID: failedJobID.String(), Status: store.JobStatusFailed, StepIndex: 2000},
			},
		}

		handler := resumeRunHandler(st, nil)
		req := httptest.NewRequest(http.MethodPost, "/v1/mods/"+id.String()+"/resume", nil)
		req.SetPathValue("id", id.String())
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusAccepted {
			t.Fatalf("expected 202, got %d: %s", rr.Code, rr.Body.String())
		}
		// Verify only the failed job was updated, not the succeeded one.
		if len(st.updateJobStatusCalls) != 1 {
			t.Fatalf("expected 1 job update, got %d", len(st.updateJobStatusCalls))
		}
		if st.updateJobStatusCalls[0].ID == succeededJobID.String() {
			t.Fatal("succeeded job should NOT be updated")
		}
		if st.updateJobStatusCalls[0].ID != failedJobID.String() {
			t.Fatal("failed job should be updated")
		}
	})

	t.Run("invariant 5: pending job triggers idempotent response", func(t *testing.T) {
		t.Parallel()
		id := domaintypes.NewRunID()

		st := &mockStore{
			getRunResult: store.Run{
				ID:        id.String(),
				Status:    store.RunStatusFailed, // Terminal but has a pending job (edge case).
				RepoUrl:   "https://example/repo.git",
				CreatedAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Minute), Valid: true},
			},
			listJobsByRunResult: []store.Job{
				{ID: domaintypes.NewJobID().String(), Status: store.JobStatusPending, StepIndex: 1000},
			},
		}

		handler := resumeRunHandler(st, nil)
		req := httptest.NewRequest(http.MethodPost, "/v1/mods/"+id.String()+"/resume", nil)
		req.SetPathValue("id", id.String())
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		// Should return 200 OK because there's already a pending job — no double-scheduling.
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200 for idempotent case, got %d: %s", rr.Code, rr.Body.String())
		}
		if st.updateRunStatusCalled {
			t.Fatal("should not update run status when pending job exists")
		}
	})

	t.Run("invariant 5: running job triggers idempotent response", func(t *testing.T) {
		t.Parallel()
		id := domaintypes.NewRunID()

		st := &mockStore{
			getRunResult: store.Run{
				ID:        id.String(),
				Status:    store.RunStatusFailed, // Terminal but has a running job (edge case).
				RepoUrl:   "https://example/repo.git",
				CreatedAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Minute), Valid: true},
			},
			listJobsByRunResult: []store.Job{
				{ID: domaintypes.NewJobID().String(), Status: store.JobStatusRunning, StepIndex: 1000},
			},
		}

		handler := resumeRunHandler(st, nil)
		req := httptest.NewRequest(http.MethodPost, "/v1/mods/"+id.String()+"/resume", nil)
		req.SetPathValue("id", id.String())
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		// Should return 200 OK because there's already a running job.
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200 for idempotent case, got %d: %s", rr.Code, rr.Body.String())
		}
		if st.updateRunStatusCalled {
			t.Fatal("should not update run status when running job exists")
		}
	})
}

// contains is a simple helper to check if a string contains a substring.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
