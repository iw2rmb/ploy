package handlers

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

// strPtr returns a pointer to a string.
func strPtr(s string) *string {
	return &s
}
