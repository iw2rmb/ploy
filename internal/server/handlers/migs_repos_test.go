package handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// TestAddModRepoHandler tests the POST /v1/migs/{mig_id}/repos endpoint.
func TestAddModRepoHandler(t *testing.T) {
	tests := []struct {
		name           string
		modID          string
		body           map[string]interface{}
		setupMock      func(m *mockStore)
		verify         func(t *testing.T, m *mockStore)
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
			verify: func(t *testing.T, m *mockStore) {
				t.Helper()
				assertCalled(t, "CreateMigRepo", m.createMigRepoCalled)
				if m.createMigRepoParams.Url != "https://github.com/org/repo" {
					t.Fatalf("CreateMigRepo repo_url mismatch: got=%q want=%q", m.createMigRepoParams.Url, "https://github.com/org/repo")
				}
			},
			wantStatus: http.StatusCreated,
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
			wantBodySubstr: "repo_url: empty",
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
			wantBodySubstr: "base_ref: empty",
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
			wantBodySubstr: "invalid repo url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms := &mockStore{}
			if tt.setupMock != nil {
				tt.setupMock(ms)
			}

			handler := addMigRepoHandler(ms)
			rr := doRequest(t, handler, http.MethodPost, "/v1/migs/"+tt.modID+"/repos", tt.body, "mig_id", tt.modID)

			assertStatus(t, rr, tt.wantStatus)
			assertBodyContains(t, rr, tt.wantBodySubstr)
			if tt.verify != nil {
				tt.verify(t, ms)
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
		verify         func(t *testing.T, rr *httptest.ResponseRecorder)
	}{
		{
			name:  "success - lists repos",
			modID: "mod123",
			setupMock: func(m *mockStore) {
				m.getModResult = store.Mig{ID: "mod123", Name: "test-mig"}
				m.listMigReposByModResult = []store.MigRepo{
					{ID: "repo0001", MigID: "mod123", RepoID: "repo0001", BaseRef: "main", TargetRef: "feature1"},
					{ID: "repo0002", MigID: "mod123", RepoID: "repo0002", BaseRef: "develop", TargetRef: "feature2"},
				}
				m.repoByID = map[types.RepoID]store.Repo{
					"repo0001": {ID: "repo0001", Url: "https://github.com/org/repo1"},
					"repo0002": {ID: "repo0002", Url: "https://github.com/org/repo2"},
				}
			},
			wantStatus: http.StatusOK,
			verify: func(t *testing.T, rr *httptest.ResponseRecorder) {
				t.Helper()
				resp := decodeBody[struct {
					Repos []struct{ ID string } `json:"repos"`
				}](t, rr)
				if len(resp.Repos) != 2 {
					t.Errorf("got %d repos, want 2", len(resp.Repos))
				}
			},
		},
		{
			name:  "success - empty list",
			modID: "mod123",
			setupMock: func(m *mockStore) {
				m.getModResult = store.Mig{ID: "mod123", Name: "test-mig"}
				m.listMigReposByModResult = []store.MigRepo{}
			},
			wantStatus: http.StatusOK,
			verify: func(t *testing.T, rr *httptest.ResponseRecorder) {
				t.Helper()
				resp := decodeBody[struct {
					Repos []struct{ ID string } `json:"repos"`
				}](t, rr)
				if len(resp.Repos) != 0 {
					t.Errorf("got %d repos, want 0", len(resp.Repos))
				}
			},
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

			handler := listMigReposHandler(ms)
			rr := doRequest(t, handler, http.MethodGet, "/v1/migs/"+tt.modID+"/repos", nil, "mig_id", tt.modID)

			assertStatus(t, rr, tt.wantStatus)
			assertBodyContains(t, rr, tt.wantBodySubstr)
			if tt.verify != nil {
				tt.verify(t, rr)
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

			handler := deleteMigRepoHandler(ms)
			rr := doRequest(t, handler, http.MethodDelete, "/v1/migs/"+tt.modID+"/repos/"+tt.repoID, nil, "mig_id", tt.modID, "repo_id", tt.repoID)

			assertStatus(t, rr, tt.wantStatus)
			assertBodyContains(t, rr, tt.wantBodySubstr)
		})
	}
}

// TestBulkUpsertMigReposHandler tests the POST /v1/migs/{mig_id}/repos/bulk endpoint.
func TestBulkUpsertMigReposHandler(t *testing.T) {
	type bulkResp struct {
		Created int `json:"created"`
		Updated int `json:"updated"`
		Failed  int `json:"failed"`
		Errors  []struct {
			Line    int    `json:"line"`
			Message string `json:"message"`
		} `json:"errors"`
	}

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
				m.getModRepoByURLErr = pgx.ErrNoRows
			},
			wantStatus:  http.StatusOK,
			wantCreated: 2,
		},
		{
			name:        "success - updates existing repos",
			modID:       "mod123",
			contentType: "text/csv",
			body: `repo_url,base_ref,target_ref
https://github.com/org/existing,main,new-feature`,
			setupMock: func(m *mockStore) {
				m.getModResult = store.Mig{ID: "mod123", Name: "test-mig"}
				m.getModRepoByURLResult = store.MigRepo{
					ID:    "repoexst",
					MigID: "mod123",
				}
			},
			wantStatus:  http.StatusOK,
			wantUpdated: 1,
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
				assertCalled(t, "GetMigRepoByURL", m.getModRepoByURLCalled)
				if m.getModRepoByURLParams.Url != "https://github.com/org/repo" {
					t.Fatalf("GetMigRepoByURL repo_url mismatch: got=%q want=%q", m.getModRepoByURLParams.Url, "https://github.com/org/repo")
				}
				assertCalled(t, "UpsertMigRepo", m.upsertModRepoCalled)
				if m.upsertModRepoParams.Url != "https://github.com/org/repo" {
					t.Fatalf("UpsertMigRepo repo_url mismatch: got=%q want=%q", m.upsertModRepoParams.Url, "https://github.com/org/repo")
				}
			},
			wantStatus:  http.StatusOK,
			wantCreated: 1,
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
			wantStatus: http.StatusOK,
			wantFailed: 1,
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
				assertNotCalled(t, "UpsertMigRepo", m.upsertModRepoCalled)
			},
			wantStatus: http.StatusOK,
			wantFailed: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms := &mockStore{}
			if tt.setupMock != nil {
				tt.setupMock(ms)
			}

			handler := bulkUpsertMigReposHandler(ms)
			rr := doRequestWithContentType(t, handler, http.MethodPost, "/v1/migs/"+tt.modID+"/repos/bulk", tt.contentType, tt.body, "mig_id", tt.modID)

			assertStatus(t, rr, tt.wantStatus)
			assertBodyContains(t, rr, tt.wantBodySubstr)
			if tt.verify != nil {
				tt.verify(t, ms)
			}
			if tt.wantStatus == http.StatusOK {
				resp := decodeBody[bulkResp](t, rr)
				if resp.Created != tt.wantCreated {
					t.Errorf("got created=%d, want %d", resp.Created, tt.wantCreated)
				}
				if resp.Updated != tt.wantUpdated {
					t.Errorf("got updated=%d, want %d", resp.Updated, tt.wantUpdated)
				}
				if resp.Failed != tt.wantFailed {
					t.Errorf("got failed=%d, want %d", resp.Failed, tt.wantFailed)
				}
			}
		})
	}
}
