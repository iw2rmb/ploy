package handlers

import (
	"context"
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

		// Create stages for the run based on the spec (single-step or multi-step).
		// For multi-step runs (mods[] array), create one stage per mod step.
		// For single-step runs (mod or legacy top-level), create a single stage.
		// Each stage metadata includes step_index and step_total to enable ordered execution.
		if err := createStagesFromSpec(r.Context(), st, run.ID, spec); err != nil {
			http.Error(w, fmt.Sprintf("failed to create stages: %v", err), http.StatusInternalServerError)
			slog.Error("submit ticket: create stages failed", "run_id", uuid.UUID(run.ID.Bytes).String(), "err", err)
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

			// Parse step metadata from stage.meta JSONB.
			// For multi-step runs, this includes step_index for ordering.
			stepIndex := 0
			if len(stg.Meta) > 0 && json.Valid(stg.Meta) {
				var stageMeta modsapi.StageMetadata
				if err := json.Unmarshal(stg.Meta, &stageMeta); err == nil {
					stepIndex = stageMeta.StepIndex
				}
			}

			summary.Stages[uuid.UUID(stg.ID.Bytes).String()] = modsapi.StageStatus{
				StageID:     domaintypes.StageID(uuid.UUID(stg.ID.Bytes).String()),
				State:       s,
				Attempts:    1,
				MaxAttempts: 1,
				Artifacts:   artMap,
				StepIndex:   stepIndex,
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(modsapi.TicketStatusResponse{Ticket: summary}); err != nil {
			slog.Error("get ticket status: encode response failed", "err", err)
		}
	}
}

// createStagesFromSpec parses the run spec and creates stages for multi-step or single-step runs.
// For multi-step runs (mods[] array in spec), creates one stage per mod.
// For single-step runs (mod or legacy top-level), creates a single stage named "mods-openrewrite".
// Each stage's meta JSONB includes step_index and step_total for ordered execution.
func createStagesFromSpec(ctx context.Context, st store.Store, runID pgtype.UUID, spec []byte) error {
	// Parse spec to detect multi-step vs single-step.
	var specMap map[string]interface{}
	if len(spec) > 0 && json.Valid(spec) {
		if err := json.Unmarshal(spec, &specMap); err != nil {
			// Invalid JSON; fallback to single stage.
			return createSingleStage(ctx, st, runID, 0, 1, "")
		}
	}

	// Check for mods[] array (multi-step run).
	if mods, ok := specMap["mods"].([]interface{}); ok && len(mods) > 0 {
		// Multi-step run: create one stage per mod.
		stepTotal := len(mods)
		for stepIndex, modInterface := range mods {
			// Extract mod image for metadata if present.
			modImage := ""
			if modMap, ok := modInterface.(map[string]interface{}); ok {
				if img, ok := modMap["image"].(string); ok {
					modImage = strings.TrimSpace(img)
				}
			}
			// Stage name: "mods-openrewrite-0", "mods-openrewrite-1", etc.
			stageName := fmt.Sprintf("mods-openrewrite-%d", stepIndex)
			if err := createStageWithMeta(ctx, st, runID, stageName, stepIndex, stepTotal, modImage); err != nil {
				return fmt.Errorf("create stage %d: %w", stepIndex, err)
			}
		}
		return nil
	}

	// Single-step run: create one stage.
	// Extract mod image from spec if present (under "mod" or top-level).
	modImage := ""
	if mod, ok := specMap["mod"].(map[string]interface{}); ok {
		if img, ok := mod["image"].(string); ok {
			modImage = strings.TrimSpace(img)
		}
	} else if img, ok := specMap["image"].(string); ok {
		// Legacy top-level image.
		modImage = strings.TrimSpace(img)
	}
	return createSingleStage(ctx, st, runID, 0, 1, modImage)
}

// createSingleStage creates a single stage for a run with the given step metadata.
func createSingleStage(ctx context.Context, st store.Store, runID pgtype.UUID, stepIndex, stepTotal int, modImage string) error {
	return createStageWithMeta(ctx, st, runID, "mods-openrewrite", stepIndex, stepTotal, modImage)
}

// createStageWithMeta creates a stage with step metadata in the meta JSONB field.
func createStageWithMeta(ctx context.Context, st store.Store, runID pgtype.UUID, name string, stepIndex, stepTotal int, modImage string) error {
	// Build stage metadata with step information.
	stageMeta := modsapi.StageMetadata{
		StepIndex: stepIndex,
		StepTotal: stepTotal,
		ModImage:  modImage,
	}
	metaBytes, err := json.Marshal(stageMeta)
	if err != nil {
		return fmt.Errorf("marshal stage metadata: %w", err)
	}

	// Create the stage with metadata.
	_, err = st.CreateStage(ctx, store.CreateStageParams{
		RunID:  runID,
		Name:   name,
		Status: store.StageStatusPending,
		Meta:   metaBytes,
	})
	return err
}

// helpers
func timeOrZero(ts pgtype.Timestamptz) time.Time {
	if ts.Valid {
		return ts.Time
	}
	return time.Unix(0, 0).UTC()
}

func storePgUUID(id pgtype.UUID) pgtype.UUID { return id }
