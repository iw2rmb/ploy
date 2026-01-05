package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// createModHandler creates a new mod project.
// Endpoint: POST /v1/mods
// Request: {name, spec?}
// Response: 201 Created with mod details
//
// v1 contract (roadmap/v1/api.md:14-30):
// - Creates a mod project with a unique name.
// - Optional spec parameter creates an initial spec row and sets mods.spec_id.
func createModHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name      string           `json:"name"`
			Spec      *json.RawMessage `json:"spec,omitempty"`
			CreatedBy *string          `json:"created_by,omitempty"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
			return
		}

		// Validate name is not empty
		if req.Name == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}

		// Create spec if provided
		var specIDPtr *string
		if req.Spec != nil && len(*req.Spec) > 0 {
			specID := domaintypes.NewSpecID().String()
			createdSpec, err := st.CreateSpec(r.Context(), store.CreateSpecParams{
				ID:        specID,
				Name:      "",
				Spec:      *req.Spec,
				CreatedBy: req.CreatedBy,
			})
			if err != nil {
				http.Error(w, fmt.Sprintf("failed to create spec: %v", err), http.StatusInternalServerError)
				slog.Error("create mod: create spec failed", "err", err)
				return
			}
			specIDPtr = &createdSpec.ID
		}

		// Create mod
		modID := domaintypes.NewModID().String()
		mod, err := st.CreateMod(r.Context(), store.CreateModParams{
			ID:        modID,
			Name:      req.Name,
			SpecID:    specIDPtr,
			CreatedBy: req.CreatedBy,
		})
		if err != nil {
			// Check for unique constraint violation (duplicate name)
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				http.Error(w, "mod with this name already exists", http.StatusConflict)
				return
			}
			http.Error(w, fmt.Sprintf("failed to create mod: %v", err), http.StatusInternalServerError)
			slog.Error("create mod: create mod failed", "mod_id", modID, "err", err)
			return
		}

		// Build response
		resp := struct {
			ID        string  `json:"id"`
			Name      string  `json:"name"`
			SpecID    *string `json:"spec_id,omitempty"`
			CreatedBy *string `json:"created_by,omitempty"`
			CreatedAt string  `json:"created_at"`
		}{
			ID:        mod.ID,
			Name:      mod.Name,
			SpecID:    mod.SpecID,
			CreatedBy: mod.CreatedBy,
			CreatedAt: mod.CreatedAt.Time.Format("2006-01-02T15:04:05Z07:00"),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("create mod: encode response failed", "err", err)
		}

		slog.Info("mod created", "mod_id", mod.ID, "name", mod.Name)
	}
}

// listModsHandler lists mod projects with optional filters.
// Endpoint: GET /v1/mods
// Query params: limit, offset, name_substring, archived, repo_url
// Response: 200 OK with list of mods
//
// v1 contract (roadmap/v1/api.md:31-46):
// - Supports pagination with limit/offset.
// - Optional filters: name_substring, archived (true/false), repo_url.
func listModsHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse query parameters
		// TODO: Implement filters (name_substring, archived, repo_url) when store methods are available.
		// For now, return all mods with basic pagination.

		mods, err := st.ListMods(r.Context(), store.ListModsParams{
			Limit:  100, // Default limit
			Offset: 0,
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to list mods: %v", err), http.StatusInternalServerError)
			slog.Error("list mods: fetch failed", "err", err)
			return
		}

		// Build response
		type modItem struct {
			ID        string  `json:"id"`
			Name      string  `json:"name"`
			SpecID    *string `json:"spec_id,omitempty"`
			CreatedBy *string `json:"created_by,omitempty"`
			Archived  bool    `json:"archived"`
			CreatedAt string  `json:"created_at"`
		}

		items := make([]modItem, 0, len(mods))
		for _, mod := range mods {
			items = append(items, modItem{
				ID:        mod.ID,
				Name:      mod.Name,
				SpecID:    mod.SpecID,
				CreatedBy: mod.CreatedBy,
				Archived:  mod.ArchivedAt.Valid,
				CreatedAt: mod.CreatedAt.Time.Format("2006-01-02T15:04:05Z07:00"),
			})
		}

		resp := struct {
			Mods []modItem `json:"mods"`
		}{Mods: items}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("list mods: encode response failed", "err", err)
		}
	}
}

// deleteModHandler deletes a mod project.
// Endpoint: DELETE /v1/mods/{mod_id}
// Response: 204 No Content on success
//
// v1 contract (roadmap/v1/api.md:47-54):
// - Refuses deletion if any runs exist for the mod.
func deleteModHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		modIDStr, err := requiredPathParam(r, "mod_id")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Check if mod exists
		_, err = st.GetMod(r.Context(), modIDStr)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "mod not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get mod: %v", err), http.StatusInternalServerError)
			slog.Error("delete mod: get mod failed", "mod_id", modIDStr, "err", err)
			return
		}

		// Check if any runs exist for this mod
		// Use ListRuns and filter by mod_id since there's no dedicated ListRunsByMod method
		allRuns, err := st.ListRuns(r.Context(), store.ListRunsParams{Limit: 1000, Offset: 0})
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to check runs: %v", err), http.StatusInternalServerError)
			slog.Error("delete mod: check runs failed", "mod_id", modIDStr, "err", err)
			return
		}

		hasRuns := false
		for _, run := range allRuns {
			if run.ModID == modIDStr {
				hasRuns = true
				break
			}
		}

		if hasRuns {
			http.Error(w, "cannot delete mod with existing runs", http.StatusConflict)
			return
		}

		// Delete the mod
		if err := st.DeleteMod(r.Context(), modIDStr); err != nil {
			http.Error(w, fmt.Sprintf("failed to delete mod: %v", err), http.StatusInternalServerError)
			slog.Error("delete mod: database error", "mod_id", modIDStr, "err", err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
		slog.Info("mod deleted", "mod_id", modIDStr)
	}
}

// archiveModHandler archives a mod project.
// Endpoint: PATCH /v1/mods/{mod_id}/archive
// Response: 200 OK with mod details
//
// v1 contract (roadmap/v1/api.md:82-88):
// - Archives a mod (prevents execution).
// - Cannot archive a mod with running jobs.
func archiveModHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		modIDStr, err := requiredPathParam(r, "mod_id")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Check if mod exists
		mod, err := st.GetMod(r.Context(), modIDStr)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "mod not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get mod: %v", err), http.StatusInternalServerError)
			slog.Error("archive mod: get mod failed", "mod_id", modIDStr, "err", err)
			return
		}

		// Check if already archived
		if mod.ArchivedAt.Valid {
			// Already archived, return current state (idempotent)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			resp := struct {
				ID       string `json:"id"`
				Name     string `json:"name"`
				Archived bool   `json:"archived"`
			}{
				ID:       mod.ID,
				Name:     mod.Name,
				Archived: true,
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		// Check for running jobs in this mod's runs
		// Get all runs and filter by mod_id
		allRuns, err := st.ListRuns(r.Context(), store.ListRunsParams{Limit: 1000, Offset: 0})
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to check runs: %v", err), http.StatusInternalServerError)
			slog.Error("archive mod: check runs failed", "mod_id", modIDStr, "err", err)
			return
		}

		// Check if any run has running jobs
		for _, run := range allRuns {
			if run.ModID != modIDStr {
				continue
			}
			jobs, err := st.ListJobsByRun(r.Context(), run.ID)
			if err != nil {
				http.Error(w, fmt.Sprintf("failed to check jobs: %v", err), http.StatusInternalServerError)
				slog.Error("archive mod: check jobs failed", "mod_id", modIDStr, "run_id", run.ID, "err", err)
				return
			}
			for _, job := range jobs {
				if job.Status == store.JobStatusRunning || job.Status == store.JobStatusQueued {
					http.Error(w, "cannot archive mod with running jobs", http.StatusConflict)
					return
				}
			}
		}

		// Archive the mod
		if err := st.ArchiveMod(r.Context(), modIDStr); err != nil {
			http.Error(w, fmt.Sprintf("failed to archive mod: %v", err), http.StatusInternalServerError)
			slog.Error("archive mod: database error", "mod_id", modIDStr, "err", err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		resp := struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Archived bool   `json:"archived"`
		}{
			ID:       mod.ID,
			Name:     mod.Name,
			Archived: true,
		}
		_ = json.NewEncoder(w).Encode(resp)

		slog.Info("mod archived", "mod_id", modIDStr)
	}
}

// unarchiveModHandler unarchives a mod project.
// Endpoint: PATCH /v1/mods/{mod_id}/unarchive
// Response: 200 OK with mod details
//
// v1 contract (roadmap/v1/api.md:89-91):
// - Unarchives a mod (allows execution again).
func unarchiveModHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		modIDStr, err := requiredPathParam(r, "mod_id")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Check if mod exists
		mod, err := st.GetMod(r.Context(), modIDStr)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "mod not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get mod: %v", err), http.StatusInternalServerError)
			slog.Error("unarchive mod: get mod failed", "mod_id", modIDStr, "err", err)
			return
		}

		// Check if not archived
		if !mod.ArchivedAt.Valid {
			// Already unarchived, return current state (idempotent)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			resp := struct {
				ID       string `json:"id"`
				Name     string `json:"name"`
				Archived bool   `json:"archived"`
			}{
				ID:       mod.ID,
				Name:     mod.Name,
				Archived: false,
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		// Unarchive the mod
		if err := st.UnarchiveMod(r.Context(), modIDStr); err != nil {
			http.Error(w, fmt.Sprintf("failed to unarchive mod: %v", err), http.StatusInternalServerError)
			slog.Error("unarchive mod: database error", "mod_id", modIDStr, "err", err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		resp := struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Archived bool   `json:"archived"`
		}{
			ID:       mod.ID,
			Name:     mod.Name,
			Archived: false,
		}
		_ = json.NewEncoder(w).Encode(resp)

		slog.Info("mod unarchived", "mod_id", modIDStr)
	}
}
