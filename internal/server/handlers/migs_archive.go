package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

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
		mig, ok := getMigByRefOrFail(w, r, st, "archive mig")
		if !ok {
			return
		}

		// Already archived — return current state (idempotent).
		if mig.ArchivedAt.Valid {
			writeMigArchiveResponse(w, mig, true)
			return
		}

		// Check for running jobs in this mig's runs.
		hasRunningJobs, err := modHasAnyRunningJobs(r.Context(), st, mig.ID)
		if err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to check jobs: %v", err)
			slog.Error("archive mig: check jobs failed", "mig_id", mig.ID, "err", err)
			return
		}
		if hasRunningJobs {
			writeHTTPError(w, http.StatusConflict, "cannot archive mig with running jobs")
			return
		}

		if err := st.ArchiveMig(r.Context(), mig.ID); err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to archive mig: %v", err)
			slog.Error("archive mig: database error", "mig_id", mig.ID, "err", err)
			return
		}

		writeMigArchiveResponse(w, mig, true)
		slog.Info("mig archived", "mig_id", mig.ID)
	}
}

func modHasAnyRunningJobs(ctx context.Context, st store.Store, modID domaintypes.MigID) (bool, error) {
	return scanRunPages(ctx, st, func(run store.Run) (bool, error) {
		if run.MigID != modID {
			return false, nil
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
		return false, nil
	})
}

// unarchiveMigHandler unarchives a mig project.
// Endpoint: PATCH /v1/migs/{mig_ref}/unarchive
// Response: 200 OK with mig details
//
// v1 contract:
// - Unarchives a mig (allows execution again).
func unarchiveMigHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mig, ok := getMigByRefOrFail(w, r, st, "unarchive mig")
		if !ok {
			return
		}

		// Already unarchived — return current state (idempotent).
		if !mig.ArchivedAt.Valid {
			writeMigArchiveResponse(w, mig, false)
			return
		}

		if err := st.UnarchiveMig(r.Context(), mig.ID); err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to unarchive mig: %v", err)
			slog.Error("unarchive mig: database error", "mig_id", mig.ID, "err", err)
			return
		}

		writeMigArchiveResponse(w, mig, false)
		slog.Info("mig unarchived", "mig_id", mig.ID)
	}
}

// writeMigArchiveResponse writes the standard archive/unarchive JSON response.
func writeMigArchiveResponse(w http.ResponseWriter, mig store.Mig, archived bool) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Archived bool   `json:"archived"`
	}{
		ID:       mig.ID.String(),
		Name:     mig.Name,
		Archived: archived,
	})
}
