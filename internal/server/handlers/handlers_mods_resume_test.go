package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/store"
)

// TestResumeTicket_FailedRun verifies that a failed run can be resumed.
func TestResumeTicket_FailedRun(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	jobID := uuid.New()
	st := &mockStore{
		getRunResult: store.Run{
			ID:         pgtype.UUID{Bytes: id, Valid: true},
			Status:     store.RunStatusFailed,
			RepoUrl:    "https://example/repo.git",
			CreatedAt:  pgtype.Timestamptz{Time: time.Now().Add(-time.Minute), Valid: true},
			FinishedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
		},
		listJobsByRunResult: []store.Job{
			{
				ID:         pgtype.UUID{Bytes: jobID, Valid: true},
				Status:     store.JobStatusFailed,
				StepIndex:  1000,
				StartedAt:  pgtype.Timestamptz{Time: time.Now().Add(-30 * time.Second), Valid: true},
				FinishedAt: pgtype.Timestamptz{Time: time.Now().Add(-10 * time.Second), Valid: true},
			},
		},
	}

	handler := resumeTicketHandler(st, nil)

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

// TestResumeTicket_CanceledRun verifies that a canceled run can be resumed.
func TestResumeTicket_CanceledRun(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	st := &mockStore{
		getRunResult: store.Run{
			ID:         pgtype.UUID{Bytes: id, Valid: true},
			Status:     store.RunStatusCanceled,
			RepoUrl:    "https://example/repo.git",
			CreatedAt:  pgtype.Timestamptz{Time: time.Now().Add(-time.Minute), Valid: true},
			FinishedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
		},
		listJobsByRunResult: []store.Job{
			{ID: pgtype.UUID{Bytes: uuid.New(), Valid: true}, Status: store.JobStatusCanceled, StepIndex: 1000},
		},
	}

	handler := resumeTicketHandler(st, nil)

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

// TestResumeTicket_Idempotent_AlreadyRunning verifies 200 when run is already in progress.
func TestResumeTicket_Idempotent_AlreadyRunning(t *testing.T) {
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
			id := uuid.New()
			st := &mockStore{getRunResult: store.Run{ID: pgtype.UUID{Bytes: id, Valid: true}, Status: tt.runStatus}}
			handler := resumeTicketHandler(st, nil)
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

// TestResumeTicket_SucceededConflict verifies 409 when trying to resume a succeeded run.
func TestResumeTicket_SucceededConflict(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	st := &mockStore{getRunResult: store.Run{ID: pgtype.UUID{Bytes: id, Valid: true}, Status: store.RunStatusSucceeded}}
	handler := resumeTicketHandler(st, nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/mods/"+id.String()+"/resume", nil)
	req.SetPathValue("id", id.String())
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestResumeTicket_BadID_And_NotFound tests error handling for invalid IDs and missing tickets.
func TestResumeTicket_BadID_And_NotFound(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		id         string
		mockStore  *mockStore
		wantStatus int
		wantBody   string
	}{
		{
			name:       "invalid uuid format",
			id:         "abc",
			mockStore:  &mockStore{},
			wantStatus: http.StatusBadRequest,
			wantBody:   "invalid id: invalid uuid",
		},
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
			name:       "ticket not found",
			id:         uuid.New().String(),
			mockStore:  &mockStore{getRunErr: pgx.ErrNoRows},
			wantStatus: http.StatusNotFound,
			wantBody:   "ticket not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			handler := resumeTicketHandler(tt.mockStore, nil)
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

// TestResumeTicket_PartiallySucceeded resumes a run where some jobs succeeded and one failed.
func TestResumeTicket_PartiallySucceeded(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	successJob := store.Job{ID: pgtype.UUID{Bytes: uuid.New(), Valid: true}, Status: store.JobStatusSucceeded, StepIndex: 1000}
	failedJob := store.Job{ID: pgtype.UUID{Bytes: uuid.New(), Valid: true}, Status: store.JobStatusFailed, StepIndex: 2000}
	createdJob := store.Job{ID: pgtype.UUID{Bytes: uuid.New(), Valid: true}, Status: store.JobStatusCreated, StepIndex: 3000}

	st := &mockStore{
		getRunResult: store.Run{
			ID:         pgtype.UUID{Bytes: id, Valid: true},
			Status:     store.RunStatusFailed,
			RepoUrl:    "https://example/repo.git",
			CreatedAt:  pgtype.Timestamptz{Time: time.Now().Add(-time.Minute), Valid: true},
			FinishedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
		},
		listJobsByRunResult: []store.Job{successJob, failedJob, createdJob},
	}

	handler := resumeTicketHandler(st, nil)

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
	if uuid.UUID(call.ID.Bytes) != uuid.UUID(failedJob.ID.Bytes) {
		t.Fatalf("expected failed job to be updated, got %s", uuid.UUID(call.ID.Bytes))
	}
	if call.Status != store.JobStatusPending {
		t.Fatalf("expected failed job status pending, got %s", call.Status)
	}
}

// TestResumeTicket_MultipleFailedJobs verifies correct ordering when multiple jobs need reset.
func TestResumeTicket_MultipleFailedJobs(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	job1 := store.Job{ID: pgtype.UUID{Bytes: uuid.New(), Valid: true}, Status: store.JobStatusSucceeded, StepIndex: 1000}
	job2 := store.Job{ID: pgtype.UUID{Bytes: uuid.New(), Valid: true}, Status: store.JobStatusFailed, StepIndex: 2000}
	job3 := store.Job{ID: pgtype.UUID{Bytes: uuid.New(), Valid: true}, Status: store.JobStatusCanceled, StepIndex: 3000}

	st := &mockStore{
		getRunResult: store.Run{
			ID:         pgtype.UUID{Bytes: id, Valid: true},
			Status:     store.RunStatusFailed,
			RepoUrl:    "https://example/repo.git",
			CreatedAt:  pgtype.Timestamptz{Time: time.Now().Add(-time.Minute), Valid: true},
			FinishedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
		},
		listJobsByRunResult: []store.Job{job1, job2, job3},
	}

	handler := resumeTicketHandler(st, nil)

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
		statusByJob[uuid.UUID(call.ID.Bytes).String()] = call.Status
	}
	if statusByJob[uuid.UUID(job2.ID.Bytes).String()] != store.JobStatusPending {
		t.Fatalf("expected job2 to be pending, got %s", statusByJob[uuid.UUID(job2.ID.Bytes).String()])
	}
	if statusByJob[uuid.UUID(job3.ID.Bytes).String()] != store.JobStatusCreated {
		t.Fatalf("expected job3 to be created, got %s", statusByJob[uuid.UUID(job3.ID.Bytes).String()])
	}
}

// TestResumeTicket_SSEPublish verifies that resume publishes ticket events.
func TestResumeTicket_SSEPublish(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	st := &mockStore{
		getRunResult: store.Run{
			ID:         pgtype.UUID{Bytes: id, Valid: true},
			Status:     store.RunStatusFailed,
			RepoUrl:    "https://example/repo.git",
			CreatedAt:  pgtype.Timestamptz{Time: time.Now().Add(-time.Minute), Valid: true},
			FinishedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
		},
		listJobsByRunResult: []store.Job{
			{ID: pgtype.UUID{Bytes: uuid.New(), Valid: true}, Status: store.JobStatusFailed, StepIndex: 1000},
		},
	}

	eventsService, err := createTestEventsService()
	if err != nil {
		t.Fatalf("failed to create events service: %v", err)
	}

	handler := resumeTicketHandler(st, eventsService)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods/"+id.String()+"/resume", nil)
	req.SetPathValue("id", id.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify ticket event was published.
	snapshot := eventsService.Hub().Snapshot(id.String())
	if len(snapshot) == 0 {
		t.Fatal("expected at least 1 ticket event in snapshot")
	}
	foundTicket := false
	for _, evt := range snapshot {
		if evt.Type == "ticket" {
			foundTicket = true
		}
	}
	if !foundTicket {
		t.Fatal("expected ticket event in snapshot")
	}
}

// TestResumeTicket_IdempotentWhenPendingJobExists verifies 200 OK when a pending job already exists.
func TestResumeTicket_IdempotentWhenPendingJobExists(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	st := &mockStore{
		getRunResult: store.Run{
			ID:         pgtype.UUID{Bytes: id, Valid: true},
			Status:     store.RunStatusFailed, // Run is terminal but has a pending job (edge case).
			RepoUrl:    "https://example/repo.git",
			CreatedAt:  pgtype.Timestamptz{Time: time.Now().Add(-time.Minute), Valid: true},
			FinishedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
		},
		listJobsByRunResult: []store.Job{
			{ID: pgtype.UUID{Bytes: uuid.New(), Valid: true}, Status: store.JobStatusPending, StepIndex: 1000},
		},
	}

	handler := resumeTicketHandler(st, nil)

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

// TestResumeTicket_AllJobsSucceeded verifies 200 OK when all jobs are already succeeded.
func TestResumeTicket_AllJobsSucceeded(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	st := &mockStore{
		getRunResult: store.Run{
			ID:         pgtype.UUID{Bytes: id, Valid: true},
			Status:     store.RunStatusFailed, // Run is failed but all jobs succeeded (edge case).
			RepoUrl:    "https://example/repo.git",
			CreatedAt:  pgtype.Timestamptz{Time: time.Now().Add(-time.Minute), Valid: true},
			FinishedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
		},
		listJobsByRunResult: []store.Job{
			{ID: pgtype.UUID{Bytes: uuid.New(), Valid: true}, Status: store.JobStatusSucceeded, StepIndex: 1000},
			{ID: pgtype.UUID{Bytes: uuid.New(), Valid: true}, Status: store.JobStatusSucceeded, StepIndex: 2000},
		},
	}

	handler := resumeTicketHandler(st, nil)

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
