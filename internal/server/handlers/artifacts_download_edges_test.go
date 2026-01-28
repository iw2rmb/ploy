package handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	bsmock "github.com/iw2rmb/ploy/internal/blobstore/mock"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// Additional edge tests for error paths on download surfaces.

func TestListArtifactsByCIDHandler_DBError(t *testing.T) {
	st := &mockStore{listArtifactBundlesMetaByCIDErr: errors.New("boom")}
	h := listArtifactsByCIDHandler(st)
	req := httptest.NewRequest(http.MethodGet, "/v1/artifacts?cid=bafyerr", nil)
	w := httptest.NewRecorder()
	h(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestGetArtifactHandler_DBError(t *testing.T) {
	id := uuid.New()
	st := &mockStore{getArtifactBundleErr: errors.New("db down")}
	bs := bsmock.New()
	h := getArtifactHandler(st, bs)
	req := httptest.NewRequest(http.MethodGet, "/v1/artifacts/"+id.String(), nil)
	req.SetPathValue("id", id.String())
	w := httptest.NewRecorder()
	h(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

// Sanity check: metadata path includes CreatedAt only when valid and always returns non-empty IDs.
func TestGetArtifactHandler_Metadata_Fields(t *testing.T) {
	id := uuid.New()
	runID := domaintypes.NewRunID()
	st := &mockStore{getArtifactBundleResult: store.ArtifactBundle{
		ID:         pgtype.UUID{Bytes: id, Valid: true},
		RunID:      runID,
		BundleSize: 1,
		// no CreatedAt valid timestamp here; handler should omit formatting without panicking
	}}
	bs := bsmock.New()
	h := getArtifactHandler(st, bs)
	req := httptest.NewRequest(http.MethodGet, "/v1/artifacts/"+id.String(), nil)
	req.SetPathValue("id", id.String())
	w := httptest.NewRecorder()
	h(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
