package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/store"
)

// createArtifactBundleHandler stores gzipped artifact bundle in artifact_bundles table (≤1 MiB), rejects oversize.
func createArtifactBundleHandler(st store.Store) http.HandlerFunc {
	// Accept up to 2 MiB for the JSON body to accommodate base64 overhead
	// while still enforcing a strict 1 MiB cap on the decoded bundle bytes.
	const maxBodySize = 2 << 20   // 2 MiB
	const maxBundleSize = 1 << 20 // 1 MiB
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract node id from path parameter.
		nodeIDStr := r.PathValue("id")
		if strings.TrimSpace(nodeIDStr) == "" {
			http.Error(w, "id path parameter is required", http.StatusBadRequest)
			return
		}

		// Parse and validate node_id.
		nodeUUID, err := uuid.Parse(nodeIDStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid id: %v", err), http.StatusBadRequest)
			return
		}

		// Extract stage id from path parameter.
		stageIDStr := r.PathValue("stage")
		if strings.TrimSpace(stageIDStr) == "" {
			http.Error(w, "stage path parameter is required", http.StatusBadRequest)
			return
		}

		// Parse and validate stage_id.
		stageUUID, err := uuid.Parse(stageIDStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid stage: %v", err), http.StatusBadRequest)
			return
		}

		// Check payload size before reading body.
		if r.ContentLength > maxBodySize {
			http.Error(w, "payload exceeds body size cap", http.StatusRequestEntityTooLarge)
			return
		}

		// Limit request body size to avoid memory exhaustion but allow base64 overhead.
		r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)

		// Decode request body.
		var req struct {
			RunID   string  `json:"run_id"`
			BuildID *string `json:"build_id"` // optional
			Name    *string `json:"name"`     // optional logical name
			Bundle  []byte  `json:"bundle"`   // gzipped tar (raw bytes)
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			// Return 413 when MaxBytesReader trips the size cap.
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				http.Error(w, "payload exceeds body size cap", http.StatusRequestEntityTooLarge)
				return
			}
			http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
			return
		}

		// Validate run_id is present.
		if strings.TrimSpace(req.RunID) == "" {
			http.Error(w, "run_id is required", http.StatusBadRequest)
			return
		}

		// Validate run_id is a valid UUID.
		runUUID, err := uuid.Parse(req.RunID)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid run_id: %v", err), http.StatusBadRequest)
			return
		}

		// Validate build_id if provided.
		var buildUUID uuid.UUID
		if req.BuildID != nil && strings.TrimSpace(*req.BuildID) != "" {
			buildUUID, err = uuid.Parse(*req.BuildID)
			if err != nil {
				http.Error(w, fmt.Sprintf("invalid build_id: %v", err), http.StatusBadRequest)
				return
			}
		}

		// Validate bundle is present.
		if len(req.Bundle) == 0 {
			http.Error(w, "bundle is required", http.StatusBadRequest)
			return
		}

		// Enforce decoded bundle size cap (≤ 1 MiB gzipped, base64-decoded here).
		if len(req.Bundle) > maxBundleSize {
			http.Error(w, "artifact bundle size exceeds 1 MiB cap", http.StatusRequestEntityTooLarge)
			return
		}

		// Check if the node exists before processing.
		_, err = st.GetNode(r.Context(), pgtype.UUID{Bytes: nodeUUID, Valid: true})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "node not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to check node: %v", err), http.StatusInternalServerError)
			slog.Error("artifact: node check failed", "node_id", nodeIDStr, "err", err)
			return
		}

		// Check if the run exists.
		_, err = st.GetRun(r.Context(), pgtype.UUID{Bytes: runUUID, Valid: true})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "run not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to check run: %v", err), http.StatusInternalServerError)
			slog.Error("artifact: run check failed", "run_id", req.RunID, "err", err)
			return
		}

		// Check if the stage exists.
		stage, err := st.GetStage(r.Context(), pgtype.UUID{Bytes: stageUUID, Valid: true})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "stage not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to check stage: %v", err), http.StatusInternalServerError)
			slog.Error("artifact: stage check failed", "stage_id", stageIDStr, "err", err)
			return
		}

		// Ensure the stage belongs to the provided run.
		if uuid.UUID(stage.RunID.Bytes) != runUUID {
			http.Error(w, "stage does not belong to run", http.StatusBadRequest)
			return
		}

		// Compute CID and digest for content-addressable storage.
		cid, digest := computeArtifactCIDAndDigest(req.Bundle)

		// Create artifact bundle params.
		params := store.CreateArtifactBundleParams{
			RunID:   pgtype.UUID{Bytes: runUUID, Valid: true},
			StageID: pgtype.UUID{Bytes: stageUUID, Valid: true},
			BuildID: pgtype.UUID{Bytes: buildUUID, Valid: req.BuildID != nil && strings.TrimSpace(*req.BuildID) != ""},
			Name:    req.Name,
			Bundle:  req.Bundle,
			Cid:     &cid,
			Digest:  &digest,
		}

		// Persist artifact bundle to DB.
		artifact, err := st.CreateArtifactBundle(r.Context(), params)
		if err != nil {
			// Check if the error is a constraint violation (size cap exceeded).
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23514" { // check_violation
				http.Error(w, "artifact bundle size exceeds 1 MiB cap", http.StatusRequestEntityTooLarge)
				return
			}
			http.Error(w, fmt.Sprintf("failed to create artifact bundle: %v", err), http.StatusInternalServerError)
			slog.Error("artifact: create failed", "node_id", nodeIDStr, "run_id", req.RunID, "stage_id", stageIDStr, "err", err)
			return
		}

		// Return success response with artifact_bundle_id.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(map[string]interface{}{"artifact_bundle_id": uuid.UUID(artifact.ID.Bytes).String()}); err != nil {
			slog.Error("artifact: encode response failed", "err", err)
		}

		slog.Debug("artifact bundle created",
			"node_id", nodeIDStr,
			"run_id", req.RunID,
			"stage_id", stageIDStr,
			"artifact_bundle_id", artifact.ID.Bytes,
		)
	}
}
