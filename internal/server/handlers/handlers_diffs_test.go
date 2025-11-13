package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/store"
)

func TestListRunDiffs_ReturnsItems(t *testing.T) {
	st := &mockStore{}
	runID := uuid.New()
	stageID := uuid.New()
	diffID := uuid.New()
	createdAt := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	st.listDiffsByRunResult = []store.Diff{{
		ID:        pgtype.UUID{Bytes: diffID, Valid: true},
		RunID:     pgtype.UUID{Bytes: runID, Valid: true},
		StageID:   pgtype.UUID{Bytes: stageID, Valid: true},
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
	if item.StageID != stageID.String() {
		t.Errorf("stage_id=%q, want %q", item.StageID, stageID.String())
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
	stageID := uuid.New()
	diffID := uuid.New()
	st.getDiffResult = store.Diff{
		ID:        pgtype.UUID{Bytes: diffID, Valid: true},
		RunID:     pgtype.UUID{Bytes: runID, Valid: true},
		StageID:   pgtype.UUID{Bytes: stageID, Valid: true},
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
