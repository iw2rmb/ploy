package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	bsmock "github.com/iw2rmb/ploy/internal/blobstore/mock"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/store"
)

// TestUploadSpecBundleHandler covers the POST /v1/spec-bundles endpoint.
func TestUploadSpecBundleHandler(t *testing.T) {
	bundleData := []byte("fake gzip bundle content for testing")
	cid, digest := computeSpecBundleCIDAndDigest(bundleData)

	t.Run("UploadSuccess", func(t *testing.T) {
		bundleID := domaintypes.NewSpecBundleID()
		objKey := "spec-bundles/" + bundleID.String() + ".gz"
		st := &mockStore{
			getSpecBundleByCIDErr: pgx.ErrNoRows, // no existing bundle
			createSpecBundleResult: store.SpecBundle{
				ID:        bundleID,
				Cid:       cid,
				Digest:    digest,
				Size:      int64(len(bundleData)),
				ObjectKey: &objKey,
			},
		}
		bs := bsmock.New()
		bp := blobpersist.New(st, bs)

		req := httptest.NewRequest(http.MethodPost, "/v1/spec-bundles", bytes.NewReader(bundleData))
		req.Header.Set("Content-Type", "application/octet-stream")
		w := httptest.NewRecorder()
		uploadSpecBundleHandler(st, bp)(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
		}

		var resp specBundleUploadResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp.BundleID != bundleID.String() {
			t.Errorf("bundle_id: got %q, want %q", resp.BundleID, bundleID.String())
		}
		if resp.CID != cid {
			t.Errorf("cid: got %q, want %q", resp.CID, cid)
		}
		if resp.Deduplicated {
			t.Error("expected deduplicated=false for new upload")
		}
	})

	t.Run("Deduplicated", func(t *testing.T) {
		existingID := domaintypes.NewSpecBundleID()
		st := &mockStore{
			getSpecBundleByCIDResult: store.SpecBundle{
				ID:     existingID,
				Cid:    cid,
				Digest: digest,
				Size:   int64(len(bundleData)),
			},
			getSpecBundleByCIDErr: nil, // found
		}
		bs := bsmock.New()
		bp := blobpersist.New(st, bs)

		req := httptest.NewRequest(http.MethodPost, "/v1/spec-bundles", bytes.NewReader(bundleData))
		w := httptest.NewRecorder()
		uploadSpecBundleHandler(st, bp)(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for deduplicated, got %d: %s", w.Code, w.Body.String())
		}

		var resp specBundleUploadResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp.BundleID != existingID.String() {
			t.Errorf("bundle_id: got %q, want %q", resp.BundleID, existingID.String())
		}
		if !resp.Deduplicated {
			t.Error("expected deduplicated=true for existing bundle")
		}
		if !st.updateSpecBundleLastRefAtCalled {
			t.Error("expected UpdateSpecBundleLastRefAt to be called on dedup")
		}
	})

	t.Run("EmptyBody", func(t *testing.T) {
		st := &mockStore{}
		bs := bsmock.New()
		bp := blobpersist.New(st, bs)

		req := httptest.NewRequest(http.MethodPost, "/v1/spec-bundles", strings.NewReader(""))
		w := httptest.NewRecorder()
		uploadSpecBundleHandler(st, bp)(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for empty body, got %d", w.Code)
		}
	})

	t.Run("ExceedsSizeLimit", func(t *testing.T) {
		st := &mockStore{}
		bs := bsmock.New()
		bp := blobpersist.New(st, bs)

		req := httptest.NewRequest(http.MethodPost, "/v1/spec-bundles", bytes.NewReader(bundleData))
		req.ContentLength = maxSpecBundleSize + 1
		w := httptest.NewRecorder()
		uploadSpecBundleHandler(st, bp)(w, req)

		if w.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("expected 413 for oversized request, got %d", w.Code)
		}
	})

	t.Run("CreatedByQueryParam", func(t *testing.T) {
		bundleID := domaintypes.NewSpecBundleID()
		objKey := "spec-bundles/" + bundleID.String() + ".gz"
		st := &mockStore{
			getSpecBundleByCIDErr: pgx.ErrNoRows,
			createSpecBundleResult: store.SpecBundle{
				ID:        bundleID,
				Cid:       cid,
				Digest:    digest,
				Size:      int64(len(bundleData)),
				ObjectKey: &objKey,
			},
		}
		bs := bsmock.New()
		bp := blobpersist.New(st, bs)

		req := httptest.NewRequest(http.MethodPost, "/v1/spec-bundles?created_by=ci-bot", bytes.NewReader(bundleData))
		w := httptest.NewRecorder()
		uploadSpecBundleHandler(st, bp)(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
		}
		if st.createSpecBundleParams.CreatedBy == nil || *st.createSpecBundleParams.CreatedBy != "ci-bot" {
			t.Errorf("expected created_by=ci-bot, got %v", st.createSpecBundleParams.CreatedBy)
		}
	})
}

// TestDownloadSpecBundleHandler covers the GET /v1/spec-bundles/{id} endpoint.
func TestDownloadSpecBundleHandler(t *testing.T) {
	bundleID := domaintypes.NewSpecBundleID()
	objectKey := "spec-bundles/" + bundleID.String() + ".gz"
	bundleContent := []byte("fake bundle bytes")

	t.Run("DownloadSuccess", func(t *testing.T) {
		st := &mockStore{
			getSpecBundleResult: store.SpecBundle{
				ID:        bundleID,
				ObjectKey: &objectKey,
			},
		}
		bs := bsmock.New()
		if _, err := bs.Put(context.Background(), objectKey, "application/gzip", bundleContent); err != nil {
			t.Fatalf("seed blob store: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/v1/spec-bundles/"+bundleID.String(), nil)
		req.SetPathValue("id", bundleID.String())
		w := httptest.NewRecorder()
		downloadSpecBundleHandler(st, bs)(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if ct := w.Header().Get("Content-Type"); ct != "application/gzip" {
			t.Errorf("Content-Type: got %q, want application/gzip", ct)
		}
		got, err := io.ReadAll(w.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if string(got) != string(bundleContent) {
			t.Errorf("body mismatch: got %q, want %q", got, bundleContent)
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		st := &mockStore{
			getSpecBundleErr: pgx.ErrNoRows,
		}
		bs := bsmock.New()

		req := httptest.NewRequest(http.MethodGet, "/v1/spec-bundles/"+bundleID.String(), nil)
		req.SetPathValue("id", bundleID.String())
		w := httptest.NewRecorder()
		downloadSpecBundleHandler(st, bs)(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d", w.Code)
		}
	})

	t.Run("InvalidID", func(t *testing.T) {
		st := &mockStore{}
		bs := bsmock.New()

		req := httptest.NewRequest(http.MethodGet, "/v1/spec-bundles/not-valid-id", nil)
		req.SetPathValue("id", "not-valid-id")
		w := httptest.NewRecorder()
		downloadSpecBundleHandler(st, bs)(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for invalid ID, got %d", w.Code)
		}
	})

	t.Run("BlobNotFound", func(t *testing.T) {
		// Metadata row exists but blob is absent from object store: expect 404, not 503.
		st := &mockStore{
			getSpecBundleResult: store.SpecBundle{
				ID:        bundleID,
				ObjectKey: &objectKey,
			},
		}
		bs := bsmock.New() // empty: key not seeded

		req := httptest.NewRequest(http.MethodGet, "/v1/spec-bundles/"+bundleID.String(), nil)
		req.SetPathValue("id", bundleID.String())
		w := httptest.NewRecorder()
		downloadSpecBundleHandler(st, bs)(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("expected 404 for missing blob, got %d", w.Code)
		}
	})

	t.Run("MissingObjectKey", func(t *testing.T) {
		st := &mockStore{
			getSpecBundleResult: store.SpecBundle{
				ID:        bundleID,
				ObjectKey: nil, // no object key
			},
		}
		bs := bsmock.New()

		req := httptest.NewRequest(http.MethodGet, "/v1/spec-bundles/"+bundleID.String(), nil)
		req.SetPathValue("id", bundleID.String())
		w := httptest.NewRecorder()
		downloadSpecBundleHandler(st, bs)(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("expected 404 for missing object key, got %d", w.Code)
		}
	})
}
