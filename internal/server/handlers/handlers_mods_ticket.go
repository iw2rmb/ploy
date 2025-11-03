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

// submitTicketHandler returns an HTTP handler that submits a new ticket (mods run).
// POST /v1/mods — Accepts TicketSubmitRequest, returns TicketSummary (ticket_id == run UUID).
// Accepts repo URL/refs directly (no pre-registered mod/repo required).
func submitTicketHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Decode request body.
		var req struct {
			RepoURL   string           `json:"repo_url"`
			BaseRef   string           `json:"base_ref"`
			TargetRef string           `json:"target_ref"`
			CommitSha *string          `json:"commit_sha,omitempty"`
			Spec      *json.RawMessage `json:"spec,omitempty"`
			CreatedBy *string          `json:"created_by,omitempty"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
			return
		}

		// Validate required fields.
		if strings.TrimSpace(req.RepoURL) == "" {
			http.Error(w, "repo_url field is required", http.StatusBadRequest)
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

		// Prepare spec (default to empty JSON object if not provided).
		spec := []byte("{}")
		if req.Spec != nil && len(*req.Spec) > 0 {
			spec = *req.Spec
		}

		// Create the run directly with repo_url and spec inlined.
		run, err := st.CreateRun(r.Context(), store.CreateRunParams{
			RepoUrl:   req.RepoURL,
			Spec:      spec,
			CreatedBy: req.CreatedBy,
			Status:    store.RunStatusQueued,
			BaseRef:   req.BaseRef,
			TargetRef: req.TargetRef,
			CommitSha: req.CommitSha,
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to create run: %v", err), http.StatusInternalServerError)
			slog.Error("submit ticket: create run failed", "repo_url", req.RepoURL, "err", err)
			return
		}

		// Build response with TicketSummary (ticket_id == run UUID).
		resp := struct {
			TicketID  string `json:"ticket_id"`
			Status    string `json:"status"`
			RepoURL   string `json:"repo_url"`
			BaseRef   string `json:"base_ref"`
			TargetRef string `json:"target_ref"`
		}{
			TicketID:  uuid.UUID(run.ID.Bytes).String(),
			Status:    string(run.Status),
			RepoURL:   run.RepoUrl,
			BaseRef:   run.BaseRef,
			TargetRef: run.TargetRef,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("submit ticket: encode response failed", "err", err)
		}

		slog.Info("ticket submitted",
			"ticket_id", resp.TicketID,
			"repo_url", req.RepoURL,
			"base_ref", req.BaseRef,
			"target_ref", req.TargetRef,
			"status", "queued",
		)
	}
}

// getTicketStatusHandler returns an HTTP handler that fetches ticket (run) status by ID.
// GET /v1/mods/{id} — Returns TicketSummary by run UUID.
func getTicketStatusHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse the ticket ID from the URL path parameter.
		ticketIDStr := r.PathValue("id")
		if ticketIDStr == "" {
			http.Error(w, "ticket id is required", http.StatusBadRequest)
			return
		}

		// Parse UUID.
		ticketID, err := uuid.Parse(ticketIDStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid ticket id: %v", err), http.StatusBadRequest)
			return
		}

		// Convert to pgtype.UUID.
		pgID := pgtype.UUID{
			Bytes: ticketID,
			Valid: true,
		}

		// Fetch run (now includes repo_url directly).
		run, err := st.GetRun(r.Context(), pgID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "ticket not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get ticket: %v", err), http.StatusInternalServerError)
			slog.Error("get ticket status: fetch run failed", "ticket_id", ticketIDStr, "err", err)
			return
		}

		// Build TicketSummary response.
		resp := struct {
			TicketID   string  `json:"ticket_id"`
			Status     string  `json:"status"`
			Reason     *string `json:"reason,omitempty"`
			RepoURL    string  `json:"repo_url"`
			BaseRef    string  `json:"base_ref"`
			TargetRef  string  `json:"target_ref"`
			CommitSha  *string `json:"commit_sha,omitempty"`
			CreatedAt  string  `json:"created_at"`
			StartedAt  *string `json:"started_at,omitempty"`
			FinishedAt *string `json:"finished_at,omitempty"`
		}{
			TicketID:  uuid.UUID(run.ID.Bytes).String(),
			Status:    string(run.Status),
			Reason:    run.Reason,
			RepoURL:   run.RepoUrl,
			BaseRef:   run.BaseRef,
			TargetRef: run.TargetRef,
			CommitSha: run.CommitSha,
		}

		// Format timestamps consistently (RFC3339).
		if run.CreatedAt.Valid {
			resp.CreatedAt = run.CreatedAt.Time.Format("2006-01-02T15:04:05Z07:00")
		}
		if run.StartedAt.Valid {
			formatted := run.StartedAt.Time.Format("2006-01-02T15:04:05Z07:00")
			resp.StartedAt = &formatted
		}
		if run.FinishedAt.Valid {
			formatted := run.FinishedAt.Time.Format("2006-01-02T15:04:05Z07:00")
			resp.FinishedAt = &formatted
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("get ticket status: encode response failed", "err", err)
		}
	}
}
