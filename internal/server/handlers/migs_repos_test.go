package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/store"
)

// TestAddModRepoHandler tests the POST /v1/migs/{mig_id}/repos endpoint.
func TestAddModRepoHandler(t *testing.T) {
	tests := []struct {
		name           string
		modID          string
		body           map[string]interface{}
		setupMock      func(m *mockStore)
		wantRepoURL    string
		wantStatus     int
		wantBodySubstr string
	}{
		{
			name:  "success - adds repo to mig",
			modID: "mod123",
			body: map[string]interface{}{
				"repo_url":   "https://github.com/org/repo",
				"base_ref":   "main",
				"target_ref": "feature-branch",
			},
			setupMock: func(m *mockStore) {
				m.getModResult = store.Mig{ID: "mod123", Name: "test-mig"}
			},
			wantStatus: http.StatusCreated,
		},
		{
			name:  "success - normalizes repo URL",
			modID: "mod123",
			body: map[string]interface{}{
				"repo_url":   "https://github.com/org/repo.git/",
				"base_ref":   "main",
				"target_ref": "feature",
			},
			setupMock: func(m *mockStore) {
				m.getModResult = store.Mig{ID: "mod123", Name: "test-mig"}
			},
			wantRepoURL: "https://github.com/org/repo",
			wantStatus:  http.StatusCreated,
		},
		{
			name:  "error - mig not found",
			modID: "mod404",
			body: map[string]interface{}{
				"repo_url":   "https://github.com/org/repo",
				"base_ref":   "main",
				"target_ref": "feature",
			},
			setupMock: func(m *mockStore) {
				m.getModErr = pgx.ErrNoRows
			},
			wantStatus:     http.StatusNotFound,
			wantBodySubstr: "mig not found",
		},
		{
			name:  "error - archived mig",
			modID: "modarc",
			body: map[string]interface{}{
				"repo_url":   "https://github.com/org/repo",
				"base_ref":   "main",
				"target_ref": "feature",
			},
			setupMock: func(m *mockStore) {
				m.getModResult = store.Mig{
					ID:         "modarc",
					Name:       "archived-mig",
					ArchivedAt: pgtype.Timestamptz{Valid: true},
				}
			},
			wantStatus:     http.StatusConflict,
			wantBodySubstr: "cannot add repo to archived mig",
		},
		{
			name:  "error - missing repo_url",
			modID: "mod123",
			body: map[string]interface{}{
				"base_ref":   "main",
				"target_ref": "feature",
			},
			setupMock: func(m *mockStore) {
				m.getModResult = store.Mig{ID: "mod123", Name: "test-mig"}
			},
			wantStatus:     http.StatusBadRequest,
			wantBodySubstr: "repo_url is required",
		},
		{
			name:  "error - missing base_ref",
			modID: "mod123",
			body: map[string]interface{}{
				"repo_url":   "https://github.com/org/repo",
				"target_ref": "feature",
			},
			setupMock: func(m *mockStore) {
				m.getModResult = store.Mig{ID: "mod123", Name: "test-mig"}
			},
			wantStatus:     http.StatusBadRequest,
			wantBodySubstr: "base_ref is required",
		},
		{
			name:  "error - invalid repo_url scheme",
			modID: "mod123",
			body: map[string]interface{}{
				"repo_url":   "ftp://invalid.com/repo",
				"base_ref":   "main",
				"target_ref": "feature",
			},
			setupMock: func(m *mockStore) {
				m.getModResult = store.Mig{ID: "mod123", Name: "test-mig"}
			},
			wantStatus:     http.StatusBadRequest,
			wantBodySubstr: "repo_url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms := &mockStore{}
			if tt.setupMock != nil {
				tt.setupMock(ms)
			}

			bodyJSON, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/v1/migs/"+tt.modID+"/repos", bytes.NewReader(bodyJSON))
			req.Header.Set("Content-Type", "application/json")
			req.SetPathValue("mig_id", tt.modID)

			rec := httptest.NewRecorder()
			handler := addMigRepoHandler(ms)
			handler(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d; body: %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantBodySubstr != "" && !bytes.Contains(rec.Body.Bytes(), []byte(tt.wantBodySubstr)) {
				t.Errorf("body %q does not contain %q", rec.Body.String(), tt.wantBodySubstr)
			}
			if tt.wantRepoURL != "" {
				if !ms.createMigRepoCalled {
					t.Fatalf("expected CreateMigRepo to be called")
				}
				if ms.createMigRepoParams.RepoUrl != tt.wantRepoURL {
					t.Fatalf("CreateMigRepo repo_url mismatch: got=%q want=%q", ms.createMigRepoParams.RepoUrl, tt.wantRepoURL)
				}
			}
		})
	}
}

// TestListModReposHandler tests the GET /v1/migs/{mig_id}/repos endpoint.
func TestListModReposHandler(t *testing.T) {
	tests := []struct {
		name           string
		modID          string
		setupMock      func(m *mockStore)
		wantStatus     int
		wantBodySubstr string
		wantCount      int
	}{
		{
			name:  "success - lists repos",
			modID: "mod123",
			setupMock: func(m *mockStore) {
				m.getModResult = store.Mig{ID: "mod123", Name: "test-mig"}
				m.listMigReposByModResult = []store.MigRepo{
					{ID: "repo0001", MigID: "mod123", RepoUrl: "https://github.com/org/repo1", BaseRef: "main", TargetRef: "feature1"},
					{ID: "repo0002", MigID: "mod123", RepoUrl: "https://github.com/org/repo2", BaseRef: "develop", TargetRef: "feature2"},
				}
			},
			wantStatus: http.StatusOK,
			wantCount:  2,
		},
		{
			name:  "success - empty list",
			modID: "mod123",
			setupMock: func(m *mockStore) {
				m.getModResult = store.Mig{ID: "mod123", Name: "test-mig"}
				m.listMigReposByModResult = []store.MigRepo{}
			},
			wantStatus: http.StatusOK,
			wantCount:  0,
		},
		{
			name:  "error - mig not found",
			modID: "mod404",
			setupMock: func(m *mockStore) {
				m.getModErr = pgx.ErrNoRows
			},
			wantStatus:     http.StatusNotFound,
			wantBodySubstr: "mig not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms := &mockStore{}
			if tt.setupMock != nil {
				tt.setupMock(ms)
			}

			req := httptest.NewRequest(http.MethodGet, "/v1/migs/"+tt.modID+"/repos", nil)
			req.SetPathValue("mig_id", tt.modID)

			rec := httptest.NewRecorder()
			handler := listMigReposHandler(ms)
			handler(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d; body: %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantBodySubstr != "" && !bytes.Contains(rec.Body.Bytes(), []byte(tt.wantBodySubstr)) {
				t.Errorf("body %q does not contain %q", rec.Body.String(), tt.wantBodySubstr)
			}
			if tt.wantStatus == http.StatusOK {
				var resp struct {
					Repos []struct {
						ID string `json:"id"`
					} `json:"repos"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
					t.Fatalf("failed to unmarshal response: %v", err)
				}
				if len(resp.Repos) != tt.wantCount {
					t.Errorf("got %d repos, want %d", len(resp.Repos), tt.wantCount)
				}
			}
		})
	}
}

// TestDeleteMigRepoHandler tests the DELETE /v1/migs/{mig_id}/repos/{repo_id} endpoint.
func TestDeleteMigRepoHandler(t *testing.T) {
	tests := []struct {
		name           string
		modID          string
		repoID         string
		setupMock      func(m *mockStore)
		wantStatus     int
		wantBodySubstr string
	}{
		{
			name:   "success - deletes repo",
			modID:  "mod123",
			repoID: "repoX789",
			setupMock: func(m *mockStore) {
				m.getModResult = store.Mig{ID: "mod123", Name: "test-mig"}
				m.getModRepoResult = store.MigRepo{ID: "repoX789", MigID: "mod123"}
				m.hasModRepoHistoryResult = false
			},
			wantStatus: http.StatusNoContent,
		},
		{
			name:   "error - mig not found",
			modID:  "mod404",
			repoID: "repoX789",
			setupMock: func(m *mockStore) {
				m.getModErr = pgx.ErrNoRows
			},
			wantStatus:     http.StatusNotFound,
			wantBodySubstr: "mig not found",
		},
		{
			name:   "error - repo not found",
			modID:  "mod123",
			repoID: "repo4040",
			setupMock: func(m *mockStore) {
				m.getModResult = store.Mig{ID: "mod123", Name: "test-mig"}
				m.getModRepoErr = pgx.ErrNoRows
			},
			wantStatus:     http.StatusNotFound,
			wantBodySubstr: "repo not found",
		},
		{
			name:   "error - repo belongs to different mig",
			modID:  "mod123",
			repoID: "repo0003",
			setupMock: func(m *mockStore) {
				m.getModResult = store.Mig{ID: "mod123", Name: "test-mig"}
				m.getModRepoResult = store.MigRepo{ID: "repo0003", MigID: "moddif"}
			},
			wantStatus:     http.StatusNotFound,
			wantBodySubstr: "repo does not belong to this mig",
		},
		{
			name:   "error - repo has historical executions",
			modID:  "mod123",
			repoID: "repohist",
			setupMock: func(m *mockStore) {
				m.getModResult = store.Mig{ID: "mod123", Name: "test-mig"}
				m.getModRepoResult = store.MigRepo{ID: "repohist", MigID: "mod123"}
				m.hasModRepoHistoryResult = true
			},
			wantStatus:     http.StatusConflict,
			wantBodySubstr: "cannot delete repo with historical executions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms := &mockStore{}
			if tt.setupMock != nil {
				tt.setupMock(ms)
			}

			req := httptest.NewRequest(http.MethodDelete, "/v1/migs/"+tt.modID+"/repos/"+tt.repoID, nil)
			req.SetPathValue("mig_id", tt.modID)
			req.SetPathValue("repo_id", tt.repoID)

			rec := httptest.NewRecorder()
			handler := deleteMigRepoHandler(ms)
			handler(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d; body: %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantBodySubstr != "" && !bytes.Contains(rec.Body.Bytes(), []byte(tt.wantBodySubstr)) {
				t.Errorf("body %q does not contain %q", rec.Body.String(), tt.wantBodySubstr)
			}
		})
	}
}

// TestBulkUpsertMigReposHandler tests the POST /v1/migs/{mig_id}/repos/bulk endpoint.
func TestBulkUpsertMigReposHandler(t *testing.T) {
	tests := []struct {
		name           string
		modID          string
		contentType    string
		body           string
		setupMock      func(m *mockStore)
		verify         func(t *testing.T, m *mockStore)
		wantStatus     int
		wantBodySubstr string
		wantCreated    int
		wantUpdated    int
		wantFailed     int
	}{
		{
			name:        "success - creates new repos",
			modID:       "mod123",
			contentType: "text/csv",
			body: `repo_url,base_ref,target_ref
https://github.com/org/repo1,main,feature1
https://github.com/org/repo2,develop,feature2`,
			setupMock: func(m *mockStore) {
				m.getModResult = store.Mig{ID: "mod123", Name: "test-mig"}
				// GetMigRepoByURL returns not found (new repos).
				m.getModRepoByURLErr = pgx.ErrNoRows
			},
			wantStatus:  http.StatusOK,
			wantCreated: 2,
			wantUpdated: 0,
			wantFailed:  0,
		},
		{
			name:        "success - updates existing repos",
			modID:       "mod123",
			contentType: "text/csv",
			body: `repo_url,base_ref,target_ref
https://github.com/org/existing,main,new-feature`,
			setupMock: func(m *mockStore) {
				m.getModResult = store.Mig{ID: "mod123", Name: "test-mig"}
				// GetMigRepoByURL returns existing repo (update case).
				m.getModRepoByURLResult = store.MigRepo{
					ID:    "repoexst",
					MigID: "mod123",
				}
			},
			wantStatus:  http.StatusOK,
			wantCreated: 0,
			wantUpdated: 1,
			wantFailed:  0,
		},
		{
			name:        "success - processes multiple repos (all new)",
			modID:       "mod123",
			contentType: "text/csv",
			body: `repo_url,base_ref,target_ref
https://github.com/org/new-repo1,main,feature1
https://github.com/org/new-repo2,develop,feature2`,
			setupMock: func(m *mockStore) {
				m.getModResult = store.Mig{ID: "mod123", Name: "test-mig"}
				// GetMigRepoByURL returns not found for all (new repos).
				m.getModRepoByURLErr = pgx.ErrNoRows
			},
			wantStatus:  http.StatusOK,
			wantCreated: 2,
			wantUpdated: 0,
			wantFailed:  0,
		},
		{
			name:        "success - parses quoted fields and unicode",
			modID:       "mod123",
			contentType: "text/csv",
			body: "repo_url,base_ref,target_ref\n" +
				"\"https://github.com/org/привет\",\"main\",\"feature\"\"one\"",
			setupMock: func(m *mockStore) {
				m.getModResult = store.Mig{ID: "mod123", Name: "test-mig"}
				m.getModRepoByURLErr = pgx.ErrNoRows
			},
			wantStatus:  http.StatusOK,
			wantCreated: 1,
			wantUpdated: 0,
			wantFailed:  0,
		},
		{
			name:        "success - normalizes repo URL before upsert",
			modID:       "mod123",
			contentType: "text/csv",
			body: "repo_url,base_ref,target_ref\n" +
				"https://github.com/org/repo.git/,main,feature",
			setupMock: func(m *mockStore) {
				m.getModResult = store.Mig{ID: "mod123", Name: "test-mig"}
				m.getModRepoByURLErr = pgx.ErrNoRows
			},
			verify: func(t *testing.T, m *mockStore) {
				t.Helper()
				if !m.getModRepoByURLCalled {
					t.Fatalf("expected GetMigRepoByURL to be called")
				}
				if m.getModRepoByURLParams.RepoUrl != "https://github.com/org/repo" {
					t.Fatalf("GetMigRepoByURL repo_url mismatch: got=%q want=%q", m.getModRepoByURLParams.RepoUrl, "https://github.com/org/repo")
				}
				if !m.upsertModRepoCalled {
					t.Fatalf("expected UpsertMigRepo to be called")
				}
				if m.upsertModRepoParams.RepoUrl != "https://github.com/org/repo" {
					t.Fatalf("UpsertMigRepo repo_url mismatch: got=%q want=%q", m.upsertModRepoParams.RepoUrl, "https://github.com/org/repo")
				}
			},
			wantStatus:  http.StatusOK,
			wantCreated: 1,
			wantUpdated: 0,
			wantFailed:  0,
		},
		{
			name:        "error - wrong content type",
			modID:       "mod123",
			contentType: "application/json",
			body:        `{}`,
			setupMock: func(m *mockStore) {
				m.getModResult = store.Mig{ID: "mod123", Name: "test-mig"}
			},
			wantStatus:     http.StatusBadRequest,
			wantBodySubstr: "Content-Type must be text/csv",
		},
		{
			name:        "error - mig not found",
			modID:       "mod404",
			contentType: "text/csv",
			body: `repo_url,base_ref,target_ref
https://github.com/org/repo,main,feature`,
			setupMock: func(m *mockStore) {
				m.getModErr = pgx.ErrNoRows
			},
			wantStatus:     http.StatusNotFound,
			wantBodySubstr: "mig not found",
		},
		{
			name:        "error - archived mig",
			modID:       "modarc",
			contentType: "text/csv",
			body: `repo_url,base_ref,target_ref
https://github.com/org/repo,main,feature`,
			setupMock: func(m *mockStore) {
				m.getModResult = store.Mig{
					ID:         "modarc",
					Name:       "archived-mig",
					ArchivedAt: pgtype.Timestamptz{Valid: true},
				}
			},
			wantStatus:     http.StatusConflict,
			wantBodySubstr: "cannot modify repos on archived mig",
		},
		{
			name:        "error - invalid header",
			modID:       "mod123",
			contentType: "text/csv",
			body: `wrong,headers,here
https://github.com/org/repo,main,feature`,
			setupMock: func(m *mockStore) {
				m.getModResult = store.Mig{ID: "mod123", Name: "test-mig"}
			},
			wantStatus:     http.StatusBadRequest,
			wantBodySubstr: "CSV header must be",
		},
		{
			name:        "partial success - invalid repo_url on one line",
			modID:       "mod123",
			contentType: "text/csv",
			body: `repo_url,base_ref,target_ref
https://github.com/org/good-repo,main,feature1
ftp://invalid.com/bad-repo,main,feature2`,
			setupMock: func(m *mockStore) {
				m.getModResult = store.Mig{ID: "mod123", Name: "test-mig"}
				m.getModRepoByURLErr = pgx.ErrNoRows
			},
			wantStatus:  http.StatusOK,
			wantCreated: 1,
			wantUpdated: 0,
			wantFailed:  1,
		},
		{
			name:        "partial success - missing fields",
			modID:       "mod123",
			contentType: "text/csv",
			body: `repo_url,base_ref,target_ref
https://github.com/org/repo,,feature`,
			setupMock: func(m *mockStore) {
				m.getModResult = store.Mig{ID: "mod123", Name: "test-mig"}
			},
			wantStatus:     http.StatusOK,
			wantCreated:    0,
			wantUpdated:    0,
			wantFailed:     1,
			wantBodySubstr: "base_ref is required",
		},
		{
			name:        "partial success - strict CSV parse error",
			modID:       "mod123",
			contentType: "text/csv",
			body: "repo_url,base_ref,target_ref\n" +
				"https://github.com/org/repo,main,\"unterminated",
			setupMock: func(m *mockStore) {
				m.getModResult = store.Mig{ID: "mod123", Name: "test-mig"}
			},
			wantStatus:  http.StatusOK,
			wantCreated: 0,
			wantUpdated: 0,
			wantFailed:  1,
		},
		{
			name:        "partial success - store lookup error is a per-line failure",
			modID:       "mod123",
			contentType: "text/csv",
			body: "repo_url,base_ref,target_ref\n" +
				"https://github.com/org/repo,main,feature",
			setupMock: func(m *mockStore) {
				m.getModResult = store.Mig{ID: "mod123", Name: "test-mig"}
				m.getModRepoByURLErr = errors.New("db down")
			},
			verify: func(t *testing.T, m *mockStore) {
				t.Helper()
				if m.upsertModRepoCalled {
					t.Fatalf("did not expect UpsertMigRepo to be called when lookup fails")
				}
			},
			wantStatus:  http.StatusOK,
			wantCreated: 0,
			wantUpdated: 0,
			wantFailed:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms := &mockStore{}
			if tt.setupMock != nil {
				tt.setupMock(ms)
			}

			req := httptest.NewRequest(http.MethodPost, "/v1/migs/"+tt.modID+"/repos/bulk", bytes.NewReader([]byte(tt.body)))
			req.Header.Set("Content-Type", tt.contentType)
			req.SetPathValue("mig_id", tt.modID)

			rec := httptest.NewRecorder()
			handler := bulkUpsertMigReposHandler(ms)
			handler(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d; body: %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantBodySubstr != "" && !bytes.Contains(rec.Body.Bytes(), []byte(tt.wantBodySubstr)) {
				t.Errorf("body %q does not contain %q", rec.Body.String(), tt.wantBodySubstr)
			}
			if tt.wantStatus == http.StatusOK {
				var resp struct {
					Created int `json:"created"`
					Updated int `json:"updated"`
					Failed  int `json:"failed"`
					Errors  []struct {
						Line    int    `json:"line"`
						Message string `json:"message"`
					} `json:"errors"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
					t.Fatalf("failed to unmarshal response: %v", err)
				}
				if resp.Created != tt.wantCreated {
					t.Errorf("got created=%d, want %d", resp.Created, tt.wantCreated)
				}
				if resp.Updated != tt.wantUpdated {
					t.Errorf("got updated=%d, want %d", resp.Updated, tt.wantUpdated)
				}
				if resp.Failed != tt.wantFailed {
					t.Errorf("got failed=%d, want %d", resp.Failed, tt.wantFailed)
				}
				if tt.verify != nil {
					tt.verify(t, ms)
				}
			}
		})
	}
}
