package handlers

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	bsmock "github.com/iw2rmb/ploy/internal/blobstore/mock"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/store"
)

// TestUploadSpecBundleHandler covers the POST /v1/spec-bundles endpoint.
func TestUploadSpecBundleHandler(t *testing.T) {
	bundleData := []byte("fake gzip bundle content for testing")
	cid, digest := computeCIDAndDigest(bundleData)

	t.Run("UploadSuccess", func(t *testing.T) {
		bundleID := domaintypes.NewSpecBundleID()
		objKey := "spec-bundles/" + bundleID.String() + ".gz"
		st := &configStore{}
		st.getSpecBundleByCID.err = pgx.ErrNoRows // no existing bundle
		st.createSpecBundle.val = store.SpecBundle{
			ID:        string(bundleID),
			Cid:       cid,
			Digest:    digest,
			Size:      int64(len(bundleData)),
			ObjectKey: &objKey,
		}
		bs := bsmock.New()
		bp := blobpersist.New(st, bs)

		req := httptest.NewRequest(http.MethodPost, "/v1/spec-bundles", bytes.NewReader(bundleData))
		req.Header.Set("Content-Type", "application/octet-stream")
		w := httptest.NewRecorder()
		uploadSpecBundleHandler(st, bp)(w, req)
		assertStatus(t, w, http.StatusCreated)

		resp := decodeBody[specBundleUploadResponse](t, w)
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
		st := &configStore{}
		st.getSpecBundleByCID.val = store.SpecBundle{
			ID:     string(existingID),
			Cid:    cid,
			Digest: digest,
			Size:   int64(len(bundleData)),
		}
		st.getSpecBundleByCID.err = nil // found
		bs := bsmock.New()
		bp := blobpersist.New(st, bs)

		req := httptest.NewRequest(http.MethodPost, "/v1/spec-bundles", bytes.NewReader(bundleData))
		w := httptest.NewRecorder()
		uploadSpecBundleHandler(st, bp)(w, req)
		assertStatus(t, w, http.StatusOK)

		resp := decodeBody[specBundleUploadResponse](t, w)
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
		st := &configStore{}
		bs := bsmock.New()
		bp := blobpersist.New(st, bs)

		req := httptest.NewRequest(http.MethodPost, "/v1/spec-bundles", strings.NewReader(""))
		w := httptest.NewRecorder()
		uploadSpecBundleHandler(st, bp)(w, req)
		assertStatus(t, w, http.StatusBadRequest)
	})

	t.Run("ExceedsSizeLimit", func(t *testing.T) {
		st := &configStore{}
		bs := bsmock.New()
		bp := blobpersist.New(st, bs)

		req := httptest.NewRequest(http.MethodPost, "/v1/spec-bundles", bytes.NewReader(bundleData))
		req.ContentLength = maxSpecBundleSize + 1
		w := httptest.NewRecorder()
		uploadSpecBundleHandler(st, bp)(w, req)
		assertStatus(t, w, http.StatusRequestEntityTooLarge)
	})

	t.Run("CreatedByQueryParam", func(t *testing.T) {
		bundleID := domaintypes.NewSpecBundleID()
		objKey := "spec-bundles/" + bundleID.String() + ".gz"
		st := &configStore{}
		st.getSpecBundleByCID.err = pgx.ErrNoRows
		st.createSpecBundle.val = store.SpecBundle{
			ID:        string(bundleID),
			Cid:       cid,
			Digest:    digest,
			Size:      int64(len(bundleData)),
			ObjectKey: &objKey,
		}
		bs := bsmock.New()
		bp := blobpersist.New(st, bs)

		req := httptest.NewRequest(http.MethodPost, "/v1/spec-bundles?created_by=ci-bot", bytes.NewReader(bundleData))
		w := httptest.NewRecorder()
		uploadSpecBundleHandler(st, bp)(w, req)
		assertStatus(t, w, http.StatusCreated)
		if st.createSpecBundle.params.CreatedBy == nil || *st.createSpecBundle.params.CreatedBy != "ci-bot" {
			t.Errorf("expected created_by=ci-bot, got %v", st.createSpecBundle.params.CreatedBy)
		}
	})
}

// TestDownloadSpecBundleHandler covers the GET /v1/spec-bundles/{id} endpoint.
func TestDownloadSpecBundleHandler(t *testing.T) {
	bundleID := domaintypes.NewSpecBundleID()
	objectKey := "spec-bundles/" + bundleID.String() + ".gz"
	bundleContent := []byte("fake bundle bytes")

	t.Run("DownloadSuccess", func(t *testing.T) {
		st := &configStore{}
		st.getSpecBundle.val = store.SpecBundle{
			ID:        string(bundleID),
			ObjectKey: &objectKey,
		}
		bs := bsmock.New()
		if _, err := bs.Put(context.Background(), objectKey, "application/gzip", bundleContent); err != nil {
			t.Fatalf("seed blob store: %v", err)
		}

		w := doRequest(t, http.HandlerFunc(downloadSpecBundleHandler(st, bs)),
			http.MethodGet, "/v1/spec-bundles/"+bundleID.String(), nil, "id", bundleID.String())
		assertStatus(t, w, http.StatusOK)
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
		st := &configStore{}
		st.getSpecBundle.err = pgx.ErrNoRows
		bs := bsmock.New()

		w := doRequest(t, http.HandlerFunc(downloadSpecBundleHandler(st, bs)),
			http.MethodGet, "/v1/spec-bundles/"+bundleID.String(), nil, "id", bundleID.String())
		assertStatus(t, w, http.StatusNotFound)
	})

	t.Run("InvalidID", func(t *testing.T) {
		st := &configStore{}
		st.getSpecBundle.err = pgx.ErrNoRows
		bs := bsmock.New()

		w := doRequest(t, http.HandlerFunc(downloadSpecBundleHandler(st, bs)),
			http.MethodGet, "/v1/spec-bundles/not-valid-id", nil, "id", "not-valid-id")
		assertStatus(t, w, http.StatusNotFound)
	})

	t.Run("BlobNotFound", func(t *testing.T) {
		// Metadata row exists but blob is absent from object store: expect 404, not 503.
		st := &configStore{}
		st.getSpecBundle.val = store.SpecBundle{
			ID:        string(bundleID),
			ObjectKey: &objectKey,
		}
		bs := bsmock.New() // empty: key not seeded

		w := doRequest(t, http.HandlerFunc(downloadSpecBundleHandler(st, bs)),
			http.MethodGet, "/v1/spec-bundles/"+bundleID.String(), nil, "id", bundleID.String())
		assertStatus(t, w, http.StatusNotFound)
	})

	t.Run("MissingObjectKey", func(t *testing.T) {
		st := &configStore{}
		st.getSpecBundle.val = store.SpecBundle{
			ID:        string(bundleID),
			ObjectKey: nil, // no object key
		}
		bs := bsmock.New()

		w := doRequest(t, http.HandlerFunc(downloadSpecBundleHandler(st, bs)),
			http.MethodGet, "/v1/spec-bundles/"+bundleID.String(), nil, "id", bundleID.String())
		assertStatus(t, w, http.StatusNotFound)
	})

}

func TestSpecBundleDownloadLastRefInvokedWhenRequestCanceledImmediatelyAfterResponse(t *testing.T) {
	bundleID := domaintypes.NewSpecBundleID()
	objectKey := "spec-bundles/" + bundleID.String() + ".gz"
	bundleContent := []byte("fake bundle bytes")

	st := &configStore{
		updateSpecBundleLastRefAtStarted: make(chan struct{}),
		updateSpecBundleLastRefAtProceed: make(chan struct{}),
		updateSpecBundleLastRefAtDone:    make(chan struct{}),
	}
	st.getSpecBundle.val = store.SpecBundle{
		ID:        string(bundleID),
		ObjectKey: &objectKey,
	}
	bs := bsmock.New()
	if _, err := bs.Put(context.Background(), objectKey, "application/gzip", bundleContent); err != nil {
		t.Fatalf("seed blob store: %v", err)
	}

	reqBase := httptest.NewRequest(http.MethodGet, "/v1/spec-bundles/"+bundleID.String(), nil)
	reqCtx, cancelReq := context.WithCancel(reqBase.Context())
	req := reqBase.WithContext(reqCtx)
	req.SetPathValue("id", bundleID.String())
	w := httptest.NewRecorder()
	downloadSpecBundleHandler(st, bs)(w, req)
	assertStatus(t, w, http.StatusOK)

	// Regression guard: cancel request context immediately after response completion.
	cancelReq()
	close(st.updateSpecBundleLastRefAtProceed)

	select {
	case <-st.updateSpecBundleLastRefAtStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for last_ref_at update goroutine to start")
	}

	select {
	case <-st.updateSpecBundleLastRefAtDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for last_ref_at update goroutine to finish")
	}

	if !st.updateSpecBundleLastRefAtCalled {
		t.Fatal("expected UpdateSpecBundleLastRefAt to be invoked for bundle download")
	}
	if st.updateSpecBundleLastRefAtParam != bundleID.String() {
		t.Fatalf("expected UpdateSpecBundleLastRefAt bundle_id=%q, got %q", bundleID.String(), st.updateSpecBundleLastRefAtParam)
	}
	if st.updateSpecBundleLastRefAtCtxErr != nil {
		t.Fatalf("expected detached context to remain active after request cancellation, got %v", st.updateSpecBundleLastRefAtCtxErr)
	}
}

func TestProbeIntegrity(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		st := &configStore{}
		bs := bsmock.New()
		bundleID := domaintypes.NewSpecBundleID().String()
		key := "spec-bundles/" + bundleID + ".tar.gz"
		st.getSpecBundle.val = store.SpecBundle{ID: bundleID, ObjectKey: &key}
		if _, err := bs.Put(context.Background(), key, "application/gzip", []byte("x")); err != nil {
			t.Fatalf("seed blob store: %v", err)
		}

		got, err := probeIntegrity(context.Background(), st, bs, bundleID)
		if err != nil {
			t.Fatalf("probeIntegrity() error: %v", err)
		}
		if got.ID != bundleID {
			t.Fatalf("bundle id=%q, want %q", got.ID, bundleID)
		}
	})

	t.Run("MetadataMissing", func(t *testing.T) {
		st := &configStore{}
		st.getSpecBundle.err = pgx.ErrNoRows
		bs := bsmock.New()
		bundleID := "bundle_missing_meta"

		_, err := probeIntegrity(context.Background(), st, bs, bundleID)
		if err == nil {
			t.Fatal("expected metadata missing error")
		}
		var integrityErr *integrityError
		if !errors.As(err, &integrityErr) {
			t.Fatalf("expected integrityError, got %T (%v)", err, err)
		}
		if integrityErr.kind != integrityMetadataMissing {
			t.Fatalf("kind=%q, want %q", integrityErr.kind, integrityMetadataMissing)
		}
		if got := integrityErr.Error(); got != `spec bundle "bundle_missing_meta" metadata is missing` {
			t.Fatalf("message=%q", got)
		}
	})

	t.Run("MissingObjectKey", func(t *testing.T) {
		st := &configStore{}
		st.getSpecBundle.val = store.SpecBundle{ID: "bundle_missing_key"}
		bs := bsmock.New()

		_, err := probeIntegrity(context.Background(), st, bs, "bundle_missing_key")
		if err == nil {
			t.Fatal("expected object key missing error")
		}
		var integrityErr *integrityError
		if !errors.As(err, &integrityErr) {
			t.Fatalf("expected integrityError, got %T (%v)", err, err)
		}
		if integrityErr.kind != integrityObjectKeyMissing {
			t.Fatalf("kind=%q, want %q", integrityErr.kind, integrityObjectKeyMissing)
		}
	})

	t.Run("BlobMissing", func(t *testing.T) {
		st := &configStore{}
		key := "spec-bundles/bundle_missing_blob.tar.gz"
		st.getSpecBundle.val = store.SpecBundle{ID: "bundle_missing_blob", ObjectKey: &key}
		bs := bsmock.New()

		_, err := probeIntegrity(context.Background(), st, bs, "bundle_missing_blob")
		if err == nil {
			t.Fatal("expected blob missing error")
		}
		var integrityErr *integrityError
		if !errors.As(err, &integrityErr) {
			t.Fatalf("expected integrityError, got %T (%v)", err, err)
		}
		if integrityErr.kind != integrityBlobMissing {
			t.Fatalf("kind=%q, want %q", integrityErr.kind, integrityBlobMissing)
		}
		if got := integrityErr.Error(); got != `spec bundle "bundle_missing_blob" blob is missing from object storage` {
			t.Fatalf("message=%q", got)
		}
	})
}
