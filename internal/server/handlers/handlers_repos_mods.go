package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/store"
)

// createRepoHandler returns an HTTP handler that creates a new repository.
func createRepoHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Decode request body.
		var req struct {
			URL       string  `json:"url"`
			Branch    *string `json:"branch,omitempty"`
			CommitSha *string `json:"commit_sha,omitempty"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
			return
		}

		// Validate required fields.
		if strings.TrimSpace(req.URL) == "" {
			http.Error(w, "url field is required", http.StatusBadRequest)
			return
		}

		// Create the repository.
		repo, err := st.CreateRepo(r.Context(), store.CreateRepoParams{
			Url:       req.URL,
			Branch:    req.Branch,
			CommitSha: req.CommitSha,
		})
		if err != nil {
			// Check if this is a duplicate URL error (UNIQUE constraint violation).
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
				http.Error(w, "repository with this url already exists", http.StatusConflict)
				return
			}
			if strings.Contains(err.Error(), "repos_url_unique") || strings.Contains(err.Error(), "duplicate key") {
				http.Error(w, "repository with this url already exists", http.StatusConflict)
				return
			}
			http.Error(w, fmt.Sprintf("failed to create repository: %v", err), http.StatusInternalServerError)
			slog.Error("create repo: database error", "url", req.URL, "err", err)
			return
		}

		// Build response.
		resp := struct {
			ID        string  `json:"id"`
			URL       string  `json:"url"`
			Branch    *string `json:"branch,omitempty"`
			CommitSha *string `json:"commit_sha,omitempty"`
			CreatedAt string  `json:"created_at"`
		}{
			ID:        uuid.UUID(repo.ID.Bytes).String(),
			URL:       repo.Url,
			Branch:    repo.Branch,
			CommitSha: repo.CommitSha,
			CreatedAt: repo.CreatedAt.Time.Format(time.RFC3339),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("create repo: encode response failed", "err", err)
		}

		slog.Info("repository created",
			"id", resp.ID,
			"url", repo.Url,
		)
	}
}

// listReposHandler returns an HTTP handler that lists all repositories.
func listReposHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// List all repositories.
		repos, err := st.ListRepos(r.Context())
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to list repositories: %v", err), http.StatusInternalServerError)
			slog.Error("list repos: database error", "err", err)
			return
		}

		// Build response.
		type repoResponse struct {
			ID        string  `json:"id"`
			URL       string  `json:"url"`
			Branch    *string `json:"branch,omitempty"`
			CommitSha *string `json:"commit_sha,omitempty"`
			CreatedAt string  `json:"created_at"`
		}

		wrapper := struct {
			Repos []repoResponse `json:"repos"`
		}{
			Repos: make([]repoResponse, len(repos)),
		}

		for i, repo := range repos {
			wrapper.Repos[i] = repoResponse{
				ID:        uuid.UUID(repo.ID.Bytes).String(),
				URL:       repo.Url,
				Branch:    repo.Branch,
				CommitSha: repo.CommitSha,
				CreatedAt: repo.CreatedAt.Time.Format(time.RFC3339),
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(wrapper); err != nil {
			slog.Error("list repos: encode response failed", "err", err)
		}

		slog.Debug("repositories listed", "count", len(repos))
	}
}

// getRepoHandler returns an HTTP handler that gets a repository by ID.
func getRepoHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract repo id from path parameter.
		repoIDStr := r.PathValue("id")
		if strings.TrimSpace(repoIDStr) == "" {
			http.Error(w, "id path parameter is required", http.StatusBadRequest)
			return
		}

		// Parse and validate repo_id.
		repoUUID, err := uuid.Parse(repoIDStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid id: %v", err), http.StatusBadRequest)
			return
		}

		// Get the repository.
		repo, err := st.GetRepo(r.Context(), pgtype.UUID{
			Bytes: repoUUID,
			Valid: true,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "repository not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get repository: %v", err), http.StatusInternalServerError)
			slog.Error("get repo: database error", "id", repoIDStr, "err", err)
			return
		}

		// Build response.
		resp := struct {
			ID        string  `json:"id"`
			URL       string  `json:"url"`
			Branch    *string `json:"branch,omitempty"`
			CommitSha *string `json:"commit_sha,omitempty"`
			CreatedAt string  `json:"created_at"`
		}{
			ID:        uuid.UUID(repo.ID.Bytes).String(),
			URL:       repo.Url,
			Branch:    repo.Branch,
			CommitSha: repo.CommitSha,
			CreatedAt: repo.CreatedAt.Time.Format(time.RFC3339),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("get repo: encode response failed", "err", err)
		}

		slog.Debug("repository retrieved", "id", resp.ID)
	}
}

// deleteRepoHandler returns an HTTP handler that deletes a repository by ID.
func deleteRepoHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract repo id from path parameter.
		repoIDStr := r.PathValue("id")
		if strings.TrimSpace(repoIDStr) == "" {
			http.Error(w, "id path parameter is required", http.StatusBadRequest)
			return
		}

		// Parse and validate repo_id.
		repoUUID, err := uuid.Parse(repoIDStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid id: %v", err), http.StatusBadRequest)
			return
		}

		// Delete the repository.
		err = st.DeleteRepo(r.Context(), pgtype.UUID{
			Bytes: repoUUID,
			Valid: true,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "repository not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to delete repository: %v", err), http.StatusInternalServerError)
			slog.Error("delete repo: database error", "id", repoIDStr, "err", err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
		slog.Info("repository deleted", "id", repoIDStr)
	}
}

// createModHandler returns an HTTP handler that creates a new mod.
func createModHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Decode request body.
		var req struct {
			RepoID    string          `json:"repo_id"`
			Spec      json.RawMessage `json:"spec"`
			CreatedBy *string         `json:"created_by,omitempty"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
			return
		}

		// Validate required fields.
		if strings.TrimSpace(req.RepoID) == "" {
			http.Error(w, "repo_id field is required", http.StatusBadRequest)
			return
		}

		// Validate repo_id format.
		repoUUID, err := uuid.Parse(req.RepoID)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid repo_id: %v", err), http.StatusBadRequest)
			return
		}

		// Validate spec is not empty.
		if len(req.Spec) == 0 {
			http.Error(w, "spec field is required", http.StatusBadRequest)
			return
		}

		// Validate spec is valid JSON.
		if !json.Valid(req.Spec) {
			http.Error(w, "spec must be valid JSON", http.StatusBadRequest)
			return
		}

		// Create the mod.
		mod, err := st.CreateMod(r.Context(), store.CreateModParams{
			RepoID: pgtype.UUID{
				Bytes: repoUUID,
				Valid: true,
			},
			Spec:      req.Spec,
			CreatedBy: req.CreatedBy,
		})
		if err != nil {
			// Check if this is a foreign key violation (repo does not exist).
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23503" { // foreign_key_violation
				http.Error(w, "repository not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to create mod: %v", err), http.StatusInternalServerError)
			slog.Error("create mod: database error", "repo_id", req.RepoID, "err", err)
			return
		}

		// Build response.
		resp := struct {
			ID        string          `json:"id"`
			RepoID    string          `json:"repo_id"`
			Spec      json.RawMessage `json:"spec"`
			CreatedBy *string         `json:"created_by,omitempty"`
			CreatedAt string          `json:"created_at"`
		}{
			ID:        uuid.UUID(mod.ID.Bytes).String(),
			RepoID:    uuid.UUID(mod.RepoID.Bytes).String(),
			Spec:      mod.Spec,
			CreatedBy: mod.CreatedBy,
			CreatedAt: mod.CreatedAt.Time.Format(time.RFC3339),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("create mod: encode response failed", "err", err)
		}

		slog.Info("mod created",
			"id", resp.ID,
			"repo_id", resp.RepoID,
		)
	}
}

// listModsHandler returns an HTTP handler that lists mods, optionally filtered by repo_id.
func listModsHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check for repo_id query parameter.
		repoIDStr := strings.TrimSpace(r.URL.Query().Get("repo_id"))

		type modResponse struct {
			ID        string          `json:"id"`
			RepoID    string          `json:"repo_id"`
			Spec      json.RawMessage `json:"spec"`
			CreatedBy *string         `json:"created_by,omitempty"`
			CreatedAt string          `json:"created_at"`
		}

		wrapper := struct {
			Mods []modResponse `json:"mods"`
		}{}

		if repoIDStr != "" {
			// Parse and validate repo_id.
			repoUUID, err := uuid.Parse(repoIDStr)
			if err != nil {
				http.Error(w, fmt.Sprintf("invalid repo_id: %v", err), http.StatusBadRequest)
				return
			}

			mods, err := st.ListModsByRepo(r.Context(), pgtype.UUID{Bytes: repoUUID, Valid: true})
			if err != nil {
				http.Error(w, fmt.Sprintf("failed to list mods: %v", err), http.StatusInternalServerError)
				slog.Error("list mods by repo: database error", "repo_id", repoIDStr, "err", err)
				return
			}

			wrapper.Mods = make([]modResponse, len(mods))
			for i, mod := range mods {
				wrapper.Mods[i] = modResponse{
					ID:        uuid.UUID(mod.ID.Bytes).String(),
					RepoID:    uuid.UUID(mod.RepoID.Bytes).String(),
					Spec:      mod.Spec,
					CreatedBy: mod.CreatedBy,
					CreatedAt: mod.CreatedAt.Time.Format(time.RFC3339),
				}
			}

			slog.Debug("mods listed by repo", "repo_id", repoIDStr, "count", len(mods))
		} else {
			// List all mods.
			mods, err := st.ListMods(r.Context())
			if err != nil {
				http.Error(w, fmt.Sprintf("failed to list mods: %v", err), http.StatusInternalServerError)
				slog.Error("list mods: database error", "err", err)
				return
			}

			wrapper.Mods = make([]modResponse, len(mods))
			for i, mod := range mods {
				wrapper.Mods[i] = modResponse{
					ID:        uuid.UUID(mod.ID.Bytes).String(),
					RepoID:    uuid.UUID(mod.RepoID.Bytes).String(),
					Spec:      mod.Spec,
					CreatedBy: mod.CreatedBy,
					CreatedAt: mod.CreatedAt.Time.Format(time.RFC3339),
				}
			}

			slog.Debug("mods listed", "count", len(mods))
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(wrapper); err != nil {
			slog.Error("list mods: encode response failed", "err", err)
		}
	}
}

// getModHandler returns an HTTP handler that gets a mod by ID.
func getModHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract mod id from path parameter.
		modIDStr := r.PathValue("id")
		if strings.TrimSpace(modIDStr) == "" {
			http.Error(w, "id path parameter is required", http.StatusBadRequest)
			return
		}

		// Parse and validate mod_id.
		modUUID, err := uuid.Parse(modIDStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid id: %v", err), http.StatusBadRequest)
			return
		}

		// Get the mod.
		mod, err := st.GetMod(r.Context(), pgtype.UUID{
			Bytes: modUUID,
			Valid: true,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "mod not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get mod: %v", err), http.StatusInternalServerError)
			slog.Error("get mod: database error", "id", modIDStr, "err", err)
			return
		}

		// Build response.
		resp := struct {
			ID        string          `json:"id"`
			RepoID    string          `json:"repo_id"`
			Spec      json.RawMessage `json:"spec"`
			CreatedBy *string         `json:"created_by,omitempty"`
			CreatedAt string          `json:"created_at"`
		}{
			ID:        uuid.UUID(mod.ID.Bytes).String(),
			RepoID:    uuid.UUID(mod.RepoID.Bytes).String(),
			Spec:      mod.Spec,
			CreatedBy: mod.CreatedBy,
			CreatedAt: mod.CreatedAt.Time.Format(time.RFC3339),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("get mod: encode response failed", "err", err)
		}

		slog.Debug("mod retrieved", "id", resp.ID)
	}
}

// deleteModHandler returns an HTTP handler that deletes a mod by ID.
func deleteModHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract mod id from path parameter.
		modIDStr := r.PathValue("id")
		if strings.TrimSpace(modIDStr) == "" {
			http.Error(w, "id path parameter is required", http.StatusBadRequest)
			return
		}

		// Parse and validate mod_id.
		modUUID, err := uuid.Parse(modIDStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid id: %v", err), http.StatusBadRequest)
			return
		}

		// Delete the mod.
		err = st.DeleteMod(r.Context(), pgtype.UUID{
			Bytes: modUUID,
			Valid: true,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "mod not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to delete mod: %v", err), http.StatusInternalServerError)
			slog.Error("delete mod: database error", "id", modIDStr, "err", err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
		slog.Info("mod deleted", "id", modIDStr)
	}
}
