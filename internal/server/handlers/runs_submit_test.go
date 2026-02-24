package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/iw2rmb/ploy/internal/store"
)

// =============================================================================
// POST /v1/runs — Create Single-Repo Run (v1 API)
// =============================================================================

// TestRunsCreateSingleRepo_Success verifies POST /v1/runs creates a run with mod side-effect.
// Tests single-repo run creation with automatic mod project creation.
// Contract:
//   - Creates a mod project (mod name == mod id).
//   - Creates a spec row and sets migs.spec_id.
//   - Creates a mod repo row for the provided repo_url.
//   - Creates a run and run repo row and enqueues jobs.
//   - Response includes run_id, mig_id, spec_id.
func TestRunsCreateSingleRepo_Success(t *testing.T) {
	st := &mockStore{}
	eventsService, _ := createTestEventsService()
	handler := createSingleRepoRunHandler(st, eventsService)

	spec := map[string]any{
		"version": "0.2.0",
		"env":     map[string]any{},
		"steps":   []any{map[string]any{"image": "docker.io/test/mod:latest"}},
	}
	reqBody := map[string]any{
		"repo_url":   "https://github.com/org/repo",
		"base_ref":   "main",
		"target_ref": "feature",
		"spec":       spec,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusCreated, rr.Body.String())
	}

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
	if !st.createJobCalled {
		t.Error("store.CreateJob was not called (jobs should be created immediately)")
	}

	// Verify mod name == mod id (v1 contract).
	if st.createMigParams.Name != st.createMigParams.ID.String() {
		t.Errorf("mod name (%q) != mod id (%q); v1 requires name == id for single-repo runs",
			st.createMigParams.Name, st.createMigParams.ID.String())
	}

	// Verify spec_id was linked to mod.
	if st.createMigParams.SpecID == nil {
		t.Error("mod was not linked to spec (spec_id is nil)")
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

// TestRunsCreateSingleRepo_FirstJobClaimable verifies that the first job is Queued
// and immediately claimable (v1 job queueing rules).
func TestRunsCreateSingleRepo_FirstJobClaimable(t *testing.T) {
	st := &mockStore{}
	eventsService, _ := createTestEventsService()
	handler := createSingleRepoRunHandler(st, eventsService)

	spec := map[string]any{
		"version": "0.2.0",
		"env":     map[string]any{},
		"steps":   []any{map[string]any{"image": "docker.io/test/mod:latest"}},
	}
	reqBody := map[string]any{
		"repo_url":   "https://github.com/org/repo",
		"base_ref":   "main",
		"target_ref": "feature",
		"spec":       spec,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusCreated, rr.Body.String())
	}

	// Verify jobs were created.
	if st.createJobCallCount == 0 {
		t.Fatal("no jobs created")
	}

	// v1 job queueing rules: first job (pre-gate) is Queued, rest are Created.
	// Check that at least one job has status Queued.
	hasQueuedJob := false
	for _, params := range st.createJobParams {
		if params.Status == store.JobStatusQueued {
			hasQueuedJob = true
			break
		}
	}
	if !hasQueuedJob {
		t.Error("no job with Queued status; v1 requires first job to be immediately claimable")
	}
}

// TestRunsCreateSingleRepo_RepoURLNormalized verifies repo URLs are normalized.
// Uses types.NormalizeRepoURL for URL normalization.
func TestRunsCreateSingleRepo_RepoURLNormalized(t *testing.T) {
	st := &mockStore{}
	eventsService, _ := createTestEventsService()
	handler := createSingleRepoRunHandler(st, eventsService)

	spec := map[string]any{
		"version": "0.2.0",
		"env":     map[string]any{},
		"steps":   []any{map[string]any{"image": "docker.io/test/mod:latest"}},
	}
	// URL with trailing slash and .git suffix — should be normalized.
	reqBody := map[string]any{
		"repo_url":   "https://github.com/org/repo.git/",
		"base_ref":   "main",
		"target_ref": "feature",
		"spec":       spec,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusCreated, rr.Body.String())
	}

	// Verify mod_repo was created with normalized URL.
	if !st.createMigRepoCalled {
		t.Fatal("store.CreateMigRepo was not called")
	}
	// types.NormalizeRepoURL trims trailing "/" and ".git".
	expectedURL := "https://github.com/org/repo"
	if st.createMigRepoParams.RepoUrl != expectedURL {
		t.Errorf("mod_repo URL = %q, want %q (normalized)", st.createMigRepoParams.RepoUrl, expectedURL)
	}
}

// TestRunsCreateSingleRepo_MissingRepoURL verifies POST /v1/runs rejects missing repo_url.
func TestRunsCreateSingleRepo_MissingRepoURL(t *testing.T) {
	st := &mockStore{}
	handler := createSingleRepoRunHandler(st, nil)

	spec := map[string]any{
		"version": "0.2.0",
		"env":     map[string]any{},
		"steps":   []any{map[string]any{"image": "docker.io/test/mod:latest"}},
	}
	reqBody := map[string]any{
		"base_ref":   "main",
		"target_ref": "feature",
		"spec":       spec,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}

	// Store should not be called.
	if st.createSpecCalled {
		t.Error("store.CreateSpec should not be called for invalid request")
	}
}

// TestRunsCreateSingleRepo_InvalidRepoURLScheme verifies POST /v1/runs rejects invalid schemes.
// Only https://, ssh://, and file:// schemes are accepted.
func TestRunsCreateSingleRepo_InvalidRepoURLScheme(t *testing.T) {
	st := &mockStore{}
	handler := createSingleRepoRunHandler(st, nil)

	spec := map[string]any{
		"version": "0.2.0",
		"env":     map[string]any{},
		"steps":   []any{map[string]any{"image": "docker.io/test/mod:latest"}},
	}
	// ftp:// is not an allowed scheme.
	reqBody := map[string]any{
		"repo_url":   "ftp://example.com/repo",
		"base_ref":   "main",
		"target_ref": "feature",
		"spec":       spec,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusBadRequest, rr.Body.String())
	}
}

// TestRunsCreateSingleRepo_MissingBaseRef verifies POST /v1/runs rejects missing base_ref.
func TestRunsCreateSingleRepo_MissingBaseRef(t *testing.T) {
	st := &mockStore{}
	handler := createSingleRepoRunHandler(st, nil)

	spec := map[string]any{
		"version": "0.2.0",
		"env":     map[string]any{},
		"steps":   []any{map[string]any{"image": "docker.io/test/mod:latest"}},
	}
	reqBody := map[string]any{
		"repo_url":   "https://github.com/org/repo",
		"target_ref": "feature",
		"spec":       spec,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

// TestRunsCreateSingleRepo_MissingTargetRef verifies POST /v1/runs rejects missing target_ref.
func TestRunsCreateSingleRepo_MissingTargetRef(t *testing.T) {
	st := &mockStore{}
	handler := createSingleRepoRunHandler(st, nil)

	spec := map[string]any{
		"version": "0.2.0",
		"env":     map[string]any{},
		"steps":   []any{map[string]any{"image": "docker.io/test/mod:latest"}},
	}
	reqBody := map[string]any{
		"repo_url": "https://github.com/org/repo",
		"base_ref": "main",
		"spec":     spec,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

// TestRunsCreateSingleRepo_MissingSpec verifies POST /v1/runs rejects missing spec.
func TestRunsCreateSingleRepo_MissingSpec(t *testing.T) {
	st := &mockStore{}
	handler := createSingleRepoRunHandler(st, nil)

	reqBody := map[string]any{
		"repo_url":   "https://github.com/org/repo",
		"base_ref":   "main",
		"target_ref": "feature",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

// TestRunsCreateSingleRepo_InvalidSpec verifies POST /v1/runs rejects invalid spec.
func TestRunsCreateSingleRepo_InvalidSpec(t *testing.T) {
	st := &mockStore{}
	handler := createSingleRepoRunHandler(st, nil)

	// Legacy spec shape with "mod" key is rejected.
	reqBody := map[string]any{
		"repo_url":   "https://github.com/org/repo",
		"base_ref":   "main",
		"target_ref": "feature",
		"spec":       map[string]any{"mod": map[string]any{"command": "echo hello"}},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusBadRequest, rr.Body.String())
	}

	// Store should not be called.
	if st.createSpecCalled {
		t.Error("store.CreateSpec should not be called for invalid spec")
	}
}

// TestRunsCreateSingleRepo_InvalidJSON verifies POST /v1/runs rejects malformed JSON.
func TestRunsCreateSingleRepo_InvalidJSON(t *testing.T) {
	st := &mockStore{}
	handler := createSingleRepoRunHandler(st, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

// TestRunsCreateSingleRepo_WithCreatedBy verifies POST /v1/runs accepts optional created_by.
func TestRunsCreateSingleRepo_WithCreatedBy(t *testing.T) {
	st := &mockStore{}
	eventsService, _ := createTestEventsService()
	handler := createSingleRepoRunHandler(st, eventsService)

	spec := map[string]any{
		"version": "0.2.0",
		"env":     map[string]any{},
		"steps":   []any{map[string]any{"image": "docker.io/test/mod:latest"}},
	}
	reqBody := map[string]any{
		"repo_url":   "https://github.com/org/repo",
		"base_ref":   "main",
		"target_ref": "feature",
		"spec":       spec,
		"created_by": "test-user@example.com",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusCreated, rr.Body.String())
	}

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

// TestRunsCreateSingleRepo_MultiStepSpec verifies POST /v1/runs creates jobs for multi-step spec.
func TestRunsCreateSingleRepo_MultiStepSpec(t *testing.T) {
	st := &mockStore{}
	eventsService, _ := createTestEventsService()
	handler := createSingleRepoRunHandler(st, eventsService)

	// Multi-step spec with steps[] array.
	spec := map[string]any{
		"version": "0.2.0",
		"env":     map[string]any{},
		"steps": []any{
			map[string]any{"image": "mod-image-1"},
			map[string]any{"image": "mod-image-2"},
		},
	}
	reqBody := map[string]any{
		"repo_url":   "https://github.com/org/repo",
		"base_ref":   "main",
		"target_ref": "feature",
		"spec":       spec,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusCreated, rr.Body.String())
	}

	// Multi-step spec creates: pre-gate + mod-0 + mod-1 + post-gate = 4 jobs.
	expectedJobCount := 4
	if st.createJobCallCount != expectedJobCount {
		t.Errorf("createJobCallCount = %d, want %d", st.createJobCallCount, expectedJobCount)
	}
}

// =============================================================================
// Store Error Tests
// =============================================================================

// TestRunsCreateSingleRepo_CreateSpecError verifies POST /v1/runs returns 500 on CreateSpec failure.
func TestRunsCreateSingleRepo_CreateSpecError(t *testing.T) {
	st := &mockStore{
		createSpecErr: errors.New("database connection failed"),
	}
	handler := createSingleRepoRunHandler(st, nil)

	spec := map[string]any{
		"version": "0.2.0",
		"env":     map[string]any{},
		"steps":   []any{map[string]any{"image": "docker.io/test/mod:latest"}},
	}
	reqBody := map[string]any{
		"repo_url":   "https://github.com/org/repo",
		"base_ref":   "main",
		"target_ref": "feature",
		"spec":       spec,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

// TestRunsCreateSingleRepo_CreateMigError verifies POST /v1/runs returns 500 on CreateMig failure.
func TestRunsCreateSingleRepo_CreateMigError(t *testing.T) {
	st := &mockStore{
		createMigErr: errors.New("database connection failed"),
	}
	handler := createSingleRepoRunHandler(st, nil)

	spec := map[string]any{
		"version": "0.2.0",
		"env":     map[string]any{},
		"steps":   []any{map[string]any{"image": "docker.io/test/mod:latest"}},
	}
	reqBody := map[string]any{
		"repo_url":   "https://github.com/org/repo",
		"base_ref":   "main",
		"target_ref": "feature",
		"spec":       spec,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

// TestRunsCreateSingleRepo_CreateMigRepoError verifies POST /v1/runs returns 500 on CreateMigRepo failure.
func TestRunsCreateSingleRepo_CreateMigRepoError(t *testing.T) {
	st := &mockStore{
		createMigRepoErr: errors.New("database connection failed"),
	}
	handler := createSingleRepoRunHandler(st, nil)

	spec := map[string]any{
		"version": "0.2.0",
		"env":     map[string]any{},
		"steps":   []any{map[string]any{"image": "docker.io/test/mod:latest"}},
	}
	reqBody := map[string]any{
		"repo_url":   "https://github.com/org/repo",
		"base_ref":   "main",
		"target_ref": "feature",
		"spec":       spec,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

// TestRunsCreateSingleRepo_CreateRunError verifies POST /v1/runs returns 500 on CreateRun failure.
func TestRunsCreateSingleRepo_CreateRunError(t *testing.T) {
	st := &mockStore{
		createRunErr: errors.New("database connection failed"),
	}
	handler := createSingleRepoRunHandler(st, nil)

	spec := map[string]any{
		"version": "0.2.0",
		"env":     map[string]any{},
		"steps":   []any{map[string]any{"image": "docker.io/test/mod:latest"}},
	}
	reqBody := map[string]any{
		"repo_url":   "https://github.com/org/repo",
		"base_ref":   "main",
		"target_ref": "feature",
		"spec":       spec,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

// TestRunsCreateSingleRepo_CreateRunRepoError verifies POST /v1/runs returns 500 on CreateRunRepo failure.
func TestRunsCreateSingleRepo_CreateRunRepoError(t *testing.T) {
	st := &mockStore{
		createRunRepoErr: errors.New("database connection failed"),
	}
	handler := createSingleRepoRunHandler(st, nil)

	spec := map[string]any{
		"version": "0.2.0",
		"env":     map[string]any{},
		"steps":   []any{map[string]any{"image": "docker.io/test/mod:latest"}},
	}
	reqBody := map[string]any{
		"repo_url":   "https://github.com/org/repo",
		"base_ref":   "main",
		"target_ref": "feature",
		"spec":       spec,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

// TestRunsCreateSingleRepo_CreateJobError verifies POST /v1/runs returns 500 on CreateJob failure.
func TestRunsCreateSingleRepo_CreateJobError(t *testing.T) {
	st := &mockStore{
		createJobErr: errors.New("database connection failed"),
	}
	handler := createSingleRepoRunHandler(st, nil)

	spec := map[string]any{
		"version": "0.2.0",
		"env":     map[string]any{},
		"steps":   []any{map[string]any{"image": "docker.io/test/mod:latest"}},
	}
	reqBody := map[string]any{
		"repo_url":   "https://github.com/org/repo",
		"base_ref":   "main",
		"target_ref": "feature",
		"spec":       spec,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}
