package handlers

import (
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

// NOTE: Run IDs are now KSUID strings; run_repo IDs are NanoID(8) strings.
// Tests use string IDs generated via domaintypes helpers (NewRunID, NewRunRepoID).

// TestListRunsHandler verifies the GET /v1/runs handler with various scenarios.
func TestListRunsHandler(t *testing.T) {
	t.Parallel()

	// Sample run for testing — use KSUID string ID.
	sampleRunID := string(domaintypes.NewRunID())
	sampleRun := store.Run{
		ID:        sampleRunID,
		Name:      ptrString("test-batch"),
		RepoUrl:   "https://github.com/example/repo.git",
		Status:    store.RunStatusQueued,
		BaseRef:   "main",
		TargetRef: "feature",
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	tests := []struct {
		name           string
		query          string
		mockRuns       []store.Run
		mockRunsErr    error
		mockRepoCounts []store.CountRunReposByStatusRow
		wantStatus     int
		wantRunCount   int
	}{
		{
			name:         "empty list",
			query:        "",
			mockRuns:     []store.Run{},
			wantStatus:   http.StatusOK,
			wantRunCount: 0,
		},
		{
			name:         "single run without repos",
			query:        "",
			mockRuns:     []store.Run{sampleRun},
			wantStatus:   http.StatusOK,
			wantRunCount: 1,
		},
		{
			name:     "single run with repo counts",
			query:    "",
			mockRuns: []store.Run{sampleRun},
			mockRepoCounts: []store.CountRunReposByStatusRow{
				{Status: store.RunRepoStatusPending, Count: 2},
				{Status: store.RunRepoStatusSucceeded, Count: 1},
			},
			wantStatus:   http.StatusOK,
			wantRunCount: 1,
		},
		{
			name:         "pagination with limit",
			query:        "?limit=10",
			mockRuns:     []store.Run{sampleRun},
			wantStatus:   http.StatusOK,
			wantRunCount: 1,
		},
		{
			name:         "pagination with offset",
			query:        "?limit=10&offset=5",
			mockRuns:     []store.Run{},
			wantStatus:   http.StatusOK,
			wantRunCount: 0,
		},
		{
			name:       "invalid limit",
			query:      "?limit=abc",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid offset",
			query:      "?offset=-1",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:        "database error",
			query:       "",
			mockRunsErr: pgx.ErrTxClosed,
			wantStatus:  http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup mock store.
			m := &mockStore{
				listRunsResult:              tc.mockRuns,
				listRunsErr:                 tc.mockRunsErr,
				countRunReposByStatusResult: tc.mockRepoCounts,
			}

			// Create handler and request.
			handler := listRunsHandler(m)
			req := httptest.NewRequest(http.MethodGet, "/v1/runs"+tc.query, nil)
			w := httptest.NewRecorder()

			// Execute handler.
			handler(w, req)

			// Check status code.
			if w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tc.wantStatus)
			}

			// For successful responses, verify run count.
			if tc.wantStatus == http.StatusOK {
				var resp struct {
					Runs []RunSummary `json:"runs"`
				}
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("decode response: %v", err)
				}
				if len(resp.Runs) != tc.wantRunCount {
					t.Errorf("run count = %d, want %d", len(resp.Runs), tc.wantRunCount)
				}

				// Verify repo counts are populated when available.
				if len(tc.mockRepoCounts) > 0 && len(resp.Runs) > 0 {
					if resp.Runs[0].Counts == nil {
						t.Error("expected repo counts to be populated")
					} else if resp.Runs[0].Counts.Total != 3 {
						t.Errorf("total count = %d, want 3", resp.Runs[0].Counts.Total)
					}
				}
			}
		})
	}
}

// TestGetRunHandler verifies the GET /v1/runs/{id} handler.
func TestGetRunHandler(t *testing.T) {
	t.Parallel()

	// Use KSUID string for run IDs.
	sampleRunID := string(domaintypes.NewRunID())
	sampleRun := store.Run{
		ID:        sampleRunID,
		Name:      ptrString("test-batch"),
		RepoUrl:   "https://github.com/example/repo.git",
		Status:    store.RunStatusRunning,
		BaseRef:   "main",
		TargetRef: "feature",
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		StartedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	tests := []struct {
		name           string
		runID          string
		mockRun        store.Run
		mockRunErr     error
		mockRepoCounts []store.CountRunReposByStatusRow
		wantStatus     int
	}{
		{
			name:       "valid run",
			runID:      sampleRunID,
			mockRun:    sampleRun,
			wantStatus: http.StatusOK,
		},
		{
			name:    "run with repo counts",
			runID:   sampleRunID,
			mockRun: sampleRun,
			mockRepoCounts: []store.CountRunReposByStatusRow{
				{Status: store.RunRepoStatusRunning, Count: 1},
				{Status: store.RunRepoStatusPending, Count: 2},
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "empty id",
			runID:      "",
			wantStatus: http.StatusBadRequest,
		},
		// NOTE: "invalid uuid" test removed — run IDs are now opaque KSUID strings,
		// no UUID parsing is performed; validation is done by the store.
		{
			name:       "run not found",
			runID:      string(domaintypes.NewRunID()), // Generate a different KSUID.
			mockRunErr: pgx.ErrNoRows,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "database error",
			runID:      sampleRunID,
			mockRunErr: pgx.ErrTxClosed,
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := &mockStore{
				getRunResult:                tc.mockRun,
				getRunErr:                   tc.mockRunErr,
				countRunReposByStatusResult: tc.mockRepoCounts,
			}

			handler := getRunHandler(m)
			req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+tc.runID, nil)
			req.SetPathValue("id", tc.runID)
			w := httptest.NewRecorder()

			handler(w, req)

			if w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d; body: %s", w.Code, tc.wantStatus, w.Body.String())
			}

			// Verify response structure for successful requests.
			if tc.wantStatus == http.StatusOK {
				var resp RunSummary
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("decode response: %v", err)
				}
				// Compare domain type ID with expected string using .String() method.
				if resp.ID.String() != tc.runID {
					t.Errorf("id = %s, want %s", resp.ID.String(), tc.runID)
				}
				if resp.Status != string(tc.mockRun.Status) {
					t.Errorf("status = %s, want %s", resp.Status, string(tc.mockRun.Status))
				}

				// Check repo counts if expected.
				if len(tc.mockRepoCounts) > 0 {
					if resp.Counts == nil {
						t.Error("expected repo counts to be populated")
					} else if resp.Counts.Total != 3 {
						t.Errorf("total count = %d, want 3", resp.Counts.Total)
					}
				}
			}
		})
	}
}

// TestStopRunHandler verifies the POST /v1/runs/{id}/stop handler.
func TestStopRunHandler(t *testing.T) {
	t.Parallel()

	// Use KSUID strings for run IDs and NanoID strings for repo IDs.
	sampleRunID := string(domaintypes.NewRunID())
	pendingRepoID := string(domaintypes.NewRunRepoID())
	runningRepoID := string(domaintypes.NewRunRepoID())

	runningRun := store.Run{
		ID:        sampleRunID,
		Name:      ptrString("test-batch"),
		RepoUrl:   "https://github.com/example/repo.git",
		Status:    store.RunStatusRunning,
		BaseRef:   "main",
		TargetRef: "feature",
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		StartedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	canceledRun := store.Run{
		ID:         sampleRunID,
		Name:       ptrString("test-batch"),
		RepoUrl:    "https://github.com/example/repo.git",
		Status:     store.RunStatusCanceled,
		BaseRef:    "main",
		TargetRef:  "feature",
		CreatedAt:  pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		StartedAt:  pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		FinishedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	tests := []struct {
		name             string
		runID            string
		mockRun          store.Run
		mockRunErr       error
		mockRepos        []store.RunRepo
		updateStatusErr  error
		wantStatus       int
		wantCanceledRun  bool
		wantReposUpdated int
	}{
		{
			name:    "stop running run",
			runID:   sampleRunID,
			mockRun: runningRun,
			mockRepos: []store.RunRepo{
				{
					ID:     pendingRepoID,
					RunID:  domaintypes.RunID(sampleRunID),
					Status: store.RunRepoStatusPending,
				},
				{
					ID:     runningRepoID,
					RunID:  domaintypes.RunID(sampleRunID),
					Status: store.RunRepoStatusRunning,
				},
			},
			wantStatus:       http.StatusOK,
			wantCanceledRun:  true,
			wantReposUpdated: 1, // Only pending repo is updated
		},
		{
			name:            "stop already canceled run (idempotent)",
			runID:           sampleRunID,
			mockRun:         canceledRun,
			wantStatus:      http.StatusOK,
			wantCanceledRun: false, // Already canceled, no update
		},
		{
			name:       "empty id",
			runID:      "",
			wantStatus: http.StatusBadRequest,
		},
		// NOTE: "invalid uuid" test removed — run IDs are now opaque KSUID strings.
		{
			name:       "run not found",
			runID:      string(domaintypes.NewRunID()), // Generate a different KSUID.
			mockRunErr: pgx.ErrNoRows,
			wantStatus: http.StatusNotFound,
		},
		{
			name:            "update status error",
			runID:           sampleRunID,
			mockRun:         runningRun,
			updateStatusErr: pgx.ErrTxClosed,
			wantStatus:      http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := &mockStore{
				getRunResult:            tc.mockRun,
				getRunErr:               tc.mockRunErr,
				updateRunStatusErr:      tc.updateStatusErr,
				listRunReposByRunResult: tc.mockRepos,
			}

			handler := stopRunHandler(m)
			req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+tc.runID+"/stop", nil)
			req.SetPathValue("id", tc.runID)
			w := httptest.NewRecorder()

			handler(w, req)

			if w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d; body: %s", w.Code, tc.wantStatus, w.Body.String())
			}

			// Verify run status was updated.
			if tc.wantCanceledRun {
				if !m.updateRunStatusCalled {
					t.Error("expected UpdateRunStatus to be called")
				}
				if m.updateRunStatusParams.Status != store.RunStatusCanceled {
					t.Errorf("update status = %s, want %s", m.updateRunStatusParams.Status, store.RunStatusCanceled)
				}
			}

			// Verify pending repos were updated.
			if tc.wantReposUpdated > 0 {
				if len(m.updateRunRepoStatusParams) != tc.wantReposUpdated {
					t.Errorf("updated repos = %d, want %d", len(m.updateRunRepoStatusParams), tc.wantReposUpdated)
				}
				for _, params := range m.updateRunRepoStatusParams {
					if params.Status != store.RunRepoStatusCancelled {
						t.Errorf("repo status = %s, want cancelled", params.Status)
					}
				}
			}
		})
	}
}

// TestAddRunRepoHandler verifies the POST /v1/runs/{id}/repos handler.
func TestAddRunRepoHandler(t *testing.T) {
	t.Parallel()

	// Use KSUID strings for run IDs and NanoID strings for repo IDs.
	sampleRunID := string(domaintypes.NewRunID())
	sampleRepoID := string(domaintypes.NewRunRepoID())

	runningRun := store.Run{
		ID:        sampleRunID,
		Name:      ptrString("test-batch"),
		RepoUrl:   "https://github.com/example/repo.git",
		Status:    store.RunStatusRunning,
		BaseRef:   "main",
		TargetRef: "feature",
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	canceledRun := store.Run{
		ID:        sampleRunID,
		Status:    store.RunStatusCanceled,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	createdRepo := store.RunRepo{
		ID:        sampleRepoID,
		RunID:     domaintypes.RunID(sampleRunID),
		RepoUrl:   "https://github.com/example/new-repo.git",
		BaseRef:   "main",
		TargetRef: "feature-2",
		Status:    store.RunRepoStatusPending,
		Attempt:   1,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	tests := []struct {
		name          string
		runID         string
		body          string
		mockRun       store.Run
		mockRunErr    error
		mockRepoRes   store.RunRepo
		mockRepoErr   error
		wantStatus    int
		wantRepoID    string
		wantCallStore bool
	}{
		{
			name:          "valid add repo",
			runID:         sampleRunID,
			body:          `{"repo_url":"https://github.com/example/new-repo.git","base_ref":"main","target_ref":"feature-2"}`,
			mockRun:       runningRun,
			mockRepoRes:   createdRepo,
			wantStatus:    http.StatusCreated,
			wantRepoID:    sampleRepoID,
			wantCallStore: true,
		},
		{
			name:       "empty id",
			runID:      "",
			body:       `{}`,
			wantStatus: http.StatusBadRequest,
		},
		// NOTE: "invalid uuid" test removed — run IDs are now opaque KSUID strings.
		{
			name:       "run not found",
			runID:      string(domaintypes.NewRunID()), // Generate a different KSUID.
			mockRunErr: pgx.ErrNoRows,
			body:       `{}`,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "run is terminal",
			runID:      sampleRunID,
			body:       `{"repo_url":"https://github.com/example/repo.git","base_ref":"main","target_ref":"feature"}`,
			mockRun:    canceledRun,
			wantStatus: http.StatusConflict,
		},
		{
			name:       "missing repo_url",
			runID:      sampleRunID,
			body:       `{"base_ref":"main","target_ref":"feature"}`,
			mockRun:    runningRun,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing base_ref",
			runID:      sampleRunID,
			body:       `{"repo_url":"https://github.com/example/repo.git","target_ref":"feature"}`,
			mockRun:    runningRun,
			wantStatus: http.StatusBadRequest,
		},
		// Note: target_ref is optional per handler design (comment on line 298 of runs_batch_http.go).
		// When omitted, downstream MR creation derives a default. Test verifies success.
		{
			name:          "missing target_ref (optional)",
			runID:         sampleRunID,
			body:          `{"repo_url":"https://github.com/example/repo.git","base_ref":"main"}`,
			mockRun:       runningRun,
			mockRepoRes:   createdRepo,
			wantStatus:    http.StatusCreated,
			wantRepoID:    sampleRepoID,
			wantCallStore: true,
		},
		{
			name:       "invalid repo_url scheme (ftp)",
			runID:      sampleRunID,
			body:       `{"repo_url":"ftp://example.com/repo.git","base_ref":"main","target_ref":"feature"}`,
			mockRun:    runningRun,
			wantStatus: http.StatusBadRequest,
		},
		// git:// and http:// are rejected per v1 repo URL rules (roadmap/v1/scope.md:30).
		// Only https://, ssh://, and file:// schemes are allowed.
		{
			name:       "invalid repo_url scheme (git)",
			runID:      sampleRunID,
			body:       `{"repo_url":"git://github.com/example/repo.git","base_ref":"main","target_ref":"feature"}`,
			mockRun:    runningRun,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid repo_url scheme (http non-TLS)",
			runID:      sampleRunID,
			body:       `{"repo_url":"http://github.com/example/repo.git","base_ref":"main","target_ref":"feature"}`,
			mockRun:    runningRun,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:        "store error",
			runID:       sampleRunID,
			body:        `{"repo_url":"https://github.com/example/repo.git","base_ref":"main","target_ref":"feature"}`,
			mockRun:     runningRun,
			mockRepoErr: pgx.ErrTxClosed,
			wantStatus:  http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := &mockStore{
				getRunResult:        tc.mockRun,
				getRunErr:           tc.mockRunErr,
				createRunRepoResult: tc.mockRepoRes,
				createRunRepoErr:    tc.mockRepoErr,
			}

			handler := addRunRepoHandler(m)
			req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+tc.runID+"/repos", strings.NewReader(tc.body))
			req.SetPathValue("id", tc.runID)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler(w, req)

			if w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d; body: %s", w.Code, tc.wantStatus, w.Body.String())
			}

			if tc.wantStatus == http.StatusCreated {
				var resp RunRepoResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("decode response: %v", err)
				}
				// Compare domain type ID with expected string using .String() method.
				if resp.ID.String() != tc.wantRepoID {
					t.Errorf("repo id = %s, want %s", resp.ID.String(), tc.wantRepoID)
				}
				// Compare typed RunRepoStatus with expected value.
				if resp.Status != store.RunRepoStatusPending {
					t.Errorf("status = %s, want pending", resp.Status)
				}
			}

			if tc.wantCallStore && !m.createRunRepoCalled {
				t.Error("expected CreateRunRepo to be called")
			}
		})
	}
}

// TestListRunReposHandler verifies the GET /v1/runs/{id}/repos handler.
func TestListRunReposHandler(t *testing.T) {
	t.Parallel()

	// Use KSUID strings for run IDs and NanoID strings for repo IDs.
	sampleRunID := string(domaintypes.NewRunID())
	sampleRepoID1 := string(domaintypes.NewRunRepoID())
	sampleRepoID2 := string(domaintypes.NewRunRepoID())

	sampleRun := store.Run{
		ID:        sampleRunID,
		Status:    store.RunStatusRunning,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	sampleRepos := []store.RunRepo{
		{
			ID:        sampleRepoID1,
			RunID:     domaintypes.RunID(sampleRunID),
			RepoUrl:   "https://github.com/example/repo1.git",
			BaseRef:   "main",
			TargetRef: "feature-1",
			Status:    store.RunRepoStatusPending,
			Attempt:   1,
			CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		},
		{
			ID:        sampleRepoID2,
			RunID:     domaintypes.RunID(sampleRunID),
			RepoUrl:   "https://github.com/example/repo2.git",
			BaseRef:   "main",
			TargetRef: "feature-2",
			Status:    store.RunRepoStatusSucceeded,
			Attempt:   1,
			CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		},
	}

	tests := []struct {
		name          string
		runID         string
		mockRun       store.Run
		mockRunErr    error
		mockRepos     []store.RunRepo
		mockReposErr  error
		wantStatus    int
		wantRepoCount int
	}{
		{
			name:          "list repos successfully",
			runID:         sampleRunID,
			mockRun:       sampleRun,
			mockRepos:     sampleRepos,
			wantStatus:    http.StatusOK,
			wantRepoCount: 2,
		},
		{
			name:          "empty list",
			runID:         sampleRunID,
			mockRun:       sampleRun,
			mockRepos:     []store.RunRepo{},
			wantStatus:    http.StatusOK,
			wantRepoCount: 0,
		},
		{
			name:       "empty id",
			runID:      "",
			wantStatus: http.StatusBadRequest,
		},
		// NOTE: "invalid uuid" test removed — run IDs are now opaque KSUID strings.
		{
			name:       "run not found",
			runID:      string(domaintypes.NewRunID()), // Generate a different KSUID.
			mockRunErr: pgx.ErrNoRows,
			wantStatus: http.StatusNotFound,
		},
		{
			name:         "list repos error",
			runID:        sampleRunID,
			mockRun:      sampleRun,
			mockReposErr: pgx.ErrTxClosed,
			wantStatus:   http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := &mockStore{
				getRunResult:            tc.mockRun,
				getRunErr:               tc.mockRunErr,
				listRunReposByRunResult: tc.mockRepos,
				listRunReposByRunErr:    tc.mockReposErr,
			}

			handler := listRunReposHandler(m)
			req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+tc.runID+"/repos", nil)
			req.SetPathValue("id", tc.runID)
			w := httptest.NewRecorder()

			handler(w, req)

			if w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d; body: %s", w.Code, tc.wantStatus, w.Body.String())
			}

			if tc.wantStatus == http.StatusOK {
				var resp struct {
					Repos []RunRepoResponse `json:"repos"`
				}
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("decode response: %v", err)
				}
				if len(resp.Repos) != tc.wantRepoCount {
					t.Errorf("repo count = %d, want %d", len(resp.Repos), tc.wantRepoCount)
				}
			}
		})
	}
}

// TestDeleteRunRepoHandler verifies the DELETE /v1/runs/{id}/repos/{repo_id} handler.
func TestDeleteRunRepoHandler(t *testing.T) {
	t.Parallel()

	// Use KSUID strings for run IDs and NanoID strings for repo IDs.
	sampleRunID := string(domaintypes.NewRunID())
	sampleRepoID := string(domaintypes.NewRunRepoID())

	sampleRun := store.Run{
		ID:        sampleRunID,
		Status:    store.RunStatusRunning,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	pendingRepo := store.RunRepo{
		ID:        sampleRepoID,
		RunID:     domaintypes.RunID(sampleRunID),
		RepoUrl:   "https://github.com/example/repo.git",
		Status:    store.RunRepoStatusPending,
		Attempt:   1,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	runningRepo := store.RunRepo{
		ID:        sampleRepoID,
		RunID:     domaintypes.RunID(sampleRunID),
		RepoUrl:   "https://github.com/example/repo.git",
		Status:    store.RunRepoStatusRunning,
		Attempt:   1,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	succeededRepo := store.RunRepo{
		ID:        sampleRepoID,
		RunID:     domaintypes.RunID(sampleRunID),
		RepoUrl:   "https://github.com/example/repo.git",
		Status:    store.RunRepoStatusSucceeded,
		Attempt:   1,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	// Repo that belongs to a different run.
	differentRunID := string(domaintypes.NewRunID())
	differentRunRepo := store.RunRepo{
		ID:        sampleRepoID,
		RunID:     domaintypes.RunID(differentRunID),
		RepoUrl:   "https://github.com/example/repo.git",
		Status:    store.RunRepoStatusPending,
		Attempt:   1,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	tests := []struct {
		name             string
		runID            string
		repoID           string
		mockRun          store.Run
		mockRunErr       error
		mockRepo         store.RunRepo
		mockRepoErr      error
		updateStatusErr  error
		wantStatus       int
		wantNewStatus    string
		wantStatusUpdate bool
	}{
		{
			name:             "delete pending repo (skipped)",
			runID:            sampleRunID,
			repoID:           sampleRepoID,
			mockRun:          sampleRun,
			mockRepo:         pendingRepo,
			wantStatus:       http.StatusOK,
			wantNewStatus:    string(store.RunRepoStatusSkipped),
			wantStatusUpdate: true,
		},
		{
			name:             "delete running repo (cancelled)",
			runID:            sampleRunID,
			repoID:           sampleRepoID,
			mockRun:          sampleRun,
			mockRepo:         runningRepo,
			wantStatus:       http.StatusOK,
			wantNewStatus:    string(store.RunRepoStatusCancelled),
			wantStatusUpdate: true,
		},
		{
			name:       "delete succeeded repo (idempotent)",
			runID:      sampleRunID,
			repoID:     sampleRepoID,
			mockRun:    sampleRun,
			mockRepo:   succeededRepo,
			wantStatus: http.StatusOK,
		},
		{
			name:       "empty run id",
			runID:      "",
			repoID:     sampleRepoID,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty repo id",
			runID:      sampleRunID,
			repoID:     "",
			wantStatus: http.StatusBadRequest,
		},
		// NOTE: "invalid uuid" tests removed — IDs are now opaque KSUID/NanoID strings.
		{
			name:       "run not found",
			runID:      string(domaintypes.NewRunID()), // Generate a different KSUID.
			repoID:     sampleRepoID,
			mockRunErr: pgx.ErrNoRows,
			wantStatus: http.StatusNotFound,
		},
		{
			name:        "repo not found",
			runID:       sampleRunID,
			repoID:      string(domaintypes.NewRunRepoID()), // Generate a different NanoID.
			mockRun:     sampleRun,
			mockRepoErr: pgx.ErrNoRows,
			wantStatus:  http.StatusNotFound,
		},
		{
			name:       "repo belongs to different run",
			runID:      sampleRunID,
			repoID:     sampleRepoID,
			mockRun:    sampleRun,
			mockRepo:   differentRunRepo,
			wantStatus: http.StatusNotFound,
		},
		{
			name:            "update status error",
			runID:           sampleRunID,
			repoID:          sampleRepoID,
			mockRun:         sampleRun,
			mockRepo:        pendingRepo,
			updateStatusErr: pgx.ErrTxClosed,
			wantStatus:      http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := &mockStore{
				getRunResult:           tc.mockRun,
				getRunErr:              tc.mockRunErr,
				getRunRepoResult:       tc.mockRepo,
				getRunRepoErr:          tc.mockRepoErr,
				updateRunRepoStatusErr: tc.updateStatusErr,
			}

			handler := deleteRunRepoHandler(m)
			req := httptest.NewRequest(http.MethodDelete, "/v1/runs/"+tc.runID+"/repos/"+tc.repoID, nil)
			req.SetPathValue("id", tc.runID)
			req.SetPathValue("repo_id", tc.repoID)
			w := httptest.NewRecorder()

			handler(w, req)

			if w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d; body: %s", w.Code, tc.wantStatus, w.Body.String())
			}

			if tc.wantStatusUpdate {
				if len(m.updateRunRepoStatusParams) == 0 {
					t.Fatal("expected UpdateRunRepoStatus to be called")
				}
				if string(m.updateRunRepoStatusParams[0].Status) != tc.wantNewStatus {
					t.Errorf("new status = %s, want %s", m.updateRunRepoStatusParams[0].Status, tc.wantNewStatus)
				}
			}
		})
	}
}

// TestRestartRunRepoHandler verifies the POST /v1/runs/{id}/repos/{repo_id}/restart handler.
func TestRestartRunRepoHandler(t *testing.T) {
	t.Parallel()

	// Use KSUID strings for run IDs and NanoID strings for repo IDs.
	sampleRunID := string(domaintypes.NewRunID())
	sampleRepoID := string(domaintypes.NewRunRepoID())

	runningRun := store.Run{
		ID:        sampleRunID,
		Name:      ptrString("test-batch"),
		Status:    store.RunStatusRunning,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	canceledRun := store.Run{
		ID:        sampleRunID,
		Status:    store.RunStatusCanceled,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	pendingRepo := store.RunRepo{
		ID:        sampleRepoID,
		RunID:     domaintypes.RunID(sampleRunID),
		Status:    store.RunRepoStatusPending,
		Attempt:   1,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	failedRepo := store.RunRepo{
		ID:        sampleRepoID,
		RunID:     domaintypes.RunID(sampleRunID),
		Status:    store.RunRepoStatusFailed,
		Attempt:   1,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	restartedRepo := failedRepo
	restartedRepo.Status = store.RunRepoStatusPending
	restartedRepo.Attempt = failedRepo.Attempt + 1

	tests := []struct {
		name                 string
		runID                string
		repoID               string
		body                 string
		mockRun              store.Run
		mockRunErr           error
		mockRepo             store.RunRepo
		mockRepoErr          error
		mockRestartedRepo    store.RunRepo
		incrementAttemptErr  error
		updateRefsErr        error
		wantStatus           int
		wantIncrementAttempt bool
		wantAttempt          int32
		wantUpdateRefs       bool
		wantBaseRef          string
		wantTargetRef        string
	}{
		{
			name:                 "restart failed repo",
			runID:                sampleRunID,
			repoID:               sampleRepoID,
			mockRun:              runningRun,
			mockRepo:             failedRepo,
			mockRestartedRepo:    restartedRepo,
			wantStatus:           http.StatusOK,
			wantIncrementAttempt: true,
			wantAttempt:          2,
			wantUpdateRefs:       false,
		},
		{
			name:                 "restart with empty body",
			runID:                sampleRunID,
			repoID:               sampleRepoID,
			body:                 "",
			mockRun:              runningRun,
			mockRepo:             failedRepo,
			mockRestartedRepo:    restartedRepo,
			wantStatus:           http.StatusOK,
			wantIncrementAttempt: true,
			wantAttempt:          2,
			wantUpdateRefs:       false,
		},
		{
			name:                 "restart with new refs",
			runID:                sampleRunID,
			repoID:               sampleRepoID,
			body:                 `{"base_ref":"main-updated","target_ref":"feature-updated"}`,
			mockRun:              runningRun,
			mockRepo:             failedRepo,
			mockRestartedRepo:    restartedRepo,
			wantStatus:           http.StatusOK,
			wantIncrementAttempt: true,
			wantAttempt:          2,
			wantUpdateRefs:       true,
			wantBaseRef:          "main-updated",
			wantTargetRef:        "feature-updated",
		},
		{
			name:       "empty run id",
			runID:      "",
			repoID:     sampleRepoID,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty repo id",
			runID:      sampleRunID,
			repoID:     "",
			wantStatus: http.StatusBadRequest,
		},
		// NOTE: "invalid uuid" tests removed — IDs are now opaque KSUID/NanoID strings.
		{
			name:       "run not found",
			runID:      string(domaintypes.NewRunID()), // Generate a different KSUID.
			repoID:     sampleRepoID,
			mockRunErr: pgx.ErrNoRows,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "run is terminal",
			runID:      sampleRunID,
			repoID:     sampleRepoID,
			mockRun:    canceledRun,
			wantStatus: http.StatusConflict,
		},
		{
			name:        "repo not found",
			runID:       sampleRunID,
			repoID:      string(domaintypes.NewRunRepoID()), // Generate a different NanoID.
			mockRun:     runningRun,
			mockRepoErr: pgx.ErrNoRows,
			wantStatus:  http.StatusNotFound,
		},
		{
			name:       "repo belongs to different run",
			runID:      sampleRunID,
			repoID:     sampleRepoID,
			mockRun:    runningRun,
			mockRepo:   store.RunRepo{RunID: domaintypes.NewRunID()}, // Different KSUID.
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "cannot restart pending repo",
			runID:      sampleRunID,
			repoID:     sampleRepoID,
			mockRun:    runningRun,
			mockRepo:   pendingRepo,
			wantStatus: http.StatusConflict,
		},
		{
			name:                 "increment attempt error",
			runID:                sampleRunID,
			repoID:               sampleRepoID,
			mockRun:              runningRun,
			mockRepo:             failedRepo,
			incrementAttemptErr:  pgx.ErrTxClosed,
			wantStatus:           http.StatusInternalServerError,
			wantIncrementAttempt: true, // We do call it, but it fails.
			wantUpdateRefs:       false,
		},
		{
			name:                 "update refs error",
			runID:                sampleRunID,
			repoID:               sampleRepoID,
			body:                 `{"base_ref":"main-updated","target_ref":"feature-updated"}`,
			mockRun:              runningRun,
			mockRepo:             failedRepo,
			mockRestartedRepo:    restartedRepo,
			updateRefsErr:        pgx.ErrTxClosed,
			wantStatus:           http.StatusInternalServerError,
			wantIncrementAttempt: true,
			wantUpdateRefs:       true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Mock returns the restarted repo on second GetRunRepo call (after increment).
			mockRepoOnGet := tc.mockRepo
			if tc.mockRestartedRepo.ID != "" {
				mockRepoOnGet = tc.mockRestartedRepo
			}

			m := &mockStore{
				getRunResult:               tc.mockRun,
				getRunErr:                  tc.mockRunErr,
				getRunRepoResult:           mockRepoOnGet,
				getRunRepoErr:              tc.mockRepoErr,
				incrementRunRepoAttemptErr: tc.incrementAttemptErr,
				updateRunRepoRefsErr:       tc.updateRefsErr,
			}

			// On successful restart, GetRunRepo is called twice (before and after increment).
			// First call returns the original repo, second returns restarted.
			// For simplicity, we set the result to the restarted repo if available.
			if tc.mockRepo.ID != "" && tc.mockRestartedRepo.ID != "" {
				m.getRunRepoResult = tc.mockRepo
			}

			handler := restartRunRepoHandler(m)
			var body *strings.Reader
			if tc.body != "" {
				body = strings.NewReader(tc.body)
			} else {
				body = strings.NewReader("")
			}
			req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+tc.runID+"/repos/"+tc.repoID+"/restart", body)
			req.SetPathValue("id", tc.runID)
			req.SetPathValue("repo_id", tc.repoID)
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			w := httptest.NewRecorder()

			handler(w, req)

			if w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d; body: %s", w.Code, tc.wantStatus, w.Body.String())
			}

			if tc.wantIncrementAttempt && !m.incrementRunRepoAttemptCalled {
				t.Error("expected IncrementRunRepoAttempt to be called")
			}
			if !tc.wantIncrementAttempt && m.incrementRunRepoAttemptCalled {
				t.Error("expected IncrementRunRepoAttempt NOT to be called")
			}

			if tc.wantUpdateRefs {
				if !m.updateRunRepoRefsCalled {
					t.Error("expected UpdateRunRepoRefs to be called")
				} else {
					if tc.wantBaseRef != "" && m.updateRunRepoRefsParams.BaseRef != tc.wantBaseRef {
						t.Errorf("base_ref = %s, want %s", m.updateRunRepoRefsParams.BaseRef, tc.wantBaseRef)
					}
					if tc.wantTargetRef != "" && m.updateRunRepoRefsParams.TargetRef != tc.wantTargetRef {
						t.Errorf("target_ref = %s, want %s", m.updateRunRepoRefsParams.TargetRef, tc.wantTargetRef)
					}
				}
			} else if m.updateRunRepoRefsCalled {
				t.Error("expected UpdateRunRepoRefs NOT to be called")
			}
		})
	}
}

// -------------------------------------------------------------------------
// Tests for POST /v1/runs/{id}/start — batch execution start handler.
// -------------------------------------------------------------------------

// TestStartRunHandler verifies the POST /v1/runs/{id}/start handler.
func TestStartRunHandler(t *testing.T) {
	t.Parallel()

	// Use KSUID strings for run IDs and NanoID strings for repo IDs.
	sampleBatchRunID := string(domaintypes.NewRunID())
	sampleRepoID1 := string(domaintypes.NewRunRepoID())
	sampleRepoID2 := string(domaintypes.NewRunRepoID())
	childRunID := string(domaintypes.NewRunID())

	// Sample batch run (queued, ready to start).
	queuedBatchRun := store.Run{
		ID:        sampleBatchRunID,
		Name:      ptrString("test-batch"),
		RepoUrl:   "https://github.com/example/batch.git",
		Spec:      []byte(`{"image":"test-image"}`),
		Status:    store.RunStatusQueued,
		BaseRef:   "main",
		TargetRef: "feature",
		CreatedBy: ptrString("test-user"),
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	// Sample canceled batch run (terminal, cannot start).
	canceledBatchRun := store.Run{
		ID:        sampleBatchRunID,
		Name:      ptrString("test-batch"),
		Status:    store.RunStatusCanceled,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	// Sample pending run repos.
	pendingRepo1 := store.RunRepo{
		ID:        sampleRepoID1,
		RunID:     domaintypes.RunID(sampleBatchRunID),
		RepoUrl:   "https://github.com/example/repo1.git",
		BaseRef:   "main",
		TargetRef: "feature-1",
		Status:    store.RunRepoStatusPending,
		Attempt:   1,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	pendingRepo2 := store.RunRepo{
		ID:        sampleRepoID2,
		RunID:     domaintypes.RunID(sampleBatchRunID),
		RepoUrl:   "https://github.com/example/repo2.git",
		BaseRef:   "main",
		TargetRef: "feature-2",
		Status:    store.RunRepoStatusPending,
		Attempt:   1,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	// Sample already succeeded repo.
	succeededRepo := store.RunRepo{
		ID:        sampleRepoID1,
		RunID:     domaintypes.RunID(sampleBatchRunID),
		RepoUrl:   "https://github.com/example/repo1.git",
		Status:    store.RunRepoStatusSucceeded,
		Attempt:   1,
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	// Sample child run created when starting pending repos.
	childRun := store.Run{
		ID:        childRunID,
		RepoUrl:   "https://github.com/example/repo1.git",
		Status:    store.RunStatusQueued,
		BaseRef:   "main",
		TargetRef: "feature-1",
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}

	tests := []struct {
		name                          string
		runID                         string
		mockRun                       store.Run
		mockRunErr                    error
		mockAllRepos                  []store.RunRepo
		mockAllReposErr               error
		mockPendingRepos              []store.RunRepo
		mockPendingErr                error
		mockCreateRunResult           store.Run
		mockCreateRunErr              error
		mockAckRunStartErr            error
		wantStatus                    int
		wantStarted                   int
		wantAlreadyDone               int
		wantPending                   int
		wantCreateRunCalled           bool
		wantSetRunRepoExecutionCalled bool
		wantAckRunStartCalled         bool
	}{
		{
			name:                          "start with pending repos",
			runID:                         sampleBatchRunID,
			mockRun:                       queuedBatchRun,
			mockAllRepos:                  []store.RunRepo{pendingRepo1, pendingRepo2},
			mockPendingRepos:              []store.RunRepo{pendingRepo1, pendingRepo2},
			mockCreateRunResult:           childRun,
			wantStatus:                    http.StatusOK,
			wantStarted:                   2,
			wantAlreadyDone:               0,
			wantPending:                   0,
			wantCreateRunCalled:           true,
			wantSetRunRepoExecutionCalled: true,
			wantAckRunStartCalled:         true,
		},
		{
			name:             "no pending repos",
			runID:            sampleBatchRunID,
			mockRun:          queuedBatchRun,
			mockAllRepos:     []store.RunRepo{succeededRepo},
			mockPendingRepos: []store.RunRepo{},
			wantStatus:       http.StatusOK,
			wantStarted:      0,
			wantAlreadyDone:  1,
			wantPending:      0,
		},
		{
			name:       "run not found",
			runID:      sampleBatchRunID,
			mockRunErr: pgx.ErrNoRows,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "run is terminal",
			runID:      sampleBatchRunID,
			mockRun:    canceledBatchRun,
			wantStatus: http.StatusConflict,
		},
		{
			name:            "list all repos error",
			runID:           sampleBatchRunID,
			mockRun:         queuedBatchRun,
			mockAllRepos:    nil,
			mockAllReposErr: pgx.ErrTxClosed,
			wantStatus:      http.StatusInternalServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := &mockStore{
				getRunResult:                   tc.mockRun,
				getRunErr:                      tc.mockRunErr,
				listRunReposByRunResult:        tc.mockAllRepos,
				listRunReposByRunErr:           tc.mockAllReposErr,
				listPendingRunReposByRunResult: tc.mockPendingRepos,
				listPendingRunReposByRunErr:    tc.mockPendingErr,
				createRunResult:                tc.mockCreateRunResult,
				createRunErr:                   tc.mockCreateRunErr,
				ackRunStartErr:                 tc.mockAckRunStartErr,
			}

			handler := startRunHandler(m)
			req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+tc.runID+"/start", nil)
			req.SetPathValue("id", tc.runID)
			w := httptest.NewRecorder()

			handler(w, req)

			if w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d; body: %s", w.Code, tc.wantStatus, w.Body.String())
			}

			if tc.wantStatus == http.StatusOK {
				var resp StartRunResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("decode response: %v", err)
				}

				if resp.Started != tc.wantStarted {
					t.Errorf("started = %d, want %d", resp.Started, tc.wantStarted)
				}
				if resp.AlreadyDone != tc.wantAlreadyDone {
					t.Errorf("already_done = %d, want %d", resp.AlreadyDone, tc.wantAlreadyDone)
				}
				if resp.Pending != tc.wantPending {
					t.Errorf("pending = %d, want %d", resp.Pending, tc.wantPending)
				}
			}

			if tc.wantCreateRunCalled && !m.createRunCalled {
				t.Error("expected CreateRun to be called")
			}
			if tc.wantSetRunRepoExecutionCalled && !m.setRunRepoExecutionRunCalled {
				t.Error("expected SetRunRepoExecutionRun to be called")
			}
			if tc.wantAckRunStartCalled && !m.ackRunStartCalled {
				t.Error("expected AckRunStart to be called")
			}
		})
	}
}
