package httpserver

import (
    "encoding/json"
    "errors"
    "net/http"
    "strings"

    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgconn"
    "github.com/jackc/pgx/v5/pgtype"

    "github.com/iw2rmb/ploy/internal/store"
)

const maxIngestionPayloadSize = 1 << 20 // 1 MiB (binary payload cap)
// Account for base64 overhead when sending binary in JSON (≈4/3) plus small JSON framing.
const maxIngestionJSONBodySize int64 = (maxIngestionPayloadSize*4)/3 + 4096

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
    if err := decodeJSONWithLimit(r, &req, maxIngestionJSONBodySize); err != nil {
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
    if err := decodeJSONWithLimit(r, &req, maxIngestionJSONBodySize); err != nil {
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
        // Database size constraint
        if isConstraintError(err, "logs_chunk_size_cap") {
            writeErrorMessage(w, http.StatusRequestEntityTooLarge, "log data exceeds database size limit")
            return
        }
        // Unique chunk per (run,stage,build,chunk_no)
        if isUniqueViolation(err, "logs_run_stage_build_chunk_uniq") {
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
    if err := decodeJSONWithLimit(r, &req, maxIngestionJSONBodySize); err != nil {
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
    if err == nil || strings.TrimSpace(constraintName) == "" {
        return false
    }
    var pgErr *pgconn.PgError
    if errors.As(err, &pgErr) {
        return pgErr.ConstraintName == constraintName
    }
    // Fallback to substring match for non-pg errors in tests/mocks
    return strings.Contains(strings.ToLower(err.Error()), strings.ToLower(constraintName))
}

// isUniqueViolation reports a unique constraint/index violation optionally matching a specific constraint/index name.
func isUniqueViolation(err error, constraintName string) bool {
    if err == nil {
        return false
    }
    var pgErr *pgconn.PgError
    if errors.As(err, &pgErr) {
        if pgErr.Code == "23505" { // unique_violation
            if strings.TrimSpace(constraintName) == "" {
                return true
            }
            return pgErr.ConstraintName == constraintName
        }
        return false
    }
    // Fallback for mock errors: look for words "duplicate" and "unique"
    lower := strings.ToLower(err.Error())
    if strings.Contains(lower, "duplicate") && strings.Contains(lower, "unique") {
        if strings.TrimSpace(constraintName) == "" {
            return true
        }
        return strings.Contains(lower, strings.ToLower(constraintName))
    }
    return false
}
