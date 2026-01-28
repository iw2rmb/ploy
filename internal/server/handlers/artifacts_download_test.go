package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	bsmock "github.com/iw2rmb/ploy/internal/blobstore/mock"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// TestListArtifactsByCIDHandler verifies the GET /v1/artifacts?cid=... endpoint.
func TestListArtifactsByCIDHandler(t *testing.T) {
	t.Run("MissingCIDParameter", func(t *testing.T) {
		st := &mockStore{}
		handler := listArtifactsByCIDHandler(st)

		req := httptest.NewRequest(http.MethodGet, "/v1/artifacts", nil)
		w := httptest.NewRecorder()
		handler(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
	})

	t.Run("SuccessWithResults", func(t *testing.T) {
		testCID := "bafy123abc"
		testDigest := "sha256:abcdef"
		testName := "test-artifact"
		testBundleSize := int64(len("test-bundle-data"))
		artifactID := uuid.New()
		runID := domaintypes.NewRunID()

		st := &mockStore{
			listArtifactBundlesMetaByCIDResult: []store.ArtifactBundle{
				{
					ID:         pgtype.UUID{Bytes: artifactID, Valid: true},
					RunID:      runID,
					Cid:        &testCID,
					Digest:     &testDigest,
					Name:       &testName,
					BundleSize: testBundleSize,
				},
			},
		}

		handler := listArtifactsByCIDHandler(st)
		req := httptest.NewRequest(http.MethodGet, "/v1/artifacts?cid="+testCID, nil)
		w := httptest.NewRecorder()
		handler(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}

		var response struct {
			Artifacts []struct {
				ID     string  `json:"id"`
				CID    string  `json:"cid"`
				Digest string  `json:"digest"`
				Name   *string `json:"name"`
				Size   int64   `json:"size"`
			} `json:"artifacts"`
		}
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if len(response.Artifacts) != 1 {
			t.Fatalf("expected 1 artifact, got %d", len(response.Artifacts))
		}
		art := response.Artifacts[0]
		if art.ID != artifactID.String() {
			t.Errorf("expected ID %s, got %s", artifactID.String(), art.ID)
		}
		if art.CID != testCID {
			t.Errorf("expected CID %s, got %s", testCID, art.CID)
		}
		if art.Digest != testDigest {
			t.Errorf("expected digest %s, got %s", testDigest, art.Digest)
		}
		if art.Size != testBundleSize {
			t.Errorf("expected size %d, got %d", testBundleSize, art.Size)
		}
	})

	t.Run("SuccessWithNoResults", func(t *testing.T) {
		st := &mockStore{
			listArtifactBundlesMetaByCIDResult: []store.ArtifactBundle{},
		}
		testCID := "bafy-not-found"

		handler := listArtifactsByCIDHandler(st)
		req := httptest.NewRequest(http.MethodGet, "/v1/artifacts?cid="+testCID, nil)
		w := httptest.NewRecorder()
		handler(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}

		var response struct {
			Artifacts []any `json:"artifacts"`
		}
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if len(response.Artifacts) != 0 {
			t.Errorf("expected 0 artifacts, got %d", len(response.Artifacts))
		}
	})
}

// TestGetArtifactHandler verifies the GET /v1/artifacts/{id} endpoint.
func TestGetArtifactHandler(t *testing.T) {
	t.Run("MissingID", func(t *testing.T) {
		st := &mockStore{}
		bs := bsmock.New()
		handler := getArtifactHandler(st, bs)

		req := httptest.NewRequest(http.MethodGet, "/v1/artifacts/", nil)
		// Simulate missing path value
		w := httptest.NewRecorder()
		handler(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
	})

	t.Run("InvalidID", func(t *testing.T) {
		st := &mockStore{}
		bs := bsmock.New()
		handler := getArtifactHandler(st, bs)

		req := httptest.NewRequest(http.MethodGet, "/v1/artifacts/not-a-uuid", nil)
		req.SetPathValue("id", "not-a-uuid")
		w := httptest.NewRecorder()
		handler(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		artifactID := uuid.New()
		st := &mockStore{
			getArtifactBundleErr: pgx.ErrNoRows,
		}
		bs := bsmock.New()
		handler := getArtifactHandler(st, bs)
		req := httptest.NewRequest(http.MethodGet, "/v1/artifacts/"+artifactID.String(), nil)
		req.SetPathValue("id", artifactID.String())
		w := httptest.NewRecorder()
		handler(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d", w.Code)
		}
	})

	t.Run("SuccessMetadata", func(t *testing.T) {
		artifactID := uuid.New()
		runID := domaintypes.NewRunID()
		testCID := "bafy123xyz"
		testDigest := "sha256:fedcba"
		testName := "metadata-test"
		testBundleSize := int64(15)

		st := &mockStore{
			getArtifactBundleResult: store.ArtifactBundle{
				ID:         pgtype.UUID{Bytes: artifactID, Valid: true},
				RunID:      runID,
				Cid:        &testCID,
				Digest:     &testDigest,
				Name:       &testName,
				BundleSize: testBundleSize,
			},
		}
		bs := bsmock.New()
		handler := getArtifactHandler(st, bs)
		req := httptest.NewRequest(http.MethodGet, "/v1/artifacts/"+artifactID.String(), nil)
		req.SetPathValue("id", artifactID.String())
		w := httptest.NewRecorder()
		handler(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}

		var response struct {
			ID     string            `json:"id"`
			RunID  domaintypes.RunID `json:"run_id"`
			CID    string            `json:"cid"`
			Digest string            `json:"digest"`
			Name   *string           `json:"name"`
			Size   int64             `json:"size"`
		}
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if response.ID != artifactID.String() {
			t.Errorf("expected ID %s, got %s", artifactID.String(), response.ID)
		}
		if response.CID != testCID {
			t.Errorf("expected CID %s, got %s", testCID, response.CID)
		}
		if response.Size != testBundleSize {
			t.Errorf("expected size %d, got %d", testBundleSize, response.Size)
		}
	})

	t.Run("SuccessDownload", func(t *testing.T) {
		artifactID := uuid.New()
		runID := domaintypes.NewRunID()
		testCID := "bafy-download"
		testDigest := "sha256:download"
		testBundle := []byte("download-bundle-data")
		objKey := "artifacts/run/" + runID.String() + "/bundle/" + artifactID.String() + ".tar.gz"

		st := &mockStore{
			getArtifactBundleResult: store.ArtifactBundle{
				ID:         pgtype.UUID{Bytes: artifactID, Valid: true},
				RunID:      runID,
				Cid:        &testCID,
				Digest:     &testDigest,
				BundleSize: int64(len(testBundle)),
				ObjectKey:  &objKey,
			},
		}

		// Pre-populate mock blobstore with the bundle data.
		bs := bsmock.New()
		_, _ = bs.Put(context.TODO(), objKey, "application/gzip", testBundle)

		handler := getArtifactHandler(st, bs)
		req := httptest.NewRequest(http.MethodGet, "/v1/artifacts/"+artifactID.String()+"?download=true", nil)
		req.SetPathValue("id", artifactID.String())
		w := httptest.NewRecorder()
		handler(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}

		// Verify content type and disposition headers.
		if ct := w.Header().Get("Content-Type"); ct != "application/octet-stream" {
			t.Errorf("expected Content-Type application/octet-stream, got %s", ct)
		}
		if cd := w.Header().Get("Content-Disposition"); cd != "attachment; filename="+artifactID.String()+".bin" {
			t.Errorf("unexpected Content-Disposition: %s", cd)
		}

		// Verify bundle bytes.
		body, err := io.ReadAll(w.Body)
		if err != nil {
			t.Fatalf("failed to read response body: %v", err)
		}
		if string(body) != string(testBundle) {
			t.Errorf("expected bundle %q, got %q", testBundle, body)
		}
	})
}
