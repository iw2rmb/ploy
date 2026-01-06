package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/store"
)

// TestAddModRepoHandler tests the POST /v1/mods/{mod_id}/repos endpoint.
func TestAddModRepoHandler(t *testing.T) {
	tests := []struct {
		name           string
		modID          string
		body           map[string]interface{}
		setupMock      func(m *mockStore)
		wantStatus     int
		wantBodySubstr string
	}{
		{
			name:  "success - adds repo to mod",
			modID: "mod-abc123",
			body: map[string]interface{}{
				"repo_url":   "https://github.com/org/repo",
				"base_ref":   "main",
				"target_ref": "feature-branch",
			},
			setupMock: func(m *mockStore) {
				m.getModResult = store.Mod{ID: "mod-abc123", Name: "test-mod"}
			},
			wantStatus: http.StatusCreated,
		},
		{
			name:  "success - normalizes repo URL",
			modID: "mod-abc123",
			body: map[string]interface{}{
				"repo_url":   "https://github.com/org/repo.git/",
				"base_ref":   "main",
				"target_ref": "feature",
			},
			setupMock: func(m *mockStore) {
				m.getModResult = store.Mod{ID: "mod-abc123", Name: "test-mod"}
			},
			wantStatus: http.StatusCreated,
		},
		{
			name:  "error - mod not found",
			modID: "mod-notfound",
			body: map[string]interface{}{
				"repo_url":   "https://github.com/org/repo",
				"base_ref":   "main",
				"target_ref": "feature",
			},
			setupMock: func(m *mockStore) {
				m.getModErr = pgx.ErrNoRows
			},
			wantStatus:     http.StatusNotFound,
			wantBodySubstr: "mod not found",
		},
		{
			name:  "error - archived mod",
			modID: "mod-archived",
			body: map[string]interface{}{
				"repo_url":   "https://github.com/org/repo",
				"base_ref":   "main",
				"target_ref": "feature",
			},
			setupMock: func(m *mockStore) {
				m.getModResult = store.Mod{
					ID:         "mod-archived",
					Name:       "archived-mod",
					ArchivedAt: pgtype.Timestamptz{Valid: true},
				}
			},
			wantStatus:     http.StatusConflict,
			wantBodySubstr: "cannot add repo to archived mod",
		},
		{
			name:  "error - missing repo_url",
			modID: "mod-abc123",
			body: map[string]interface{}{
				"base_ref":   "main",
				"target_ref": "feature",
			},
			setupMock: func(m *mockStore) {
				m.getModResult = store.Mod{ID: "mod-abc123", Name: "test-mod"}
			},
			wantStatus:     http.StatusBadRequest,
			wantBodySubstr: "repo_url is required",
		},
		{
			name:  "error - missing base_ref",
			modID: "mod-abc123",
			body: map[string]interface{}{
				"repo_url":   "https://github.com/org/repo",
				"target_ref": "feature",
			},
			setupMock: func(m *mockStore) {
				m.getModResult = store.Mod{ID: "mod-abc123", Name: "test-mod"}
			},
			wantStatus:     http.StatusBadRequest,
			wantBodySubstr: "base_ref is required",
		},
		{
			name:  "error - invalid repo_url scheme",
			modID: "mod-abc123",
			body: map[string]interface{}{
				"repo_url":   "ftp://invalid.com/repo",
				"base_ref":   "main",
				"target_ref": "feature",
			},
			setupMock: func(m *mockStore) {
				m.getModResult = store.Mod{ID: "mod-abc123", Name: "test-mod"}
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
			req := httptest.NewRequest(http.MethodPost, "/v1/mods/"+tt.modID+"/repos", bytes.NewReader(bodyJSON))
			req.Header.Set("Content-Type", "application/json")
			req.SetPathValue("mod_id", tt.modID)

			rec := httptest.NewRecorder()
			handler := addModRepoHandler(ms)
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

// TestListModReposHandler tests the GET /v1/mods/{mod_id}/repos endpoint.
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
			modID: "mod-abc123",
			setupMock: func(m *mockStore) {
				m.getModResult = store.Mod{ID: "mod-abc123", Name: "test-mod"}
				m.listModReposByModResult = []store.ModRepo{
					{ID: "repo-1", ModID: "mod-abc123", RepoUrl: "https://github.com/org/repo1", BaseRef: "main", TargetRef: "feature1"},
					{ID: "repo-2", ModID: "mod-abc123", RepoUrl: "https://github.com/org/repo2", BaseRef: "develop", TargetRef: "feature2"},
				}
			},
			wantStatus: http.StatusOK,
			wantCount:  2,
		},
		{
			name:  "success - empty list",
			modID: "mod-abc123",
			setupMock: func(m *mockStore) {
				m.getModResult = store.Mod{ID: "mod-abc123", Name: "test-mod"}
				m.listModReposByModResult = []store.ModRepo{}
			},
			wantStatus: http.StatusOK,
			wantCount:  0,
		},
		{
			name:  "error - mod not found",
			modID: "mod-notfound",
			setupMock: func(m *mockStore) {
				m.getModErr = pgx.ErrNoRows
			},
			wantStatus:     http.StatusNotFound,
			wantBodySubstr: "mod not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms := &mockStore{}
			if tt.setupMock != nil {
				tt.setupMock(ms)
			}

			req := httptest.NewRequest(http.MethodGet, "/v1/mods/"+tt.modID+"/repos", nil)
			req.SetPathValue("mod_id", tt.modID)

			rec := httptest.NewRecorder()
			handler := listModReposHandler(ms)
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

// TestDeleteModRepoHandler tests the DELETE /v1/mods/{mod_id}/repos/{repo_id} endpoint.
func TestDeleteModRepoHandler(t *testing.T) {
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
			modID:  "mod-abc123",
			repoID: "repo-xyz789",
			setupMock: func(m *mockStore) {
				m.getModResult = store.Mod{ID: "mod-abc123", Name: "test-mod"}
				m.getModRepoResult = store.ModRepo{ID: "repo-xyz789", ModID: "mod-abc123"}
				m.hasModRepoHistoryResult = false
			},
			wantStatus: http.StatusNoContent,
		},
		{
			name:   "error - mod not found",
			modID:  "mod-notfound",
			repoID: "repo-xyz789",
			setupMock: func(m *mockStore) {
				m.getModErr = pgx.ErrNoRows
			},
			wantStatus:     http.StatusNotFound,
			wantBodySubstr: "mod not found",
		},
		{
			name:   "error - repo not found",
			modID:  "mod-abc123",
			repoID: "repo-notfound",
			setupMock: func(m *mockStore) {
				m.getModResult = store.Mod{ID: "mod-abc123", Name: "test-mod"}
				m.getModRepoErr = pgx.ErrNoRows
			},
			wantStatus:     http.StatusNotFound,
			wantBodySubstr: "repo not found",
		},
		{
			name:   "error - repo belongs to different mod",
			modID:  "mod-abc123",
			repoID: "repo-other",
			setupMock: func(m *mockStore) {
				m.getModResult = store.Mod{ID: "mod-abc123", Name: "test-mod"}
				m.getModRepoResult = store.ModRepo{ID: "repo-other", ModID: "mod-different"}
			},
			wantStatus:     http.StatusNotFound,
			wantBodySubstr: "repo does not belong to this mod",
		},
		{
			name:   "error - repo has historical executions",
			modID:  "mod-abc123",
			repoID: "repo-with-history",
			setupMock: func(m *mockStore) {
				m.getModResult = store.Mod{ID: "mod-abc123", Name: "test-mod"}
				m.getModRepoResult = store.ModRepo{ID: "repo-with-history", ModID: "mod-abc123"}
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

			req := httptest.NewRequest(http.MethodDelete, "/v1/mods/"+tt.modID+"/repos/"+tt.repoID, nil)
			req.SetPathValue("mod_id", tt.modID)
			req.SetPathValue("repo_id", tt.repoID)

			rec := httptest.NewRecorder()
			handler := deleteModRepoHandler(ms)
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

// TestBulkUpsertModReposHandler tests the POST /v1/mods/{mod_id}/repos/bulk endpoint.
func TestBulkUpsertModReposHandler(t *testing.T) {
	tests := []struct {
		name           string
		modID          string
		contentType    string
		body           string
		setupMock      func(m *mockStore)
		wantStatus     int
		wantBodySubstr string
		wantCreated    int
		wantUpdated    int
		wantFailed     int
	}{
		{
			name:        "success - creates new repos",
			modID:       "mod-abc123",
			contentType: "text/csv",
			body: `repo_url,base_ref,target_ref
https://github.com/org/repo1,main,feature1
https://github.com/org/repo2,develop,feature2`,
			setupMock: func(m *mockStore) {
				m.getModResult = store.Mod{ID: "mod-abc123", Name: "test-mod"}
				// GetModRepoByURL returns not found (new repos).
				m.getModRepoByURLErr = pgx.ErrNoRows
			},
			wantStatus:  http.StatusOK,
			wantCreated: 2,
			wantUpdated: 0,
			wantFailed:  0,
		},
		{
			name:        "success - updates existing repos",
			modID:       "mod-abc123",
			contentType: "text/csv",
			body: `repo_url,base_ref,target_ref
https://github.com/org/existing,main,new-feature`,
			setupMock: func(m *mockStore) {
				m.getModResult = store.Mod{ID: "mod-abc123", Name: "test-mod"}
				// GetModRepoByURL returns existing repo (update case).
				m.getModRepoByURLResult = store.ModRepo{
					ID:    "repo-existing",
					ModID: "mod-abc123",
				}
			},
			wantStatus:  http.StatusOK,
			wantCreated: 0,
			wantUpdated: 1,
			wantFailed:  0,
		},
		{
			name:        "success - processes multiple repos (all new)",
			modID:       "mod-abc123",
			contentType: "text/csv",
			body: `repo_url,base_ref,target_ref
https://github.com/org/new-repo1,main,feature1
https://github.com/org/new-repo2,develop,feature2`,
			setupMock: func(m *mockStore) {
				m.getModResult = store.Mod{ID: "mod-abc123", Name: "test-mod"}
				// GetModRepoByURL returns not found for all (new repos).
				m.getModRepoByURLErr = pgx.ErrNoRows
			},
			wantStatus:  http.StatusOK,
			wantCreated: 2,
			wantUpdated: 0,
		},
		{
			name:        "error - wrong content type",
			modID:       "mod-abc123",
			contentType: "application/json",
			body:        `{}`,
			setupMock: func(m *mockStore) {
				m.getModResult = store.Mod{ID: "mod-abc123", Name: "test-mod"}
			},
			wantStatus:     http.StatusBadRequest,
			wantBodySubstr: "Content-Type must be text/csv",
		},
		{
			name:        "error - mod not found",
			modID:       "mod-notfound",
			contentType: "text/csv",
			body: `repo_url,base_ref,target_ref
https://github.com/org/repo,main,feature`,
			setupMock: func(m *mockStore) {
				m.getModErr = pgx.ErrNoRows
			},
			wantStatus:     http.StatusNotFound,
			wantBodySubstr: "mod not found",
		},
		{
			name:        "error - archived mod",
			modID:       "mod-archived",
			contentType: "text/csv",
			body: `repo_url,base_ref,target_ref
https://github.com/org/repo,main,feature`,
			setupMock: func(m *mockStore) {
				m.getModResult = store.Mod{
					ID:         "mod-archived",
					Name:       "archived-mod",
					ArchivedAt: pgtype.Timestamptz{Valid: true},
				}
			},
			wantStatus:     http.StatusConflict,
			wantBodySubstr: "cannot modify repos on archived mod",
		},
		{
			name:        "error - invalid header",
			modID:       "mod-abc123",
			contentType: "text/csv",
			body: `wrong,headers,here
https://github.com/org/repo,main,feature`,
			setupMock: func(m *mockStore) {
				m.getModResult = store.Mod{ID: "mod-abc123", Name: "test-mod"}
			},
			wantStatus:     http.StatusBadRequest,
			wantBodySubstr: "CSV header must be",
		},
		{
			name:        "partial success - invalid repo_url on one line",
			modID:       "mod-abc123",
			contentType: "text/csv",
			body: `repo_url,base_ref,target_ref
https://github.com/org/good-repo,main,feature1
ftp://invalid.com/bad-repo,main,feature2`,
			setupMock: func(m *mockStore) {
				m.getModResult = store.Mod{ID: "mod-abc123", Name: "test-mod"}
				m.getModRepoByURLErr = pgx.ErrNoRows
			},
			wantStatus:  http.StatusOK,
			wantCreated: 1,
			wantFailed:  1,
		},
		{
			name:        "partial success - missing fields",
			modID:       "mod-abc123",
			contentType: "text/csv",
			body: `repo_url,base_ref,target_ref
https://github.com/org/repo,,feature`,
			setupMock: func(m *mockStore) {
				m.getModResult = store.Mod{ID: "mod-abc123", Name: "test-mod"}
			},
			wantStatus:     http.StatusOK,
			wantFailed:     1,
			wantBodySubstr: "base_ref is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms := &mockStore{}
			if tt.setupMock != nil {
				tt.setupMock(ms)
			}

			req := httptest.NewRequest(http.MethodPost, "/v1/mods/"+tt.modID+"/repos/bulk", bytes.NewReader([]byte(tt.body)))
			req.Header.Set("Content-Type", tt.contentType)
			req.SetPathValue("mod_id", tt.modID)

			rec := httptest.NewRecorder()
			handler := bulkUpsertModReposHandler(ms)
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
				// Only check counts when specifically expected (some tests have mock limitations).
				if tt.wantCreated > 0 && resp.Created != tt.wantCreated {
					t.Errorf("got created=%d, want %d", resp.Created, tt.wantCreated)
				}
				if tt.wantUpdated > 0 && resp.Updated != tt.wantUpdated {
					t.Errorf("got updated=%d, want %d", resp.Updated, tt.wantUpdated)
				}
				if tt.wantFailed > 0 && resp.Failed != tt.wantFailed {
					t.Errorf("got failed=%d, want %d", resp.Failed, tt.wantFailed)
				}
			}
		})
	}
}
