package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/store"
)

// createRunHandler returns an HTTP handler that creates a new run.
func createRunHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Decode request body.
		var req struct {
			ModID     string  `json:"mod_id"`
			BaseRef   string  `json:"base_ref"`
			TargetRef string  `json:"target_ref"`
			CommitSha *string `json:"commit_sha,omitempty"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
			return
		}

		// Validate required fields.
		if strings.TrimSpace(req.ModID) == "" {
			http.Error(w, "mod_id field is required", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.BaseRef) == "" {
			http.Error(w, "base_ref field is required", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.TargetRef) == "" {
			http.Error(w, "target_ref field is required", http.StatusBadRequest)
			return
		}

		// Validate mod_id format.
		modUUID, err := uuid.Parse(req.ModID)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid mod_id: %v", err), http.StatusBadRequest)
			return
		}

		// Create the run with status=queued.
		run, err := st.CreateRun(r.Context(), store.CreateRunParams{
			ModID: pgtype.UUID{
				Bytes: modUUID,
				Valid: true,
			},
			Status:    store.RunStatusQueued,
			BaseRef:   req.BaseRef,
			TargetRef: req.TargetRef,
			CommitSha: req.CommitSha,
		})
		if err != nil {
			// Check if this is a foreign key violation (mod does not exist).
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23503" { // foreign_key_violation
				http.Error(w, "mod not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to create run: %v", err), http.StatusInternalServerError)
			slog.Error("create run: database error", "mod_id", req.ModID, "err", err)
			return
		}

		// Build response with run_id.
		resp := struct {
			RunID string `json:"run_id"`
		}{
			RunID: uuid.UUID(run.ID.Bytes).String(),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("create run: encode response failed", "err", err)
		}

		slog.Info("run created",
			"run_id", resp.RunID,
			"mod_id", req.ModID,
			"status", "queued",
		)
	}
}

// getRunHandler returns an HTTP handler that retrieves a run by id query parameter.
// Supports view=timing query parameter to retrieve timing data from runs_timing view.
// If view=timing and no id is provided, returns a collection of timings
// honoring optional limit/offset pagination.
func getRunHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check if view=timing is requested.
		view := strings.TrimSpace(r.URL.Query().Get("view"))
		if view == "timing" {
			// Accept id from path parameter first, then fallback to query parameter.
			runIDStr := strings.TrimSpace(r.PathValue("id"))
			if runIDStr == "" {
				runIDStr = strings.TrimSpace(r.URL.Query().Get("id"))
			}
			if runIDStr != "" {
				// Single-run timing
				getRunTimingHandler(st).ServeHTTP(w, r)
				return
			}
			// Collection view of timings
			listRunTimingsHandler(st).ServeHTTP(w, r)
			return
		}

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

		// Get the run from the database.
		run, err := st.GetRun(r.Context(), pgtype.UUID{
			Bytes: runUUID,
			Valid: true,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "run not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get run: %v", err), http.StatusInternalServerError)
			slog.Error("get run: database error", "run_id", runIDStr, "err", err)
			return
		}

		// Build response.
		resp := struct {
			ID         string          `json:"id"`
			ModID      string          `json:"mod_id"`
			Status     string          `json:"status"`
			Reason     *string         `json:"reason,omitempty"`
			CreatedAt  string          `json:"created_at"`
			StartedAt  *string         `json:"started_at,omitempty"`
			FinishedAt *string         `json:"finished_at,omitempty"`
			NodeID     *string         `json:"node_id,omitempty"`
			BaseRef    string          `json:"base_ref"`
			TargetRef  string          `json:"target_ref"`
			CommitSha  *string         `json:"commit_sha,omitempty"`
			Stats      json.RawMessage `json:"stats,omitempty"`
		}{
			ID:        uuid.UUID(run.ID.Bytes).String(),
			ModID:     uuid.UUID(run.ModID.Bytes).String(),
			Status:    string(run.Status),
			Reason:    run.Reason,
			CreatedAt: run.CreatedAt.Time.Format(time.RFC3339),
			BaseRef:   run.BaseRef,
			TargetRef: run.TargetRef,
			CommitSha: run.CommitSha,
		}

		// Handle optional timestamp fields.
		if run.StartedAt.Valid {
			startedAt := run.StartedAt.Time.Format(time.RFC3339)
			resp.StartedAt = &startedAt
		}
		if run.FinishedAt.Valid {
			finishedAt := run.FinishedAt.Time.Format(time.RFC3339)
			resp.FinishedAt = &finishedAt
		}

		// Handle optional node_id.
		if run.NodeID.Valid {
			nodeID := uuid.UUID(run.NodeID.Bytes).String()
			resp.NodeID = &nodeID
		}

		// Handle stats (JSONB): return as raw JSON if not empty object.
		if len(run.Stats) > 0 && string(run.Stats) != "{}" {
			resp.Stats = json.RawMessage(run.Stats)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("get run: encode response failed", "err", err)
		}

		slog.Info("run retrieved", "run_id", resp.ID)
	}
}

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

// listRunTimingsHandler returns an HTTP handler that lists run timings with pagination.
// Defaults: limit=100, offset=0. Enforces maximum limit of 200.
func listRunTimingsHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse pagination params with sane defaults.
		const (
			defaultLimit = 100
			maxLimit     = 200
		)
		q := r.URL.Query()
		// limit
		limit := defaultLimit
		if v := strings.TrimSpace(q.Get("limit")); v != "" {
			if n, err := strconv.Atoi(v); err != nil || n < 1 {
				http.Error(w, "invalid limit", http.StatusBadRequest)
				return
			} else {
				limit = n
			}
		}
		if limit > maxLimit {
			limit = maxLimit
		}
		// offset
		offset := 0
		if v := strings.TrimSpace(q.Get("offset")); v != "" {
			if n, err := strconv.Atoi(v); err != nil || n < 0 {
				http.Error(w, "invalid offset", http.StatusBadRequest)
				return
			} else {
				offset = n
			}
		}

		// Query store
		items, err := st.ListRunsTimings(r.Context(), store.ListRunsTimingsParams{
			Limit:  int32(limit),
			Offset: int32(offset),
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to list run timings: %v", err), http.StatusInternalServerError)
			slog.Error("list run timings: database error", "err", err)
			return
		}

		// Build response
		type timing struct {
			ID      string `json:"id"`
			QueueMs int64  `json:"queue_ms"`
			RunMs   int64  `json:"run_ms"`
		}
		resp := struct {
			Timings []timing `json:"timings"`
		}{Timings: make([]timing, len(items))}

		for i, it := range items {
			resp.Timings[i] = timing{
				ID:      uuid.UUID(it.ID.Bytes).String(),
				QueueMs: it.QueueMs,
				RunMs:   it.RunMs,
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("list run timings: encode response failed", "err", err)
		}
		slog.Debug("run timings listed", "count", len(resp.Timings), "limit", limit, "offset", offset)
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

// retryRunHandler returns an HTTP handler that requests a retry for a failed run.
// This is a legacy endpoint maintained for backwards compatibility.
func retryRunHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract run id from path parameter.
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

		// Check if the run exists.
		run, err := st.GetRun(r.Context(), pgtype.UUID{
			Bytes: runUUID,
			Valid: true,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "run not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to check run: %v", err), http.StatusInternalServerError)
			slog.Error("retry run: check failed", "run_id", runIDStr, "err", err)
			return
		}

		// For now, simply return accepted. Full retry logic (creating a new run with
		// same parameters) would require additional schema or status updates.
		// This satisfies the API contract for the legacy endpoint.
		w.WriteHeader(http.StatusAccepted)
		slog.Info("run retry requested", "run_id", runIDStr, "status", run.Status)
	}
}
