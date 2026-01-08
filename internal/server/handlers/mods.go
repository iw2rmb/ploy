package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/vcs"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// createModHandler creates a new mod project.
// Endpoint: POST /v1/mods
// Request: {name, spec?}
// Response: 201 Created with mod details
//
// v1 contract:
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

		// Validate and normalize name.
		name := strings.TrimSpace(req.Name)
		if name == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		if err := domaintypes.ModRef(name).Validate(); err != nil {
			http.Error(w, fmt.Sprintf("name: %v", err), http.StatusBadRequest)
			return
		}

		// Validate spec early so invalid specs do not create a mod row.
		if req.Spec != nil && len(*req.Spec) > 0 {
			if _, err := contracts.ParseModsSpecJSON(*req.Spec); err != nil {
				http.Error(w, fmt.Sprintf("spec: %v", err), http.StatusBadRequest)
				return
			}
		}

		// Create mod (create spec only after the mod row exists to avoid creating
		// orphaned specs on mod-name collisions).
		modID := domaintypes.NewModID()
		mod, err := st.CreateMod(r.Context(), store.CreateModParams{
			ID:        modID,
			Name:      name,
			SpecID:    nil,
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
			slog.Error("create mod: create mod failed", "mod_id", modID.String(), "err", err)
			return
		}

		// Create spec if provided and attach it to the mod.
		var specIDPtr *string
		if req.Spec != nil && len(*req.Spec) > 0 {
			specID := domaintypes.NewSpecID()
			createdSpec, err := st.CreateSpec(r.Context(), store.CreateSpecParams{
				ID:        specID,
				Name:      "",
				Spec:      *req.Spec,
				CreatedBy: req.CreatedBy,
			})
			if err != nil {
				http.Error(w, fmt.Sprintf("failed to create spec: %v", err), http.StatusInternalServerError)
				slog.Error("create mod: create spec failed", "mod_id", modID.String(), "err", err)
				return
			}
			if err := st.UpdateModSpec(r.Context(), store.UpdateModSpecParams{ID: modID, SpecID: &createdSpec.ID}); err != nil {
				http.Error(w, fmt.Sprintf("failed to update mod spec: %v", err), http.StatusInternalServerError)
				slog.Error("create mod: update spec failed", "mod_id", modID.String(), "spec_id", createdSpec.ID.String(), "err", err)
				return
			}
			s := createdSpec.ID.String()
			specIDPtr = &s
		}

		// Build response
		resp := struct {
			ID        string  `json:"id"`
			Name      string  `json:"name"`
			SpecID    *string `json:"spec_id,omitempty"`
			CreatedBy *string `json:"created_by,omitempty"`
			CreatedAt string  `json:"created_at"`
		}{
			ID:        mod.ID.String(),
			Name:      mod.Name,
			SpecID:    specIDPtr,
			CreatedBy: mod.CreatedBy,
			CreatedAt: mod.CreatedAt.Time.Format("2006-01-02T15:04:05Z07:00"),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("create mod: encode response failed", "err", err)
		}

		slog.Info("mod created", "mod_id", mod.ID.String(), "name", mod.Name)
	}
}

func resolveModByRef(ctx context.Context, st store.Store, ref domaintypes.ModRef) (store.Mod, error) {
	// Prefer direct ID lookup; fall back to exact name lookup.
	mod, err := st.GetMod(ctx, domaintypes.ModID(ref.String()))
	if err == nil {
		return mod, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return store.Mod{}, err
	}
	return st.GetModByName(ctx, ref.String())
}

// listModsHandler lists mod projects with optional filters.
// Endpoint: GET /v1/mods
// Query params: limit, offset, name_substring, archived, repo_url
// Response: 200 OK with list of mods
//
// v1 contract:
// - Supports pagination with limit/offset.
// - Optional filters: name_substring, archived (true/false), repo_url.
func listModsHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := int32(50)
		offset := int32(0)

		if l := r.URL.Query().Get("limit"); l != "" {
			parsed, err := strconv.ParseInt(l, 10, 32)
			if err != nil || parsed < 1 {
				http.Error(w, "invalid limit parameter", http.StatusBadRequest)
				return
			}
			limit = int32(parsed)
			if limit > 100 {
				limit = 100
			}
		}
		if o := r.URL.Query().Get("offset"); o != "" {
			parsed, err := strconv.ParseInt(o, 10, 32)
			if err != nil || parsed < 0 {
				http.Error(w, "invalid offset parameter", http.StatusBadRequest)
				return
			}
			offset = int32(parsed)
		}

		nameSubstring := strings.TrimSpace(r.URL.Query().Get("name_substring"))
		var nameFilter *string
		if nameSubstring != "" {
			nameFilter = &nameSubstring
		}

		archivedStr := strings.TrimSpace(r.URL.Query().Get("archived"))
		var archivedOnly *bool
		if archivedStr != "" {
			parsed, err := strconv.ParseBool(archivedStr)
			if err != nil {
				http.Error(w, "invalid archived parameter", http.StatusBadRequest)
				return
			}
			archivedOnly = &parsed
		}

		repoURLFilter := strings.TrimSpace(r.URL.Query().Get("repo_url"))
		if repoURLFilter != "" {
			repoURLFilter = vcs.NormalizeRepoURL(repoURLFilter)
			if err := domaintypes.RepoURL(repoURLFilter).Validate(); err != nil {
				http.Error(w, fmt.Sprintf("repo_url: %v", err), http.StatusBadRequest)
				return
			}
		}

		// Repo URL filtering is implemented in the handler because the store does
		// not currently provide a repo_url-filtered ListMods query.
		if repoURLFilter != "" {
			// Fetch all mods matching archived/name filters, then filter by repo_url membership.
			const pageLimit = int32(200)
			pageOffset := int32(0)
			var filtered []store.Mod
			for {
				page, err := st.ListMods(r.Context(), store.ListModsParams{
					Limit:        pageLimit,
					Offset:       pageOffset,
					ArchivedOnly: archivedOnly,
					NameFilter:   nameFilter,
				})
				if err != nil {
					http.Error(w, fmt.Sprintf("failed to list mods: %v", err), http.StatusInternalServerError)
					slog.Error("list mods: fetch failed", "err", err)
					return
				}
				if len(page) == 0 {
					break
				}
				for _, mod := range page {
					repos, err := st.ListModReposByMod(r.Context(), mod.ID)
					if err != nil {
						http.Error(w, fmt.Sprintf("failed to list mod repos: %v", err), http.StatusInternalServerError)
						slog.Error("list mods: list mod repos failed", "mod_id", mod.ID, "err", err)
						return
					}
					for _, mr := range repos {
						if vcs.NormalizeRepoURL(mr.RepoUrl) == repoURLFilter {
							filtered = append(filtered, mod)
							break
						}
					}
				}
				pageOffset += pageLimit
			}

			// Apply pagination after filtering.
			start := int(offset)
			if start > len(filtered) {
				start = len(filtered)
			}
			end := start + int(limit)
			if end > len(filtered) {
				end = len(filtered)
			}
			mods := filtered[start:end]

			writeModsListResponse(w, mods)
			return
		}

		mods, err := st.ListMods(r.Context(), store.ListModsParams{
			Limit:        limit,
			Offset:       offset,
			ArchivedOnly: archivedOnly,
			NameFilter:   nameFilter,
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to list mods: %v", err), http.StatusInternalServerError)
			slog.Error("list mods: fetch failed", "err", err)
			return
		}

		writeModsListResponse(w, mods)
	}
}

func writeModsListResponse(w http.ResponseWriter, mods []store.Mod) {
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
		var specIDStrPtr *string
		if mod.SpecID != nil && !mod.SpecID.IsZero() {
			s := mod.SpecID.String()
			specIDStrPtr = &s
		}
		items = append(items, modItem{
			ID:        mod.ID.String(),
			Name:      mod.Name,
			SpecID:    specIDStrPtr,
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

// deleteModHandler deletes a mod project.
// Endpoint: DELETE /v1/mods/{mod_ref}
// Response: 204 No Content on success
//
// v1 contract:
// - Refuses deletion if any runs exist for the mod.
func deleteModHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		modRefStr, err := requiredPathParam(r, "mod_ref")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		modRef := domaintypes.ModRef(modRefStr)
		if err := modRef.Validate(); err != nil {
			http.Error(w, fmt.Sprintf("mod_ref: %v", err), http.StatusBadRequest)
			return
		}

		// Resolve mod by ID-or-name.
		mod, err := resolveModByRef(r.Context(), st, modRef)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "mod not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get mod: %v", err), http.StatusInternalServerError)
			slog.Error("delete mod: get mod failed", "mod_ref", modRefStr, "err", err)
			return
		}
		modID := mod.ID

		// Check if any runs exist for this mod
		hasRuns, err := modHasAnyRuns(r.Context(), st, modID)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to check runs: %v", err), http.StatusInternalServerError)
			slog.Error("delete mod: check runs failed", "mod_id", modID, "err", err)
			return
		}
		if hasRuns {
			http.Error(w, "cannot delete mod with existing runs", http.StatusConflict)
			return
		}

		// Delete the mod
		if err := st.DeleteMod(r.Context(), modID); err != nil {
			http.Error(w, fmt.Sprintf("failed to delete mod: %v", err), http.StatusInternalServerError)
			slog.Error("delete mod: database error", "mod_id", modID, "err", err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
		slog.Info("mod deleted", "mod_id", modID)
	}
}

func modHasAnyRuns(ctx context.Context, st store.Store, modID domaintypes.ModID) (bool, error) {
	const pageLimit = int32(200)
	pageOffset := int32(0)
	for {
		page, err := st.ListRuns(ctx, store.ListRunsParams{Limit: pageLimit, Offset: pageOffset})
		if err != nil {
			return false, err
		}
		if len(page) == 0 {
			return false, nil
		}
		for _, run := range page {
			if run.ModID == modID {
				return true, nil
			}
		}
		pageOffset += pageLimit
	}
}

// archiveModHandler archives a mod project.
// Endpoint: PATCH /v1/mods/{mod_ref}/archive
// Response: 200 OK with mod details
//
// v1 contract:
// - Archives a mod (prevents execution).
// - Cannot archive a mod with running jobs.
func archiveModHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		modRefStr, err := requiredPathParam(r, "mod_ref")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		modRef := domaintypes.ModRef(modRefStr)
		if err := modRef.Validate(); err != nil {
			http.Error(w, fmt.Sprintf("mod_ref: %v", err), http.StatusBadRequest)
			return
		}

		// Resolve mod by ID-or-name.
		mod, err := resolveModByRef(r.Context(), st, modRef)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "mod not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get mod: %v", err), http.StatusInternalServerError)
			slog.Error("archive mod: get mod failed", "mod_ref", modRefStr, "err", err)
			return
		}
		modID := mod.ID

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
				ID:       mod.ID.String(),
				Name:     mod.Name,
				Archived: true,
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		// Check for running jobs in this mod's runs
		hasRunningJobs, err := modHasAnyRunningJobs(r.Context(), st, modID)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to check jobs: %v", err), http.StatusInternalServerError)
			slog.Error("archive mod: check jobs failed", "mod_id", modID, "err", err)
			return
		}
		if hasRunningJobs {
			http.Error(w, "cannot archive mod with running jobs", http.StatusConflict)
			return
		}

		// Archive the mod
		if err := st.ArchiveMod(r.Context(), modID); err != nil {
			http.Error(w, fmt.Sprintf("failed to archive mod: %v", err), http.StatusInternalServerError)
			slog.Error("archive mod: database error", "mod_id", modID, "err", err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		resp := struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Archived bool   `json:"archived"`
		}{
			ID:       mod.ID.String(),
			Name:     mod.Name,
			Archived: true,
		}
		_ = json.NewEncoder(w).Encode(resp)

		slog.Info("mod archived", "mod_id", modID)
	}
}

func modHasAnyRunningJobs(ctx context.Context, st store.Store, modID domaintypes.ModID) (bool, error) {
	const pageLimit = int32(200)
	pageOffset := int32(0)
	for {
		page, err := st.ListRuns(ctx, store.ListRunsParams{Limit: pageLimit, Offset: pageOffset})
		if err != nil {
			return false, err
		}
		if len(page) == 0 {
			return false, nil
		}
		for _, run := range page {
			if run.ModID != modID {
				continue
			}
			jobs, err := st.ListJobsByRun(ctx, run.ID)
			if err != nil {
				return false, err
			}
			for _, job := range jobs {
				if job.Status == store.JobStatusRunning || job.Status == store.JobStatusQueued {
					return true, nil
				}
			}
		}
		pageOffset += pageLimit
	}
}

// setModSpecHandler creates a new spec row and updates mods.spec_id to point at it.
// Endpoint: POST /v1/mods/{mod_ref}/specs
// Request: {name?, spec}
// Response: 201 Created with spec details (id, created_at)
//
// v1 contract:
// - Specs are append-only: each call inserts a new specs row.
// - mods.spec_id is updated to point at the newly created spec.
// - This is the canonical way to "set" or "update" a mod's spec.
func setModSpecHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		modRefStr, err := requiredPathParam(r, "mod_ref")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		modRef := domaintypes.ModRef(modRefStr)
		if err := modRef.Validate(); err != nil {
			http.Error(w, fmt.Sprintf("mod_ref: %v", err), http.StatusBadRequest)
			return
		}

		// Parse request body.
		var req struct {
			Name      string          `json:"name,omitempty"`
			Spec      json.RawMessage `json:"spec"`
			CreatedBy *string         `json:"created_by,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
			return
		}

		// Validate spec is present and non-empty.
		if len(req.Spec) == 0 {
			http.Error(w, "spec is required", http.StatusBadRequest)
			return
		}

		// Validate spec structure (same validation as in createModHandler).
		if _, err := contracts.ParseModsSpecJSON(req.Spec); err != nil {
			http.Error(w, fmt.Sprintf("spec: %v", err), http.StatusBadRequest)
			return
		}

		// Resolve mod by ID-or-name.
		mod, err := resolveModByRef(r.Context(), st, modRef)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "mod not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get mod: %v", err), http.StatusInternalServerError)
			slog.Error("set mod spec: get mod failed", "mod_ref", modRefStr, "err", err)
			return
		}
		modID := mod.ID

		// Check if mod is archived — cannot update spec on archived mods.
		if mod.ArchivedAt.Valid {
			http.Error(w, "cannot set spec on archived mod", http.StatusConflict)
			return
		}

		// Create new spec row (append-only).
		specID := domaintypes.NewSpecID()
		createdSpec, err := st.CreateSpec(r.Context(), store.CreateSpecParams{
			ID:        specID,
			Name:      req.Name,
			Spec:      req.Spec,
			CreatedBy: req.CreatedBy,
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to create spec: %v", err), http.StatusInternalServerError)
			slog.Error("set mod spec: create spec failed", "mod_id", modID, "err", err)
			return
		}

		// Update mods.spec_id to point at the new spec.
		if err := st.UpdateModSpec(r.Context(), store.UpdateModSpecParams{ID: modID, SpecID: &createdSpec.ID}); err != nil {
			http.Error(w, fmt.Sprintf("failed to update mod spec: %v", err), http.StatusInternalServerError)
			slog.Error("set mod spec: update mod failed", "mod_id", modID, "spec_id", createdSpec.ID, "err", err)
			return
		}

		// Build response with spec ID and creation timestamp.
		resp := struct {
			ID        string `json:"id"`
			CreatedAt string `json:"created_at"`
		}{
			ID:        createdSpec.ID.String(),
			CreatedAt: createdSpec.CreatedAt.Time.Format("2006-01-02T15:04:05Z07:00"),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("set mod spec: encode response failed", "err", err)
		}

		slog.Info("mod spec set", "mod_id", modID, "spec_id", createdSpec.ID.String())
	}
}

// unarchiveModHandler unarchives a mod project.
// Endpoint: PATCH /v1/mods/{mod_ref}/unarchive
// Response: 200 OK with mod details
//
// v1 contract:
// - Unarchives a mod (allows execution again).
func unarchiveModHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		modRefStr, err := requiredPathParam(r, "mod_ref")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		modRef := domaintypes.ModRef(modRefStr)
		if err := modRef.Validate(); err != nil {
			http.Error(w, fmt.Sprintf("mod_ref: %v", err), http.StatusBadRequest)
			return
		}

		// Resolve mod by ID-or-name.
		mod, err := resolveModByRef(r.Context(), st, modRef)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "mod not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get mod: %v", err), http.StatusInternalServerError)
			slog.Error("unarchive mod: get mod failed", "mod_ref", modRefStr, "err", err)
			return
		}
		modID := mod.ID

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
				ID:       mod.ID.String(),
				Name:     mod.Name,
				Archived: false,
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		// Unarchive the mod
		if err := st.UnarchiveMod(r.Context(), modID); err != nil {
			http.Error(w, fmt.Sprintf("failed to unarchive mod: %v", err), http.StatusInternalServerError)
			slog.Error("unarchive mod: database error", "mod_id", modID, "err", err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		resp := struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Archived bool   `json:"archived"`
		}{
			ID:       mod.ID.String(),
			Name:     mod.Name,
			Archived: false,
		}
		_ = json.NewEncoder(w).Encode(resp)

		slog.Info("mod unarchived", "mod_id", modID)
	}
}
