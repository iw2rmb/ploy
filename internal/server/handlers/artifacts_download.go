package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/blobstore"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// listArtifactsByCIDHandler handles GET /v1/artifacts?cid=... for artifact lookup by CID.
// Returns a list of matching artifacts with metadata (but not the bundle content).
func listArtifactsByCIDHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cid := strings.TrimSpace(r.URL.Query().Get("cid"))
		if cid == "" {
			httpErr(w, http.StatusBadRequest, "cid query parameter is required")
			return
		}

		// Query artifacts by CID (metadata only; excludes bundle bytes).
		bundles, err := st.ListArtifactBundlesMetaByCID(r.Context(), &cid)
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to query artifacts: %v", err)
			return
		}

		// Build response with artifact metadata (exclude bundle bytes).
		type artifactSummary struct {
			ID     string  `json:"id"`
			CID    string  `json:"cid"`
			Digest string  `json:"digest"`
			Name   *string `json:"name,omitempty"`
			Size   int64   `json:"size"`
		}
		artifacts := make([]artifactSummary, 0, len(bundles))
		for _, bundle := range bundles {
			summary := artifactSummary{
				ID:   uuid.UUID(bundle.ID.Bytes).String(),
				Size: bundle.BundleSize,
			}
			if bundle.Cid != nil {
				summary.CID = *bundle.Cid
			}
			if bundle.Digest != nil {
				summary.Digest = *bundle.Digest
			}
			if bundle.Name != nil {
				summary.Name = bundle.Name
			}
			artifacts = append(artifacts, summary)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(map[string]any{"artifacts": artifacts}); err != nil {
			// Log encoding errors but response is already committed.
			return
		}
	}
}

// getArtifactHandler handles GET /v1/artifacts/{id}[?download=true] for artifact retrieval.
// If download=true, returns the raw bundle bytes streamed from object storage with Content-Disposition header.
// Otherwise, returns artifact metadata in JSON.
func getArtifactHandler(st store.Store, bs blobstore.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr, err := requiredPathParam(r, "id")
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		}

		// Parse and validate artifact ID.
		artifactUUID, err := uuid.Parse(idStr)
		if err != nil {
			httpErr(w, http.StatusBadRequest, "invalid id: %v", err)
			return
		}

		// Retrieve artifact bundle from DB.
		bundle, err := st.GetArtifactBundle(r.Context(), pgtype.UUID{Bytes: artifactUUID, Valid: true})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				httpErr(w, http.StatusNotFound, "artifact not found")
				return
			}
			httpErr(w, http.StatusInternalServerError, "failed to retrieve artifact: %v", err)
			return
		}

		// Check if download mode is requested.
		download := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("download"))) == "true"
		if download {
			// Stream from object storage.
			if bundle.ObjectKey == nil || *bundle.ObjectKey == "" {
				httpErr(w, http.StatusNotFound, "artifact blob not found")
				slog.Error("download artifact: no object_key", "artifact_id", idStr)
				return
			}

			rc, size, err := bs.Get(r.Context(), *bundle.ObjectKey)
			if err != nil {
				httpErr(w, http.StatusServiceUnavailable, "failed to retrieve artifact blob")
				slog.Error("download artifact: blob get failed", "artifact_id", idStr, "object_key", *bundle.ObjectKey, "err", err)
				return
			}
			defer rc.Close()

			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.bin", idStr))
			w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
			w.WriteHeader(http.StatusOK)
			if _, err := io.Copy(w, rc); err != nil {
				slog.Error("download artifact: stream failed", "artifact_id", idStr, "err", err)
			}
			return
		}

		// Return artifact metadata as JSON.
		// Note: build_id removed as part of builds table removal; artifacts now use job-level grouping only.
		// Uses domain types (RunID, JobID) for type-safe API output.
		type artifactDetail struct {
			ID        string             `json:"id"`
			RunID     domaintypes.RunID  `json:"run_id"`           // Run ID (KSUID-backed)
			JobID     *domaintypes.JobID `json:"job_id,omitempty"` // Job ID (KSUID-backed, optional)
			Name      *string            `json:"name,omitempty"`
			CID       string             `json:"cid"`
			Digest    string             `json:"digest"`
			Size      int64              `json:"size"`
			CreatedAt string             `json:"created_at"`
		}
		// bundle.ID is still pgtype.UUID; run_id and job_id are now KSUID strings.
		// RunID and JobID are already domain types in the store model.
		detail := artifactDetail{
			ID:    uuid.UUID(bundle.ID.Bytes).String(),
			RunID: bundle.RunID,
			Size:  bundle.BundleSize,
		}
		if bundle.JobID != nil && *bundle.JobID != "" {
			jobID := *bundle.JobID
			detail.JobID = &jobID
		}
		if bundle.Name != nil {
			detail.Name = bundle.Name
		}
		if bundle.Cid != nil {
			detail.CID = *bundle.Cid
		}
		if bundle.Digest != nil {
			detail.Digest = *bundle.Digest
		}
		if bundle.CreatedAt.Valid {
			detail.CreatedAt = bundle.CreatedAt.Time.Format("2006-01-02T15:04:05Z07:00")
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(detail); err != nil {
			// Log encoding errors but response is already committed.
			return
		}
	}
}
