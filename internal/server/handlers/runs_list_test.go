package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestListRunsHandlerIncludesRepoMetadata(t *testing.T) {
	runID := domaintypes.NewRunID()
	migID := domaintypes.NewMigID()
	specID := domaintypes.NewSpecID()
	repoID := domaintypes.NewRepoID()
	sourceSHA := "0123456789abcdef0123456789abcdef01234567"
	now := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	st := &runStore{
		listRunsWithMetadata: mockCall[store.ListRunsWithMetadataParams, []store.ListRunsWithMetadataRow]{
			val: []store.ListRunsWithMetadataRow{{
				ID:               runID,
				WaveID:           domaintypes.WaveID(runID.String()),
				MigID:            migID,
				SpecID:           specID,
				RepoID:           repoID,
				RepoBaseRef:      "main",
				SourceCommitSha:  sourceSHA,
				RepoSha0:         sourceSHA,
				Status:           domaintypes.RunStatusRunning,
				Attempt:          1,
				CreatedAt:        now,
				RepoUrl:          "https://gitlab.example.com/team/service.git",
				SpecName:         "upgrade-java",
				SpecSourceDomain: "gitlab.example.com",
				SpecSourceRepo:   "team/specs",
			}},
		},
	}

	rr := doRequest(t, listRunsHandler(st), http.MethodGet, "/v1/runs", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Runs []domaintypes.RunSummary `json:"runs"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Runs) != 1 {
		t.Fatalf("runs length = %d, want 1", len(resp.Runs))
	}
	if resp.Runs[0].RepoURL != "https://gitlab.example.com/team/service.git" {
		t.Fatalf("repo_url = %q", resp.Runs[0].RepoURL)
	}
	if resp.Runs[0].SourceCommitSHA != sourceSHA {
		t.Fatalf("source_commit_sha = %q", resp.Runs[0].SourceCommitSHA)
	}
	if resp.Runs[0].SpecName != "upgrade-java" || resp.Runs[0].SpecSourceDomain != "gitlab.example.com" || resp.Runs[0].SpecSourceRepo != "team/specs" {
		t.Fatalf("spec display fields not propagated: %+v", resp.Runs[0])
	}
	if !st.listRunsWithMetadata.called {
		t.Fatal("ListRunsWithMetadata was not called")
	}
	if !st.listRunsWithMetadata.params.AllRuns && st.listRunsWithMetadata.params.CreatedBy != "" {
		t.Fatalf("created_by filter = %q, want empty fallback without identity", st.listRunsWithMetadata.params.CreatedBy)
	}
}

func TestListRunsHandlerOwnershipFilter(t *testing.T) {
	tests := []struct {
		name           string
		target         string
		identity       auth.Identity
		tokenUsername  *string
		wantAll        bool
		wantCreatedBy  string
		wantTokenQuery bool
	}{
		{
			name:           "token username wins over query fallback",
			target:         "/v1/runs?created_by=request-user",
			identity:       auth.Identity{Role: auth.RoleControlPlane, TokenID: "token-1"},
			tokenUsername:  stringPtrOrNil("token-user"),
			wantCreatedBy:  "token-user",
			wantTokenQuery: true,
		},
		{
			name:           "query fallback used when token has no username",
			target:         "/v1/runs?created_by=request-user",
			identity:       auth.Identity{Role: auth.RoleControlPlane, TokenID: "token-1"},
			wantCreatedBy:  "request-user",
			wantTokenQuery: true,
		},
		{
			name:          "all disables ownership filter",
			target:        "/v1/runs?created_by=request-user&all=true",
			identity:      auth.Identity{Role: auth.RoleControlPlane, TokenID: "token-1"},
			tokenUsername: stringPtrOrNil("token-user"),
			wantAll:       true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &runStore{
				getAPITokenByID: mockCall[string, store.GetAPITokenByIDRow]{
					val: store.GetAPITokenByIDRow{Username: tt.tokenUsername},
				},
			}
			body := bytes.NewReader(nil)
			req := httptest.NewRequest(http.MethodGet, tt.target, body)
			req = req.WithContext(auth.ContextWithIdentity(req.Context(), tt.identity))
			rr := httptest.NewRecorder()

			listRunsHandler(st).ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
			}
			if st.getAPITokenByID.called != tt.wantTokenQuery {
				t.Fatalf("GetAPITokenByID called = %v, want %v", st.getAPITokenByID.called, tt.wantTokenQuery)
			}
			if !st.listRunsWithMetadata.called {
				t.Fatal("ListRunsWithMetadata was not called")
			}
			if st.listRunsWithMetadata.params.AllRuns != tt.wantAll {
				t.Fatalf("AllRuns = %v, want %v", st.listRunsWithMetadata.params.AllRuns, tt.wantAll)
			}
			if st.listRunsWithMetadata.params.CreatedBy != tt.wantCreatedBy {
				t.Fatalf("CreatedBy = %q, want %q", st.listRunsWithMetadata.params.CreatedBy, tt.wantCreatedBy)
			}
		})
	}
}

func TestListRunsHandlerRepoURLAppliesOwnershipFilter(t *testing.T) {
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	owner := "alice"
	other := "bob"
	tests := []struct {
		name      string
		target    string
		createdBy *string
		wantCount int
	}{
		{
			name:      "filters out other creator",
			target:    "/v1/runs?repo_url=https://gitlab.example.com/team/service&created_by=alice",
			createdBy: &other,
			wantCount: 0,
		},
		{
			name:      "all includes other creator",
			target:    "/v1/runs?repo_url=https://gitlab.example.com/team/service&created_by=alice&all=true",
			createdBy: &other,
			wantCount: 1,
		},
		{
			name:      "matching creator included",
			target:    "/v1/runs?repo_url=https://gitlab.example.com/team/service&created_by=alice",
			createdBy: &owner,
			wantCount: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &runStore{
				listDistinctRepos: mockCall[string, []store.ListDistinctReposRow]{
					val: []store.ListDistinctReposRow{{
						RepoID:  repoID,
						RepoUrl: "https://gitlab.example.com/team/service",
					}},
				},
				listRunsForRepo: mockCall[store.ListRunsForRepoParams, []store.ListRunsForRepoRow]{
					val: []store.ListRunsForRepoRow{{RunID: runID}},
				},
				getRun: mockCall[string, store.Run]{
					val: store.Run{
						ID:        runID,
						WaveID:    domaintypes.WaveID(runID.String()),
						MigID:     domaintypes.NewMigID(),
						SpecID:    domaintypes.NewSpecID(),
						RepoID:    repoID,
						Status:    domaintypes.RunStatusRunning,
						CreatedBy: tt.createdBy,
						CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
					},
				},
			}
			req := httptest.NewRequest(http.MethodGet, tt.target, nil)
			rr := httptest.NewRecorder()

			listRunsHandler(st).ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
			}
			var resp struct {
				Runs []domaintypes.RunSummary `json:"runs"`
			}
			if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if len(resp.Runs) != tt.wantCount {
				t.Fatalf("runs length = %d, want %d", len(resp.Runs), tt.wantCount)
			}
		})
	}
}
