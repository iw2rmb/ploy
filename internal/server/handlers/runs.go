package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// getRunTimingHandler returns an HTTP handler that retrieves timing data for a run.
//
// Run IDs are now KSUID-backed strings; no UUID parsing is performed.
// IDs are treated as opaque; validation is limited to non-empty checks.
func getRunTimingHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Accept id from path parameter first, then fallback to query parameter.
		// Uses domain type helpers for validation at the boundary.
		var runID domaintypes.RunID
		if idPtr, err := domaintypes.OptionalRunIDParam(r, "id"); err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		} else if idPtr != nil {
			runID = *idPtr
		} else if qID, err := domaintypes.OptionalRunIDQuery(r, "id"); err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		} else if qID != nil {
			runID = *qID
		} else {
			httpErr(w, http.StatusBadRequest, "id query parameter is required")
			return
		}
		timing, err := st.GetRunTiming(r.Context(), runID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				httpErr(w, http.StatusNotFound, "run not found")
				return
			}
			httpErr(w, http.StatusInternalServerError, "failed to get run timing: %v", err)
			slog.Error("get run timing: database error", "run_id", runID, "err", err)
			return
		}

		// Build response. timing.ID is now a string (KSUID).
		resp := struct {
			ID      string `json:"id"`
			QueueMs int64  `json:"queue_ms"`
			RunMs   int64  `json:"run_ms"`
		}{
			ID:      timing.ID.String(),
			QueueMs: timing.QueueMs,
			RunMs:   timing.RunMs,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("get run timing: encode response failed", "err", err)
		}

		slog.Info("run timing retrieved", "run_id", resp.ID, "queue_ms", resp.QueueMs, "run_ms", resp.RunMs)
	}
}

// deleteRunHandler returns an HTTP handler that deletes a run by id.
//
// Run IDs are now KSUID-backed strings; no UUID parsing is performed.
// IDs are treated as opaque; validation is limited to non-empty checks.
func deleteRunHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract id from path parameter using domain type helper.
		runID, err := domaintypes.ParseRunIDParam(r, "id")
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		}

		// Check if the run exists before attempting to delete.
		_, err = st.GetRun(r.Context(), runID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				httpErr(w, http.StatusNotFound, "run not found")
				return
			}
			httpErr(w, http.StatusInternalServerError, "failed to check run: %v", err)
			slog.Error("delete run: check failed", "run_id", runID, "err", err)
			return
		}

		// Delete the run using string ID directly.
		err = st.DeleteRun(r.Context(), runID)
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "failed to delete run: %v", err)
			slog.Error("delete run: database error", "run_id", runID, "err", err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
		slog.Info("run deleted", "run_id", runID)
	}
}
