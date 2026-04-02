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

// =============================================================================
// POST /v1/migs/{mig_id}/repos — Add Mig Repo
// =============================================================================

func TestAddMigRepoHandler(t *testing.T) {
	activeMig := store.Mig{ID: "mig123", Name: "test-mig"}

	tests := []struct {
		name           string
		store          *migStore
		migID          string
		body           map[string]interface{}
		wantStatus     int
		wantBodySubstr string
		verify         func(t *testing.T, m *migStore)
	}{
		{
			name:       "success - adds repo to mig",
			store:      &migStore{getMigResult: activeMig},
			migID:      "mig123",
			body:       map[string]interface{}{"repo_url": "https://github.com/org/repo", "base_ref": "main", "target_ref": "feature-branch"},
			wantStatus: http.StatusCreated,
		},
		{
			name:       "success - normalizes repo URL",
			store:      &migStore{getMigResult: activeMig},
			migID:      "mig123",
			body:       map[string]interface{}{"repo_url": "https://github.com/org/repo.git/", "base_ref": "main", "target_ref": "feature"},
			wantStatus: http.StatusCreated,
			verify: func(t *testing.T, m *migStore) {
				t.Helper()
				assertCalled(t, "CreateMigRepo", m.createMigRepoCalled)
				if m.createMigRepoParams.Url != "https://github.com/org/repo" {
					t.Fatalf("CreateMigRepo repo_url mismatch: got=%q want=%q", m.createMigRepoParams.Url, "https://github.com/org/repo")
				}
			},
		},
		{
			name:           "error - mig not found",
			store:          &migStore{getMigErr: pgx.ErrNoRows},
			migID:          "mig404",
			body:           map[string]interface{}{"repo_url": "https://github.com/org/repo", "base_ref": "main", "target_ref": "feature"},
			wantStatus:     http.StatusNotFound,
			wantBodySubstr: "mig not found",
		},
		{
			name:           "error - archived mig",
			store:          &migStore{getMigResult: store.Mig{ID: "modarc", Name: "archived-mig", ArchivedAt: pgtype.Timestamptz{Valid: true}}},
			migID:          "modarc",
			body:           map[string]interface{}{"repo_url": "https://github.com/org/repo", "base_ref": "main", "target_ref": "feature"},
			wantStatus:     http.StatusConflict,
			wantBodySubstr: "cannot add repo to archived mig",
		},
		{
			name:           "error - missing repo_url",
			store:          &migStore{getMigResult: activeMig},
			migID:          "mig123",
			body:           map[string]interface{}{"base_ref": "main", "target_ref": "feature"},
			wantStatus:     http.StatusBadRequest,
			wantBodySubstr: "repo_url: empty",
		},
		{
			name:           "error - missing base_ref",
			store:          &migStore{getMigResult: activeMig},
			migID:          "mig123",
			body:           map[string]interface{}{"repo_url": "https://github.com/org/repo", "target_ref": "feature"},
			wantStatus:     http.StatusBadRequest,
			wantBodySubstr: "base_ref: empty",
		},
		{
			name:           "error - invalid repo_url scheme",
			store:          &migStore{getMigResult: activeMig},
			migID:          "mig123",
			body:           map[string]interface{}{"repo_url": "ftp://invalid.com/repo", "base_ref": "main", "target_ref": "feature"},
			wantStatus:     http.StatusBadRequest,
			wantBodySubstr: "invalid repo url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := addMigRepoHandler(tt.store)
			rr := doRequest(t, handler, http.MethodPost, "/v1/migs/"+tt.migID+"/repos", tt.body, "mig_id", tt.migID)
			assertStatus(t, rr, tt.wantStatus)
			assertBodyContains(t, rr, tt.wantBodySubstr)
			if tt.verify != nil {
				tt.verify(t, tt.store)
			}
		})
	}
}

// =============================================================================
// GET /v1/migs/{mig_id}/repos — List Mig Repos
// =============================================================================

func TestListMigReposHandler(t *testing.T) {
	tests := []struct {
		name           string
		store          *migStore
		migID          string
		wantStatus     int
		wantBodySubstr string
		verify         func(t *testing.T, rr *httptest.ResponseRecorder)
	}{
		{
			name:  "success - lists repos",
			migID: "mig123",
			store: &migStore{
				getMigResult: store.Mig{ID: "mig123", Name: "test-mig"},
				listMigReposByMigResult: []store.MigRepo{
					{ID: "repo0001", MigID: "mig123", RepoID: "repo0001", BaseRef: "main", TargetRef: "feature1"},
					{ID: "repo0002", MigID: "mig123", RepoID: "repo0002", BaseRef: "develop", TargetRef: "feature2"},
				},
				repoByID: map[types.RepoID]store.Repo{
					"repo0001": {ID: "repo0001", Url: "https://github.com/org/repo1"},
					"repo0002": {ID: "repo0002", Url: "https://github.com/org/repo2"},
				},
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
			migID: "mig123",
			store: &migStore{
				getMigResult:            store.Mig{ID: "mig123", Name: "test-mig"},
				listMigReposByMigResult: []store.MigRepo{},
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
			name:           "error - mig not found",
			migID:          "mig404",
			store:          &migStore{getMigErr: pgx.ErrNoRows},
			wantStatus:     http.StatusNotFound,
			wantBodySubstr: "mig not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := listMigReposHandler(tt.store)
			rr := doRequest(t, handler, http.MethodGet, "/v1/migs/"+tt.migID+"/repos", nil, "mig_id", tt.migID)
			assertStatus(t, rr, tt.wantStatus)
			assertBodyContains(t, rr, tt.wantBodySubstr)
			if tt.verify != nil {
				tt.verify(t, rr)
			}
		})
	}
}

// =============================================================================
// DELETE /v1/migs/{mig_id}/repos/{repo_id} — Delete Mig Repo
// =============================================================================

func TestDeleteMigRepoHandler(t *testing.T) {
	activeMig := store.Mig{ID: "mig123", Name: "test-mig"}

	tests := []struct {
		name           string
		store          *migStore
		migID          string
		repoID         string
		wantStatus     int
		wantBodySubstr string
	}{
		{
			name: "success - deletes repo",
			store: func() *migStore {
				st := &migStore{getMigResult: activeMig}
				st.getMigRepo.val = store.MigRepo{ID: "repoX789", MigID: "mig123"}
				st.hasMigRepoHistory.val = false
				return st
			}(),
			migID:      "mig123",
			repoID:     "repoX789",
			wantStatus: http.StatusNoContent,
		},
		{
			name:           "error - mig not found",
			store:          &migStore{getMigErr: pgx.ErrNoRows},
			migID:          "mig404",
			repoID:         "repoX789",
			wantStatus:     http.StatusNotFound,
			wantBodySubstr: "mig not found",
		},
		{
			name: "error - repo not found",
			store: func() *migStore {
				st := &migStore{getMigResult: activeMig}
				st.getMigRepo.err = pgx.ErrNoRows
				return st
			}(),
			migID:          "mig123",
			repoID:         "repo4040",
			wantStatus:     http.StatusNotFound,
			wantBodySubstr: "repo not found",
		},
		{
			name: "error - repo belongs to different mig",
			store: func() *migStore {
				st := &migStore{getMigResult: activeMig}
				st.getMigRepo.val = store.MigRepo{ID: "repo0003", MigID: "migdif"}
				return st
			}(),
			migID:          "mig123",
			repoID:         "repo0003",
			wantStatus:     http.StatusNotFound,
			wantBodySubstr: "repo does not belong to this mig",
		},
		{
			name: "error - repo has historical executions",
			store: func() *migStore {
				st := &migStore{getMigResult: activeMig}
				st.getMigRepo.val = store.MigRepo{ID: "repohist", MigID: "mig123"}
				st.hasMigRepoHistory.val = true
				return st
			}(),
			migID:          "mig123",
			repoID:         "repohist",
			wantStatus:     http.StatusConflict,
			wantBodySubstr: "cannot delete repo with historical executions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := deleteMigRepoHandler(tt.store)
			rr := doRequest(t, handler, http.MethodDelete, "/v1/migs/"+tt.migID+"/repos/"+tt.repoID, nil, "mig_id", tt.migID, "repo_id", tt.repoID)
			assertStatus(t, rr, tt.wantStatus)
			assertBodyContains(t, rr, tt.wantBodySubstr)
		})
	}
}

// =============================================================================
// POST /v1/migs/{mig_id}/repos/bulk — Bulk Upsert Mig Repos
// =============================================================================

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

	activeMig := store.Mig{ID: "mig123", Name: "test-mig"}
	archivedMig := store.Mig{ID: "modarc", Name: "archived-mig", ArchivedAt: pgtype.Timestamptz{Valid: true}}

	tests := []struct {
		name           string
		store          *migStore
		migID          string
		contentType    string
		body           string
		wantStatus     int
		wantBodySubstr string
		wantCreated    int
		wantUpdated    int
		wantFailed     int
		verify         func(t *testing.T, m *migStore)
	}{
		{
			name: "success - creates new repos",
			store: func() *migStore {
				st := &migStore{getMigResult: activeMig}
				st.getMigRepoByURL.err = pgx.ErrNoRows
				return st
			}(),
			migID:       "mig123",
			contentType: "text/csv",
			body: `repo_url,base_ref,target_ref
https://github.com/org/repo1,main,feature1
https://github.com/org/repo2,develop,feature2`,
			wantStatus:  http.StatusOK,
			wantCreated: 2,
		},
		{
			name: "success - updates existing repos",
			store: func() *migStore {
				st := &migStore{getMigResult: activeMig}
				st.getMigRepoByURL.val = store.MigRepo{ID: "repoexst", MigID: "mig123"}
				return st
			}(),
			migID:       "mig123",
			contentType: "text/csv",
			body: `repo_url,base_ref,target_ref
https://github.com/org/existing,main,new-feature`,
			wantStatus:  http.StatusOK,
			wantUpdated: 1,
		},
		{
			name: "success - parses quoted fields and unicode",
			store: func() *migStore {
				st := &migStore{getMigResult: activeMig}
				st.getMigRepoByURL.err = pgx.ErrNoRows
				return st
			}(),
			migID:       "mig123",
			contentType: "text/csv",
			body:        "repo_url,base_ref,target_ref\n\"https://github.com/org/привет\",\"main\",\"feature\"\"one\"",
			wantStatus:  http.StatusOK,
			wantCreated: 1,
		},
		{
			name: "success - normalizes repo URL before upsert",
			store: func() *migStore {
				st := &migStore{getMigResult: activeMig}
				st.getMigRepoByURL.err = pgx.ErrNoRows
				return st
			}(),
			migID:       "mig123",
			contentType: "text/csv",
			body:        "repo_url,base_ref,target_ref\nhttps://github.com/org/repo.git/,main,feature",
			wantStatus:  http.StatusOK,
			wantCreated: 1,
			verify: func(t *testing.T, m *migStore) {
				t.Helper()
				assertCalled(t, "GetMigRepoByURL", m.getMigRepoByURL.called)
				if m.getMigRepoByURL.params.Url != "https://github.com/org/repo" {
					t.Fatalf("GetMigRepoByURL repo_url mismatch: got=%q want=%q", m.getMigRepoByURL.params.Url, "https://github.com/org/repo")
				}
				assertCalled(t, "UpsertMigRepo", m.upsertMigRepoCalled)
				if m.upsertMigRepoParams.Url != "https://github.com/org/repo" {
					t.Fatalf("UpsertMigRepo repo_url mismatch: got=%q want=%q", m.upsertMigRepoParams.Url, "https://github.com/org/repo")
				}
			},
		},
		{
			name:           "error - wrong content type",
			store:          &migStore{getMigResult: activeMig},
			migID:          "mig123",
			contentType:    "application/json",
			body:           `{}`,
			wantStatus:     http.StatusBadRequest,
			wantBodySubstr: "Content-Type must be text/csv",
		},
		{
			name:           "error - mig not found",
			store:          &migStore{getMigErr: pgx.ErrNoRows},
			migID:          "mig404",
			contentType:    "text/csv",
			body:           "repo_url,base_ref,target_ref\nhttps://github.com/org/repo,main,feature",
			wantStatus:     http.StatusNotFound,
			wantBodySubstr: "mig not found",
		},
		{
			name:           "error - archived mig",
			store:          &migStore{getMigResult: archivedMig},
			migID:          "modarc",
			contentType:    "text/csv",
			body:           "repo_url,base_ref,target_ref\nhttps://github.com/org/repo,main,feature",
			wantStatus:     http.StatusConflict,
			wantBodySubstr: "cannot modify repos on archived mig",
		},
		{
			name:           "error - invalid header",
			store:          &migStore{getMigResult: activeMig},
			migID:          "mig123",
			contentType:    "text/csv",
			body:           "wrong,headers,here\nhttps://github.com/org/repo,main,feature",
			wantStatus:     http.StatusBadRequest,
			wantBodySubstr: "CSV header must be",
		},
		{
			name: "partial success - invalid repo_url on one line",
			store: func() *migStore {
				st := &migStore{getMigResult: activeMig}
				st.getMigRepoByURL.err = pgx.ErrNoRows
				return st
			}(),
			migID:       "mig123",
			contentType: "text/csv",
			body:        "repo_url,base_ref,target_ref\nhttps://github.com/org/good-repo,main,feature1\nftp://invalid.com/bad-repo,main,feature2",
			wantStatus:  http.StatusOK,
			wantCreated: 1,
			wantFailed:  1,
		},
		{
			name:           "partial success - missing fields",
			store:          &migStore{getMigResult: activeMig},
			migID:          "mig123",
			contentType:    "text/csv",
			body:           "repo_url,base_ref,target_ref\nhttps://github.com/org/repo,,feature",
			wantStatus:     http.StatusOK,
			wantFailed:     1,
			wantBodySubstr: "base_ref is required",
		},
		{
			name:        "partial success - strict CSV parse error",
			store:       &migStore{getMigResult: activeMig},
			migID:       "mig123",
			contentType: "text/csv",
			body:        "repo_url,base_ref,target_ref\nhttps://github.com/org/repo,main,\"unterminated",
			wantStatus:  http.StatusOK,
			wantFailed:  1,
		},
		{
			name: "partial success - store lookup error is a per-line failure",
			store: func() *migStore {
				st := &migStore{getMigResult: activeMig}
				st.getMigRepoByURL.err = errors.New("db down")
				return st
			}(),
			migID:       "mig123",
			contentType: "text/csv",
			body:        "repo_url,base_ref,target_ref\nhttps://github.com/org/repo,main,feature",
			wantStatus:  http.StatusOK,
			wantFailed:  1,
			verify: func(t *testing.T, m *migStore) {
				t.Helper()
				assertNotCalled(t, "UpsertMigRepo", m.upsertMigRepoCalled)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := bulkUpsertMigReposHandler(tt.store)
			rr := doRequestWithContentType(t, handler, http.MethodPost, "/v1/migs/"+tt.migID+"/repos/bulk", tt.contentType, tt.body, "mig_id", tt.migID)
			assertStatus(t, rr, tt.wantStatus)
			assertBodyContains(t, rr, tt.wantBodySubstr)
			if tt.verify != nil {
				tt.verify(t, tt.store)
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
