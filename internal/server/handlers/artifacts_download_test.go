package handlers

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	bsmock "github.com/iw2rmb/ploy/internal/blobstore/mock"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestListArtifactsByCIDHandler(t *testing.T) {
	t.Parallel()

	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		testCID := "bafy123abc"
		testDigest := "sha256:abcdef"
		testName := "test-artifact"
		testBundleSize := int64(len("test-bundle-data"))
		artifactID := uuid.New()

		st := &artifactStore{}
		st.listArtifactBundlesByCID.val = []store.ArtifactBundle{
			{
				ID:         pgtype.UUID{Bytes: artifactID, Valid: true},
				RunID:      domaintypes.NewRunID(),
				Cid:        &testCID,
				Digest:     &testDigest,
				Name:       &testName,
				BundleSize: testBundleSize,
			},
			}

		rr := doRequest(t, listArtifactsByCIDHandler(st), http.MethodGet, "/v1/artifacts?cid="+testCID, nil)
		assertStatus(t, rr, http.StatusOK)

		type listResp struct {
			Artifacts []artifactSummary `json:"artifacts"`
		}
		resp := decodeBody[listResp](t, rr)
		if len(resp.Artifacts) != 1 {
			t.Fatalf("expected 1 artifact, got %d", len(resp.Artifacts))
		}
		art := resp.Artifacts[0]
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

	t.Run("Errors", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name   string
			query  string
			st     *artifactStore
			status int
		}{
			{
				name:   "MissingCID",
				query:  "",
				st:     &artifactStore{},
				status: http.StatusBadRequest,
			},
			{
				name:   "DBError",
				query:  "?cid=bafyerr",
				st:     func() *artifactStore { st := &artifactStore{}; st.listArtifactBundlesByCID.err = errors.New("boom"); return st }(),
				status: http.StatusInternalServerError,
			},
			{
				name:   "NoResults",
				query:  "?cid=bafy-not-found",
				st:     func() *artifactStore { st := &artifactStore{}; st.listArtifactBundlesByCID.val = []store.ArtifactBundle{}; return st }(),
				status: http.StatusOK,
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				rr := doRequest(t, listArtifactsByCIDHandler(tc.st), http.MethodGet, "/v1/artifacts"+tc.query, nil)
				assertStatus(t, rr, tc.status)
			})
		}
	})
}

func TestGetArtifactHandler(t *testing.T) {
	t.Parallel()

	t.Run("SuccessMetadata", func(t *testing.T) {
		t.Parallel()
		artifactID := uuid.New()
		runID := domaintypes.NewRunID()
		testCID := "bafy123xyz"
		testDigest := "sha256:fedcba"
		testName := "metadata-test"
		testBundleSize := int64(15)

		st := &artifactStore{}
		st.getArtifactBundle.val = store.ArtifactBundle{
			ID:         pgtype.UUID{Bytes: artifactID, Valid: true},
			RunID:      runID,
			Cid:        &testCID,
			Digest:     &testDigest,
			Name:       &testName,
			BundleSize: testBundleSize,
			}
		bs := bsmock.New()
		rr := doRequest(t, getArtifactHandler(st, bs), http.MethodGet,
			"/v1/artifacts/"+artifactID.String(), nil,
			"id", artifactID.String())
		assertStatus(t, rr, http.StatusOK)

		type detailResp struct {
			ID     string            `json:"id"`
			RunID  domaintypes.RunID `json:"run_id"`
			CID    string            `json:"cid"`
			Digest string            `json:"digest"`
			Name   *string           `json:"name"`
			Size   int64             `json:"size"`
		}
		resp := decodeBody[detailResp](t, rr)
		if resp.ID != artifactID.String() {
			t.Errorf("expected ID %s, got %s", artifactID.String(), resp.ID)
		}
		if resp.CID != testCID {
			t.Errorf("expected CID %s, got %s", testCID, resp.CID)
		}
		if resp.Size != testBundleSize {
			t.Errorf("expected size %d, got %d", testBundleSize, resp.Size)
		}
	})

	t.Run("SuccessDownload", func(t *testing.T) {
		t.Parallel()
		artifactID := uuid.New()
		runID := domaintypes.NewRunID()
		testCID := "bafy-download"
		testDigest := "sha256:download"
		testBundle := []byte("download-bundle-data")
		objKey := "artifacts/run/" + runID.String() + "/bundle/" + artifactID.String() + ".tar.gz"

		st := &artifactStore{}
		st.getArtifactBundle.val = store.ArtifactBundle{
			ID:         pgtype.UUID{Bytes: artifactID, Valid: true},
			RunID:      runID,
			Cid:        &testCID,
			Digest:     &testDigest,
			BundleSize: int64(len(testBundle)),
			ObjectKey:  &objKey,
			}
		bs := bsmock.New()
		_, _ = bs.Put(context.TODO(), objKey, "application/gzip", testBundle)

		rr := doRequest(t, getArtifactHandler(st, bs), http.MethodGet,
			"/v1/artifacts/"+artifactID.String()+"?download=true", nil,
			"id", artifactID.String())
		assertStatus(t, rr, http.StatusOK)

		if ct := rr.Header().Get("Content-Type"); ct != "application/octet-stream" {
			t.Errorf("expected Content-Type application/octet-stream, got %s", ct)
		}
		if cd := rr.Header().Get("Content-Disposition"); cd != "attachment; filename="+artifactID.String()+".bin" {
			t.Errorf("unexpected Content-Disposition: %s", cd)
		}
		body, err := io.ReadAll(rr.Body)
		if err != nil {
			t.Fatalf("failed to read response body: %v", err)
		}
		if string(body) != string(testBundle) {
			t.Errorf("expected bundle %q, got %q", testBundle, body)
		}
	})

	t.Run("Errors", func(t *testing.T) {
		t.Parallel()
		artifactID := uuid.New()
		cases := []struct {
			name   string
			id     string
			st     *artifactStore
			status int
		}{
			{
				name:   "MissingID",
				id:     "",
				st:     &artifactStore{},
				status: http.StatusBadRequest,
			},
			{
				name:   "InvalidID",
				id:     "not-a-uuid",
				st:     &artifactStore{},
				status: http.StatusBadRequest,
			},
			{
				name:   "NotFound",
				id:     artifactID.String(),
				st:     func() *artifactStore { st := &artifactStore{}; st.getArtifactBundle.err = pgx.ErrNoRows; return st }(),
				status: http.StatusNotFound,
			},
			{
				name:   "DBError",
				id:     artifactID.String(),
				st:     func() *artifactStore { st := &artifactStore{}; st.getArtifactBundle.err = errors.New("db down"); return st }(),
				status: http.StatusInternalServerError,
			},
			{
				name: "MetadataNoCreatedAt",
				id:   artifactID.String(),
				st: func() *artifactStore {
					st := &artifactStore{}
					st.getArtifactBundle.val = store.ArtifactBundle{
						ID:         pgtype.UUID{Bytes: artifactID, Valid: true},
						RunID:      domaintypes.NewRunID(),
						BundleSize: 1,
					}
					return st
				}(),
				status: http.StatusOK,
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				bs := bsmock.New()
				pathParams := []string{}
				if tc.id != "" {
					pathParams = []string{"id", tc.id}
				}
				rr := doRequest(t, getArtifactHandler(tc.st, bs), http.MethodGet,
					"/v1/artifacts/"+tc.id, nil, pathParams...)
				assertStatus(t, rr, tc.status)
			})
		}
	})
}
