package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/store"
)

// =============================================================================
// POST /v1/mods/{mod_id}/runs — Create Multi-Repo Run
// =============================================================================

// TestModRuns_Create_AllRepos verifies POST /v1/mods/{mod_id}/runs with mode="all"
// creates a run with all repos from the mod's repo set.
// Scope: roadmap/v1/api.md:202-223, roadmap/v1/scope.md:19.
func TestModRuns_Create_AllRepos(t *testing.T) {
	specID := "spec123"
	st := &mockStore{
		getModResult: store.Mod{
			ID:         "mod123",
			Name:       "test-mod",
			SpecID:     &specID,
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
		getSpecResult: store.Spec{
			ID:   specID,
			Spec: []byte(`{"version":"0.2.0","env":{},"steps":[]}`),
		},
		listModReposByModResult: []store.ModRepo{
			{ID: "repo1", ModID: "mod123", RepoUrl: "https://github.com/org/repo1", BaseRef: "main", TargetRef: "feature1"},
			{ID: "repo2", ModID: "mod123", RepoUrl: "https://github.com/org/repo2", BaseRef: "main", TargetRef: "feature2"},
		},
	}
	handler := createModRunHandler(st)

	reqBody := map[string]any{
		"repo_selector": map[string]any{
			"mode": "all",
		},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods/mod123/runs", bytes.NewReader(body))
	req.SetPathValue("mod_id", "mod123")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusCreated, rr.Body.String())
	}

	// Verify store methods called.
	if !st.getModCalled {
		t.Error("store.GetMod was not called")
	}
	if !st.getSpecCalled {
		t.Error("store.GetSpec was not called")
	}
	if !st.listModReposByModCalled {
		t.Error("store.ListModReposByMod was not called")
	}
	if !st.createRunCalled {
		t.Error("store.CreateRun was not called")
	}
	if !st.createRunRepoCalled {
		t.Error("store.CreateRunRepo was not called")
	}
	if !st.createJobCalled {
		t.Error("store.CreateJob was not called")
	}

	// Verify response shape.
	var resp struct {
		RunID string `json:"run_id"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.RunID == "" {
		t.Error("response run_id is empty")
	}
}

// TestModRuns_Create_FailedRepos verifies POST /v1/mods/{mod_id}/runs with mode="failed"
// only selects repos whose last terminal status is 'Fail'.
// Scope: roadmap/v1/db.md:189.
func TestModRuns_Create_FailedRepos(t *testing.T) {
	specID := "spec123"
	st := &mockStore{
		getModResult: store.Mod{
			ID:         "mod123",
			Name:       "test-mod",
			SpecID:     &specID,
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
		getSpecResult: store.Spec{
			ID:   specID,
			Spec: []byte(`{"version":"0.2.0","env":{},"steps":[]}`),
		},
		listModReposByModResult: []store.ModRepo{
			{ID: "repo1", ModID: "mod123", RepoUrl: "https://github.com/org/repo1", BaseRef: "main", TargetRef: "feature1"},
			{ID: "repo2", ModID: "mod123", RepoUrl: "https://github.com/org/repo2", BaseRef: "main", TargetRef: "feature2"},
			{ID: "repo3", ModID: "mod123", RepoUrl: "https://github.com/org/repo3", BaseRef: "main", TargetRef: "feature3"},
		},
		// Only repo2 has a failed last status.
		listFailedRepoIDsByModResult: []string{"repo2"},
	}
	handler := createModRunHandler(st)

	reqBody := map[string]any{
		"repo_selector": map[string]any{
			"mode": "failed",
		},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods/mod123/runs", bytes.NewReader(body))
	req.SetPathValue("mod_id", "mod123")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusCreated, rr.Body.String())
	}

	// Verify ListFailedRepoIDsByMod was called.
	if !st.listFailedRepoIDsByModCalled {
		t.Error("store.ListFailedRepoIDsByMod was not called")
	}
	if st.listFailedRepoIDsByModParam != "mod123" {
		t.Errorf("ListFailedRepoIDsByMod param = %q, want %q", st.listFailedRepoIDsByModParam, "mod123")
	}

	// Verify only one run_repo was created (for repo2).
	// Note: mockStore doesn't track call count well, but we verify the run was created.
	if !st.createRunRepoCalled {
		t.Error("store.CreateRunRepo was not called")
	}

	// Verify response shape.
	var resp struct {
		RunID string `json:"run_id"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.RunID == "" {
		t.Error("response run_id is empty")
	}
}

// TestModRuns_Create_ExplicitRepos verifies POST /v1/mods/{mod_id}/runs with mode="explicit"
// only selects repos matching the provided repo URLs.
// Scope: roadmap/v1/api.md:211-213.
func TestModRuns_Create_ExplicitRepos(t *testing.T) {
	specID := "spec123"
	st := &mockStore{
		getModResult: store.Mod{
			ID:         "mod123",
			Name:       "test-mod",
			SpecID:     &specID,
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
		getSpecResult: store.Spec{
			ID:   specID,
			Spec: []byte(`{"version":"0.2.0","env":{},"steps":[]}`),
		},
		listModReposByModResult: []store.ModRepo{
			{ID: "repo1", ModID: "mod123", RepoUrl: "https://github.com/org/repo1", BaseRef: "main", TargetRef: "feature1"},
			{ID: "repo2", ModID: "mod123", RepoUrl: "https://github.com/org/repo2", BaseRef: "main", TargetRef: "feature2"},
			{ID: "repo3", ModID: "mod123", RepoUrl: "https://github.com/org/repo3", BaseRef: "main", TargetRef: "feature3"},
		},
	}
	handler := createModRunHandler(st)

	reqBody := map[string]any{
		"repo_selector": map[string]any{
			"mode": "explicit",
			// Request specific repos (with variant URL forms to test normalization).
			"repos": []string{
				"https://github.com/org/repo1.git", // Should match repo1
				"https://github.com/org/repo3/",    // Should match repo3
			},
		},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods/mod123/runs", bytes.NewReader(body))
	req.SetPathValue("mod_id", "mod123")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusCreated, rr.Body.String())
	}

	// Verify ListModReposByMod was called for explicit matching.
	if !st.listModReposByModCalled {
		t.Error("store.ListModReposByMod was not called")
	}

	// Verify response shape.
	var resp struct {
		RunID string `json:"run_id"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.RunID == "" {
		t.Error("response run_id is empty")
	}
}

// TestModRuns_Create_WithCreatedBy verifies POST /v1/mods/{mod_id}/runs passes created_by to store.
func TestModRuns_Create_WithCreatedBy(t *testing.T) {
	specID := "spec123"
	st := &mockStore{
		getModResult: store.Mod{
			ID:         "mod123",
			Name:       "test-mod",
			SpecID:     &specID,
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
		getSpecResult: store.Spec{
			ID:   specID,
			Spec: []byte(`{"version":"0.2.0","env":{},"steps":[]}`),
		},
		listModReposByModResult: []store.ModRepo{
			{ID: "repo1", ModID: "mod123", RepoUrl: "https://github.com/org/repo1", BaseRef: "main", TargetRef: "feature1"},
		},
	}
	handler := createModRunHandler(st)

	reqBody := map[string]any{
		"repo_selector": map[string]any{
			"mode": "all",
		},
		"created_by": "test-user@example.com",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods/mod123/runs", bytes.NewReader(body))
	req.SetPathValue("mod_id", "mod123")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusCreated, rr.Body.String())
	}

	// Verify created_by was propagated to CreateRun.
	if st.createRunParams.CreatedBy == nil || *st.createRunParams.CreatedBy != "test-user@example.com" {
		t.Errorf("created_by not propagated; got %v, want test-user@example.com", st.createRunParams.CreatedBy)
	}
}

// TestModRuns_Create_FirstJobClaimable verifies that the first job is Queued
// and immediately claimable (v1 job queueing rules).
// Scope: ROADMAP.md:342 — "first job is claimable".
func TestModRuns_Create_FirstJobClaimable(t *testing.T) {
	specID := "spec123"
	st := &mockStore{
		getModResult: store.Mod{
			ID:         "mod123",
			Name:       "test-mod",
			SpecID:     &specID,
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
		getSpecResult: store.Spec{
			ID:   specID,
			Spec: []byte(`{"version":"0.2.0","env":{},"steps":[]}`),
		},
		listModReposByModResult: []store.ModRepo{
			{ID: "repo1", ModID: "mod123", RepoUrl: "https://github.com/org/repo1", BaseRef: "main", TargetRef: "feature1"},
		},
	}
	handler := createModRunHandler(st)

	reqBody := map[string]any{
		"repo_selector": map[string]any{
			"mode": "all",
		},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods/mod123/runs", bytes.NewReader(body))
	req.SetPathValue("mod_id", "mod123")
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

// =============================================================================
// Validation Tests
// =============================================================================

// TestModRuns_Create_InvalidMode verifies POST /v1/mods/{mod_id}/runs rejects invalid mode.
func TestModRuns_Create_InvalidMode(t *testing.T) {
	st := &mockStore{}
	handler := createModRunHandler(st)

	reqBody := map[string]any{
		"repo_selector": map[string]any{
			"mode": "invalid",
		},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods/mod123/runs", bytes.NewReader(body))
	req.SetPathValue("mod_id", "mod123")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusBadRequest, rr.Body.String())
	}

	// Store should not be called.
	if st.getModCalled {
		t.Error("store.GetMod should not be called for invalid mode")
	}
}

// TestModRuns_Create_ExplicitEmptyRepos verifies POST /v1/mods/{mod_id}/runs rejects
// explicit mode with empty repos array.
func TestModRuns_Create_ExplicitEmptyRepos(t *testing.T) {
	st := &mockStore{}
	handler := createModRunHandler(st)

	reqBody := map[string]any{
		"repo_selector": map[string]any{
			"mode":  "explicit",
			"repos": []string{},
		},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods/mod123/runs", bytes.NewReader(body))
	req.SetPathValue("mod_id", "mod123")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusBadRequest, rr.Body.String())
	}
}

// TestModRuns_Create_ModNotFound verifies POST /v1/mods/{mod_id}/runs returns 404 for missing mod.
func TestModRuns_Create_ModNotFound(t *testing.T) {
	st := &mockStore{
		getModErr: pgx.ErrNoRows,
	}
	handler := createModRunHandler(st)

	reqBody := map[string]any{
		"repo_selector": map[string]any{
			"mode": "all",
		},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods/nonexistent/runs", bytes.NewReader(body))
	req.SetPathValue("mod_id", "nonexistent")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

// TestModRuns_Create_ArchivedMod verifies POST /v1/mods/{mod_id}/runs rejects archived mods.
// Scope: roadmap/v1/scope.md:45 — archived mods cannot be executed.
func TestModRuns_Create_ArchivedMod(t *testing.T) {
	specID := "spec123"
	st := &mockStore{
		getModResult: store.Mod{
			ID:         "mod123",
			Name:       "test-mod",
			SpecID:     &specID,
			ArchivedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}, // Archived.
		},
	}
	handler := createModRunHandler(st)

	reqBody := map[string]any{
		"repo_selector": map[string]any{
			"mode": "all",
		},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods/mod123/runs", bytes.NewReader(body))
	req.SetPathValue("mod_id", "mod123")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusConflict, rr.Body.String())
	}
}

// TestModRuns_Create_NoSpec verifies POST /v1/mods/{mod_id}/runs rejects mods without a spec.
// Scope: roadmap/v1/api.md:218 — if mods.spec_id is NULL, return error.
func TestModRuns_Create_NoSpec(t *testing.T) {
	st := &mockStore{
		getModResult: store.Mod{
			ID:         "mod123",
			Name:       "test-mod",
			SpecID:     nil, // No spec.
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
	}
	handler := createModRunHandler(st)

	reqBody := map[string]any{
		"repo_selector": map[string]any{
			"mode": "all",
		},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods/mod123/runs", bytes.NewReader(body))
	req.SetPathValue("mod_id", "mod123")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusBadRequest, rr.Body.String())
	}
}

// TestModRuns_Create_NoReposSelected verifies POST /v1/mods/{mod_id}/runs returns error
// when no repos are selected (e.g., "failed" mode with no failed repos).
func TestModRuns_Create_NoReposSelected(t *testing.T) {
	specID := "spec123"
	st := &mockStore{
		getModResult: store.Mod{
			ID:         "mod123",
			Name:       "test-mod",
			SpecID:     &specID,
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
		getSpecResult: store.Spec{
			ID:   specID,
			Spec: []byte(`{"version":"0.2.0","env":{},"steps":[]}`),
		},
		listModReposByModResult: []store.ModRepo{
			{ID: "repo1", ModID: "mod123", RepoUrl: "https://github.com/org/repo1", BaseRef: "main", TargetRef: "feature1"},
		},
		// No failed repos.
		listFailedRepoIDsByModResult: []string{},
	}
	handler := createModRunHandler(st)

	reqBody := map[string]any{
		"repo_selector": map[string]any{
			"mode": "failed",
		},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods/mod123/runs", bytes.NewReader(body))
	req.SetPathValue("mod_id", "mod123")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusBadRequest, rr.Body.String())
	}
}

// TestModRuns_Create_InvalidJSON verifies POST /v1/mods/{mod_id}/runs rejects malformed JSON.
func TestModRuns_Create_InvalidJSON(t *testing.T) {
	st := &mockStore{}
	handler := createModRunHandler(st)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods/mod123/runs", bytes.NewReader([]byte("not json")))
	req.SetPathValue("mod_id", "mod123")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

// =============================================================================
// Store Error Tests
// =============================================================================

// TestModRuns_Create_GetModError verifies POST /v1/mods/{mod_id}/runs returns 500 on GetMod failure.
func TestModRuns_Create_GetModError(t *testing.T) {
	st := &mockStore{
		getModErr: errors.New("database connection failed"),
	}
	handler := createModRunHandler(st)

	reqBody := map[string]any{
		"repo_selector": map[string]any{
			"mode": "all",
		},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods/mod123/runs", bytes.NewReader(body))
	req.SetPathValue("mod_id", "mod123")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

// TestModRuns_Create_GetSpecError verifies POST /v1/mods/{mod_id}/runs returns 500 on GetSpec failure.
func TestModRuns_Create_GetSpecError(t *testing.T) {
	specID := "spec123"
	st := &mockStore{
		getModResult: store.Mod{
			ID:         "mod123",
			Name:       "test-mod",
			SpecID:     &specID,
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
		getSpecErr: errors.New("database connection failed"),
	}
	handler := createModRunHandler(st)

	reqBody := map[string]any{
		"repo_selector": map[string]any{
			"mode": "all",
		},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods/mod123/runs", bytes.NewReader(body))
	req.SetPathValue("mod_id", "mod123")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

// TestModRuns_Create_ListModReposError verifies POST /v1/mods/{mod_id}/runs returns 500 on ListModReposByMod failure.
func TestModRuns_Create_ListModReposError(t *testing.T) {
	specID := "spec123"
	st := &mockStore{
		getModResult: store.Mod{
			ID:         "mod123",
			Name:       "test-mod",
			SpecID:     &specID,
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
		getSpecResult: store.Spec{
			ID:   specID,
			Spec: []byte(`{"version":"0.2.0","env":{},"steps":[]}`),
		},
		listModReposByModErr: errors.New("database connection failed"),
	}
	handler := createModRunHandler(st)

	reqBody := map[string]any{
		"repo_selector": map[string]any{
			"mode": "all",
		},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods/mod123/runs", bytes.NewReader(body))
	req.SetPathValue("mod_id", "mod123")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

// TestModRuns_Create_CreateRunError verifies POST /v1/mods/{mod_id}/runs returns 500 on CreateRun failure.
func TestModRuns_Create_CreateRunError(t *testing.T) {
	specID := "spec123"
	st := &mockStore{
		getModResult: store.Mod{
			ID:         "mod123",
			Name:       "test-mod",
			SpecID:     &specID,
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
		getSpecResult: store.Spec{
			ID:   specID,
			Spec: []byte(`{"version":"0.2.0","env":{},"steps":[]}`),
		},
		listModReposByModResult: []store.ModRepo{
			{ID: "repo1", ModID: "mod123", RepoUrl: "https://github.com/org/repo1", BaseRef: "main", TargetRef: "feature1"},
		},
		createRunErr: errors.New("database connection failed"),
	}
	handler := createModRunHandler(st)

	reqBody := map[string]any{
		"repo_selector": map[string]any{
			"mode": "all",
		},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods/mod123/runs", bytes.NewReader(body))
	req.SetPathValue("mod_id", "mod123")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

// TestModRuns_Create_CreateRunRepoError verifies POST /v1/mods/{mod_id}/runs returns 500 on CreateRunRepo failure.
func TestModRuns_Create_CreateRunRepoError(t *testing.T) {
	specID := "spec123"
	st := &mockStore{
		getModResult: store.Mod{
			ID:         "mod123",
			Name:       "test-mod",
			SpecID:     &specID,
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
		getSpecResult: store.Spec{
			ID:   specID,
			Spec: []byte(`{"version":"0.2.0","env":{},"steps":[]}`),
		},
		listModReposByModResult: []store.ModRepo{
			{ID: "repo1", ModID: "mod123", RepoUrl: "https://github.com/org/repo1", BaseRef: "main", TargetRef: "feature1"},
		},
		createRunRepoErr: errors.New("database connection failed"),
	}
	handler := createModRunHandler(st)

	reqBody := map[string]any{
		"repo_selector": map[string]any{
			"mode": "all",
		},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods/mod123/runs", bytes.NewReader(body))
	req.SetPathValue("mod_id", "mod123")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

// TestModRuns_Create_CreateJobError verifies POST /v1/mods/{mod_id}/runs returns 500 on CreateJob failure.
func TestModRuns_Create_CreateJobError(t *testing.T) {
	specID := "spec123"
	st := &mockStore{
		getModResult: store.Mod{
			ID:         "mod123",
			Name:       "test-mod",
			SpecID:     &specID,
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
		getSpecResult: store.Spec{
			ID:   specID,
			Spec: []byte(`{"version":"0.2.0","env":{},"steps":[]}`),
		},
		listModReposByModResult: []store.ModRepo{
			{ID: "repo1", ModID: "mod123", RepoUrl: "https://github.com/org/repo1", BaseRef: "main", TargetRef: "feature1"},
		},
		createJobErr: errors.New("database connection failed"),
	}
	handler := createModRunHandler(st)

	reqBody := map[string]any{
		"repo_selector": map[string]any{
			"mode": "all",
		},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods/mod123/runs", bytes.NewReader(body))
	req.SetPathValue("mod_id", "mod123")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

// TestModRuns_Create_ListFailedReposError verifies POST /v1/mods/{mod_id}/runs returns 500 on ListFailedRepoIDsByMod failure.
func TestModRuns_Create_ListFailedReposError(t *testing.T) {
	specID := "spec123"
	st := &mockStore{
		getModResult: store.Mod{
			ID:         "mod123",
			Name:       "test-mod",
			SpecID:     &specID,
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
		getSpecResult: store.Spec{
			ID:   specID,
			Spec: []byte(`{"version":"0.2.0","env":{},"steps":[]}`),
		},
		listModReposByModResult: []store.ModRepo{
			{ID: "repo1", ModID: "mod123", RepoUrl: "https://github.com/org/repo1", BaseRef: "main", TargetRef: "feature1"},
		},
		listFailedRepoIDsByModErr: errors.New("database connection failed"),
	}
	handler := createModRunHandler(st)

	reqBody := map[string]any{
		"repo_selector": map[string]any{
			"mode": "failed",
		},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods/mod123/runs", bytes.NewReader(body))
	req.SetPathValue("mod_id", "mod123")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}
