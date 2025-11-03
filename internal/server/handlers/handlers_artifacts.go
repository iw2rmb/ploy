package handlers

import (
	"crypto/sha256"
	"encoding/hex"
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

// computeArtifactCIDAndDigest computes a content identifier and SHA256 digest for an artifact bundle.
// CID uses a simple "bafy-" prefix with hex-encoded SHA256 for compatibility with existing test fixtures.
// Digest is the full SHA256 hex string with "sha256:" prefix.
func computeArtifactCIDAndDigest(bundle []byte) (cid, digest string) {
	hash := sha256.Sum256(bundle)
	hexHash := hex.EncodeToString(hash[:])
	// Use bafy prefix (like IPFS CIDv1) followed by first 32 chars of hash for readability
	cid = "bafy" + hexHash[:32]
	digest = "sha256:" + hexHash
	return cid, digest
}

// createDiffHandler stores gzipped diff in diffs table (≤1 MiB), rejects oversize.
func createDiffHandler(st store.Store) http.HandlerFunc {
	// Accept up to 2 MiB for the JSON body to accommodate base64 overhead
	// while still enforcing a strict 1 MiB cap on the decoded patch bytes.
	const maxBodySize = 2 << 20  // 2 MiB
	const maxPatchSize = 1 << 20 // 1 MiB
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
			RunID   string                 `json:"run_id"`
			Patch   []byte                 `json:"patch"`   // gzipped diff (raw bytes)
			Summary map[string]interface{} `json:"summary"` // optional summary metadata
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

		// Validate patch is present.
		if len(req.Patch) == 0 {
			http.Error(w, "patch is required", http.StatusBadRequest)
			return
		}

		// Enforce decoded patch size cap (≤ 1 MiB gzipped, base64-decoded here).
		if len(req.Patch) > maxPatchSize {
			http.Error(w, "diff size exceeds 1 MiB cap", http.StatusRequestEntityTooLarge)
			return
		}

		// Check if the node exists before processing.
		_, err = st.GetNode(r.Context(), pgtype.UUID{
			Bytes: nodeUUID,
			Valid: true,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "node not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to check node: %v", err), http.StatusInternalServerError)
			slog.Error("diff: node check failed", "node_id", nodeIDStr, "err", err)
			return
		}

		// Check if the run exists.
		_, err = st.GetRun(r.Context(), pgtype.UUID{
			Bytes: runUUID,
			Valid: true,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "run not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to check run: %v", err), http.StatusInternalServerError)
			slog.Error("diff: run check failed", "run_id", req.RunID, "err", err)
			return
		}

		// Check if the stage exists.
		stage, err := st.GetStage(r.Context(), pgtype.UUID{
			Bytes: stageUUID,
			Valid: true,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "stage not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to check stage: %v", err), http.StatusInternalServerError)
			slog.Error("diff: stage check failed", "stage_id", stageIDStr, "err", err)
			return
		}

		// Ensure the stage belongs to the provided run.
		if uuid.UUID(stage.RunID.Bytes) != runUUID {
			http.Error(w, "stage does not belong to run", http.StatusBadRequest)
			return
		}

		// Create diff params.
		summaryBytes, err := json.Marshal(req.Summary)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to marshal summary: %v", err), http.StatusBadRequest)
			return
		}

		params := store.CreateDiffParams{
			RunID: pgtype.UUID{
				Bytes: runUUID,
				Valid: true,
			},
			StageID: pgtype.UUID{
				Bytes: stageUUID,
				Valid: true,
			},
			Patch:   req.Patch,
			Summary: summaryBytes,
		}

		// Persist diff to DB.
		diff, err := st.CreateDiff(r.Context(), params)
		if err != nil {
			// Check if the error is a constraint violation (size cap exceeded).
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23514" { // check_violation
				http.Error(w, "diff size exceeds 1 MiB cap", http.StatusRequestEntityTooLarge)
				return
			}
			http.Error(w, fmt.Sprintf("failed to create diff: %v", err), http.StatusInternalServerError)
			slog.Error("diff: create failed", "node_id", nodeIDStr, "run_id", req.RunID, "stage_id", stageIDStr, "err", err)
			return
		}

		// Return success response with diff_id.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"diff_id": uuid.UUID(diff.ID.Bytes).String(),
		}); err != nil {
			slog.Error("diff: encode response failed", "err", err)
		}

		slog.Debug("diff created",
			"node_id", nodeIDStr,
			"run_id", req.RunID,
			"stage_id", stageIDStr,
			"diff_id", diff.ID.Bytes,
		)
	}
}

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
		_, err = st.GetNode(r.Context(), pgtype.UUID{
			Bytes: nodeUUID,
			Valid: true,
		})
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
		_, err = st.GetRun(r.Context(), pgtype.UUID{
			Bytes: runUUID,
			Valid: true,
		})
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
		stage, err := st.GetStage(r.Context(), pgtype.UUID{
			Bytes: stageUUID,
			Valid: true,
		})
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
			RunID: pgtype.UUID{
				Bytes: runUUID,
				Valid: true,
			},
			StageID: pgtype.UUID{
				Bytes: stageUUID,
				Valid: true,
			},
			BuildID: pgtype.UUID{
				Bytes: buildUUID,
				Valid: req.BuildID != nil && strings.TrimSpace(*req.BuildID) != "",
			},
			Name:   req.Name,
			Bundle: req.Bundle,
			Cid:    &cid,
			Digest: &digest,
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
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"artifact_bundle_id": uuid.UUID(artifact.ID.Bytes).String(),
		}); err != nil {
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
