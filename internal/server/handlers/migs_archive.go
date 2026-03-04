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

// archiveMigHandler archives a mig project.
// Endpoint: PATCH /v1/migs/{mig_ref}/archive
// Response: 200 OK with mig details
//
// v1 contract:
// - Archives a mig (prevents execution).
// - Cannot archive a mig with running jobs.
func archiveMigHandler(st store.Store) http.HandlerFunc {
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
			slog.Error("archive mig: get mig failed", "mig_ref", modRef, "err", err)
			return
		}
		modID := mig.ID

		// Check if already archived
		if mig.ArchivedAt.Valid {
			// Already archived, return current state (idempotent)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			resp := struct {
				ID       string `json:"id"`
				Name     string `json:"name"`
				Archived bool   `json:"archived"`
			}{
				ID:       mig.ID.String(),
				Name:     mig.Name,
				Archived: true,
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		// Check for running jobs in this mig's runs
		hasRunningJobs, err := modHasAnyRunningJobs(r.Context(), st, modID)
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to check jobs: %v", err)
			slog.Error("archive mig: check jobs failed", "mig_id", modID, "err", err)
			return
		}
		if hasRunningJobs {
			httpErr(w, http.StatusConflict, "cannot archive mig with running jobs")
			return
		}

		// Archive the mig
		if err := st.ArchiveMig(r.Context(), modID); err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to archive mig: %v", err)
			slog.Error("archive mig: database error", "mig_id", modID, "err", err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		resp := struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Archived bool   `json:"archived"`
		}{
			ID:       mig.ID.String(),
			Name:     mig.Name,
			Archived: true,
		}
		_ = json.NewEncoder(w).Encode(resp)

		slog.Info("mig archived", "mig_id", modID)
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
				if job.Status == domaintypes.JobStatusRunning || job.Status == domaintypes.JobStatusQueued {
					return true, nil
				}
			}
		}
		pageOffset += pageLimit
	}
}

// unarchiveMigHandler unarchives a mig project.
// Endpoint: PATCH /v1/migs/{mig_ref}/unarchive
// Response: 200 OK with mig details
//
// v1 contract:
// - Unarchives a mig (allows execution again).
func unarchiveMigHandler(st store.Store) http.HandlerFunc {
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
			slog.Error("unarchive mig: get mig failed", "mig_ref", modRef, "err", err)
			return
		}
		modID := mig.ID

		// Check if not archived
		if !mig.ArchivedAt.Valid {
			// Already unarchived, return current state (idempotent)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			resp := struct {
				ID       string `json:"id"`
				Name     string `json:"name"`
				Archived bool   `json:"archived"`
			}{
				ID:       mig.ID.String(),
				Name:     mig.Name,
				Archived: false,
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		// Unarchive the mig
		if err := st.UnarchiveMig(r.Context(), modID); err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to unarchive mig: %v", err)
			slog.Error("unarchive mig: database error", "mig_id", modID, "err", err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		resp := struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Archived bool   `json:"archived"`
		}{
			ID:       mig.ID.String(),
			Name:     mig.Name,
			Archived: false,
		}
		_ = json.NewEncoder(w).Encode(resp)

		slog.Info("mig unarchived", "mig_id", modID)
	}
}
