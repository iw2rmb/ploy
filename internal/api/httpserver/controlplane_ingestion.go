package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/store"
)

const maxIngestionPayloadSize = 1 << 20 // 1 MiB

// DiffDTO represents a diff in API responses.
type DiffDTO struct {
	ID        string          `json:"id"`
	RunID     string          `json:"run_id"`
	StageID   string          `json:"stage_id,omitempty"`
	Patch     []byte          `json:"patch"`
	Summary   json.RawMessage `json:"summary"`
	CreatedAt string          `json:"created_at"`
}

// LogDTO represents a log chunk in API responses.
type LogDTO struct {
	ID        int64  `json:"id"`
	RunID     string `json:"run_id"`
	StageID   string `json:"stage_id,omitempty"`
	BuildID   string `json:"build_id,omitempty"`
	ChunkNo   int32  `json:"chunk_no"`
	Data      []byte `json:"data"`
	CreatedAt string `json:"created_at"`
}

// ArtifactBundleDTO represents an artifact bundle in API responses.
type ArtifactBundleDTO struct {
	ID        string  `json:"id"`
	RunID     string  `json:"run_id"`
	StageID   string  `json:"stage_id,omitempty"`
	BuildID   string  `json:"build_id,omitempty"`
	Name      *string `json:"name,omitempty"`
	Bundle    []byte  `json:"bundle"`
	CreatedAt string  `json:"created_at"`
}

// CreateDiffRequest represents a request to create a diff.
type CreateDiffRequest struct {
	StageID string          `json:"stage_id,omitempty"`
	Patch   []byte          `json:"patch"`
	Summary json.RawMessage `json:"summary,omitempty"`
}

// CreateLogRequest represents a request to create a log chunk.
type CreateLogRequest struct {
	StageID string `json:"stage_id,omitempty"`
	BuildID string `json:"build_id,omitempty"`
	ChunkNo int32  `json:"chunk_no"`
	Data    []byte `json:"data"`
}

// CreateArtifactBundleRequest represents a request to create an artifact bundle.
type CreateArtifactBundleRequest struct {
	StageID string  `json:"stage_id,omitempty"`
	BuildID string  `json:"build_id,omitempty"`
	Name    *string `json:"name,omitempty"`
	Bundle  []byte  `json:"bundle"`
}

// handleRunsDiffs handles POST /v1/runs/{id}/diffs to ingest a diff.
func (s *controlPlaneServer) handleRunsDiffs(w http.ResponseWriter, r *http.Request, runID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	runUUID, err := parseUUID(runID)
	if err != nil {
		writeErrorMessage(w, http.StatusBadRequest, "invalid run id")
		return
	}

	// Verify the run exists
	_, err = s.store.GetRun(r.Context(), runUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeErrorMessage(w, http.StatusNotFound, "run not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	var req CreateDiffRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	// Enforce 1 MiB size constraint
	if len(req.Patch) > maxIngestionPayloadSize {
		writeErrorMessage(w, http.StatusRequestEntityTooLarge, "patch exceeds 1 MiB limit")
		return
	}

	// Parse optional stage_id
	var stageUUID pgtype.UUID
	if req.StageID != "" {
		stageUUID, err = parseUUID(req.StageID)
		if err != nil {
			writeErrorMessage(w, http.StatusBadRequest, "invalid stage_id")
			return
		}
	}

	// Default summary to empty JSON object if not provided
	summary := req.Summary
	if len(summary) == 0 {
		summary = json.RawMessage("{}")
	}

	// Create the diff
	diff, err := s.store.CreateDiff(r.Context(), store.CreateDiffParams{
		RunID:   runUUID,
		StageID: stageUUID,
		Patch:   req.Patch,
		Summary: summary,
	})
	if err != nil {
		// Check for size constraint violation from database
		if isConstraintError(err, "diffs_patch_size_cap") {
			writeErrorMessage(w, http.StatusRequestEntityTooLarge, "patch exceeds database size limit")
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusCreated, diffDTOFrom(diff))
}

// handleRunsLogs handles POST /v1/runs/{id}/logs to ingest a log chunk.
func (s *controlPlaneServer) handleRunsLogs(w http.ResponseWriter, r *http.Request, runID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	runUUID, err := parseUUID(runID)
	if err != nil {
		writeErrorMessage(w, http.StatusBadRequest, "invalid run id")
		return
	}

	// Verify the run exists
	_, err = s.store.GetRun(r.Context(), runUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeErrorMessage(w, http.StatusNotFound, "run not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	var req CreateLogRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	// Enforce 1 MiB size constraint
	if len(req.Data) > maxIngestionPayloadSize {
		writeErrorMessage(w, http.StatusRequestEntityTooLarge, "log data exceeds 1 MiB limit")
		return
	}

	// Parse optional stage_id and build_id
	var stageUUID, buildUUID pgtype.UUID
	if req.StageID != "" {
		stageUUID, err = parseUUID(req.StageID)
		if err != nil {
			writeErrorMessage(w, http.StatusBadRequest, "invalid stage_id")
			return
		}
	}
	if req.BuildID != "" {
		buildUUID, err = parseUUID(req.BuildID)
		if err != nil {
			writeErrorMessage(w, http.StatusBadRequest, "invalid build_id")
			return
		}
	}

	// Create the log chunk
	logChunk, err := s.store.CreateLog(r.Context(), store.CreateLogParams{
		RunID:   runUUID,
		StageID: stageUUID,
		BuildID: buildUUID,
		ChunkNo: req.ChunkNo,
		Data:    req.Data,
	})
	if err != nil {
		// Check for size constraint violation from database
		if isConstraintError(err, "logs_chunk_size_cap") {
			writeErrorMessage(w, http.StatusRequestEntityTooLarge, "log data exceeds database size limit")
			return
		}
		// Check for unique constraint violation (duplicate chunk_no)
		msg := err.Error()
		if contains(msg, "duplicate") || contains(msg, "unique") {
			writeErrorMessage(w, http.StatusConflict, "log chunk already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusCreated, logDTOFrom(logChunk))
}

// handleRunsArtifactBundles handles POST /v1/runs/{id}/artifact_bundles to ingest an artifact bundle.
func (s *controlPlaneServer) handleRunsArtifactBundles(w http.ResponseWriter, r *http.Request, runID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	runUUID, err := parseUUID(runID)
	if err != nil {
		writeErrorMessage(w, http.StatusBadRequest, "invalid run id")
		return
	}

	// Verify the run exists
	_, err = s.store.GetRun(r.Context(), runUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeErrorMessage(w, http.StatusNotFound, "run not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	var req CreateArtifactBundleRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	// Enforce 1 MiB size constraint
	if len(req.Bundle) > maxIngestionPayloadSize {
		writeErrorMessage(w, http.StatusRequestEntityTooLarge, "artifact bundle exceeds 1 MiB limit")
		return
	}

	// Parse optional stage_id and build_id
	var stageUUID, buildUUID pgtype.UUID
	if req.StageID != "" {
		stageUUID, err = parseUUID(req.StageID)
		if err != nil {
			writeErrorMessage(w, http.StatusBadRequest, "invalid stage_id")
			return
		}
	}
	if req.BuildID != "" {
		buildUUID, err = parseUUID(req.BuildID)
		if err != nil {
			writeErrorMessage(w, http.StatusBadRequest, "invalid build_id")
			return
		}
	}

	// Create the artifact bundle
	bundle, err := s.store.CreateArtifactBundle(r.Context(), store.CreateArtifactBundleParams{
		RunID:   runUUID,
		StageID: stageUUID,
		BuildID: buildUUID,
		Name:    req.Name,
		Bundle:  req.Bundle,
	})
	if err != nil {
		// Check for size constraint violation from database
		if isConstraintError(err, "artifact_bundles_size_cap") {
			writeErrorMessage(w, http.StatusRequestEntityTooLarge, "artifact bundle exceeds database size limit")
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusCreated, artifactBundleDTOFrom(bundle))
}

// diffDTOFrom converts a store.Diff to a DiffDTO.
func diffDTOFrom(diff store.Diff) DiffDTO {
	summary := json.RawMessage("{}")
	if len(diff.Summary) > 0 {
		summary = diff.Summary
	}
	dto := DiffDTO{
		ID:        uuidToString(diff.ID),
		RunID:     uuidToString(diff.RunID),
		Patch:     diff.Patch,
		Summary:   summary,
		CreatedAt: timestampToString(diff.CreatedAt),
	}
	if diff.StageID.Valid {
		dto.StageID = uuidToString(diff.StageID)
	}
	return dto
}

// logDTOFrom converts a store.Log to a LogDTO.
func logDTOFrom(log store.Log) LogDTO {
	dto := LogDTO{
		ID:        log.ID,
		RunID:     uuidToString(log.RunID),
		ChunkNo:   log.ChunkNo,
		Data:      log.Data,
		CreatedAt: timestampToString(log.CreatedAt),
	}
	if log.StageID.Valid {
		dto.StageID = uuidToString(log.StageID)
	}
	if log.BuildID.Valid {
		dto.BuildID = uuidToString(log.BuildID)
	}
	return dto
}

// artifactBundleDTOFrom converts a store.ArtifactBundle to an ArtifactBundleDTO.
func artifactBundleDTOFrom(bundle store.ArtifactBundle) ArtifactBundleDTO {
	dto := ArtifactBundleDTO{
		ID:        uuidToString(bundle.ID),
		RunID:     uuidToString(bundle.RunID),
		Name:      bundle.Name,
		Bundle:    bundle.Bundle,
		CreatedAt: timestampToString(bundle.CreatedAt),
	}
	if bundle.StageID.Valid {
		dto.StageID = uuidToString(bundle.StageID)
	}
	if bundle.BuildID.Valid {
		dto.BuildID = uuidToString(bundle.BuildID)
	}
	return dto
}

// isConstraintError checks if the error is a PostgreSQL constraint violation for the given constraint name.
func isConstraintError(err error, constraintName string) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return contains(msg, constraintName)
}

// contains checks if a string contains a substring (case-insensitive).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || anyIndex(s, substr) >= 0)
}

func anyIndex(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
