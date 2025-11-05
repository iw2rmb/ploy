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
	st.listDiffsByRunResult = []store.Diff{{
		ID:        pgtype.UUID{Bytes: diffID, Valid: true},
		RunID:     pgtype.UUID{Bytes: runID, Valid: true},
		StageID:   pgtype.UUID{Bytes: stageID, Valid: true},
		Patch:     []byte{0x1f, 0x8b},
		Summary:   []byte(`{"exit_code":0}`),
		CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	}}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/mods/"+runID.String()+"/diffs", nil)
	req.SetPathValue("id", runID.String())
	listRunDiffsHandler(st).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
	var resp struct{ Diffs []struct{ ID string } }
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Diffs) != 1 || resp.Diffs[0].ID != diffID.String() {
		t.Fatalf("unexpected body: %s", rr.Body.String())
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
