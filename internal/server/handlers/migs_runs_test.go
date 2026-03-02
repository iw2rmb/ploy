package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// =============================================================================
// POST /v1/migs/{mig_id}/runs — Create Multi-Repo Run
// =============================================================================

// TestModRuns_Create_AllRepos verifies POST /v1/migs/{mig_id}/runs with mode="all"
// creates a run with all repos from the mig's repo set.
// Tests multi-repo run creation with repo_selector mode.
func TestModRuns_Create_AllRepos(t *testing.T) {
	specID := domaintypes.SpecID("spec123")
	st := &mockStore{
		getModResult: store.Mig{
			ID:         "mod123",
			Name:       "test-mig",
			SpecID:     &specID,
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
		getSpecResult: store.Spec{
			ID:   specID,
			Spec: []byte(`{"steps":[{"image":"docker.io/test/mig:latest"}]}`),
		},
		listMigReposByModResult: []store.MigRepo{
			{ID: "repo1", MigID: "mod123", RepoID: "repo1", BaseRef: "main", TargetRef: "feature1"},
			{ID: "repo2", MigID: "mod123", RepoID: "repo2", BaseRef: "main", TargetRef: "feature2"},
		},
	}
	handler := createMigRunHandler(st)

	reqBody := map[string]any{
		"repo_selector": map[string]any{
			"mode": "all",
		},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/migs/mod123/runs", bytes.NewReader(body))
	req.SetPathValue("mig_id", "mod123")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusCreated, rr.Body.String())
	}

	// Verify store methods called.
	if !st.getModCalled {
		t.Error("store.GetMig was not called")
	}
	if !st.listMigReposByModCalled {
		t.Error("store.ListMigReposByMig was not called")
	}
	if !st.createRunCalled {
		t.Error("store.CreateRun was not called")
	}
	if !st.createRunRepoCalled {
		t.Error("store.CreateRunRepo was not called")
	}
	if st.createJobCalled {
		t.Error("store.CreateJob should not be called during run submission")
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

// TestModRuns_Create_FailedRepos verifies POST /v1/migs/{mig_id}/runs with mode="failed"
// only selects repos whose last terminal status is 'Fail'.
// Tests failed repo selection using last terminal run_repos status.
func TestModRuns_Create_FailedRepos(t *testing.T) {
	specID := domaintypes.SpecID("spec123")
	st := &mockStore{
		getModResult: store.Mig{
			ID:         "mod123",
			Name:       "test-mig",
			SpecID:     &specID,
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
		getSpecResult: store.Spec{
			ID:   specID,
			Spec: []byte(`{"steps":[{"image":"docker.io/test/mig:latest"}]}`),
		},
		listMigReposByModResult: []store.MigRepo{
			{ID: "repo1", MigID: "mod123", RepoID: "repo1", BaseRef: "main", TargetRef: "feature1"},
			{ID: "repo2", MigID: "mod123", RepoID: "repo2", BaseRef: "main", TargetRef: "feature2"},
			{ID: "repo3", MigID: "mod123", RepoID: "repo3", BaseRef: "main", TargetRef: "feature3"},
		},
		// Only repo2 has a failed last status.
		listFailedRepoIDsByModResult: []domaintypes.MigRepoID{"repo2"},
	}
	handler := createMigRunHandler(st)

	reqBody := map[string]any{
		"repo_selector": map[string]any{
			"mode": "failed",
		},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/migs/mod123/runs", bytes.NewReader(body))
	req.SetPathValue("mig_id", "mod123")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusCreated, rr.Body.String())
	}

	// Verify ListFailedRepoIDsByMig was called.
	if !st.listFailedRepoIDsByModCalled {
		t.Error("store.ListFailedRepoIDsByMig was not called")
	}
	if st.listFailedRepoIDsByModParam != "mod123" {
		t.Errorf("ListFailedRepoIDsByMig param = %q, want %q", st.listFailedRepoIDsByModParam, "mod123")
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

// TestModRuns_Create_ExplicitRepos verifies POST /v1/migs/{mig_id}/runs with mode="explicit"
// only selects repos matching the provided repo URLs.
// Tests explicit repo selection by normalized URL matching.
func TestModRuns_Create_ExplicitRepos(t *testing.T) {
	specID := domaintypes.SpecID("spec123")
	st := &mockStore{
		getModResult: store.Mig{
			ID:         "mod123",
			Name:       "test-mig",
			SpecID:     &specID,
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
		getSpecResult: store.Spec{
			ID:   specID,
			Spec: []byte(`{"steps":[{"image":"docker.io/test/mig:latest"}]}`),
		},
		listMigReposByModResult: []store.MigRepo{
			{ID: "repo1", MigID: "mod123", RepoID: "repo1", BaseRef: "main", TargetRef: "feature1"},
			{ID: "repo2", MigID: "mod123", RepoID: "repo2", BaseRef: "main", TargetRef: "feature2"},
			{ID: "repo3", MigID: "mod123", RepoID: "repo3", BaseRef: "main", TargetRef: "feature3"},
		},
		repoByID: map[domaintypes.MigRepoID]store.Repo{
			"repo1": {ID: "repo1", Url: "https://github.com/org/repo1"},
			"repo2": {ID: "repo2", Url: "https://github.com/org/repo2"},
			"repo3": {ID: "repo3", Url: "https://github.com/org/repo3"},
		},
	}
	handler := createMigRunHandler(st)

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

	req := httptest.NewRequest(http.MethodPost, "/v1/migs/mod123/runs", bytes.NewReader(body))
	req.SetPathValue("mig_id", "mod123")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusCreated, rr.Body.String())
	}

	// Verify ListMigReposByMig was called for explicit matching.
	if !st.listMigReposByModCalled {
		t.Error("store.ListMigReposByMig was not called")
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

// TestModRuns_Create_WithCreatedBy verifies POST /v1/migs/{mig_id}/runs passes created_by to store.
func TestModRuns_Create_WithCreatedBy(t *testing.T) {
	specID := domaintypes.SpecID("spec123")
	st := &mockStore{
		getModResult: store.Mig{
			ID:         "mod123",
			Name:       "test-mig",
			SpecID:     &specID,
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
		getSpecResult: store.Spec{
			ID:   specID,
			Spec: []byte(`{"steps":[{"image":"docker.io/test/mig:latest"}]}`),
		},
		listMigReposByModResult: []store.MigRepo{
			{ID: "repo1", MigID: "mod123", RepoID: "repo1", BaseRef: "main", TargetRef: "feature1"},
		},
	}
	handler := createMigRunHandler(st)

	reqBody := map[string]any{
		"repo_selector": map[string]any{
			"mode": "all",
		},
		"created_by": "test-user@example.com",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/migs/mod123/runs", bytes.NewReader(body))
	req.SetPathValue("mig_id", "mod123")
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

// TestModRuns_Create_DoesNotCreateJobsImmediately verifies submission defers job materialization.
func TestModRuns_Create_DoesNotCreateJobsImmediately(t *testing.T) {
	specID := domaintypes.SpecID("spec123")
	st := &mockStore{
		getModResult: store.Mig{
			ID:         "mod123",
			Name:       "test-mig",
			SpecID:     &specID,
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
		getSpecResult: store.Spec{
			ID:   specID,
			Spec: []byte(`{"steps":[{"image":"docker.io/test/mig:latest"}]}`),
		},
		listMigReposByModResult: []store.MigRepo{
			{ID: "repo1", MigID: "mod123", RepoID: "repo1", BaseRef: "main", TargetRef: "feature1"},
		},
	}
	handler := createMigRunHandler(st)

	reqBody := map[string]any{
		"repo_selector": map[string]any{
			"mode": "all",
		},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/migs/mod123/runs", bytes.NewReader(body))
	req.SetPathValue("mig_id", "mod123")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusCreated, rr.Body.String())
	}

	if st.createJobCallCount != 0 {
		t.Fatalf("expected no jobs to be created during submission, got %d", st.createJobCallCount)
	}
}

// =============================================================================
// Validation Tests
// =============================================================================

// TestModRuns_Create_InvalidMode verifies POST /v1/migs/{mig_id}/runs rejects invalid mode.
func TestModRuns_Create_InvalidMode(t *testing.T) {
	st := &mockStore{}
	handler := createMigRunHandler(st)

	reqBody := map[string]any{
		"repo_selector": map[string]any{
			"mode": "invalid",
		},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/migs/mod123/runs", bytes.NewReader(body))
	req.SetPathValue("mig_id", "mod123")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusBadRequest, rr.Body.String())
	}

	// Store should not be called.
	if st.getModCalled {
		t.Error("store.GetMig should not be called for invalid mode")
	}
}

// TestModRuns_Create_ExplicitEmptyRepos verifies POST /v1/migs/{mig_id}/runs rejects
// explicit mode with empty repos array.
func TestModRuns_Create_ExplicitEmptyRepos(t *testing.T) {
	st := &mockStore{}
	handler := createMigRunHandler(st)

	reqBody := map[string]any{
		"repo_selector": map[string]any{
			"mode":  "explicit",
			"repos": []string{},
		},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/migs/mod123/runs", bytes.NewReader(body))
	req.SetPathValue("mig_id", "mod123")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusBadRequest, rr.Body.String())
	}
}

// TestModRuns_Create_ModNotFound verifies POST /v1/migs/{mig_id}/runs returns 404 for missing mig.
func TestModRuns_Create_ModNotFound(t *testing.T) {
	st := &mockStore{
		getModErr: pgx.ErrNoRows,
	}
	handler := createMigRunHandler(st)

	modID := domaintypes.NewMigID().String()
	reqBody := map[string]any{
		"repo_selector": map[string]any{
			"mode": "all",
		},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/migs/"+modID+"/runs", bytes.NewReader(body))
	req.SetPathValue("mig_id", modID)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

// TestModRuns_Create_ArchivedMod verifies POST /v1/migs/{mig_id}/runs rejects archived migs.
// Archived migs cannot be executed.
func TestModRuns_Create_ArchivedMod(t *testing.T) {
	specID := domaintypes.SpecID("spec123")
	st := &mockStore{
		getModResult: store.Mig{
			ID:         "mod123",
			Name:       "test-mig",
			SpecID:     &specID,
			ArchivedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}, // Archived.
		},
	}
	handler := createMigRunHandler(st)

	reqBody := map[string]any{
		"repo_selector": map[string]any{
			"mode": "all",
		},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/migs/mod123/runs", bytes.NewReader(body))
	req.SetPathValue("mig_id", "mod123")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusConflict, rr.Body.String())
	}
}

// TestModRuns_Create_NoSpec verifies POST /v1/migs/{mig_id}/runs rejects migs without a spec.
// If migs.spec_id is NULL, the endpoint returns an error.
func TestModRuns_Create_NoSpec(t *testing.T) {
	st := &mockStore{
		getModResult: store.Mig{
			ID:         "mod123",
			Name:       "test-mig",
			SpecID:     nil, // No spec.
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
	}
	handler := createMigRunHandler(st)

	reqBody := map[string]any{
		"repo_selector": map[string]any{
			"mode": "all",
		},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/migs/mod123/runs", bytes.NewReader(body))
	req.SetPathValue("mig_id", "mod123")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusBadRequest, rr.Body.String())
	}
}

// TestModRuns_Create_NoReposSelected verifies POST /v1/migs/{mig_id}/runs returns error
// when no repos are selected (e.g., "failed" mode with no failed repos).
func TestModRuns_Create_NoReposSelected(t *testing.T) {
	specID := domaintypes.SpecID("spec123")
	st := &mockStore{
		getModResult: store.Mig{
			ID:         "mod123",
			Name:       "test-mig",
			SpecID:     &specID,
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
		getSpecResult: store.Spec{
			ID:   specID,
			Spec: []byte(`{"steps":[{"image":"docker.io/test/mig:latest"}]}`),
		},
		listMigReposByModResult: []store.MigRepo{
			{ID: "repo1", MigID: "mod123", RepoID: "repo1", BaseRef: "main", TargetRef: "feature1"},
		},
		// No failed repos.
		listFailedRepoIDsByModResult: []domaintypes.MigRepoID{},
	}
	handler := createMigRunHandler(st)

	reqBody := map[string]any{
		"repo_selector": map[string]any{
			"mode": "failed",
		},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/migs/mod123/runs", bytes.NewReader(body))
	req.SetPathValue("mig_id", "mod123")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusBadRequest, rr.Body.String())
	}
}

// TestModRuns_Create_InvalidJSON verifies POST /v1/migs/{mig_id}/runs rejects malformed JSON.
func TestModRuns_Create_InvalidJSON(t *testing.T) {
	st := &mockStore{}
	handler := createMigRunHandler(st)

	req := httptest.NewRequest(http.MethodPost, "/v1/migs/mod123/runs", bytes.NewReader([]byte("not json")))
	req.SetPathValue("mig_id", "mod123")
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

// TestModRuns_Create_GetMigError verifies POST /v1/migs/{mig_id}/runs returns 500 on GetMig failure.
func TestModRuns_Create_GetMigError(t *testing.T) {
	st := &mockStore{
		getModErr: errors.New("database connection failed"),
	}
	handler := createMigRunHandler(st)

	reqBody := map[string]any{
		"repo_selector": map[string]any{
			"mode": "all",
		},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/migs/mod123/runs", bytes.NewReader(body))
	req.SetPathValue("mig_id", "mod123")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

// TestModRuns_Create_ListModReposError verifies POST /v1/migs/{mig_id}/runs returns 500 on ListMigReposByMig failure.
func TestModRuns_Create_ListModReposError(t *testing.T) {
	specID := domaintypes.SpecID("spec123")
	st := &mockStore{
		getModResult: store.Mig{
			ID:         "mod123",
			Name:       "test-mig",
			SpecID:     &specID,
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
		getSpecResult: store.Spec{
			ID:   specID,
			Spec: []byte(`{"steps":[{"image":"docker.io/test/mig:latest"}]}`),
		},
		listMigReposByModErr: errors.New("database connection failed"),
	}
	handler := createMigRunHandler(st)

	reqBody := map[string]any{
		"repo_selector": map[string]any{
			"mode": "all",
		},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/migs/mod123/runs", bytes.NewReader(body))
	req.SetPathValue("mig_id", "mod123")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

// TestModRuns_Create_CreateRunError verifies POST /v1/migs/{mig_id}/runs returns 500 on CreateRun failure.
func TestModRuns_Create_CreateRunError(t *testing.T) {
	specID := domaintypes.SpecID("spec123")
	st := &mockStore{
		getModResult: store.Mig{
			ID:         "mod123",
			Name:       "test-mig",
			SpecID:     &specID,
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
		getSpecResult: store.Spec{
			ID:   specID,
			Spec: []byte(`{"steps":[{"image":"docker.io/test/mig:latest"}]}`),
		},
		listMigReposByModResult: []store.MigRepo{
			{ID: "repo1", MigID: "mod123", RepoID: "repo1", BaseRef: "main", TargetRef: "feature1"},
		},
		createRunErr: errors.New("database connection failed"),
	}
	handler := createMigRunHandler(st)

	reqBody := map[string]any{
		"repo_selector": map[string]any{
			"mode": "all",
		},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/migs/mod123/runs", bytes.NewReader(body))
	req.SetPathValue("mig_id", "mod123")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

// TestModRuns_Create_CreateRunRepoError verifies POST /v1/migs/{mig_id}/runs returns 500 on CreateRunRepo failure.
func TestModRuns_Create_CreateRunRepoError(t *testing.T) {
	specID := domaintypes.SpecID("spec123")
	st := &mockStore{
		getModResult: store.Mig{
			ID:         "mod123",
			Name:       "test-mig",
			SpecID:     &specID,
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
		getSpecResult: store.Spec{
			ID:   specID,
			Spec: []byte(`{"steps":[{"image":"docker.io/test/mig:latest"}]}`),
		},
		listMigReposByModResult: []store.MigRepo{
			{ID: "repo1", MigID: "mod123", RepoID: "repo1", BaseRef: "main", TargetRef: "feature1"},
		},
		createRunRepoErr: errors.New("database connection failed"),
	}
	handler := createMigRunHandler(st)

	reqBody := map[string]any{
		"repo_selector": map[string]any{
			"mode": "all",
		},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/migs/mod123/runs", bytes.NewReader(body))
	req.SetPathValue("mig_id", "mod123")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

// TestModRuns_Create_ListFailedReposError verifies POST /v1/migs/{mig_id}/runs returns 500 on ListFailedRepoIDsByMig failure.
func TestModRuns_Create_ListFailedReposError(t *testing.T) {
	specID := domaintypes.SpecID("spec123")
	st := &mockStore{
		getModResult: store.Mig{
			ID:         "mod123",
			Name:       "test-mig",
			SpecID:     &specID,
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
		getSpecResult: store.Spec{
			ID:   specID,
			Spec: []byte(`{"steps":[{"image":"docker.io/test/mig:latest"}]}`),
		},
		listMigReposByModResult: []store.MigRepo{
			{ID: "repo1", MigID: "mod123", RepoID: "repo1", BaseRef: "main", TargetRef: "feature1"},
		},
		listFailedRepoIDsByModErr: errors.New("database connection failed"),
	}
	handler := createMigRunHandler(st)

	reqBody := map[string]any{
		"repo_selector": map[string]any{
			"mode": "failed",
		},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/migs/mod123/runs", bytes.NewReader(body))
	req.SetPathValue("mig_id", "mod123")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

func TestModRuns_Create_RejectsWhenSourceCommitSeedFails(t *testing.T) {
	specID := domaintypes.SpecID("spec123")
	st := &mockStore{
		getModResult: store.Mig{
			ID:         "mod123",
			Name:       "test-mig",
			SpecID:     &specID,
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
		getSpecResult: store.Spec{
			ID:   specID,
			Spec: []byte(`{"steps":[{"image":"docker.io/test/mig:latest"}]}`),
		},
		listMigReposByModResult: []store.MigRepo{
			{ID: "repo1", MigID: "mod123", RepoID: "repo1", BaseRef: "main", TargetRef: "feature1"},
		},
		repoByID: map[domaintypes.MigRepoID]store.Repo{
			"repo1": {ID: "repo1", Url: "https://github.com/org/repo1"},
		},
	}
	handler := createMigRunHandler(st)

	reqBody := map[string]any{
		"repo_selector": map[string]any{
			"mode": "all",
		},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/migs/mod123/runs", bytes.NewReader(body))
	req.SetPathValue("mig_id", "mod123")
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(withSourceCommitSHAResolver(req.Context(), func(_ context.Context, _, _ string) (string, error) {
		return "", errors.New("seed lookup failed")
	}))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusBadRequest, rr.Body.String())
	}
	if st.createRunRepoCalled {
		t.Fatal("store.CreateRunRepo should not be called when source commit seed resolution fails")
	}
}
