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

	"github.com/iw2rmb/ploy/internal/store"
)

func TestListRunDiffs_ReturnsItems(t *testing.T) {
	st := &mockStore{}
	runID := uuid.New()
	jobID := uuid.New()
	diffID := uuid.New()
	createdAt := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	st.listDiffsByRunResult = []store.Diff{{
		ID:        pgtype.UUID{Bytes: diffID, Valid: true},
		RunID:     pgtype.UUID{Bytes: runID, Valid: true},
		JobID:     pgtype.UUID{Bytes: jobID, Valid: true},
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
	if item.JobID != jobID.String() {
		t.Errorf("job_id=%q, want %q", item.JobID, jobID.String())
	}
	if !item.CreatedAt.Equal(createdAt) {
		t.Errorf("created_at=%v, want %v", item.CreatedAt, createdAt)
	}
	if item.Size != 2 {
		t.Errorf("gzipped_size=%d, want 2", item.Size)
	}
	if exitCode, ok := item.Summary["exit_code"].(float64); !ok || exitCode != 0 {
		t.Errorf("summary[exit_code]=%v, want 0", item.Summary["exit_code"])
	}
}

func TestGetDiff_Download(t *testing.T) {
	st := &mockStore{}
	runID := uuid.New()
	jobID := uuid.New()
	diffID := uuid.New()
	st.getDiffResult = store.Diff{
		ID:        pgtype.UUID{Bytes: diffID, Valid: true},
		RunID:     pgtype.UUID{Bytes: runID, Valid: true},
		JobID:     pgtype.UUID{Bytes: jobID, Valid: true},
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
	runID := uuid.New()
	jobID := uuid.New()
	diffID := uuid.New()
	createdAt := time.Date(2025, 1, 15, 14, 30, 0, 0, time.UTC)
	st.getDiffResult = store.Diff{
		ID:        pgtype.UUID{Bytes: diffID, Valid: true},
		RunID:     pgtype.UUID{Bytes: runID, Valid: true},
		JobID:     pgtype.UUID{Bytes: jobID, Valid: true},
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
	if resp.RunID != runID.String() {
		t.Errorf("run_id=%q, want %q", resp.RunID, runID.String())
	}
	if resp.JobID == nil || *resp.JobID != jobID.String() {
		t.Errorf("job_id=%v, want %q", resp.JobID, jobID.String())
	}
	if !resp.CreatedAt.Equal(createdAt) {
		t.Errorf("created_at=%v, want %v", resp.CreatedAt, createdAt)
	}
	if resp.GzippedSize != 3 {
		t.Errorf("gzipped_size=%d, want 3", resp.GzippedSize)
	}
	if exitCode, ok := resp.Summary["exit_code"].(float64); !ok || exitCode != 0 {
		t.Errorf("summary[exit_code]=%v, want 0", resp.Summary["exit_code"])
	}
	if filesChanged, ok := resp.Summary["files_changed"].(float64); !ok || filesChanged != 3 {
		t.Errorf("summary[files_changed]=%v, want 3", resp.Summary["files_changed"])
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
	runID := uuid.New()
	jobID := uuid.New()
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
	runID := uuid.New()
	diffID := uuid.New()
	createdAt := time.Date(2025, 1, 15, 14, 30, 0, 0, time.UTC)
	st.getDiffResult = store.Diff{
		ID:        pgtype.UUID{Bytes: diffID, Valid: true},
		RunID:     pgtype.UUID{Bytes: runID, Valid: true},
		JobID:     pgtype.UUID{Valid: false},
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
