package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/iw2rmb/ploy/internal/blobstore"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/store"
)

// maxSpecBundleSize is the maximum allowed size for an uploaded spec bundle (50 MiB).
// Spec bundles may contain user files/directories, so a larger cap than artifact bundles is warranted.
const maxSpecBundleSize = 50 << 20 // 50 MiB

const specBundleLastRefUpdateTimeout = 5 * time.Second

type specBundleIntegrityErrorKind string

const (
	specBundleIntegrityMetadataMissing  specBundleIntegrityErrorKind = "metadata_missing"
	specBundleIntegrityObjectKeyMissing specBundleIntegrityErrorKind = "object_key_missing"
	specBundleIntegrityBlobMissing      specBundleIntegrityErrorKind = "blob_missing"
)

type specBundleIntegrityError struct {
	kind     specBundleIntegrityErrorKind
	bundleID string
	err      error
}

func (e *specBundleIntegrityError) Error() string {
	if e == nil {
		return ""
	}
	switch e.kind {
	case specBundleIntegrityMetadataMissing:
		return fmt.Sprintf("spec bundle %q metadata is missing", e.bundleID)
	case specBundleIntegrityObjectKeyMissing:
		return fmt.Sprintf("spec bundle %q metadata has no object key", e.bundleID)
	case specBundleIntegrityBlobMissing:
		return fmt.Sprintf("spec bundle %q blob is missing from object storage", e.bundleID)
	default:
		if e.err != nil {
			return e.err.Error()
		}
		return fmt.Sprintf("spec bundle %q integrity check failed", e.bundleID)
	}
}

func (e *specBundleIntegrityError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func probeSpecBundleIntegrity(ctx context.Context, st store.Store, bs blobstore.Store, bundleID string) (store.SpecBundle, error) {
	id := strings.TrimSpace(bundleID)
	if id == "" {
		return store.SpecBundle{}, fmt.Errorf("spec bundle id is required")
	}
	if bs == nil {
		return store.SpecBundle{}, fmt.Errorf("blob store is required to verify spec bundle %q", id)
	}

	bundle, err := st.GetSpecBundle(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return store.SpecBundle{}, &specBundleIntegrityError{
				kind:     specBundleIntegrityMetadataMissing,
				bundleID: id,
				err:      err,
			}
		}
		return store.SpecBundle{}, fmt.Errorf("get spec bundle %q: %w", id, err)
	}
	if bundle.ObjectKey == nil || strings.TrimSpace(*bundle.ObjectKey) == "" {
		return store.SpecBundle{}, &specBundleIntegrityError{
			kind:     specBundleIntegrityObjectKeyMissing,
			bundleID: id,
		}
	}

	key := strings.TrimSpace(*bundle.ObjectKey)
	reader, _, err := bs.Get(ctx, key)
	if err != nil {
		if errors.Is(err, blobstore.ErrNotFound) {
			return store.SpecBundle{}, &specBundleIntegrityError{
				kind:     specBundleIntegrityBlobMissing,
				bundleID: id,
				err:      err,
			}
		}
		return store.SpecBundle{}, fmt.Errorf("download spec bundle blob %q: %w", key, err)
	}
	_ = reader.Close()
	return bundle, nil
}

// computeSpecBundleCIDAndDigest computes a content identifier and SHA256 digest for a spec bundle.
// Uses the same scheme as artifact bundles for consistency.
func computeSpecBundleCIDAndDigest(data []byte) (cid, digest string) {
	hash := sha256.Sum256(data)
	hexHash := hex.EncodeToString(hash[:])
	cid = "bafy" + hexHash[:32]
	digest = "sha256:" + hexHash
	return cid, digest
}

// uploadSpecBundleHandler accepts a raw gzip-compressed spec bundle from the CLI,
// persists metadata to spec_bundles and the blob to object storage, and returns
// the assigned bundle_id. Deduplication is performed by CID: if a bundle with the
// same content already exists its bundle_id is returned without re-uploading.
//
// Route: POST /v1/spec-bundles
// Auth:  RoleControlPlane
// Body:  raw binary (application/octet-stream), ≤ 50 MiB
// Query: created_by (optional)
func uploadSpecBundleHandler(st store.Store, bp *blobpersist.Service) http.HandlerFunc {
	if bp == nil {
		panic("uploadSpecBundleHandler: blobpersist is required")
	}
	return func(w http.ResponseWriter, r *http.Request) {
		// Reject oversized requests before reading.
		if r.ContentLength > maxSpecBundleSize {
			writeHTTPError(w, http.StatusRequestEntityTooLarge, "spec bundle exceeds size cap of 50 MiB")
			return
		}

		limited := io.LimitReader(r.Body, maxSpecBundleSize+1)
		data, err := io.ReadAll(limited)
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "failed to read request body: %v", err)
			return
		}
		if len(data) == 0 {
			writeHTTPError(w, http.StatusBadRequest, "request body is required")
			return
		}
		if int64(len(data)) > maxSpecBundleSize {
			writeHTTPError(w, http.StatusRequestEntityTooLarge, "spec bundle exceeds size cap of 50 MiB")
			return
		}

		createdBy := strings.TrimSpace(r.URL.Query().Get("created_by"))

		cid, digest := computeSpecBundleCIDAndDigest(data)

		// Deduplication: if a bundle with this CID already exists, reuse it.
		existing, err := st.GetSpecBundleByCID(r.Context(), cid)
		if err == nil {
			// Update last_ref_at to keep GC metadata fresh.
			if refErr := st.UpdateSpecBundleLastRefAt(r.Context(), existing.ID); refErr != nil {
				slog.Warn("spec bundle upload: failed to update last_ref_at on deduplicated bundle",
					"bundle_id", existing.ID, "err", refErr)
			}
			writeSpecBundleUploadResponse(w, existing, true)
			return
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			writeHTTPError(w, http.StatusInternalServerError, "failed to check for existing bundle: %v", err)
			slog.Error("spec bundle upload: cid lookup failed", "cid", cid, "err", err)
			return
		}

		bundleID := domaintypes.NewSpecBundleID()
		var createdByPtr *string
		if createdBy != "" {
			createdByPtr = &createdBy
		}
		params := store.CreateSpecBundleParams{
			ID:        string(bundleID),
			Cid:       cid,
			Digest:    digest,
			Size:      int64(len(data)),
			CreatedBy: createdByPtr,
		}

		bundle, err := bp.CreateSpecBundle(r.Context(), params, data)
		if err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to persist spec bundle: %v", err)
			slog.Error("spec bundle upload: persist failed", "bundle_id", bundleID, "err", err)
			return
		}

		slog.Info("spec bundle uploaded", "bundle_id", bundle.ID, "size", bundle.Size)
		writeSpecBundleUploadResponse(w, bundle, false)
	}
}

type specBundleUploadResponse struct {
	BundleID     string `json:"bundle_id"`
	CID          string `json:"cid"`
	Digest       string `json:"digest"`
	Size         int64  `json:"size"`
	Deduplicated bool   `json:"deduplicated"`
}

func writeSpecBundleUploadResponse(w http.ResponseWriter, bundle store.SpecBundle, deduplicated bool) {
	resp := specBundleUploadResponse{
		BundleID:     bundle.ID,
		CID:          bundle.Cid,
		Digest:       bundle.Digest,
		Size:         bundle.Size,
		Deduplicated: deduplicated,
	}
	w.Header().Set("Content-Type", "application/json")
	if deduplicated {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusCreated)
	}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.Error("spec bundle upload: encode response failed", "err", err)
	}
}

// probeSpecBundleHandler checks whether a spec bundle with the given CID already exists.
// CLI callers use this to skip uploading content that the server already has.
//
// Route: HEAD /v1/spec-bundles?cid={cid}
// Auth:  RoleControlPlane
// Returns 200 if a bundle with the CID exists, 404 otherwise.
func probeSpecBundleHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cid := strings.TrimSpace(r.URL.Query().Get("cid"))
		if cid == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		bundle, err := st.GetSpecBundleByCID(r.Context(), cid)
		if err == nil {
			w.Header().Set("X-Bundle-ID", bundle.ID)
			w.WriteHeader(http.StatusOK)
			return
		}
		if errors.Is(err, pgx.ErrNoRows) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		slog.Error("spec bundle probe: cid lookup failed", "cid", cid, "err", err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

// downloadSpecBundleHandler retrieves a spec bundle blob from object storage by bundle_id
// and streams the raw bytes to the caller. Workers use this to fetch bundles before execution.
//
// Route: GET /v1/spec-bundles/{id}
// Auth:  RoleWorker, RoleControlPlane
func downloadSpecBundleHandler(st store.Store, bs blobstore.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bundleID, err := requiredPathParam(r, "id")
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}

		bundle, err := st.GetSpecBundle(r.Context(), bundleID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeHTTPError(w, http.StatusNotFound, "spec bundle not found")
				return
			}
			writeHTTPError(w, http.StatusInternalServerError, "failed to retrieve spec bundle: %v", err)
			slog.Error("spec bundle download: metadata lookup failed", "bundle_id", bundleID, "err", err)
			return
		}

		rc, size, ok := openBlobForHTTP(w, r, bs, bundle.ObjectKey, "spec bundle", "bundle_id", bundleID)
		if !ok {
			return
		}
		defer rc.Close()

		// Update last_ref_at asynchronously to track GC eligibility.
		go func(id string) {
			ctx, cancel := context.WithTimeout(context.Background(), specBundleLastRefUpdateTimeout)
			defer cancel()
			if refErr := st.UpdateSpecBundleLastRefAt(ctx, id); refErr != nil {
				slog.Warn("spec bundle download: failed to update last_ref_at",
					"bundle_id", id, "err", refErr)
			}
		}(bundle.ID)

		streamBlob(w, rc, size, fmt.Sprintf("%q", "bundle-"+bundleID+".tar.gz"), "application/gzip")
	}
}
