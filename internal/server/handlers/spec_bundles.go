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
					"bundle_id", existing.ID.String(), "err", refErr)
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
			ID:        bundleID,
			Cid:       cid,
			Digest:    digest,
			Size:      int64(len(data)),
			CreatedBy: createdByPtr,
		}

		bundle, err := bp.CreateSpecBundle(r.Context(), params, data)
		if err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to persist spec bundle: %v", err)
			slog.Error("spec bundle upload: persist failed", "bundle_id", bundleID.String(), "err", err)
			return
		}

		slog.Info("spec bundle uploaded", "bundle_id", bundle.ID.String(), "size", bundle.Size)
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
		BundleID:     bundle.ID.String(),
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

// downloadSpecBundleHandler retrieves a spec bundle blob from object storage by bundle_id
// and streams the raw bytes to the caller. Workers use this to fetch bundles before execution.
//
// Route: GET /v1/spec-bundles/{id}
// Auth:  RoleWorker, RoleControlPlane
func downloadSpecBundleHandler(st store.Store, bs blobstore.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bundleID, err := parseRequiredPathID[domaintypes.SpecBundleID](r, "id")
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
			slog.Error("spec bundle download: metadata lookup failed", "bundle_id", bundleID.String(), "err", err)
			return
		}

		if bundle.ObjectKey == nil || *bundle.ObjectKey == "" {
			writeHTTPError(w, http.StatusNotFound, "spec bundle blob not found")
			slog.Error("spec bundle download: no object_key", "bundle_id", bundleID.String())
			return
		}

		rc, size, err := bs.Get(r.Context(), *bundle.ObjectKey)
		if err != nil {
			if errors.Is(err, blobstore.ErrNotFound) {
				writeHTTPError(w, http.StatusNotFound, "spec bundle blob not found")
				slog.Error("spec bundle download: blob missing from object store",
					"bundle_id", bundleID.String(), "object_key", *bundle.ObjectKey)
				return
			}
			writeHTTPError(w, http.StatusServiceUnavailable, "failed to retrieve spec bundle blob")
			slog.Error("spec bundle download: blob get failed",
				"bundle_id", bundleID.String(), "object_key", *bundle.ObjectKey, "err", err)
			return
		}
		defer rc.Close()

		// Update last_ref_at asynchronously to track GC eligibility.
		go func(bundleID domaintypes.SpecBundleID) {
			ctx, cancel := context.WithTimeout(context.Background(), specBundleLastRefUpdateTimeout)
			defer cancel()
			if refErr := st.UpdateSpecBundleLastRefAt(ctx, bundleID); refErr != nil {
				slog.Warn("spec bundle download: failed to update last_ref_at",
					"bundle_id", bundleID.String(), "err", refErr)
			}
		}(bundle.ID)

		streamBlob(w, rc, size, fmt.Sprintf("%q", "bundle-"+bundleID.String()+".tar.gz"), "application/gzip")
	}
}
