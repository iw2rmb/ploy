package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// archiveMigHandler archives a mod project.
// Endpoint: PATCH /v1/migs/{mig_ref}/archive
// Response: 200 OK with mod details
//
// v1 contract:
// - Archives a mod (prevents execution).
// - Cannot archive a mod with running jobs.
func archiveMigHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		modRef, err := parseParam[domaintypes.MigRef](r, "mig_ref")
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		}

		// Resolve mod by ID-or-name.
		mod, err := resolveMigByRef(r.Context(), st, modRef)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				httpErr(w, http.StatusNotFound, "mod not found")
				return
			}
			httpErr(w, http.StatusInternalServerError, "failed to get mod: %v", err)
			slog.Error("archive mod: get mod failed", "mig_ref", modRef, "err", err)
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
			httpErr(w, http.StatusInternalServerError, "failed to check jobs: %v", err)
			slog.Error("archive mod: check jobs failed", "mig_id", modID, "err", err)
			return
		}
		if hasRunningJobs {
			httpErr(w, http.StatusConflict, "cannot archive mod with running jobs")
			return
		}

		// Archive the mod
		if err := st.ArchiveMig(r.Context(), modID); err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to archive mod: %v", err)
			slog.Error("archive mod: database error", "mig_id", modID, "err", err)
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

		slog.Info("mod archived", "mig_id", modID)
	}
}

func modHasAnyRunningJobs(ctx context.Context, st store.Store, modID domaintypes.MigID) (bool, error) {
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
			if run.MigID != modID {
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

// unarchiveMigHandler unarchives a mod project.
// Endpoint: PATCH /v1/migs/{mig_ref}/unarchive
// Response: 200 OK with mod details
//
// v1 contract:
// - Unarchives a mod (allows execution again).
func unarchiveMigHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		modRef, err := parseParam[domaintypes.MigRef](r, "mig_ref")
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		}

		// Resolve mod by ID-or-name.
		mod, err := resolveMigByRef(r.Context(), st, modRef)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				httpErr(w, http.StatusNotFound, "mod not found")
				return
			}
			httpErr(w, http.StatusInternalServerError, "failed to get mod: %v", err)
			slog.Error("unarchive mod: get mod failed", "mig_ref", modRef, "err", err)
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
		if err := st.UnarchiveMig(r.Context(), modID); err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to unarchive mod: %v", err)
			slog.Error("unarchive mod: database error", "mig_id", modID, "err", err)
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

		slog.Info("mod unarchived", "mig_id", modID)
	}
}
