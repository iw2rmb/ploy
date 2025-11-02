package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/store"
)

// TestCreateRepoHandlerSuccess verifies successful repository creation.
func TestCreateRepoHandlerSuccess(t *testing.T) {
	repoID := uuid.New()
	now := time.Now()

	st := &mockStore{
		createRepoResult: store.Repo{
			ID:        pgtype.UUID{Bytes: repoID, Valid: true},
			Url:       "https://github.com/user/repo.git",
			Branch:    strPtr("main"),
			CommitSha: strPtr("abc123"),
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
	}

	handler := createRepoHandler(st)

	reqBody := map[string]interface{}{
		"url":        "https://github.com/user/repo.git",
		"branch":     "main",
		"commit_sha": "abc123",
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/repos", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		ID        string  `json:"id"`
		URL       string  `json:"url"`
		Branch    *string `json:"branch,omitempty"`
		CommitSha *string `json:"commit_sha,omitempty"`
		CreatedAt string  `json:"created_at"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.ID != repoID.String() {
		t.Errorf("expected id %s, got %s", repoID.String(), resp.ID)
	}
	if resp.URL != "https://github.com/user/repo.git" {
		t.Errorf("expected url https://github.com/user/repo.git, got %s", resp.URL)
	}
	if resp.Branch == nil || *resp.Branch != "main" {
		t.Error("expected branch main")
	}
	if resp.CommitSha == nil || *resp.CommitSha != "abc123" {
		t.Error("expected commit_sha abc123")
	}
	if _, err := time.Parse(time.RFC3339, resp.CreatedAt); err != nil {
		t.Errorf("invalid created_at timestamp: %v", err)
	}

	if !st.createRepoCalled {
		t.Error("expected CreateRepo to be called")
	}
}

// TestCreateRepoHandlerMissingURL verifies that missing url is rejected.
func TestCreateRepoHandlerMissingURL(t *testing.T) {
	st := &mockStore{}
	handler := createRepoHandler(st)

	cases := []struct {
		name string
		body map[string]interface{}
	}{
		{"empty url", map[string]interface{}{"url": ""}},
		{"whitespace url", map[string]interface{}{"url": "   "}},
		{"no url field", map[string]interface{}{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(tc.body)
			req := httptest.NewRequest(http.MethodPost, "/v1/repos", bytes.NewReader(body))
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Fatalf("expected status 400, got %d", rr.Code)
			}
			if !strings.Contains(rr.Body.String(), "url field is required") {
				t.Errorf("expected error about url, got: %s", rr.Body.String())
			}
		})
	}
}

// TestCreateRepoHandlerMalformedJSON verifies that malformed JSON is rejected.
func TestCreateRepoHandlerMalformedJSON(t *testing.T) {
	st := &mockStore{}
	handler := createRepoHandler(st)

	req := httptest.NewRequest(http.MethodPost, "/v1/repos", strings.NewReader("{invalid json"))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "invalid request") {
		t.Errorf("expected error about invalid request, got: %s", rr.Body.String())
	}
}

// TestCreateRepoHandlerDuplicateURL verifies that duplicate URL is rejected with 409 Conflict.
func TestCreateRepoHandlerDuplicateURL(t *testing.T) {
	st := &mockStore{
		createRepoErr: &pgconn.PgError{Code: "23505"}, // unique_violation
	}
	handler := createRepoHandler(st)

	reqBody := map[string]interface{}{
		"url": "https://github.com/user/repo.git",
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/repos", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "repository with this url already exists") {
		t.Errorf("expected error about duplicate url, got: %s", rr.Body.String())
	}
}

// TestListReposHandlerSuccess verifies successful repository listing.
func TestListReposHandlerSuccess(t *testing.T) {
	repo1ID := uuid.New()
	repo2ID := uuid.New()
	now := time.Now()

	st := &mockStore{
		listReposResult: []store.Repo{
			{
				ID:        pgtype.UUID{Bytes: repo1ID, Valid: true},
				Url:       "https://github.com/user/repo1.git",
				Branch:    strPtr("main"),
				CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
			},
			{
				ID:        pgtype.UUID{Bytes: repo2ID, Valid: true},
				Url:       "https://github.com/user/repo2.git",
				Branch:    nil,
				CreatedAt: pgtype.Timestamptz{Time: now.Add(time.Hour), Valid: true},
			},
		},
	}

	handler := listReposHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/repos", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Repos []struct {
			ID        string  `json:"id"`
			URL       string  `json:"url"`
			Branch    *string `json:"branch,omitempty"`
			CreatedAt string  `json:"created_at"`
		} `json:"repos"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Repos) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(resp.Repos))
	}

	if resp.Repos[0].ID != repo1ID.String() {
		t.Errorf("expected id %s, got %s", repo1ID.String(), resp.Repos[0].ID)
	}
	if resp.Repos[0].URL != "https://github.com/user/repo1.git" {
		t.Errorf("expected url https://github.com/user/repo1.git, got %s", resp.Repos[0].URL)
	}

	if !st.listReposCalled {
		t.Error("expected ListRepos to be called")
	}
}

// TestListReposHandlerEmpty verifies that empty list is returned when no repos exist.
func TestListReposHandlerEmpty(t *testing.T) {
	st := &mockStore{
		listReposResult: []store.Repo{},
	}

	handler := listReposHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/repos", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var resp struct {
		Repos []interface{} `json:"repos"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Repos) != 0 {
		t.Errorf("expected empty repos list, got %d items", len(resp.Repos))
	}
}

// TestCreateModHandlerSuccess verifies successful mod creation.
func TestCreateModHandlerSuccess(t *testing.T) {
	modID := uuid.New()
	repoID := uuid.New()
	now := time.Now()
	spec := json.RawMessage(`{"key": "value"}`)

	st := &mockStore{
		createModResult: store.Mod{
			ID:        pgtype.UUID{Bytes: modID, Valid: true},
			RepoID:    pgtype.UUID{Bytes: repoID, Valid: true},
			Spec:      spec,
			CreatedBy: strPtr("user@example.com"),
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
	}

	handler := createModHandler(st)

	reqBody := map[string]interface{}{
		"repo_id":    repoID.String(),
		"spec":       map[string]string{"key": "value"},
		"created_by": "user@example.com",
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/mods/crud", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		ID        string          `json:"id"`
		RepoID    string          `json:"repo_id"`
		Spec      json.RawMessage `json:"spec"`
		CreatedBy *string         `json:"created_by,omitempty"`
		CreatedAt string          `json:"created_at"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.ID != modID.String() {
		t.Errorf("expected id %s, got %s", modID.String(), resp.ID)
	}
	if resp.RepoID != repoID.String() {
		t.Errorf("expected repo_id %s, got %s", repoID.String(), resp.RepoID)
	}
	// Verify spec by comparing as JSON (ignoring whitespace).
	var respSpecJSON, expectedSpecJSON map[string]interface{}
	if err := json.Unmarshal(resp.Spec, &respSpecJSON); err != nil {
		t.Errorf("failed to unmarshal response spec: %v", err)
	}
	if err := json.Unmarshal(spec, &expectedSpecJSON); err != nil {
		t.Errorf("failed to unmarshal expected spec: %v", err)
	}
	if respSpecJSON["key"] != expectedSpecJSON["key"] {
		t.Errorf("expected spec %s, got %s", spec, resp.Spec)
	}

	if !st.createModCalled {
		t.Error("expected CreateMod to be called")
	}
}

// TestCreateModHandlerMissingFields verifies that missing required fields are rejected.
func TestCreateModHandlerMissingFields(t *testing.T) {
	st := &mockStore{}
	handler := createModHandler(st)

	cases := []struct {
		name     string
		bodyJSON string
		wantErr  string
		wantCode int
	}{
		{
			name:     "missing repo_id",
			bodyJSON: `{"spec": {"key": "value"}}`,
			wantErr:  "repo_id field is required",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "empty repo_id",
			bodyJSON: `{"repo_id": "", "spec": {"key": "value"}}`,
			wantErr:  "repo_id field is required",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "invalid repo_id format",
			bodyJSON: `{"repo_id": "not-a-uuid", "spec": {"key": "value"}}`,
			wantErr:  "invalid repo_id",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "missing spec",
			bodyJSON: `{"repo_id": "` + uuid.New().String() + `"}`,
			wantErr:  "spec field is required",
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/mods/crud", strings.NewReader(tc.bodyJSON))
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != tc.wantCode {
				t.Fatalf("expected status %d, got %d", tc.wantCode, rr.Code)
			}
			if !strings.Contains(rr.Body.String(), tc.wantErr) {
				t.Errorf("expected error containing %q, got: %s", tc.wantErr, rr.Body.String())
			}
		})
	}
}

// TestCreateModHandlerInvalidSpecJSON verifies that truly invalid JSON in spec is rejected.
func TestCreateModHandlerInvalidSpecJSON(t *testing.T) {
	st := &mockStore{}
	handler := createModHandler(st)

	// Send a request where spec contains invalid JSON (not parseable).
	// We need to craft this carefully - the outer JSON is valid, but spec field value is invalid JSON.
	bodyJSON := `{"repo_id": "` + uuid.New().String() + `", "spec": {invalid}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/mods/crud", strings.NewReader(bodyJSON))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// The entire request should fail to parse because spec contains invalid JSON.
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "invalid request") {
		t.Errorf("expected error about invalid request, got: %s", rr.Body.String())
	}
}

// TestCreateModHandlerRepoNotFound verifies that non-existent repo_id is rejected with 404.
func TestCreateModHandlerRepoNotFound(t *testing.T) {
	st := &mockStore{
		createModErr: &pgconn.PgError{Code: "23503"}, // foreign_key_violation
	}
	handler := createModHandler(st)

	reqBody := map[string]interface{}{
		"repo_id": uuid.New().String(),
		"spec":    map[string]string{"key": "value"},
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/mods/crud", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "repository not found") {
		t.Errorf("expected error about repository not found, got: %s", rr.Body.String())
	}
}

// TestListModsHandlerSuccess verifies successful mod listing.
func TestListModsHandlerSuccess(t *testing.T) {
	mod1ID := uuid.New()
	mod2ID := uuid.New()
	repoID := uuid.New()
	now := time.Now()
	spec := json.RawMessage(`{"key": "value"}`)

	st := &mockStore{
		listModsResult: []store.Mod{
			{
				ID:        pgtype.UUID{Bytes: mod1ID, Valid: true},
				RepoID:    pgtype.UUID{Bytes: repoID, Valid: true},
				Spec:      spec,
				CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
			},
			{
				ID:        pgtype.UUID{Bytes: mod2ID, Valid: true},
				RepoID:    pgtype.UUID{Bytes: repoID, Valid: true},
				Spec:      spec,
				CreatedAt: pgtype.Timestamptz{Time: now.Add(time.Hour), Valid: true},
			},
		},
	}

	handler := listModsHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/mods/crud", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Mods []struct {
			ID        string          `json:"id"`
			RepoID    string          `json:"repo_id"`
			Spec      json.RawMessage `json:"spec"`
			CreatedAt string          `json:"created_at"`
		} `json:"mods"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Mods) != 2 {
		t.Fatalf("expected 2 mods, got %d", len(resp.Mods))
	}

	if !st.listModsCalled {
		t.Error("expected ListMods to be called")
	}
}

// TestListModsByRepoHandlerSuccess verifies successful mod listing filtered by repo_id.
func TestListModsByRepoHandlerSuccess(t *testing.T) {
	modID := uuid.New()
	repoID := uuid.New()
	now := time.Now()
	spec := json.RawMessage(`{"key": "value"}`)

	st := &mockStore{
		listModsByRepoResult: []store.Mod{
			{
				ID:        pgtype.UUID{Bytes: modID, Valid: true},
				RepoID:    pgtype.UUID{Bytes: repoID, Valid: true},
				Spec:      spec,
				CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
			},
		},
	}

	handler := listModsHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/mods/crud?repo_id="+repoID.String(), nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Mods []struct {
			ID     string `json:"id"`
			RepoID string `json:"repo_id"`
		} `json:"mods"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Mods) != 1 {
		t.Fatalf("expected 1 mod, got %d", len(resp.Mods))
	}
	if resp.Mods[0].RepoID != repoID.String() {
		t.Errorf("expected repo_id %s, got %s", repoID.String(), resp.Mods[0].RepoID)
	}

	if !st.listModsByRepoCalled {
		t.Error("expected ListModsByRepo to be called")
	}
}

// TestListModsByRepoHandlerInvalidRepoID verifies that invalid repo_id query param is rejected.
func TestListModsByRepoHandlerInvalidRepoID(t *testing.T) {
	st := &mockStore{}
	handler := listModsHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/mods/crud?repo_id=not-a-uuid", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "invalid repo_id") {
		t.Errorf("expected error about invalid repo_id, got: %s", rr.Body.String())
	}
}

// strPtr returns a pointer to a string.
func strPtr(s string) *string {
	return &s
}
