package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/store"
)

// getRunTimingHandler returns an HTTP handler that retrieves timing data for a run.
func getRunTimingHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Accept id from path parameter first, then fallback to query parameter.
		runIDStr := strings.TrimSpace(r.PathValue("id"))
		if runIDStr == "" {
			runIDStr = strings.TrimSpace(r.URL.Query().Get("id"))
		}
		if runIDStr == "" {
			http.Error(w, "id query parameter is required", http.StatusBadRequest)
			return
		}

		// Parse and validate run_id.
		runUUID, err := uuid.Parse(runIDStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid id: %v", err), http.StatusBadRequest)
			return
		}

		// Get timing data from the database.
		timing, err := st.GetRunTiming(r.Context(), pgtype.UUID{
			Bytes: runUUID,
			Valid: true,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "run not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get run timing: %v", err), http.StatusInternalServerError)
			slog.Error("get run timing: database error", "run_id", runIDStr, "err", err)
			return
		}

		// Build response.
		resp := struct {
			ID      string `json:"id"`
			QueueMs int64  `json:"queue_ms"`
			RunMs   int64  `json:"run_ms"`
		}{
			ID:      uuid.UUID(timing.ID.Bytes).String(),
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
func deleteRunHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract id from path parameter.
		runIDStr := r.PathValue("id")
		if strings.TrimSpace(runIDStr) == "" {
			http.Error(w, "id path parameter is required", http.StatusBadRequest)
			return
		}

		// Parse and validate run_id.
		runUUID, err := uuid.Parse(runIDStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid id: %v", err), http.StatusBadRequest)
			return
		}

		// Check if the run exists before attempting to delete.
		_, err = st.GetRun(r.Context(), pgtype.UUID{
			Bytes: runUUID,
			Valid: true,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "run not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to check run: %v", err), http.StatusInternalServerError)
			slog.Error("delete run: check failed", "run_id", runIDStr, "err", err)
			return
		}

		// Delete the run.
		err = st.DeleteRun(r.Context(), pgtype.UUID{
			Bytes: runUUID,
			Valid: true,
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to delete run: %v", err), http.StatusInternalServerError)
			slog.Error("delete run: database error", "run_id", runIDStr, "err", err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
		slog.Info("run deleted", "run_id", runIDStr)
	}
}
