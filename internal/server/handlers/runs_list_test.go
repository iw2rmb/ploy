package handlers

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

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
		listRuns: mockResult[[]store.Run]{val: []store.Run{{
			ID:              runID,
			WaveID:          domaintypes.WaveID(runID.String()),
			MigID:           migID,
			SpecID:          specID,
			RepoID:          repoID,
			RepoBaseRef:     "main",
			SourceCommitSha: sourceSHA,
			RepoSha0:        sourceSHA,
			Status:          domaintypes.RunStatusRunning,
			Attempt:         1,
			CreatedAt:       now,
		}}},
		repoByID: map[domaintypes.RepoID]store.Repo{
			repoID: {ID: repoID, Url: "https://gitlab.example.com/team/service.git"},
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
}
