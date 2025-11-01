package httpserver

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/iw2rmb/ploy/internal/store"
)

// handleRuns handles collection-level operations for /v1/runs.
func (s *controlPlaneServer) handleRuns(w http.ResponseWriter, r *http.Request) {
	if !s.ensureStore(w) {
		return
	}
	// Check for special query parameter to retrieve timings
	if r.Method == http.MethodGet && r.URL.Query().Get("view") == "timing" {
		s.handleRunsTimingsList(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleRunsList(w, r)
	case http.MethodPost:
		s.handleRunsCreate(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleRunsSubpath handles /v1/runs/{id} and /v1/runs/{id}/{action} routes.
func (s *controlPlaneServer) handleRunsSubpath(w http.ResponseWriter, r *http.Request) {
	if !s.ensureStore(w) {
		return
	}
	trimmed := strings.TrimPrefix(r.URL.Path, "/v1/runs/")
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(trimmed, "/")
	runID := strings.TrimSpace(parts[0])
	if runID == "" {
		http.NotFound(w, r)
		return
	}

	// /v1/runs/{id}
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			s.handleRunsGet(w, r, runID)
		case http.MethodDelete:
			s.handleRunsDelete(w, r, runID)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	// /v1/runs/{id}/{action}
	switch parts[1] {
	case "events":
		s.handleRunsEvents(w, r, runID)
	case "timing":
		s.handleRunsTiming(w, r, runID)
	default:
		http.NotFound(w, r)
	}
}

// handleRunsList returns a paginated list of runs.
func (s *controlPlaneServer) handleRunsList(w http.ResponseWriter, r *http.Request) {
	// Parse pagination parameters
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")

	limit := int32(50) // default
	if limitStr != "" {
		parsed, err := strconv.ParseInt(limitStr, 10, 32)
		if err != nil || parsed <= 0 {
			writeErrorMessage(w, http.StatusBadRequest, "invalid limit")
			return
		}
		limit = int32(parsed)
	}

	offset := int32(0)
	if offsetStr != "" {
		parsed, err := strconv.ParseInt(offsetStr, 10, 32)
		if err != nil || parsed < 0 {
			writeErrorMessage(w, http.StatusBadRequest, "invalid offset")
			return
		}
		offset = int32(parsed)
	}

	// Check if filtering by mod_id
	modID := r.URL.Query().Get("mod_id")
	var runs []store.Run
	var err error

	if modID != "" {
		uuid, parseErr := parseUUID(modID)
		if parseErr != nil {
			writeErrorMessage(w, http.StatusBadRequest, "invalid mod_id")
			return
		}
		runs, err = s.store.ListRunsByMod(r.Context(), uuid)
	} else {
		runs, err = s.store.ListRuns(r.Context(), store.ListRunsParams{
			Limit:  limit,
			Offset: offset,
		})
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	dtos := make([]RunDTO, 0, len(runs))
	for _, run := range runs {
		dtos = append(dtos, runDTOFrom(run))
	}
	writeJSON(w, http.StatusOK, ListRunsResponse{Runs: dtos})
}

// handleRunsCreate creates a new run and returns run_id.
func (s *controlPlaneServer) handleRunsCreate(w http.ResponseWriter, r *http.Request) {
	var req CreateRunRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(req.ModID) == "" {
		writeErrorMessage(w, http.StatusBadRequest, "mod_id is required")
		return
	}
	if strings.TrimSpace(req.BaseRef) == "" {
		writeErrorMessage(w, http.StatusBadRequest, "base_ref is required")
		return
	}
	if strings.TrimSpace(req.TargetRef) == "" {
		writeErrorMessage(w, http.StatusBadRequest, "target_ref is required")
		return
	}

	modUUID, err := parseUUID(req.ModID)
	if err != nil {
		writeErrorMessage(w, http.StatusBadRequest, "invalid mod_id")
		return
	}

	run, err := s.store.CreateRun(r.Context(), store.CreateRunParams{
		ModID:     modUUID,
		Status:    store.RunStatusQueued,
		BaseRef:   strings.TrimSpace(req.BaseRef),
		TargetRef: strings.TrimSpace(req.TargetRef),
		CommitSha: req.CommitSha,
	})
	if err != nil {
		code, msg := mapStoreError(err)
		writeErrorMessage(w, code, msg)
		return
	}

	// Return run_id as required by the spec
	resp := CreateRunResponse{
		RunID: uuidToString(run.ID),
		Run:   runDTOFrom(run),
	}
	writeJSON(w, http.StatusCreated, resp)
}

// handleRunsGet retrieves a single run by ID.
func (s *controlPlaneServer) handleRunsGet(w http.ResponseWriter, r *http.Request, runID string) {
	uuid, err := parseUUID(runID)
	if err != nil {
		writeErrorMessage(w, http.StatusBadRequest, "invalid run id")
		return
	}
	run, err := s.store.GetRun(r.Context(), uuid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeErrorMessage(w, http.StatusNotFound, "run not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, runDTOFrom(run))
}

// handleRunsDelete deletes a run by ID.
func (s *controlPlaneServer) handleRunsDelete(w http.ResponseWriter, r *http.Request, runID string) {
	uuid, err := parseUUID(runID)
	if err != nil {
		writeErrorMessage(w, http.StatusBadRequest, "invalid run id")
		return
	}
	if err := s.store.DeleteRun(r.Context(), uuid); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeErrorMessage(w, http.StatusNotFound, "run not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleRunsTiming retrieves timing information for a single run.
func (s *controlPlaneServer) handleRunsTiming(w http.ResponseWriter, r *http.Request, runID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	uuid, err := parseUUID(runID)
	if err != nil {
		writeErrorMessage(w, http.StatusBadRequest, "invalid run id")
		return
	}
	timing, err := s.store.GetRunTiming(r.Context(), uuid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeErrorMessage(w, http.StatusNotFound, "run timing not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, runTimingDTOFrom(timing))
}

// handleRunsTimingsList returns a paginated list of run timings.
func (s *controlPlaneServer) handleRunsTimingsList(w http.ResponseWriter, r *http.Request) {
	// Parse pagination parameters
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")

	limit := int32(50) // default
	if limitStr != "" {
		parsed, err := strconv.ParseInt(limitStr, 10, 32)
		if err != nil || parsed <= 0 {
			writeErrorMessage(w, http.StatusBadRequest, "invalid limit")
			return
		}
		limit = int32(parsed)
	}

	offset := int32(0)
	if offsetStr != "" {
		parsed, err := strconv.ParseInt(offsetStr, 10, 32)
		if err != nil || parsed < 0 {
			writeErrorMessage(w, http.StatusBadRequest, "invalid offset")
			return
		}
		offset = int32(parsed)
	}

	timings, err := s.store.ListRunsTimings(r.Context(), store.ListRunsTimingsParams{
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	dtos := make([]RunTimingDTO, 0, len(timings))
	for _, timing := range timings {
		dtos = append(dtos, runTimingDTOFrom(timing))
	}
	writeJSON(w, http.StatusOK, ListRunsTimingsResponse{Timings: dtos})
}
