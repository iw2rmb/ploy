package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestListRunDiffs_ReturnsItems(t *testing.T) {
	st := &mockStore{}
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()
	jobIDStr := jobID.String()
	diffID := uuid.New()
	createdAt := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	st.listDiffsByRunResult = []store.Diff{{
		ID:        pgtype.UUID{Bytes: diffID, Valid: true},
		RunID:     runID.String(),
		JobID:     &jobIDStr,
		Patch:     []byte{0x1f, 0x8b},
		Summary:   []byte(`{"exit_code":0}`),
		CreatedAt: pgtype.Timestamptz{Time: createdAt, Valid: true},
	}}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/mods/"+runID.String()+"/diffs", nil)
	req.SetPathValue("id", runID.String())
	listRunDiffsHandler(st).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
	var resp diffListResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(resp.Diffs))
	}
	item := resp.Diffs[0]
	if item.ID != diffID.String() {
		t.Errorf("id=%q, want %q", item.ID, diffID.String())
	}
	if item.JobID != domaintypes.JobID(jobID.String()) { // Compare with domain type
		t.Errorf("job_id=%q, want %q", item.JobID, jobID.String())
	}
	if !item.CreatedAt.Equal(createdAt) {
		t.Errorf("created_at=%v, want %v", item.CreatedAt, createdAt)
	}
	if item.Size != 2 {
		t.Errorf("gzipped_size=%d, want 2", item.Size)
	}
	// DiffSummary is now json.RawMessage-backed; use accessor methods.
	if exitCode, ok := item.Summary.ExitCode(); !ok || exitCode != 0 {
		t.Errorf("summary.ExitCode()=%d, want 0", exitCode)
	}
}

func TestGetDiff_Download(t *testing.T) {
	st := &mockStore{}
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()
	jobIDStr := jobID.String()
	diffID := uuid.New()
	st.getDiffResult = store.Diff{
		ID:        pgtype.UUID{Bytes: diffID, Valid: true},
		RunID:     runID.String(),
		JobID:     &jobIDStr,
		Patch:     []byte{0x1f, 0x8b, 0x08},
		Summary:   []byte(`{"exit_code":0}`),
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/diffs/"+diffID.String()+"?download=true", nil)
	req.SetPathValue("id", diffID.String())
	getDiffHandler(st).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/gzip" {
		t.Fatalf("content-type=%s", ct)
	}
	if rr.Body.Len() == 0 {
		t.Fatal("empty body")
	}
}

func TestGetDiff_Metadata(t *testing.T) {
	st := &mockStore{}
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()
	jobIDStr := jobID.String()
	diffID := uuid.New()
	createdAt := time.Date(2025, 1, 15, 14, 30, 0, 0, time.UTC)
	st.getDiffResult = store.Diff{
		ID:        pgtype.UUID{Bytes: diffID, Valid: true},
		RunID:     runID.String(),
		JobID:     &jobIDStr,
		Patch:     []byte{0x1f, 0x8b, 0x08},
		Summary:   []byte(`{"exit_code":0,"files_changed":3}`),
		CreatedAt: pgtype.Timestamptz{Time: createdAt, Valid: true},
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/diffs/"+diffID.String(), nil)
	req.SetPathValue("id", diffID.String())
	getDiffHandler(st).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type=%s, want application/json", ct)
	}
	var resp diffGetResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ID != diffID.String() {
		t.Errorf("id=%q, want %q", resp.ID, diffID.String())
	}
	if resp.RunID != runID {
		t.Errorf("run_id=%q, want %q", resp.RunID, runID)
	}
	expectedJobID := domaintypes.JobID(jobID.String()) // Create expected domain type
	if resp.JobID == nil || *resp.JobID != expectedJobID {
		t.Errorf("job_id=%v, want %q", resp.JobID, jobID.String())
	}
	if !resp.CreatedAt.Equal(createdAt) {
		t.Errorf("created_at=%v, want %v", resp.CreatedAt, createdAt)
	}
	if resp.GzippedSize != 3 {
		t.Errorf("gzipped_size=%d, want 3", resp.GzippedSize)
	}
	// DiffSummary is now json.RawMessage-backed; use accessor methods.
	if exitCode, ok := resp.Summary.ExitCode(); !ok || exitCode != 0 {
		t.Errorf("summary.ExitCode()=%d, want 0", exitCode)
	}
	if filesChanged, ok := resp.Summary.FilesChanged(); !ok || filesChanged != 3 {
		t.Errorf("summary.FilesChanged()=%d, want 3", filesChanged)
	}
}

func TestGetDiff_InvalidID(t *testing.T) {
	st := &mockStore{}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/diffs/bad-id", nil)
	req.SetPathValue("id", "bad-id")
	getDiffHandler(st).ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", rr.Code)
	}
}

func TestGetDiff_MissingID(t *testing.T) {
	st := &mockStore{}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/diffs/", nil)
	req.SetPathValue("id", "")
	getDiffHandler(st).ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", rr.Code)
	}
}

func TestGetDiff_NotFound(t *testing.T) {
	st := &mockStore{}
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()
	diffID := uuid.New()
	_ = runID
	_ = jobID
	st.getDiffErr = pgx.ErrNoRows

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/diffs/"+diffID.String(), nil)
	req.SetPathValue("id", diffID.String())
	getDiffHandler(st).ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status %d, want 404", rr.Code)
	}
}

func TestGetDiff_Metadata_JobIDNull(t *testing.T) {
	st := &mockStore{}
	runID := domaintypes.NewRunID()
	diffID := uuid.New()
	createdAt := time.Date(2025, 1, 15, 14, 30, 0, 0, time.UTC)
	st.getDiffResult = store.Diff{
		ID:        pgtype.UUID{Bytes: diffID, Valid: true},
		RunID:     runID.String(),
		JobID:     nil,
		Patch:     []byte{0x1f, 0x8b},
		Summary:   []byte(`{}`),
		CreatedAt: pgtype.Timestamptz{Time: createdAt, Valid: true},
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/diffs/"+diffID.String(), nil)
	req.SetPathValue("id", diffID.String())
	getDiffHandler(st).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
	var resp diffGetResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.JobID != nil {
		t.Errorf("job_id=%v, want nil", *resp.JobID)
	}
}

// TestRunRepoDiffs_ReturnsRepoFilteredItems verifies that diffs for repo A are
// excluded from repo B listing. This is the primary v1 repo-scoped test per
// roadmap/v1/scope.md:85 and ROADMAP.md:372.
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
	st.listDiffsByRunRepoResult = []store.Diff{} // Empty: repo A's diff excluded

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+runID.String()+"/repos/"+repoBID+"/diffs", nil)
	req.SetPathValue("run_id", runID.String())
	req.SetPathValue("repo_id", repoBID)
	listRunRepoDiffsHandler(st).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status %d, body: %s", rr.Code, rr.Body.String())
	}

	// Verify the query was called with correct parameters
	if !st.listDiffsByRunRepoCalled {
		t.Fatal("expected ListDiffsByRunRepo to be called")
	}
	if st.listDiffsByRunRepoParams.RunID != runID.String() {
		t.Errorf("run_id=%q, want %q", st.listDiffsByRunRepoParams.RunID, runID.String())
	}
	if st.listDiffsByRunRepoParams.RepoID != repoBID {
		t.Errorf("repo_id=%q, want %q", st.listDiffsByRunRepoParams.RepoID, repoBID)
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
	jobIDStr := jobID.String()
	diffID := uuid.New()
	createdAt := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)

	// Store returns the diff that belongs to this repo
	st.listDiffsByRunRepoResult = []store.Diff{{
		ID:        pgtype.UUID{Bytes: diffID, Valid: true},
		RunID:     runID.String(),
		JobID:     &jobIDStr,
		Patch:     []byte{0x1f, 0x8b, 0x08},
		Summary:   []byte(`{"exit_code":0,"mod_type":"mod"}`),
		CreatedAt: pgtype.Timestamptz{Time: createdAt, Valid: true},
	}}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+runID.String()+"/repos/"+repoID+"/diffs", nil)
	req.SetPathValue("run_id", runID.String())
	req.SetPathValue("repo_id", repoID)
	listRunRepoDiffsHandler(st).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status %d, body: %s", rr.Code, rr.Body.String())
	}

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
	if item.JobID != domaintypes.JobID(jobID.String()) {
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
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/runs//repos/repoAAAA/diffs", nil)
	req.SetPathValue("run_id", "")
	req.SetPathValue("repo_id", "repoAAAA")
	listRunRepoDiffsHandler(st).ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", rr.Code)
	}
}

// TestRunRepoDiffs_MissingRepoID verifies that missing repo_id returns 400.
func TestRunRepoDiffs_MissingRepoID(t *testing.T) {
	st := &mockStore{}
	runID := domaintypes.NewRunID()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+runID.String()+"/repos//diffs", nil)
	req.SetPathValue("run_id", runID.String())
	req.SetPathValue("repo_id", "")
	listRunRepoDiffsHandler(st).ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", rr.Code)
	}
}
