package main

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/store"
)

// legacyJobHeartbeatHandler implements POST /v1/jobs/{id}/heartbeat.
// For compatibility, it validates the run exists and returns 204.
func legacyJobHeartbeatHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.PathValue("id"))
		if id == "" {
			http.Error(w, "id path parameter is required", http.StatusBadRequest)
			return
		}
		runID, err := uuid.Parse(id)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid id: %v", err), http.StatusBadRequest)
			return
		}
		if _, err := st.GetRun(r.Context(), pgtype.UUID{Bytes: runID, Valid: true}); err != nil {
			if err == pgx.ErrNoRows {
				http.Error(w, "run not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to check run: %v", err), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// legacyJobCompleteHandler implements POST /v1/jobs/{id}/complete.
// For compatibility, it validates the run exists and returns 204.
func legacyJobCompleteHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.PathValue("id"))
		if id == "" {
			http.Error(w, "id path parameter is required", http.StatusBadRequest)
			return
		}
		runID, err := uuid.Parse(id)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid id: %v", err), http.StatusBadRequest)
			return
		}
		if _, err := st.GetRun(r.Context(), pgtype.UUID{Bytes: runID, Valid: true}); err != nil {
			if err == pgx.ErrNoRows {
				http.Error(w, "run not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to check run: %v", err), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
