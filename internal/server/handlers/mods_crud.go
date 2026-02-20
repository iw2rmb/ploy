package handlers

import (
	"context"
	"encoding/json"
	"errors"
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

// maxModSpecSize is the body size limit for mod spec creation endpoints.
// Specs can be large (JSON blobs), so we allow up to 4 MiB.
const maxModSpecSize = 4 << 20

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

		if err := DecodeJSON(w, r, &req, maxModSpecSize); err != nil {
			return
		}

		// Validate and normalize name.
		name := strings.TrimSpace(req.Name)
		if name == "" {
			httpErr(w, http.StatusBadRequest, "name is required")
			return
		}
		if err := domaintypes.ModRef(name).Validate(); err != nil {
			httpErr(w, http.StatusBadRequest, "name: %v", err)
			return
		}

		// Validate spec early so invalid specs do not create a mod row.
		if req.Spec != nil && len(*req.Spec) > 0 {
			if _, err := contracts.ParseModsSpecJSON(*req.Spec); err != nil {
				httpErr(w, http.StatusBadRequest, "spec: %v", err)
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
				httpErr(w, http.StatusConflict, "mod with this name already exists")
				return
			}
			httpErr(w, http.StatusInternalServerError, "failed to create mod: %v", err)
			slog.Error("create mod: create mod failed", "mod_id", modID.String(), "err", err)
			return
		}

		// Create spec if provided and attach it to the mod.
		var specIDPtr *domaintypes.SpecID
		if req.Spec != nil && len(*req.Spec) > 0 {
			specID := domaintypes.NewSpecID()
			createdSpec, err := st.CreateSpec(r.Context(), store.CreateSpecParams{
				ID:        specID,
				Name:      "",
				Spec:      *req.Spec,
				CreatedBy: req.CreatedBy,
			})
			if err != nil {
				httpErr(w, http.StatusInternalServerError, "failed to create spec: %v", err)
				slog.Error("create mod: create spec failed", "mod_id", modID.String(), "err", err)
				return
			}
			if err := st.UpdateModSpec(r.Context(), store.UpdateModSpecParams{ID: modID, SpecID: &createdSpec.ID}); err != nil {
				httpErr(w, http.StatusInternalServerError, "failed to update mod spec: %v", err)
				slog.Error("create mod: update spec failed", "mod_id", modID.String(), "spec_id", createdSpec.ID.String(), "err", err)
				return
			}
			createdID := createdSpec.ID
			specIDPtr = &createdID
		}

		// Build response
		resp := struct {
			ID        string              `json:"id"`
			Name      string              `json:"name"`
			SpecID    *domaintypes.SpecID `json:"spec_id,omitempty"`
			CreatedBy *string             `json:"created_by,omitempty"`
			CreatedAt string              `json:"created_at"`
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
				httpErr(w, http.StatusBadRequest, "invalid limit parameter")
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
				httpErr(w, http.StatusBadRequest, "invalid offset parameter")
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
				httpErr(w, http.StatusBadRequest, "invalid archived parameter")
				return
			}
			archivedOnly = &parsed
		}

		repoURLFilter := strings.TrimSpace(r.URL.Query().Get("repo_url"))
		if repoURLFilter != "" {
			repoURLFilter = vcs.NormalizeRepoURL(repoURLFilter)
			if err := domaintypes.RepoURL(repoURLFilter).Validate(); err != nil {
				httpErr(w, http.StatusBadRequest, "repo_url: %v", err)
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
					httpErr(w, http.StatusInternalServerError, "failed to list mods: %v", err)
					slog.Error("list mods: fetch failed", "err", err)
					return
				}
				if len(page) == 0 {
					break
				}
				for _, mod := range page {
					repos, err := st.ListModReposByMod(r.Context(), mod.ID)
					if err != nil {
						httpErr(w, http.StatusInternalServerError, "failed to list mod repos: %v", err)
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
			httpErr(w, http.StatusInternalServerError, "failed to list mods: %v", err)
			slog.Error("list mods: fetch failed", "err", err)
			return
		}

		writeModsListResponse(w, mods)
	}
}

func writeModsListResponse(w http.ResponseWriter, mods []store.Mod) {
	type modItem struct {
		ID        string              `json:"id"`
		Name      string              `json:"name"`
		SpecID    *domaintypes.SpecID `json:"spec_id,omitempty"`
		CreatedBy *string             `json:"created_by,omitempty"`
		Archived  bool                `json:"archived"`
		CreatedAt string              `json:"created_at"`
	}

	items := make([]modItem, 0, len(mods))
	for _, mod := range mods {
		var specIDPtr *domaintypes.SpecID
		if mod.SpecID != nil && !mod.SpecID.IsZero() {
			specIDPtr = mod.SpecID
		}
		items = append(items, modItem{
			ID:        mod.ID.String(),
			Name:      mod.Name,
			SpecID:    specIDPtr,
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
		modRef, err := domaintypes.ParseModRefParam(r, "mod_ref")
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		}

		// Resolve mod by ID-or-name.
		mod, err := resolveModByRef(r.Context(), st, modRef)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				httpErr(w, http.StatusNotFound, "mod not found")
				return
			}
			httpErr(w, http.StatusInternalServerError, "failed to get mod: %v", err)
			slog.Error("delete mod: get mod failed", "mod_ref", modRef, "err", err)
			return
		}
		modID := mod.ID

		// Check if any runs exist for this mod
		hasRuns, err := modHasAnyRuns(r.Context(), st, modID)
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to check runs: %v", err)
			slog.Error("delete mod: check runs failed", "mod_id", modID, "err", err)
			return
		}
		if hasRuns {
			httpErr(w, http.StatusConflict, "cannot delete mod with existing runs")
			return
		}

		// Delete the mod
		if err := st.DeleteMod(r.Context(), modID); err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to delete mod: %v", err)
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
