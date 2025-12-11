package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// TestMaybeUpdateRunRepoFromExecution verifies the completion callback updates RunRepo status.
func TestMaybeUpdateRunRepoFromExecution(t *testing.T) {
	t.Parallel()

	sampleRunRepoID := domaintypes.NewRunRepoID()
	sampleBatchRunID := domaintypes.NewRunID()
	sampleExecutionRunID := domaintypes.NewRunID()

	executionRunIDStr := sampleExecutionRunID.String()
	linkedRunRepo := store.RunRepo{
		ID:             sampleRunRepoID.String(),
		RunID:          sampleBatchRunID,
		ExecutionRunID: &executionRunIDStr,
		RepoUrl:        "https://github.com/example/repo.git",
		Status:         store.RunRepoStatusRunning,
		Attempt:        1,
		CreatedAt:      pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	tests := []struct {
		name             string
		runStatus        store.RunStatus
		mockRunRepo      store.RunRepo
		mockRunRepoErr   error
		mockUpdateErr    error
		wantRepoStatus   store.RunRepoStatus
		wantUpdateCalled bool
		wantErr          bool
	}{
		{
			name:             "succeeded execution updates repo to succeeded",
			runStatus:        store.RunStatusSucceeded,
			mockRunRepo:      linkedRunRepo,
			wantRepoStatus:   store.RunRepoStatusSucceeded,
			wantUpdateCalled: true,
		},
		{
			name:             "failed execution updates repo to failed",
			runStatus:        store.RunStatusFailed,
			mockRunRepo:      linkedRunRepo,
			wantRepoStatus:   store.RunRepoStatusFailed,
			wantUpdateCalled: true,
		},
		{
			name:             "canceled execution updates repo to cancelled",
			runStatus:        store.RunStatusCanceled,
			mockRunRepo:      linkedRunRepo,
			wantRepoStatus:   store.RunRepoStatusCancelled,
			wantUpdateCalled: true,
		},
		{
			name:             "standalone run (no linked run_repo) — no update",
			runStatus:        store.RunStatusSucceeded,
			mockRunRepoErr:   pgx.ErrNoRows,
			wantUpdateCalled: false,
			wantErr:          false, // Not an error — expected for standalone runs.
		},
		{
			name:             "lookup error propagates",
			runStatus:        store.RunStatusSucceeded,
			mockRunRepoErr:   pgx.ErrTxClosed,
			wantUpdateCalled: false,
			wantErr:          true,
		},
		{
			name:             "update error propagates",
			runStatus:        store.RunStatusSucceeded,
			mockRunRepo:      linkedRunRepo,
			mockUpdateErr:    pgx.ErrTxClosed,
			wantRepoStatus:   store.RunRepoStatusSucceeded, // We still try to update to succeeded.
			wantUpdateCalled: true,
			wantErr:          true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := &mockStore{
				getRunRepoByExecutionRunResult: tc.mockRunRepo,
				getRunRepoByExecutionRunErr:    tc.mockRunRepoErr,
				updateRunRepoStatusErr:         tc.mockUpdateErr,
			}

			execRunID := domaintypes.RunID(sampleExecutionRunID.String())
			err := maybeUpdateRunRepoFromExecution(context.Background(), m, execRunID, tc.runStatus)

			if tc.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}

			if tc.wantUpdateCalled {
				if !m.updateRunRepoStatusCalled {
					t.Error("expected UpdateRunRepoStatus to be called")
				} else if len(m.updateRunRepoStatusParams) > 0 {
					updatedStatus := m.updateRunRepoStatusParams[0].Status
					if updatedStatus != tc.wantRepoStatus {
						t.Errorf("updated status = %s, want %s", updatedStatus, tc.wantRepoStatus)
					}
				}
			} else if m.updateRunRepoStatusCalled {
				t.Error("expected UpdateRunRepoStatus NOT to be called")
			}
		})
	}
}

// =========================================================================
// Focused batch run workflow tests (ROADMAP.md line 267):
// Covers happy path and error paths for batch run operations.
// =========================================================================

// TestBatchRunWorkflow_HappyPath exercises a complete batch run lifecycle:
// 1. Create a batch run (list runs empty → create)
// 2. Add two repos to the batch
// 3. Start execution → repos transition to running
// 4. Mark child runs as succeeded → repos marked succeeded
// 5. Batch summary shows correct counts and derived status
//
// This test simulates the end-to-end workflow using mock store interactions.
func TestBatchRunWorkflow_HappyPath(t *testing.T) {
	t.Parallel()

	// Sample KSUID/NanoID-based IDs for the batch run and repos.
	batchRunID := domaintypes.NewRunID()
	repo1ID := domaintypes.NewRunRepoID()
	repo2ID := domaintypes.NewRunRepoID()
	childRun1ID := domaintypes.NewRunID()

	// Batch run (parent) with queued status.
	batchRun := store.Run{
		ID:        batchRunID.String(),
		Name:      ptrString("integration-batch"),
		RepoUrl:   "https://github.com/batch/placeholder.git",
		Status:    store.RunStatusQueued,
		Spec:      []byte(`{"mod":{"image":"test-image"}}`),
		BaseRef:   "main",
		TargetRef: "feature",
		CreatedBy: ptrString("test-user"),
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	// Run repos representing individual repositories in the batch.
	pendingRepo1 := store.RunRepo{
		ID:        repo1ID.String(),
		RunID:     batchRunID,
		RepoUrl:   "https://github.com/org/repo1.git",
		BaseRef:   "main",
		TargetRef: "feature-1",
		Status:    store.RunRepoStatusPending,
		Attempt:   1,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	pendingRepo2 := store.RunRepo{
		ID:        repo2ID.String(),
		RunID:     batchRunID,
		RepoUrl:   "https://github.com/org/repo2.git",
		BaseRef:   "main",
		TargetRef: "feature-2",
		Status:    store.RunRepoStatusPending,
		Attempt:   1,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	// Child runs created when batch execution starts.
	childRun1 := store.Run{
		ID:        childRun1ID.String(),
		RepoUrl:   "https://github.com/org/repo1.git",
		Status:    store.RunStatusQueued,
		BaseRef:   "main",
		TargetRef: "feature-1",
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	// Test scenario 1: List runs — returns the batch run with repo counts.
	t.Run("list runs with repos", func(t *testing.T) {
		t.Parallel()

		m := &mockStore{
			listRunsResult: []store.Run{batchRun},
			countRunReposByStatusResult: []store.CountRunReposByStatusRow{
				{Status: store.RunRepoStatusPending, Count: 2},
			},
		}

		handler := listRunsHandler(m)
		req := httptest.NewRequest(http.MethodGet, "/v1/runs", nil)
		w := httptest.NewRecorder()

		handler(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
		}

		var resp struct {
			Runs []RunSummary `json:"runs"`
		}
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}

		// Verify batch run is returned with repo counts.
		if len(resp.Runs) != 1 {
			t.Fatalf("expected 1 run, got %d", len(resp.Runs))
		}
		if resp.Runs[0].Counts == nil {
			t.Fatal("expected repo counts to be populated")
		}
		if resp.Runs[0].Counts.Total != 2 {
			t.Errorf("total = %d, want 2", resp.Runs[0].Counts.Total)
		}
		if resp.Runs[0].Counts.Pending != 2 {
			t.Errorf("pending = %d, want 2", resp.Runs[0].Counts.Pending)
		}
		// Before starting, derived status should be "pending".
		if resp.Runs[0].Counts.DerivedStatus != DerivedStatusPending {
			t.Errorf("derived_status = %s, want %s", resp.Runs[0].Counts.DerivedStatus, DerivedStatusPending)
		}
	})

	// Test scenario 2: Start batch — pending repos transition to running.
	t.Run("start batch creates child runs", func(t *testing.T) {
		t.Parallel()

		m := &mockStore{
			getRunResult:                   batchRun,
			listRunReposByRunResult:        []store.RunRepo{pendingRepo1, pendingRepo2},
			listPendingRunReposByRunResult: []store.RunRepo{pendingRepo1, pendingRepo2},
			createRunResult:                childRun1,
		}

		handler := startRunHandler(m)
		req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+batchRunID.String()+"/start", nil)
		req.SetPathValue("id", batchRunID.String())
		w := httptest.NewRecorder()

		handler(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
		}

		var resp StartRunResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}

		// Both repos should have started.
		if resp.Started != 2 {
			t.Errorf("started = %d, want 2", resp.Started)
		}
		if resp.AlreadyDone != 0 {
			t.Errorf("already_done = %d, want 0", resp.AlreadyDone)
		}
		if resp.Pending != 0 {
			t.Errorf("pending = %d, want 0", resp.Pending)
		}

		// Verify store calls: CreateRun called for each repo.
		if !m.createRunCalled {
			t.Error("expected CreateRun to be called")
		}
		// Verify SetRunRepoExecutionRun called to link repos to child runs.
		if !m.setRunRepoExecutionRunCalled {
			t.Error("expected SetRunRepoExecutionRun to be called")
		}
		// Verify AckRunStart called to transition batch to running.
		if !m.ackRunStartCalled {
			t.Error("expected AckRunStart to be called")
		}
	})

	// Test scenario 3: Batch completion — all repos succeeded.
	t.Run("batch completes when all repos succeed", func(t *testing.T) {
		t.Parallel()

		// Mock store returns succeeded status for both repos.
		m := &mockStore{
			getRunResult: batchRun,
			countRunReposByStatusResult: []store.CountRunReposByStatusRow{
				{Status: store.RunRepoStatusSucceeded, Count: 2},
			},
		}

		handler := getRunHandler(m)
		req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+batchRunID.String(), nil)
		req.SetPathValue("id", batchRunID.String())
		w := httptest.NewRecorder()

		handler(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
		}

		var resp RunSummary
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}

		// Verify batch summary shows completed status.
		if resp.Counts == nil {
			t.Fatal("expected repo counts to be populated")
		}
		if resp.Counts.Total != 2 {
			t.Errorf("total = %d, want 2", resp.Counts.Total)
		}
		if resp.Counts.Succeeded != 2 {
			t.Errorf("succeeded = %d, want 2", resp.Counts.Succeeded)
		}
		if resp.Counts.DerivedStatus != DerivedStatusCompleted {
			t.Errorf("derived_status = %s, want %s", resp.Counts.DerivedStatus, DerivedStatusCompleted)
		}
	})

	// Test scenario 4: Partial failure — one repo fails.
	t.Run("batch failed when some repos fail", func(t *testing.T) {
		t.Parallel()

		m := &mockStore{
			getRunResult: batchRun,
			countRunReposByStatusResult: []store.CountRunReposByStatusRow{
				{Status: store.RunRepoStatusSucceeded, Count: 1},
				{Status: store.RunRepoStatusFailed, Count: 1},
			},
		}

		handler := getRunHandler(m)
		req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+batchRunID.String(), nil)
		req.SetPathValue("id", batchRunID.String())
		w := httptest.NewRecorder()

		handler(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
		}

		var resp RunSummary
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}

		if resp.Counts == nil {
			t.Fatal("expected repo counts")
		}
		// Derived status should be "failed" when any repo fails.
		if resp.Counts.DerivedStatus != DerivedStatusFailed {
			t.Errorf("derived_status = %s, want %s", resp.Counts.DerivedStatus, DerivedStatusFailed)
		}
	})
}

// TestBatchRunWorkflow_ErrorPaths exercises error handling scenarios:
// - Invalid repo URL when adding a repo
// - Unknown run ID in various operations
// - Restart on non-terminal repo
func TestBatchRunWorkflow_ErrorPaths(t *testing.T) {
	t.Parallel()

	batchRunID := domaintypes.NewRunID()
	repoID := domaintypes.NewRunRepoID()
	unknownID := domaintypes.NewRunRepoID()

	runningRun := store.Run{
		ID:        batchRunID.String(),
		Name:      ptrString("test-batch"),
		RepoUrl:   "https://github.com/batch/placeholder.git",
		Status:    store.RunStatusRunning,
		BaseRef:   "main",
		TargetRef: "feature",
		CreatedBy: ptrString("test-user"),
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	failedRepo := store.RunRepo{
		ID:        repoID.String(),
		RunID:     batchRunID,
		RepoUrl:   "https://github.com/org/repo.git",
		Status:    store.RunRepoStatusFailed,
		Attempt:   1,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	runningRepo := store.RunRepo{
		ID:        repoID.String(),
		RunID:     batchRunID,
		RepoUrl:   "https://github.com/org/repo.git",
		Status:    store.RunRepoStatusRunning,
		Attempt:   1,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	// Error path 1: Add repo with invalid URL scheme.
	t.Run("add repo with invalid URL", func(t *testing.T) {
		t.Parallel()

		m := &mockStore{
			getRunResult: runningRun,
		}

		handler := addRunRepoHandler(m)
		body := `{"repo_url":"ftp://example.com/repo.git","base_ref":"main","target_ref":"feature"}`
		req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+batchRunID.String()+"/repos", strings.NewReader(body))
		req.SetPathValue("id", batchRunID.String())
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusBadRequest, w.Body.String())
		}
	})

	// Error path 2: Get unknown run.
	t.Run("get unknown run returns 404", func(t *testing.T) {
		t.Parallel()

		m := &mockStore{
			getRunErr: pgx.ErrNoRows,
		}

		handler := getRunHandler(m)
		req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+unknownID.String(), nil)
		req.SetPathValue("id", unknownID.String())
		w := httptest.NewRecorder()

		handler(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
		}
	})

	// Error path 3: Restart non-terminal repo (pending).
	t.Run("restart pending repo returns conflict", func(t *testing.T) {
		t.Parallel()

		m := &mockStore{
			getRunResult: runningRun,
			getRunRepoResult: store.RunRepo{
				ID:        repoID.String(),
				RunID:     batchRunID,
				Status:    store.RunRepoStatusPending,
				CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
			},
		}

		handler := restartRunRepoHandler(m)
		req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+batchRunID.String()+"/repos/"+repoID.String()+"/restart", nil)
		req.SetPathValue("id", batchRunID.String())
		req.SetPathValue("repo_id", repoID.String())
		w := httptest.NewRecorder()

		handler(w, req)

		// Should return 409 Conflict — can only restart terminal repos.
		if w.Code != http.StatusConflict {
			t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusConflict, w.Body.String())
		}
	})

	// Error path 4: Restart failed repo but run is canceled.
	t.Run("restart repo in canceled run returns conflict", func(t *testing.T) {
		t.Parallel()

		canceledRun := runningRun
		canceledRun.Status = store.RunStatusCanceled

		m := &mockStore{
			getRunResult:     canceledRun,
			getRunRepoResult: failedRepo,
		}

		handler := restartRunRepoHandler(m)
		req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+batchRunID.String()+"/repos/"+repoID.String()+"/restart", nil)
		req.SetPathValue("id", batchRunID.String())
		req.SetPathValue("repo_id", repoID.String())
		w := httptest.NewRecorder()

		handler(w, req)

		if w.Code != http.StatusConflict {
			t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusConflict, w.Body.String())
		}
	})

	// Error path 5: Restart running repo (not terminal, should fail).
	t.Run("restart running repo returns conflict", func(t *testing.T) {
		t.Parallel()

		m := &mockStore{
			getRunResult:     runningRun,
			getRunRepoResult: runningRepo,
		}

		handler := restartRunRepoHandler(m)
		req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+batchRunID.String()+"/repos/"+repoID.String()+"/restart", nil)
		req.SetPathValue("id", batchRunID.String())
		req.SetPathValue("repo_id", repoID.String())
		w := httptest.NewRecorder()

		handler(w, req)

		// Should return 409 Conflict — running repos cannot be restarted.
		if w.Code != http.StatusConflict {
			t.Errorf("status = %d, want %d; body: %s", w.Code, http.StatusConflict, w.Body.String())
		}
	})

	// Error path 6: Stop unknown run.
	t.Run("stop unknown run returns 404", func(t *testing.T) {
		t.Parallel()

		m := &mockStore{
			getRunErr: pgx.ErrNoRows,
		}

		handler := stopRunHandler(m)
		req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+unknownID.String()+"/stop", nil)
		req.SetPathValue("id", unknownID.String())
		w := httptest.NewRecorder()

		handler(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
		}
	})

	// Error path 7: Start execution for unknown run.
	t.Run("start unknown run returns 404", func(t *testing.T) {
		t.Parallel()

		m := &mockStore{
			getRunErr: pgx.ErrNoRows,
		}

		handler := startRunHandler(m)
		req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+unknownID.String()+"/start", nil)
		req.SetPathValue("id", unknownID.String())
		w := httptest.NewRecorder()

		handler(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
		}
	})
}
