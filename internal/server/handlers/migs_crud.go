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
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// maxModSpecSize is the body size limit for mig spec creation endpoints.
// Specs can be large (JSON blobs), so we allow up to 4 MiB.
const maxModSpecSize = 4 << 20

// createMigHandler creates a new mig project.
// Endpoint: POST /v1/migs
// Request: {name, spec?}
// Response: 201 Created with mig details
//
// v1 contract:
// - Creates a mig project with a unique name.
// - Optional spec parameter creates an initial spec row and sets migs.spec_id.
func createMigHandler(st store.Store) http.HandlerFunc {
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
		if err := domaintypes.MigRef(name).Validate(); err != nil {
			httpErr(w, http.StatusBadRequest, "name: %v", err)
			return
		}

		// Validate spec early so invalid specs do not create a mig row.
		if req.Spec != nil && len(*req.Spec) > 0 {
			if _, err := contracts.ParseModsSpecJSON(*req.Spec); err != nil {
				httpErr(w, http.StatusBadRequest, "spec: %v", err)
				return
			}
		}

		// Create mig (create spec only after the mig row exists to avoid creating
		// orphaned specs on mig-name collisions).
		modID := domaintypes.NewMigID()
		mig, err := st.CreateMig(r.Context(), store.CreateMigParams{
			ID:        modID,
			Name:      name,
			SpecID:    nil,
			CreatedBy: req.CreatedBy,
		})
		if err != nil {
			// Check for unique constraint violation (duplicate name)
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				httpErr(w, http.StatusConflict, "mig with this name already exists")
				return
			}
			httpErr(w, http.StatusInternalServerError, "failed to create mig: %v", err)
			slog.Error("create mig: create mig failed", "mig_id", modID.String(), "err", err)
			return
		}

		// Create spec if provided and attach it to the mig.
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
				slog.Error("create mig: create spec failed", "mig_id", modID.String(), "err", err)
				return
			}
			if err := st.UpdateMigSpec(r.Context(), store.UpdateMigSpecParams{ID: modID, SpecID: &createdSpec.ID}); err != nil {
				httpErr(w, http.StatusInternalServerError, "failed to update mig spec: %v", err)
				slog.Error("create mig: update spec failed", "mig_id", modID.String(), "spec_id", createdSpec.ID.String(), "err", err)
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
			ID:        mig.ID.String(),
			Name:      mig.Name,
			SpecID:    specIDPtr,
			CreatedBy: mig.CreatedBy,
			CreatedAt: mig.CreatedAt.Time.Format("2006-01-02T15:04:05Z07:00"),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("create mig: encode response failed", "err", err)
		}

		slog.Info("mig created", "mig_id", mig.ID.String(), "name", mig.Name)
	}
}

func resolveMigByRef(ctx context.Context, st store.Store, ref domaintypes.MigRef) (store.Mig, error) {
	// Prefer direct ID lookup; fall back to exact name lookup.
	mig, err := st.GetMig(ctx, domaintypes.MigID(ref.String()))
	if err == nil {
		return mig, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return store.Mig{}, err
	}
	return st.GetMigByName(ctx, ref.String())
}

// listMigsHandler lists mig projects with optional filters.
// Endpoint: GET /v1/migs
// Query params: limit, offset, name_substring, archived, repo_url
// Response: 200 OK with list of migs
//
// v1 contract:
// - Supports pagination with limit/offset.
// - Optional filters: name_substring, archived (true/false), repo_url.
func listMigsHandler(st store.Store) http.HandlerFunc {
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
			repoURLFilter = domaintypes.NormalizeRepoURL(repoURLFilter)
			if err := domaintypes.RepoURL(repoURLFilter).Validate(); err != nil {
				httpErr(w, http.StatusBadRequest, "repo_url: %v", err)
				return
			}
		}

		// Repo URL filtering is implemented in the handler because the store does
		// not currently provide a repo_url-filtered ListMigs query.
		if repoURLFilter != "" {
			// Fetch all migs matching archived/name filters, then filter by repo_url membership.
			const pageLimit = int32(200)
			pageOffset := int32(0)
			var filtered []store.Mig
			for {
				page, err := st.ListMigs(r.Context(), store.ListMigsParams{
					Limit:        pageLimit,
					Offset:       pageOffset,
					ArchivedOnly: archivedOnly,
					NameFilter:   nameFilter,
				})
				if err != nil {
					httpErr(w, http.StatusInternalServerError, "failed to list migs: %v", err)
					slog.Error("list migs: fetch failed", "err", err)
					return
				}
				if len(page) == 0 {
					break
				}
				for _, mig := range page {
					repos, err := st.ListMigReposByMig(r.Context(), mig.ID)
					if err != nil {
						httpErr(w, http.StatusInternalServerError, "failed to list mig repos: %v", err)
						slog.Error("list migs: list mig repos failed", "mig_id", mig.ID, "err", err)
						return
					}
					for _, mr := range repos {
						repoURL, err := repoURLForID(r.Context(), st, mr.RepoID)
						if err != nil {
							httpErr(w, http.StatusInternalServerError, "failed to get repo: %v", err)
							slog.Error("list migs: get repo failed", "mig_id", mig.ID, "repo_id", mr.RepoID, "err", err)
							return
						}
						if domaintypes.NormalizeRepoURL(repoURL) == repoURLFilter {
							filtered = append(filtered, mig)
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
			migs := filtered[start:end]

			writeModsListResponse(w, migs)
			return
		}

		migs, err := st.ListMigs(r.Context(), store.ListMigsParams{
			Limit:        limit,
			Offset:       offset,
			ArchivedOnly: archivedOnly,
			NameFilter:   nameFilter,
		})
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to list migs: %v", err)
			slog.Error("list migs: fetch failed", "err", err)
			return
		}

		writeModsListResponse(w, migs)
	}
}

func writeModsListResponse(w http.ResponseWriter, migs []store.Mig) {
	type modItem struct {
		ID        string              `json:"id"`
		Name      string              `json:"name"`
		SpecID    *domaintypes.SpecID `json:"spec_id,omitempty"`
		CreatedBy *string             `json:"created_by,omitempty"`
		Archived  bool                `json:"archived"`
		CreatedAt string              `json:"created_at"`
	}

	items := make([]modItem, 0, len(migs))
	for _, mig := range migs {
		var specIDPtr *domaintypes.SpecID
		if mig.SpecID != nil && !mig.SpecID.IsZero() {
			specIDPtr = mig.SpecID
		}
		items = append(items, modItem{
			ID:        mig.ID.String(),
			Name:      mig.Name,
			SpecID:    specIDPtr,
			CreatedBy: mig.CreatedBy,
			Archived:  mig.ArchivedAt.Valid,
			CreatedAt: mig.CreatedAt.Time.Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	resp := struct {
		Migs []modItem `json:"migs"`
	}{Migs: items}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.Error("list migs: encode response failed", "err", err)
	}
}

// deleteMigHandler deletes a mig project.
// Endpoint: DELETE /v1/migs/{mig_ref}
// Response: 204 No Content on success
//
// v1 contract:
// - Refuses deletion if any runs exist for the mig.
func deleteMigHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		modRef, err := parseParam[domaintypes.MigRef](r, "mig_ref")
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		}

		// Resolve mig by ID-or-name.
		mig, err := resolveMigByRef(r.Context(), st, modRef)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				httpErr(w, http.StatusNotFound, "mig not found")
				return
			}
			httpErr(w, http.StatusInternalServerError, "failed to get mig: %v", err)
			slog.Error("delete mig: get mig failed", "mig_ref", modRef, "err", err)
			return
		}
		modID := mig.ID

		// Check if any runs exist for this mig
		hasRuns, err := migHasAnyRuns(r.Context(), st, modID)
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to check runs: %v", err)
			slog.Error("delete mig: check runs failed", "mig_id", modID, "err", err)
			return
		}
		if hasRuns {
			httpErr(w, http.StatusConflict, "cannot delete mig with existing runs")
			return
		}

		// Delete the mig
		if err := st.DeleteMig(r.Context(), modID); err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to delete mig: %v", err)
			slog.Error("delete mig: database error", "mig_id", modID, "err", err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
		slog.Info("mig deleted", "mig_id", modID)
	}
}

func migHasAnyRuns(ctx context.Context, st store.Store, modID domaintypes.MigID) (bool, error) {
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
			if run.MigID == modID {
				return true, nil
			}
		}
		pageOffset += pageLimit
	}
}
