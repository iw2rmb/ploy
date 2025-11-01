package httpserver

import (
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/store"
)

// handleRepos handles collection-level operations for /v1/repos.
func (s *controlPlaneServer) handleRepos(w http.ResponseWriter, r *http.Request) {
	if !s.ensureStore(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleReposList(w, r)
	case http.MethodPost:
		s.handleReposCreate(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleReposSubpath handles /v1/repos/{id} routes.
func (s *controlPlaneServer) handleReposSubpath(w http.ResponseWriter, r *http.Request) {
	if !s.ensureStore(w) {
		return
	}
	trimmed := strings.TrimPrefix(r.URL.Path, "/v1/repos/")
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		http.NotFound(w, r)
		return
	}
	repoID := strings.TrimSpace(trimmed)
	if repoID == "" {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleReposGet(w, r, repoID)
	case http.MethodDelete:
		s.handleReposDelete(w, r, repoID)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleReposList returns a list of all repositories.
func (s *controlPlaneServer) handleReposList(w http.ResponseWriter, r *http.Request) {
	repos, err := s.store.ListRepos(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	dtos := make([]RepoDTO, 0, len(repos))
	for _, repo := range repos {
		dtos = append(dtos, repoDTOFrom(repo))
	}
	writeJSON(w, http.StatusOK, ListReposResponse{Repos: dtos})
}

// handleReposCreate creates a new repository.
func (s *controlPlaneServer) handleReposCreate(w http.ResponseWriter, r *http.Request) {
	var req CreateRepoRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(req.URL) == "" {
		writeErrorMessage(w, http.StatusBadRequest, "url is required")
		return
	}
	repo, err := s.store.CreateRepo(r.Context(), store.CreateRepoParams{
		Url:       strings.TrimSpace(req.URL),
		Branch:    req.Branch,
		CommitSha: req.CommitSha,
	})
	if err != nil {
		code, msg := mapStoreError(err)
		writeErrorMessage(w, code, msg)
		return
	}
	writeJSON(w, http.StatusCreated, repoDTOFrom(repo))
}

// handleReposGet retrieves a single repository by ID.
func (s *controlPlaneServer) handleReposGet(w http.ResponseWriter, r *http.Request, repoID string) {
	uuid, err := parseUUID(repoID)
	if err != nil {
		writeErrorMessage(w, http.StatusBadRequest, "invalid repo id")
		return
	}
	repo, err := s.store.GetRepo(r.Context(), uuid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeErrorMessage(w, http.StatusNotFound, "repo not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, repoDTOFrom(repo))
}

// handleReposDelete deletes a repository by ID.
func (s *controlPlaneServer) handleReposDelete(w http.ResponseWriter, r *http.Request, repoID string) {
	uuid, err := parseUUID(repoID)
	if err != nil {
		writeErrorMessage(w, http.StatusBadRequest, "invalid repo id")
		return
	}
	if err := s.store.DeleteRepo(r.Context(), uuid); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeErrorMessage(w, http.StatusNotFound, "repo not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ensureStore checks if the store is available.
func (s *controlPlaneServer) ensureStore(w http.ResponseWriter) bool {
	if s.store == nil {
		writeErrorMessage(w, http.StatusServiceUnavailable, "store unavailable")
		return false
	}
	return true
}

// parseUUID parses a UUID string.
func parseUUID(s string) (pgtype.UUID, error) {
	var uuid pgtype.UUID
	if err := uuid.Scan(s); err != nil {
		return uuid, err
	}
	return uuid, nil
}

// mapStoreError maps database errors to HTTP status codes.
func mapStoreError(err error) (int, string) {
	if err == nil {
		return http.StatusOK, ""
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return http.StatusNotFound, "not found"
	}
	// Check for unique constraint violations (duplicate URL, etc.)
	msg := err.Error()
	if strings.Contains(msg, "duplicate") || strings.Contains(msg, "unique") {
		return http.StatusConflict, "resource already exists"
	}
	if strings.Contains(msg, "foreign key") {
		return http.StatusBadRequest, "invalid reference"
	}
	return http.StatusInternalServerError, "internal server error"
}
