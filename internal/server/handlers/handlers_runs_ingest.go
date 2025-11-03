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
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/store"
)

// createRunLogHandler handles POST /v1/runs/{id}/logs for receiving gzipped log chunks.
// This variant does not require a node path parameter; it ingests logs scoped to a run.
func createRunLogHandler(st store.Store) http.HandlerFunc {
	// Accept up to 2 MiB for the JSON body to accommodate base64 overhead
	// while still enforcing a strict 1 MiB cap on the decoded gzipped bytes.
	const maxBodySize = 2 << 20  // 2 MiB
	const maxChunkSize = 1 << 20 // 1 MiB
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract run id from path parameter.
		runIDStr := r.PathValue("id")
		if strings.TrimSpace(runIDStr) == "" {
			http.Error(w, "id path parameter is required", http.StatusBadRequest)
			return
		}
		runUUID, err := uuid.Parse(runIDStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid id: %v", err), http.StatusBadRequest)
			return
		}

		// Check payload size before reading body.
		if r.ContentLength > maxBodySize {
			http.Error(w, "payload exceeds body size cap", http.StatusRequestEntityTooLarge)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)

		// Decode request body.
		var req struct {
			StageID *string `json:"stage_id,omitempty"`
			BuildID *string `json:"build_id,omitempty"`
			ChunkNo int32   `json:"chunk_no"`
			Data    []byte  `json:"data"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				http.Error(w, "payload exceeds body size cap", http.StatusRequestEntityTooLarge)
				return
			}
			http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
			return
		}

		if len(req.Data) == 0 {
			http.Error(w, "data is required and must not be empty", http.StatusBadRequest)
			return
		}
		if len(req.Data) > maxChunkSize {
			http.Error(w, fmt.Sprintf("data exceeds 1 MiB: %d bytes", len(req.Data)), http.StatusRequestEntityTooLarge)
			return
		}

		// Parse optional stage/build IDs.
		var stageID pgtype.UUID
		if req.StageID != nil && strings.TrimSpace(*req.StageID) != "" {
			stageUUID, err := uuid.Parse(*req.StageID)
			if err != nil {
				http.Error(w, fmt.Sprintf("invalid stage_id: %v", err), http.StatusBadRequest)
				return
			}
			stageID = pgtype.UUID{Bytes: stageUUID, Valid: true}
		}
		var buildID pgtype.UUID
		if req.BuildID != nil && strings.TrimSpace(*req.BuildID) != "" {
			buildUUID, err := uuid.Parse(*req.BuildID)
			if err != nil {
				http.Error(w, fmt.Sprintf("invalid build_id: %v", err), http.StatusBadRequest)
				return
			}
			buildID = pgtype.UUID{Bytes: buildUUID, Valid: true}
		}

		// Create log row.
		params := store.CreateLogParams{
			RunID:   pgtype.UUID{Bytes: runUUID, Valid: true},
			StageID: stageID,
			BuildID: buildID,
			ChunkNo: req.ChunkNo,
			Data:    req.Data,
		}
		logRow, err := st.CreateLog(r.Context(), params)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to create log: %v", err), http.StatusInternalServerError)
			slog.Error("run logs: create failed", "run_id", runIDStr, "chunk_no", req.ChunkNo, "err", err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(map[string]any{"id": logRow.ID, "chunk_no": logRow.ChunkNo}); err != nil {
			slog.Error("run logs: encode response failed", "err", err)
		}
	}
}

// createRunDiffHandler stores a gzipped diff for a run using body-provided stage_id.
func createRunDiffHandler(st store.Store) http.HandlerFunc {
	// Accept up to 2 MiB for the JSON body to accommodate base64 overhead
	// while still enforcing a strict 1 MiB cap on the decoded patch bytes.
	const maxBodySize = 2 << 20  // 2 MiB
	const maxPatchSize = 1 << 20 // 1 MiB
	return func(w http.ResponseWriter, r *http.Request) {
		runIDStr := r.PathValue("id")
		if strings.TrimSpace(runIDStr) == "" {
			http.Error(w, "id path parameter is required", http.StatusBadRequest)
			return
		}
		runUUID, err := uuid.Parse(runIDStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid id: %v", err), http.StatusBadRequest)
			return
		}

		if r.ContentLength > maxBodySize {
			http.Error(w, "payload exceeds body size cap", http.StatusRequestEntityTooLarge)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)

		var req struct {
			StageID *string                `json:"stage_id,omitempty"`
			Patch   []byte                 `json:"patch"`
			Summary map[string]interface{} `json:"summary"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				http.Error(w, "payload exceeds body size cap", http.StatusRequestEntityTooLarge)
				return
			}
			http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
			return
		}
		if len(req.Patch) == 0 {
			http.Error(w, "patch is required", http.StatusBadRequest)
			return
		}
		if len(req.Patch) > maxPatchSize {
			http.Error(w, "diff size exceeds 1 MiB cap", http.StatusRequestEntityTooLarge)
			return
		}

		// If stage_id provided, validate it belongs to the run.
		var stageID pgtype.UUID
		if req.StageID != nil && strings.TrimSpace(*req.StageID) != "" {
			stageUUID, err := uuid.Parse(*req.StageID)
			if err != nil {
				http.Error(w, fmt.Sprintf("invalid stage_id: %v", err), http.StatusBadRequest)
				return
			}
			stage, err := st.GetStage(r.Context(), pgtype.UUID{Bytes: stageUUID, Valid: true})
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					http.Error(w, "stage not found", http.StatusNotFound)
					return
				}
				http.Error(w, fmt.Sprintf("failed to check stage: %v", err), http.StatusInternalServerError)
				slog.Error("run diff: stage check failed", "stage_id", *req.StageID, "err", err)
				return
			}
			if uuid.UUID(stage.RunID.Bytes) != runUUID {
				http.Error(w, "stage does not belong to run", http.StatusBadRequest)
				return
			}
			stageID = stage.ID
		}

		// Ensure the run exists.
		if _, err := st.GetRun(r.Context(), pgtype.UUID{Bytes: runUUID, Valid: true}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "run not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to check run: %v", err), http.StatusInternalServerError)
			slog.Error("run diff: run check failed", "run_id", runIDStr, "err", err)
			return
		}

		summaryBytes, err := json.Marshal(req.Summary)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to marshal summary: %v", err), http.StatusBadRequest)
			return
		}
		params := store.CreateDiffParams{
			RunID:   pgtype.UUID{Bytes: runUUID, Valid: true},
			StageID: stageID,
			Patch:   req.Patch,
			Summary: summaryBytes,
		}
		diff, err := st.CreateDiff(r.Context(), params)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to create diff: %v", err), http.StatusInternalServerError)
			slog.Error("run diff: create failed", "run_id", runIDStr, "err", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(map[string]any{"diff_id": uuid.UUID(diff.ID.Bytes).String()}); err != nil {
			slog.Error("run diff: encode response failed", "err", err)
		}
	}
}

// createRunArtifactBundleHandler stores a gzipped artifact bundle for a run using body-provided stage_id.
func createRunArtifactBundleHandler(st store.Store) http.HandlerFunc {
	// Accept up to 2 MiB for the JSON body to accommodate base64 overhead
	// while still enforcing a strict 1 MiB cap on the decoded bundle bytes.
	const maxBodySize = 2 << 20   // 2 MiB
	const maxBundleSize = 1 << 20 // 1 MiB
	return func(w http.ResponseWriter, r *http.Request) {
		runIDStr := r.PathValue("id")
		if strings.TrimSpace(runIDStr) == "" {
			http.Error(w, "id path parameter is required", http.StatusBadRequest)
			return
		}
		runUUID, err := uuid.Parse(runIDStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid id: %v", err), http.StatusBadRequest)
			return
		}

		if r.ContentLength > maxBodySize {
			http.Error(w, "payload exceeds body size cap", http.StatusRequestEntityTooLarge)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)

		var req struct {
			StageID *string `json:"stage_id,omitempty"`
			BuildID *string `json:"build_id,omitempty"`
			Name    *string `json:"name"`
			Bundle  []byte  `json:"bundle"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				http.Error(w, "payload exceeds body size cap", http.StatusRequestEntityTooLarge)
				return
			}
			http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
			return
		}
		if len(req.Bundle) == 0 {
			http.Error(w, "bundle is required", http.StatusBadRequest)
			return
		}
		if len(req.Bundle) > maxBundleSize {
			http.Error(w, "artifact bundle size exceeds 1 MiB cap", http.StatusRequestEntityTooLarge)
			return
		}

		// Validate stage belongs to run if provided.
		var stageID pgtype.UUID
		if req.StageID != nil && strings.TrimSpace(*req.StageID) != "" {
			stageUUID, err := uuid.Parse(*req.StageID)
			if err != nil {
				http.Error(w, fmt.Sprintf("invalid stage_id: %v", err), http.StatusBadRequest)
				return
			}
			stage, err := st.GetStage(r.Context(), pgtype.UUID{Bytes: stageUUID, Valid: true})
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					http.Error(w, "stage not found", http.StatusNotFound)
					return
				}
				http.Error(w, fmt.Sprintf("failed to check stage: %v", err), http.StatusInternalServerError)
				slog.Error("run artifact: stage check failed", "stage_id", *req.StageID, "err", err)
				return
			}
			if uuid.UUID(stage.RunID.Bytes) != runUUID {
				http.Error(w, "stage does not belong to run", http.StatusBadRequest)
				return
			}
			stageID = stage.ID
		}

		// Ensure the run exists.
		if _, err := st.GetRun(r.Context(), pgtype.UUID{Bytes: runUUID, Valid: true}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "run not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to check run: %v", err), http.StatusInternalServerError)
			slog.Error("run artifact: run check failed", "run_id", runIDStr, "err", err)
			return
		}

		// Build optional build_id
		var buildID pgtype.UUID
		if req.BuildID != nil && strings.TrimSpace(*req.BuildID) != "" {
			buildUUID, err := uuid.Parse(*req.BuildID)
			if err != nil {
				http.Error(w, fmt.Sprintf("invalid build_id: %v", err), http.StatusBadRequest)
				return
			}
			buildID = pgtype.UUID{Bytes: buildUUID, Valid: true}
		}

		// Compute CID and digest for content-addressable storage.
		cid, digest := computeArtifactCIDAndDigest(req.Bundle)
		params := store.CreateArtifactBundleParams{
			RunID:   pgtype.UUID{Bytes: runUUID, Valid: true},
			StageID: stageID,
			BuildID: buildID,
			Name:    req.Name,
			Bundle:  req.Bundle,
			Cid:     &cid,
			Digest:  &digest,
		}
		artifact, err := st.CreateArtifactBundle(r.Context(), params)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to create artifact bundle: %v", err), http.StatusInternalServerError)
			slog.Error("run artifact: create failed", "run_id", runIDStr, "err", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(map[string]any{"artifact_bundle_id": uuid.UUID(artifact.ID.Bytes).String()}); err != nil {
			slog.Error("run artifact: encode response failed", "err", err)
		}
	}
}
