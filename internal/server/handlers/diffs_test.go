package handlers

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
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
	st := &artifactStore{}
	runID := domaintypes.NewRunID()
	repoID := "repoAAAA"
	jobID := domaintypes.NewJobID()
	diffID := uuid.New()
	patch := []byte{0x1f, 0x8b, 0x08, 0x00}
	repoIDTyped := domaintypes.RepoID(repoID)
	objKey := "diffs/run/" + runID.String() + "/diff/" + diffID.String() + ".patch.gz"

	st.getRunRepo.val = store.Run{
		ID:      runID,
		RepoID:  repoIDTyped,
		Attempt: 1,
	}
	st.listJobsByRunRepoAttempt.val = []store.Job{
		{ID: jobID, RunID: runID, RepoID: repoIDTyped, Attempt: 1},
	}
	st.getJobByID = map[domaintypes.JobID]store.Job{
		jobID: {ID: jobID, RunID: runID, RepoID: repoIDTyped, Attempt: 1},
	}
	st.getLatestDiffByJob.val = store.Diff{
		ID:        pgtype.UUID{Bytes: diffID, Valid: true},
		RunID:     runID,
		JobID:     &jobID,
		PatchSize: int64(len(patch)),
		ObjectKey: &objKey,
	}

	// Create mock blobstore and pre-populate with patch data.
	bs := bsmock.New()
	_, _ = bs.Put(context.TODO(), objKey, "application/gzip", patch)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+runID.String()+"/diffs?download=true&diff_id="+diffID.String(), nil)
	req.SetPathValue("run_id", runID.String())
	listRunDiffsHandler(st, bs).ServeHTTP(rr, req)
	assertStatus(t, rr, http.StatusOK)
	if ct := rr.Header().Get("Content-Type"); ct != "application/gzip" {
		t.Fatalf("content-type=%s, want application/gzip", ct)
	}
	if !bytes.Equal(rr.Body.Bytes(), patch) {
		t.Fatalf("patch len=%d, want %d", rr.Body.Len(), len(patch))
	}
	if !st.getLatestDiffByJob.called {
		t.Fatal("expected GetLatestDiffByJob to be called")
	}
	if !st.getJob.called {
		t.Fatal("expected GetJob to be called")
	}
}

func TestRunRepoDiffs_DownloadAccumulated(t *testing.T) {
	st := &artifactStore{}
	runID := domaintypes.NewRunID()
	repoID := "repoAAAA"
	repoIDTyped := domaintypes.RepoID(repoID)
	jobID1 := domaintypes.NewJobID()
	jobID2 := domaintypes.NewJobID()
	diffID1 := uuid.New()
	diffID2 := uuid.New()
	objKey1 := "diffs/run/" + runID.String() + "/diff/" + diffID1.String() + ".patch.gz"
	objKey2 := "diffs/run/" + runID.String() + "/diff/" + diffID2.String() + ".patch.gz"
	patch1 := "diff --git a/a.txt b/a.txt\n+one\n"
	patch2 := "diff --git a/b.txt b/b.txt\n+two\n"

	st.getRunRepo.val = store.Run{
		ID:      runID,
		RepoID:  repoIDTyped,
		Attempt: 1,
	}
	st.listJobsByRunRepoAttempt.val = []store.Job{
		{ID: jobID1, RunID: runID, RepoID: repoIDTyped, Attempt: 1, NextID: &jobID2},
		{ID: jobID2, RunID: runID, RepoID: repoIDTyped, Attempt: 1},
	}
	st.getJobByID = map[domaintypes.JobID]store.Job{
		jobID1: {ID: jobID1, RunID: runID, RepoID: repoIDTyped, Attempt: 1},
		jobID2: {ID: jobID2, RunID: runID, RepoID: repoIDTyped, Attempt: 1},
	}
	st.getLatestDiffByJobByID = map[domaintypes.JobID]store.Diff{
		jobID1: {ID: pgtype.UUID{Bytes: diffID1, Valid: true}, RunID: runID, JobID: &jobID1, ObjectKey: &objKey1},
		jobID2: {ID: pgtype.UUID{Bytes: diffID2, Valid: true}, RunID: runID, JobID: &jobID2, ObjectKey: &objKey2},
	}

	bs := bsmock.New()
	_, _ = bs.Put(context.TODO(), objKey1, "application/gzip", gzipTestBytes(t, []byte(patch1)))
	_, _ = bs.Put(context.TODO(), objKey2, "application/gzip", gzipTestBytes(t, []byte(patch2)))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+runID.String()+"/diffs?download=true&accumulated=true&diff_id="+diffID2.String(), nil)
	req.SetPathValue("run_id", runID.String())
	listRunDiffsHandler(st, bs).ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusOK)
	gotPlain := gunzipTestBytes(t, rr.Body.Bytes())
	if string(gotPlain) != patch1+patch2 {
		t.Fatalf("accumulated patch mismatch: got %q, want %q", string(gotPlain), patch1+patch2)
	}
}

func TestRunRepoDiffs_ReturnsEmptyListWhenRunHasNoDiffJobs(t *testing.T) {
	st := &artifactStore{}
	runID := domaintypes.NewRunID()
	repoBID := "repoBBBB" // NanoID-backed

	st.getRunRepo.val = store.Run{
		ID:      runID,
		RepoID:  domaintypes.RepoID(repoBID),
		Attempt: 1,
	}
	st.listJobsByRunRepoAttempt.val = []store.Job{}

	bs := bsmock.New()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+runID.String()+"/diffs", nil)
	req.SetPathValue("run_id", runID.String())
	listRunDiffsHandler(st, bs).ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusOK)

	// Verify the run was resolved before listing diffs.
	if !st.getRunRepo.called {
		t.Fatal("expected GetRun to be called")
	}
	if st.getRunRepo.params != runID {
		t.Errorf("run_id=%q, want %q", st.getRunRepo.params, runID)
	}

	var resp diffListResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(resp.Diffs) != 0 {
		t.Fatalf("expected 0 diffs, got %d", len(resp.Diffs))
	}
}

// TestRunRepoDiffs_ReturnsOwnDiffs verifies that a repo sees its own diffs.
// This tests the positive case: querying repo A returns repo A's diffs.
func TestRunRepoDiffs_ReturnsOwnDiffs(t *testing.T) {
	st := &artifactStore{}
	runID := domaintypes.NewRunID()
	repoID := "repoAAAA" // NanoID-backed
	repoIDTyped := domaintypes.RepoID(repoID)
	jobID := domaintypes.NewJobID()
	diffID := uuid.New()
	createdAt := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)

	st.getRunRepo.val = store.Run{
		ID:      runID,
		RepoID:  repoIDTyped,
		Attempt: 1,
	}
	st.listJobsByRunRepoAttempt.val = []store.Job{{
		ID:      jobID,
		RunID:   runID,
		RepoID:  repoIDTyped,
		Attempt: 1,
	}}
	st.getJobByID = map[domaintypes.JobID]store.Job{
		jobID: {ID: jobID, RunID: runID, RepoID: repoIDTyped, Attempt: 1},
	}
	st.getLatestDiffByJob.val = store.Diff{
		ID:        pgtype.UUID{Bytes: diffID, Valid: true},
		RunID:     runID,
		JobID:     &jobID,
		Summary:   []byte(`{"exit_code":0,"job_type":"mig"}`),
		CreatedAt: pgtype.Timestamptz{Time: createdAt, Valid: true},
		PatchSize: 3,
	}

	bs := bsmock.New()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+runID.String()+"/diffs", nil)
	req.SetPathValue("run_id", runID.String())
	listRunDiffsHandler(st, bs).ServeHTTP(rr, req)

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
	st := &artifactStore{}
	bs := bsmock.New()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/runs//diffs", nil)
	req.SetPathValue("run_id", "")
	listRunDiffsHandler(st, bs).ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
}

func gzipTestBytes(t *testing.T, input []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(input); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.Bytes()
}

func gunzipTestBytes(t *testing.T, input []byte) []byte {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(input))
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer zr.Close()
	out, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("gzip read: %v", err)
	}
	return out
}
