package httpserver

import (
	"errors"
	"net/http"
	"strings"

	"github.com/iw2rmb/ploy/internal/store"
	"github.com/jackc/pgx/v5"
)

// handleModsCRUD handles collection-level CRUD operations for /v1/mods (not tickets).
func (s *controlPlaneServer) handleModsCRUD(w http.ResponseWriter, r *http.Request) {
	if !s.ensureStore(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleModsList(w, r)
	case http.MethodPost:
		s.handleModsCreate(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleModsCRUDSubpath handles /v1/mods/crud/{id} routes.
func (s *controlPlaneServer) handleModsCRUDSubpath(w http.ResponseWriter, r *http.Request) {
	if !s.ensureStore(w) {
		return
	}
	trimmed := strings.TrimPrefix(r.URL.Path, "/v1/mods/crud/")
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		http.NotFound(w, r)
		return
	}
	modID := strings.TrimSpace(trimmed)
	if modID == "" {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleModsGetByID(w, r, modID)
	case http.MethodDelete:
		s.handleModsDeleteByID(w, r, modID)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleModsList returns a list of all mods.
func (s *controlPlaneServer) handleModsList(w http.ResponseWriter, r *http.Request) {
	// Check if filtering by repo_id
	repoID := r.URL.Query().Get("repo_id")
	var mods []store.Mod
	var err error

	if repoID != "" {
		uuid, parseErr := parseUUID(repoID)
		if parseErr != nil {
			writeErrorMessage(w, http.StatusBadRequest, "invalid repo_id")
			return
		}
		mods, err = s.store.ListModsByRepo(r.Context(), uuid)
	} else {
		mods, err = s.store.ListMods(r.Context())
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	dtos := make([]ModDTO, 0, len(mods))
	for _, mod := range mods {
		dtos = append(dtos, modDTOFrom(mod))
	}
	writeJSON(w, http.StatusOK, ListModsResponse{Mods: dtos})
}

// handleModsCreate creates a new mod.
func (s *controlPlaneServer) handleModsCreate(w http.ResponseWriter, r *http.Request) {
	var req CreateModRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(req.RepoID) == "" {
		writeErrorMessage(w, http.StatusBadRequest, "repo_id is required")
		return
	}
	if len(req.Spec) == 0 {
		writeErrorMessage(w, http.StatusBadRequest, "spec is required")
		return
	}

	repoUUID, err := parseUUID(req.RepoID)
	if err != nil {
		writeErrorMessage(w, http.StatusBadRequest, "invalid repo_id")
		return
	}

	mod, err := s.store.CreateMod(r.Context(), store.CreateModParams{
		RepoID:    repoUUID,
		Spec:      req.Spec,
		CreatedBy: req.CreatedBy,
	})
	if err != nil {
		code, msg := mapStoreError(err)
		writeErrorMessage(w, code, msg)
		return
	}
	writeJSON(w, http.StatusCreated, modDTOFrom(mod))
}

// handleModsGetByID retrieves a single mod by ID.
func (s *controlPlaneServer) handleModsGetByID(w http.ResponseWriter, r *http.Request, modID string) {
	uuid, err := parseUUID(modID)
	if err != nil {
		writeErrorMessage(w, http.StatusBadRequest, "invalid mod id")
		return
	}
	mod, err := s.store.GetMod(r.Context(), uuid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeErrorMessage(w, http.StatusNotFound, "mod not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, modDTOFrom(mod))
}

// handleModsDeleteByID deletes a mod by ID.
func (s *controlPlaneServer) handleModsDeleteByID(w http.ResponseWriter, r *http.Request, modID string) {
	uuid, err := parseUUID(modID)
	if err != nil {
		writeErrorMessage(w, http.StatusBadRequest, "invalid mod id")
		return
	}
	if err := s.store.DeleteMod(r.Context(), uuid); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeErrorMessage(w, http.StatusNotFound, "mod not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
