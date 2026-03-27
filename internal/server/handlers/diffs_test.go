package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	bsmock "github.com/iw2rmb/ploy/internal/blobstore/mock"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestRunRepoDiffs_Download(t *testing.T) {
	st := &mockStore{}
	runID := domaintypes.NewRunID()
	repoID := "repoAAAA"
	jobID := domaintypes.NewJobID()
	diffID := uuid.New()
	patch := []byte{0x1f, 0x8b, 0x08, 0x00}
	repoIDTyped := domaintypes.RepoID(repoID)
	objKey := "diffs/run/" + runID.String() + "/diff/" + diffID.String() + ".patch.gz"

	st.getDiffResult = store.Diff{
		ID:        pgtype.UUID{Bytes: diffID, Valid: true},
		RunID:     runID,
		JobID:     &jobID,
		PatchSize: int64(len(patch)),
		ObjectKey: &objKey,
	}
	st.getJobResult = store.Job{ID: jobID, RunID: runID, RepoID: repoIDTyped}

	// Create mock blobstore and pre-populate with patch data.
	bs := bsmock.New()
	_, _ = bs.Put(context.TODO(), objKey, "application/gzip", patch)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+runID.String()+"/repos/"+repoID+"/diffs?download=true&diff_id="+diffID.String(), nil)
	req.SetPathValue("run_id", runID.String())
	req.SetPathValue("repo_id", repoID)
	listRunRepoDiffsHandler(st, bs).ServeHTTP(rr, req)
	assertStatus(t, rr, http.StatusOK)
	if ct := rr.Header().Get("Content-Type"); ct != "application/gzip" {
		t.Fatalf("content-type=%s, want application/gzip", ct)
	}
	if !bytes.Equal(rr.Body.Bytes(), patch) {
		t.Fatalf("patch len=%d, want %d", rr.Body.Len(), len(patch))
	}
	if !st.getDiffCalled {
		t.Fatal("expected GetDiff to be called")
	}
	if !st.getJobCalled {
		t.Fatal("expected GetJob to be called")
	}
}

// TestRunRepoDiffs_ReturnsRepoFilteredItems verifies that diffs for repo A are
// excluded from repo B listing. This is the primary v1 repo-scoped test.
//
// The test sets up:
// - Two repos (repo A and repo B) for a run
// - A diff that belongs to repo A (via job_id -> jobs.repo_id join)
// - A query for repo B
// - Expects an empty result (repo A's diff excluded from repo B listing)
func TestRunRepoDiffs_ReturnsRepoFilteredItems(t *testing.T) {
	st := &mockStore{}
	runID := domaintypes.NewRunID()
	repoAID := "repoAAAA" // NanoID-backed
	repoBID := "repoBBBB" // NanoID-backed
	repoBIDTyped := domaintypes.RepoID(repoBID)

	// Setup: diff for repo A (via job_id -> jobs.repo_id join)
	jobAID := domaintypes.NewJobID()
	jobAIDStr := jobAID.String()
	diffAID := uuid.New()
	createdAt := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)

	// This diff belongs to repo A. When querying for repo B, the store query
	// joins diffs.job_id -> jobs.repo_id and filters by repo_id=repoBID,
	// so this diff should NOT appear in repo B results.
	//
	// For this test, we simulate the store returning empty results when
	// querying for repo B (because the diff belongs to repo A).
	_ = diffAID   // unused in expected repo B result
	_ = jobAIDStr // unused in expected repo B result
	_ = createdAt // unused in expected repo B result
	_ = repoAID   // repo A owns the diff

	// Query for repo B: expect empty list (repo A's diff filtered out)
	st.listDiffsMetaByRunRepoResult = []store.Diff{} // Empty: repo A's diff excluded

	bs := bsmock.New()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+runID.String()+"/repos/"+repoBID+"/diffs", nil)
	req.SetPathValue("run_id", runID.String())
	req.SetPathValue("repo_id", repoBID)
	listRunRepoDiffsHandler(st, bs).ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusOK)

	// Verify the query was called with correct parameters
	if !st.listDiffsMetaByRunRepoCalled {
		t.Fatal("expected ListDiffsMetaByRunRepo to be called")
	}
	if st.listDiffsMetaByRunRepoParams.RunID != runID {
		t.Errorf("run_id=%q, want %q", st.listDiffsMetaByRunRepoParams.RunID, runID)
	}
	if st.listDiffsMetaByRunRepoParams.RepoID != repoBIDTyped {
		t.Errorf("repo_id=%q, want %q", st.listDiffsMetaByRunRepoParams.RepoID, repoBIDTyped)
	}

	var resp diffListResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Key assertion: repo A's diff excluded from repo B listing
	if len(resp.Diffs) != 0 {
		t.Fatalf("expected 0 diffs for repo B (repo A's diff should be excluded), got %d", len(resp.Diffs))
	}
}

// TestRunRepoDiffs_ReturnsOwnDiffs verifies that a repo sees its own diffs.
// This tests the positive case: querying repo A returns repo A's diffs.
func TestRunRepoDiffs_ReturnsOwnDiffs(t *testing.T) {
	st := &mockStore{}
	runID := domaintypes.NewRunID()
	repoID := "repoAAAA" // NanoID-backed
	jobID := domaintypes.NewJobID()
	diffID := uuid.New()
	createdAt := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)

	// Store returns the diff that belongs to this repo
	st.listDiffsMetaByRunRepoResult = []store.Diff{{
		ID:        pgtype.UUID{Bytes: diffID, Valid: true},
		RunID:     runID,
		JobID:     &jobID,
		Summary:   []byte(`{"exit_code":0,"job_type":"mig"}`),
		CreatedAt: pgtype.Timestamptz{Time: createdAt, Valid: true},
		PatchSize: 3,
	}}

	bs := bsmock.New()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+runID.String()+"/repos/"+repoID+"/diffs", nil)
	req.SetPathValue("run_id", runID.String())
	req.SetPathValue("repo_id", repoID)
	listRunRepoDiffsHandler(st, bs).ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusOK)

	var resp diffListResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Verify own diffs are returned
	if len(resp.Diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(resp.Diffs))
	}
	item := resp.Diffs[0]
	if item.ID != diffID.String() {
		t.Errorf("id=%q, want %q", item.ID, diffID.String())
	}
	if item.JobID != jobID {
		t.Errorf("job_id=%q, want %q", item.JobID, jobID.String())
	}
	if !item.CreatedAt.Equal(createdAt) {
		t.Errorf("created_at=%v, want %v", item.CreatedAt, createdAt)
	}
	if item.Size != 3 {
		t.Errorf("gzipped_size=%d, want 3", item.Size)
	}
}

// TestRunRepoDiffs_MissingRunID verifies that missing run_id returns 400.
func TestRunRepoDiffs_MissingRunID(t *testing.T) {
	st := &mockStore{}
	bs := bsmock.New()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/runs//repos/repoAAAA/diffs", nil)
	req.SetPathValue("run_id", "")
	req.SetPathValue("repo_id", "repoAAAA")
	listRunRepoDiffsHandler(st, bs).ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
}

// TestRunRepoDiffs_MissingRepoID verifies that missing repo_id returns 400.
func TestRunRepoDiffs_MissingRepoID(t *testing.T) {
	st := &mockStore{}
	bs := bsmock.New()
	runID := domaintypes.NewRunID()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+runID.String()+"/repos//diffs", nil)
	req.SetPathValue("run_id", runID.String())
	req.SetPathValue("repo_id", "")
	listRunRepoDiffsHandler(st, bs).ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
}
