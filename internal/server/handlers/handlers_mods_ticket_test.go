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
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/store"
)

// TestSubmitTicketHandlerSuccess verifies successful ticket submission when repo already exists.
func TestSubmitTicketHandlerSuccess(t *testing.T) {
	repoID := uuid.New()
	modID := uuid.New()
	runID := uuid.New()
	now := time.Now()

	st := &mockStore{
		getRepoByURLResult: store.Repo{
			ID:        pgtype.UUID{Bytes: repoID, Valid: true},
			Url:       "https://github.com/user/repo.git",
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
		createModResult: store.Mod{
			ID:        pgtype.UUID{Bytes: modID, Valid: true},
			RepoID:    pgtype.UUID{Bytes: repoID, Valid: true},
			Spec:      []byte("{}"),
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
		createRunResult: store.Run{
			ID:        pgtype.UUID{Bytes: runID, Valid: true},
			ModID:     pgtype.UUID{Bytes: modID, Valid: true},
			Status:    store.RunStatusQueued,
			BaseRef:   "main",
			TargetRef: "feature",
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
	}

	handler := submitTicketHandler(st)

	reqBody := map[string]interface{}{
		"repo_url":   "https://github.com/user/repo.git",
		"base_ref":   "main",
		"target_ref": "feature",
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/mods", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		TicketID  string `json:"ticket_id"`
		Status    string `json:"status"`
		RepoURL   string `json:"repo_url"`
		BaseRef   string `json:"base_ref"`
		TargetRef string `json:"target_ref"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.TicketID != runID.String() {
		t.Errorf("expected ticket_id %s, got %s", runID.String(), resp.TicketID)
	}
	if resp.Status != "queued" {
		t.Errorf("expected status queued, got %s", resp.Status)
	}
	if resp.RepoURL != "https://github.com/user/repo.git" {
		t.Errorf("expected repo_url https://github.com/user/repo.git, got %s", resp.RepoURL)
	}
	if resp.BaseRef != "main" {
		t.Errorf("expected base_ref main, got %s", resp.BaseRef)
	}
	if resp.TargetRef != "feature" {
		t.Errorf("expected target_ref feature, got %s", resp.TargetRef)
	}

	if !st.getRepoByURLCalled {
		t.Error("expected GetRepoByURL to be called")
	}
	if !st.createModCalled {
		t.Error("expected CreateMod to be called")
	}
	if !st.createRunCalled {
		t.Error("expected CreateRun to be called")
	}
}

// TestSubmitTicketHandlerRepoCreation verifies ticket submission creates repo if it doesn't exist.
func TestSubmitTicketHandlerRepoCreation(t *testing.T) {
	repoID := uuid.New()
	modID := uuid.New()
	runID := uuid.New()
	now := time.Now()

	st := &mockStore{
		getRepoByURLErr: pgx.ErrNoRows, // Repo doesn't exist initially
		createRepoResult: store.Repo{
			ID:        pgtype.UUID{Bytes: repoID, Valid: true},
			Url:       "https://github.com/newuser/newrepo.git",
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
		createModResult: store.Mod{
			ID:        pgtype.UUID{Bytes: modID, Valid: true},
			RepoID:    pgtype.UUID{Bytes: repoID, Valid: true},
			Spec:      []byte("{}"),
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
		createRunResult: store.Run{
			ID:        pgtype.UUID{Bytes: runID, Valid: true},
			ModID:     pgtype.UUID{Bytes: modID, Valid: true},
			Status:    store.RunStatusQueued,
			BaseRef:   "main",
			TargetRef: "develop",
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
	}

	handler := submitTicketHandler(st)

	reqBody := map[string]interface{}{
		"repo_url":   "https://github.com/newuser/newrepo.git",
		"base_ref":   "main",
		"target_ref": "develop",
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/mods", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", rr.Code, rr.Body.String())
	}

	if !st.getRepoByURLCalled {
		t.Error("expected GetRepoByURL to be called")
	}
	if !st.createRepoCalled {
		t.Error("expected CreateRepo to be called (repo didn't exist)")
	}
	if !st.createModCalled {
		t.Error("expected CreateMod to be called")
	}
	if !st.createRunCalled {
		t.Error("expected CreateRun to be called")
	}
}

// TestSubmitTicketHandlerMissingFields verifies validation of required fields.
func TestSubmitTicketHandlerMissingFields(t *testing.T) {
	st := &mockStore{}
	handler := submitTicketHandler(st)

	cases := []struct {
		name string
		body map[string]interface{}
		err  string
	}{
		{"empty repo_url", map[string]interface{}{"repo_url": "", "base_ref": "main", "target_ref": "feature"}, "repo_url field is required"},
		{"whitespace repo_url", map[string]interface{}{"repo_url": "   ", "base_ref": "main", "target_ref": "feature"}, "repo_url field is required"},
		{"no repo_url", map[string]interface{}{"base_ref": "main", "target_ref": "feature"}, "repo_url field is required"},
		{"empty base_ref", map[string]interface{}{"repo_url": "https://github.com/user/repo.git", "base_ref": "", "target_ref": "feature"}, "base_ref field is required"},
		{"whitespace base_ref", map[string]interface{}{"repo_url": "https://github.com/user/repo.git", "base_ref": "   ", "target_ref": "feature"}, "base_ref field is required"},
		{"no base_ref", map[string]interface{}{"repo_url": "https://github.com/user/repo.git", "target_ref": "feature"}, "base_ref field is required"},
		{"empty target_ref", map[string]interface{}{"repo_url": "https://github.com/user/repo.git", "base_ref": "main", "target_ref": ""}, "target_ref field is required"},
		{"whitespace target_ref", map[string]interface{}{"repo_url": "https://github.com/user/repo.git", "base_ref": "main", "target_ref": "   "}, "target_ref field is required"},
		{"no target_ref", map[string]interface{}{"repo_url": "https://github.com/user/repo.git", "base_ref": "main"}, "target_ref field is required"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(tc.body)
			req := httptest.NewRequest(http.MethodPost, "/v1/mods", bytes.NewReader(body))
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Fatalf("expected status 400, got %d", rr.Code)
			}
			if !strings.Contains(rr.Body.String(), tc.err) {
				t.Errorf("expected error %q, got: %s", tc.err, rr.Body.String())
			}
		})
	}
}

// TestSubmitTicketHandlerInvalidJSON verifies rejection of malformed JSON.
func TestSubmitTicketHandlerInvalidJSON(t *testing.T) {
	st := &mockStore{}
	handler := submitTicketHandler(st)

	req := httptest.NewRequest(http.MethodPost, "/v1/mods", strings.NewReader("{invalid json"))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "invalid request") {
		t.Errorf("expected 'invalid request' error, got: %s", rr.Body.String())
	}
}

// TestSubmitTicketHandlerWithOptionalFields verifies optional fields are handled correctly.
func TestSubmitTicketHandlerWithOptionalFields(t *testing.T) {
	repoID := uuid.New()
	modID := uuid.New()
	runID := uuid.New()
	now := time.Now()
	commitSha := "abc1234567890"
	createdBy := "user@example.com"
	customSpec := json.RawMessage(`{"key": "value"}`)

	st := &mockStore{
		getRepoByURLResult: store.Repo{
			ID:        pgtype.UUID{Bytes: repoID, Valid: true},
			Url:       "https://github.com/user/repo.git",
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
		createModResult: store.Mod{
			ID:        pgtype.UUID{Bytes: modID, Valid: true},
			RepoID:    pgtype.UUID{Bytes: repoID, Valid: true},
			Spec:      customSpec,
			CreatedBy: &createdBy,
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
		createRunResult: store.Run{
			ID:        pgtype.UUID{Bytes: runID, Valid: true},
			ModID:     pgtype.UUID{Bytes: modID, Valid: true},
			Status:    store.RunStatusQueued,
			BaseRef:   "main",
			TargetRef: "feature",
			CommitSha: &commitSha,
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
	}

	handler := submitTicketHandler(st)

	reqBody := map[string]interface{}{
		"repo_url":   "https://github.com/user/repo.git",
		"base_ref":   "main",
		"target_ref": "feature",
		"commit_sha": commitSha,
		"spec":       map[string]string{"key": "value"},
		"created_by": createdBy,
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/mods", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify mod was created with custom spec (compare as JSON, not string).
	var expectedSpec, actualSpec map[string]interface{}
	if err := json.Unmarshal(customSpec, &expectedSpec); err != nil {
		t.Fatalf("failed to unmarshal expected spec: %v", err)
	}
	if err := json.Unmarshal(st.createModParams.Spec, &actualSpec); err != nil {
		t.Fatalf("failed to unmarshal actual spec: %v", err)
	}
	if len(expectedSpec) != len(actualSpec) || expectedSpec["key"] != actualSpec["key"] {
		t.Errorf("expected spec %s, got %s", string(customSpec), string(st.createModParams.Spec))
	}
	if st.createModParams.CreatedBy == nil || *st.createModParams.CreatedBy != createdBy {
		t.Error("expected created_by to be passed to CreateMod")
	}

	// Verify run was created with commit_sha.
	if st.createRunParams.CommitSha == nil || *st.createRunParams.CommitSha != commitSha {
		t.Error("expected commit_sha to be passed to CreateRun")
	}
}
