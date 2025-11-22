package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
	"github.com/iw2rmb/ploy/internal/server/events"
	"github.com/iw2rmb/ploy/internal/store"
)

// submitTicketHandler returns an HTTP handler that submits a new ticket (mods run).
// POST /v1/mods — Accepts TicketSubmitRequest, returns TicketSummary (ticket_id == run UUID).
// Accepts repo URL/refs directly (no pre-registered mod/repo required).
func submitTicketHandler(st store.Store, eventsService *events.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Decode request body with domain types for VCS fields.
		// JSON unmarshaling will automatically validate repo URL scheme and non-empty refs.
		var req struct {
			RepoURL   domaintypes.RepoURL    `json:"repo_url"`
			BaseRef   domaintypes.GitRef     `json:"base_ref"`
			TargetRef domaintypes.GitRef     `json:"target_ref"`
			CommitSha *domaintypes.CommitSHA `json:"commit_sha,omitempty"`
			Spec      *json.RawMessage       `json:"spec,omitempty"`
			CreatedBy *string                `json:"created_by,omitempty"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
			return
		}

		// Validate domain types explicitly to catch missing/zero-value fields.
		// When JSON fields are omitted, domain types remain at zero value and need validation.
		if err := req.RepoURL.Validate(); err != nil {
			http.Error(w, fmt.Sprintf("repo_url: %v", err), http.StatusBadRequest)
			return
		}
		if err := req.BaseRef.Validate(); err != nil {
			http.Error(w, fmt.Sprintf("base_ref: %v", err), http.StatusBadRequest)
			return
		}
		if err := req.TargetRef.Validate(); err != nil {
			http.Error(w, fmt.Sprintf("target_ref: %v", err), http.StatusBadRequest)
			return
		}
		if req.CommitSha != nil {
			if err := req.CommitSha.Validate(); err != nil {
				http.Error(w, fmt.Sprintf("commit_sha: %v", err), http.StatusBadRequest)
				return
			}
		}

		// Prepare spec (default to empty JSON object if not provided).
		spec := []byte("{}")
		if req.Spec != nil && len(*req.Spec) > 0 {
			spec = *req.Spec
		}

		// Create the run directly with repo_url and spec inlined.
		// Convert domain types to strings for storage layer.
		var commitShaStr *string
		if req.CommitSha != nil {
			s := req.CommitSha.String()
			commitShaStr = &s
		}
		run, err := st.CreateRun(r.Context(), store.CreateRunParams{
			RepoUrl:   req.RepoURL.String(),
			Spec:      spec,
			CreatedBy: req.CreatedBy,
			Status:    store.RunStatusQueued,
			BaseRef:   req.BaseRef.String(),
			TargetRef: req.TargetRef.String(),
			CommitSha: commitShaStr,
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to create run: %v", err), http.StatusInternalServerError)
			slog.Error("submit ticket: create run failed", "repo_url", req.RepoURL, "err", err)
			return
		}

		// Create initial stage for the run (single-stage model: mods-openrewrite).
		// Future: derive stage name from spec if provided.
		_, err = st.CreateStage(r.Context(), store.CreateStageParams{
			RunID:  storePgUUID(run.ID),
			Name:   "mods-openrewrite",
			Status: store.StageStatusPending,
			Meta:   []byte("{}"),
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to create stage: %v", err), http.StatusInternalServerError)
			slog.Error("submit ticket: create stage failed", "run_id", uuid.UUID(run.ID.Bytes).String(), "err", err)
			return
		}

		// Build response with TicketSummary (ticket_id == run UUID).
		// Use typed status (store.RunStatus) instead of string cast for type safety;
		// JSON encoder will serialize the underlying string value.
		resp := struct {
			TicketID  string          `json:"ticket_id"`
			Status    store.RunStatus `json:"status"` // Typed status instead of string cast
			RepoURL   string          `json:"repo_url"`
			BaseRef   string          `json:"base_ref"`
			TargetRef string          `json:"target_ref"`
		}{
			TicketID:  uuid.UUID(run.ID.Bytes).String(),
			Status:    run.Status,
			RepoURL:   run.RepoUrl,
			BaseRef:   run.BaseRef,
			TargetRef: run.TargetRef,
		}

		// Publish queued event to SSE hub.
		if eventsService != nil {
			ticketSummary := modsapi.TicketSummary{
				TicketID:   domaintypes.TicketID(resp.TicketID),
				State:      modsapi.TicketState(run.Status),
				Repository: run.RepoUrl,
				CreatedAt:  run.CreatedAt.Time,
				UpdatedAt:  run.CreatedAt.Time,
				Stages:     make(map[string]modsapi.StageStatus),
			}
			if err := eventsService.PublishTicket(r.Context(), resp.TicketID, ticketSummary); err != nil {
				slog.Error("submit ticket: publish ticket event failed", "ticket_id", resp.TicketID, "err", err)
			}
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

		// Build mods-style TicketStatusResponse with Stages and Artifacts.
		// Use conversion helper to map store.RunStatus to modsapi.TicketState.
		ticketState := modsapi.TicketStatusFromStore(run.Status)

		summary := modsapi.TicketSummary{
			TicketID:   domaintypes.TicketID(uuid.UUID(run.ID.Bytes).String()),
			State:      ticketState,
			Submitter:  "",
			Repository: run.RepoUrl,
			Metadata:   map[string]string{"repo_base_ref": run.BaseRef, "repo_target_ref": run.TargetRef},
			CreatedAt:  timeOrZero(run.CreatedAt),
			UpdatedAt:  time.Now().UTC(),
			Stages:     make(map[string]modsapi.StageStatus),
		}

		// Include claiming node id when available for easier diagnostics.
		if run.NodeID.Valid {
			summary.Metadata["node_id"] = uuid.UUID(run.NodeID.Bytes).String()
		}

		// Surface MR URL (and other future metadata) from runs.stats if present.
		// Node stores MR URL under stats.metadata.mr_url; copy it into summary.Metadata.
		if len(run.Stats) > 0 && json.Valid(run.Stats) {
			var stats domaintypes.RunStats
			if err := json.Unmarshal(run.Stats, &stats); err == nil {
				if mr := stats.MRURL(); mr != "" {
					if summary.Metadata == nil {
						summary.Metadata = map[string]string{}
					}
					summary.Metadata["mr_url"] = mr
				}
			}
		}

		// Include terminal reason if available for quick diagnostics.
		if run.Reason != nil && strings.TrimSpace(*run.Reason) != "" {
			summary.Metadata["reason"] = strings.TrimSpace(*run.Reason)
		}

		// Load stages and their artifacts.
		stages, err := st.ListStagesByRun(r.Context(), storePgUUID(run.ID))
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to list stages: %v", err), http.StatusInternalServerError)
			slog.Error("get ticket status: list stages failed", "ticket_id", uuid.UUID(run.ID.Bytes).String(), "err", err)
			return
		}
		for _, stg := range stages {
			// Use conversion helper to map store.StageStatus -> modsapi.StageState
			s := modsapi.StageStatusFromStore(stg.Status)
			artMap := make(map[string]string)
			bundles, err := st.ListArtifactBundlesByRunAndStage(r.Context(), store.ListArtifactBundlesByRunAndStageParams{RunID: storePgUUID(run.ID), StageID: stg.ID})
			if err != nil {
				http.Error(w, fmt.Sprintf("failed to list artifacts: %v", err), http.StatusInternalServerError)
				slog.Error("get ticket status: list artifacts failed", "run_id", uuid.UUID(run.ID.Bytes).String(), "stage_id", uuid.UUID(stg.ID.Bytes).String(), "err", err)
				return
			}
			for _, b := range bundles {
				name := "artifact"
				if b.Name != nil && strings.TrimSpace(*b.Name) != "" {
					name = strings.TrimSpace(*b.Name)
				}
				if b.Cid != nil && strings.TrimSpace(*b.Cid) != "" {
					artMap[name] = strings.TrimSpace(*b.Cid)
				}
			}
			summary.Stages[uuid.UUID(stg.ID.Bytes).String()] = modsapi.StageStatus{
				StageID:     domaintypes.StageID(uuid.UUID(stg.ID.Bytes).String()),
				State:       s,
				Attempts:    1,
				MaxAttempts: 1,
				Artifacts:   artMap,
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(modsapi.TicketStatusResponse{Ticket: summary}); err != nil {
			slog.Error("get ticket status: encode response failed", "err", err)
		}
	}
}

// helpers
func timeOrZero(ts pgtype.Timestamptz) time.Time {
	if ts.Valid {
		return ts.Time
	}
	return time.Unix(0, 0).UTC()
}

func storePgUUID(id pgtype.UUID) pgtype.UUID { return id }
