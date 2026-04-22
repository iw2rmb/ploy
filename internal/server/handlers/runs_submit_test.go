package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// =============================================================================
// POST /v1/runs — Create Single-Repo Run (v1 API)
// =============================================================================

// TestRunsCreateSingleRepo_Success verifies POST /v1/runs creates a run with mig side-effect.
// Tests single-repo run creation with automatic mig project creation.
// Contract:
//   - Creates a mig project (mig name == mig id).
//   - Creates a spec row and sets migs.spec_id.
//   - Creates a mig repo row for the provided repo_url.
//   - Creates a run and run repo row.
//   - Response includes run_id, mig_id, spec_id.
func TestRunsCreateSingleRepo_Success(t *testing.T) {
	st := &migStore{}
	eventsService, _ := createTestEventsService()
	handler := createSingleRepoRunHandler(st, eventsService)

	rr := doRequest(t, handler, http.MethodPost, "/v1/runs", validRunRequestBody())

	assertStatus(t, rr, http.StatusCreated)

	// Verify store methods were called in order.
	if !st.createSpecCalled {
		t.Error("store.CreateSpec was not called")
	}
	if !st.createMigCalled {
		t.Error("store.CreateMig was not called")
	}
	if !st.createMigRepoCalled {
		t.Error("store.CreateMigRepo was not called")
	}
	if !st.createRunCalled {
		t.Error("store.CreateRun was not called")
	}
	if !st.createRunRepoCalled {
		t.Error("store.CreateRunRepo was not called")
	}
	if st.createJobCalled {
		t.Error("store.CreateJob should not be called during submission")
	}

	// Verify mig name == mig id (v1 contract).
	if st.createMigParams.Name != st.createMigParams.ID.String() {
		t.Errorf("mig name (%q) != mig id (%q); v1 requires name == id for single-repo runs",
			st.createMigParams.Name, st.createMigParams.ID.String())
	}

	// Verify spec_id was linked to mig.
	if st.createMigParams.SpecID == nil {
		t.Error("mig was not linked to spec (spec_id is nil)")
	}

	// Verify response shape matches v1 contract.
	var resp struct {
		RunID  string `json:"run_id"`
		MigID  string `json:"mig_id"`
		SpecID string `json:"spec_id"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.RunID == "" {
		t.Error("response run_id is empty")
	}
	if resp.MigID == "" {
		t.Error("response mig_id is empty")
	}
	if resp.SpecID == "" {
		t.Error("response spec_id is empty")
	}
}

// TestRunsCreateSingleRepo_DoesNotCreateJobsImmediately verifies submission defers job materialization.
func TestRunsCreateSingleRepo_DoesNotCreateJobsImmediately(t *testing.T) {
	st := &migStore{}
	eventsService, _ := createTestEventsService()
	handler := createSingleRepoRunHandler(st, eventsService)

	rr := doRequest(t, handler, http.MethodPost, "/v1/runs", validRunRequestBody())
	assertStatus(t, rr, http.StatusCreated)

	if st.createJobCallCount != 0 {
		t.Fatalf("expected no jobs to be created during submission, got %d", st.createJobCallCount)
	}
}

// TestRunsCreateSingleRepo_RepoURLNormalized verifies repo URLs are normalized.
// Uses types.NormalizeRepoURL for URL normalization.
func TestRunsCreateSingleRepo_RepoURLNormalized(t *testing.T) {
	st := &migStore{}
	eventsService, _ := createTestEventsService()
	handler := createSingleRepoRunHandler(st, eventsService)

	// URL with trailing slash and .git suffix — should be normalized.
	rr := doRequest(t, handler, http.MethodPost, "/v1/runs", validRunRequestBodyWith(map[string]any{
		"repo_url": "https://github.com/org/repo.git/",
	}))
	assertStatus(t, rr, http.StatusCreated)

	// Verify mig_repo was created with normalized URL.
	if !st.createMigRepoCalled {
		t.Fatal("store.CreateMigRepo was not called")
	}
	// types.NormalizeRepoURL trims trailing "/" and ".git".
	expectedURL := "https://github.com/org/repo"
	if st.createMigRepoParams.Url != expectedURL {
		t.Errorf("mig_repo URL = %q, want %q (normalized)", st.createMigRepoParams.Url, expectedURL)
	}
}

// TestRunsCreateSingleRepo_ValidationErrors merges individual validation error tests.
func TestRunsCreateSingleRepo_ValidationErrors(t *testing.T) {
	tests := []struct {
		name       string
		body       any
		wantStatus int
	}{
		{
			name:       "MissingRepoURL",
			body:       validRunRequestBodyWithout("repo_url"),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "InvalidRepoURLScheme",
			body:       validRunRequestBodyWith(map[string]any{"repo_url": "ftp://example.com/repo"}),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "MissingBaseRef",
			body:       validRunRequestBodyWithout("base_ref"),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "MissingTargetRef",
			body:       validRunRequestBodyWithout("target_ref"),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "MissingSpec",
			body:       validRunRequestBodyWithout("spec"),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "InvalidSpec",
			body:       validRunRequestBodyWith(map[string]any{"spec": map[string]any{"steps": "not-array"}}),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "InvalidJSON",
			body:       "not json",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &migStore{}
			handler := createSingleRepoRunHandler(st, nil)
			rr := doRequest(t, handler, http.MethodPost, "/v1/runs", tt.body)
			assertStatus(t, rr, tt.wantStatus)
		})
	}
}

// TestRunsCreateSingleRepo_WithCreatedBy verifies POST /v1/runs accepts optional created_by.
func TestRunsCreateSingleRepo_WithCreatedBy(t *testing.T) {
	st := &migStore{}
	eventsService, _ := createTestEventsService()
	handler := createSingleRepoRunHandler(st, eventsService)

	rr := doRequest(t, handler, http.MethodPost, "/v1/runs", validRunRequestBodyWith(map[string]any{
		"created_by": "test-user@example.com",
	}))
	assertStatus(t, rr, http.StatusCreated)

	// Verify created_by was propagated to store calls.
	if st.createSpecParams.CreatedBy == nil || *st.createSpecParams.CreatedBy != "test-user@example.com" {
		t.Error("created_by not propagated to CreateSpec")
	}
	if st.createMigParams.CreatedBy == nil || *st.createMigParams.CreatedBy != "test-user@example.com" {
		t.Error("created_by not propagated to CreateMig")
	}
	if st.createRunParams.CreatedBy == nil || *st.createRunParams.CreatedBy != "test-user@example.com" {
		t.Error("created_by not propagated to CreateRun")
	}
}

// TestRunsCreateSingleRepo_MultiStepSpec verifies POST /v1/runs accepts multi-step spec without job creation.
func TestRunsCreateSingleRepo_MultiStepSpec(t *testing.T) {
	st := &migStore{}
	eventsService, _ := createTestEventsService()
	handler := createSingleRepoRunHandler(st, eventsService)

	// Multi-step spec with steps[] array.
	multiStepSpec := map[string]any{
		"version": "0.2.0",
		"envs":    map[string]any{},
		"steps": []any{
			map[string]any{"image": "mig-image-1"},
			map[string]any{"image": "mig-image-2"},
		},
	}
	rr := doRequest(t, handler, http.MethodPost, "/v1/runs", validRunRequestBodyWith(map[string]any{
		"spec": multiStepSpec,
	}))
	assertStatus(t, rr, http.StatusCreated)

	if st.createJobCallCount != 0 {
		t.Errorf("createJobCallCount = %d, want 0", st.createJobCallCount)
	}
}

// =============================================================================
// Store Error Tests (table-driven)
// =============================================================================

// TestRunsCreateSingleRepo_StoreErrors merges individual store error tests.
func TestRunsCreateSingleRepo_StoreErrors(t *testing.T) {
	tests := []struct {
		name    string
		setupFn func(st *migStore)
	}{
		{
			name:    "CreateSpecError",
			setupFn: func(st *migStore) { st.createSpecErr = errors.New("database connection failed") },
		},
		{
			name:    "CreateMigError",
			setupFn: func(st *migStore) { st.createMigErr = errors.New("database connection failed") },
		},
		{
			name:    "CreateMigRepoError",
			setupFn: func(st *migStore) { st.createMigRepoErr = errors.New("database connection failed") },
		},
		{
			name:    "CreateRunError",
			setupFn: func(st *migStore) { st.createRunErr = errors.New("database connection failed") },
		},
		{
			name:    "CreateRunRepoError",
			setupFn: func(st *migStore) { st.createRunRepoErr = errors.New("database connection failed") },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &migStore{}
			tt.setupFn(st)
			handler := createSingleRepoRunHandler(st, nil)
			rr := doRequest(t, handler, http.MethodPost, "/v1/runs", validRunRequestBody())
			assertStatus(t, rr, http.StatusInternalServerError)
		})
	}
}

func TestRunsCreateSingleRepo_RejectsWhenSourceCommitSeedFails(t *testing.T) {
	st := &migStore{}
	eventsService, _ := createTestEventsService()
	handler := createSingleRepoRunHandler(st, eventsService)

	body, _ := json.Marshal(validRunRequestBody())
	req := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(withSourceCommitSHAResolver(req.Context(), func(_ context.Context, _, _ string) (string, error) {
		return "", errors.New("seed lookup failed")
	}))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	if st.createRunRepoCalled {
		t.Fatal("store.CreateRunRepo should not be called when source commit seed resolution fails")
	}
}
