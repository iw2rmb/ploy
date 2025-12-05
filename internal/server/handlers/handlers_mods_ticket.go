package handlers

import (
	"context"
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
	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
	"github.com/iw2rmb/ploy/internal/server/events"
	"github.com/iw2rmb/ploy/internal/store"
)

// Mods ticket handlers implement the /v1/mods facade: ticket submission from
// repo/spec, ticket status as mods-style TicketSummary, and stage/run_step
// materialization for single- and multi-step runs.

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

		// Create jobs for the run execution pipeline.
		// Jobs are created with float step_index for ordered execution:
		//   pre-gate (1000) → mod-0 (2000) → post-gate (3000)
		// For multi-step runs (mods[] array), creates one mod job per entry.
		if err := createJobsFromSpec(r.Context(), st, run.ID, spec); err != nil {
			http.Error(w, fmt.Sprintf("failed to create jobs: %v", err), http.StatusInternalServerError)
			slog.Error("submit ticket: create jobs failed", "run_id", uuid.UUID(run.ID.Bytes).String(), "err", err)
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

		// Surface MR URL, gate summary, and resume metadata from runs.stats if present.
		// Node stores MR URL under stats.metadata.mr_url and gate data under stats.gate.
		// Gate summary exposes gate health without requiring raw artifact inspection.
		// Resume metadata (resume_count, last_resumed_at) tracks resume history.
		if len(run.Stats) > 0 && json.Valid(run.Stats) {
			var stats domaintypes.RunStats
			if err := json.Unmarshal(run.Stats, &stats); err == nil {
				if mr := stats.MRURL(); mr != "" {
					if summary.Metadata == nil {
						summary.Metadata = map[string]string{}
					}
					summary.Metadata["mr_url"] = mr
				}
				// Extract gate summary for quick gate health visibility.
				if gateSummary := stats.GateSummary(); gateSummary != "" {
					if summary.Metadata == nil {
						summary.Metadata = map[string]string{}
					}
					summary.Metadata["gate_summary"] = gateSummary
				}
				// Extract resume metadata so clients can see resume history.
				if rc := stats.ResumeCount(); rc > 0 {
					if summary.Metadata == nil {
						summary.Metadata = map[string]string{}
					}
					summary.Metadata["resume_count"] = strconv.Itoa(rc)
				}
				if lra := stats.LastResumedAt(); lra != "" {
					if summary.Metadata == nil {
						summary.Metadata = map[string]string{}
					}
					summary.Metadata["last_resumed_at"] = lra
				}
			}
		}

		// Load jobs and their artifacts.
		jobs, err := st.ListJobsByRun(r.Context(), storePgUUID(run.ID))
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to list jobs: %v", err), http.StatusInternalServerError)
			slog.Error("get ticket status: list jobs failed", "ticket_id", uuid.UUID(run.ID.Bytes).String(), "err", err)
			return
		}
		for _, job := range jobs {
			// Use conversion helper to map store.JobStatus -> modsapi.StageState
			s := modsapi.StageStatusFromStore(job.Status)
			artMap := make(map[string]string)
			bundles, err := st.ListArtifactBundlesByRunAndJob(r.Context(), store.ListArtifactBundlesByRunAndJobParams{RunID: storePgUUID(run.ID), JobID: job.ID})
			if err != nil {
				http.Error(w, fmt.Sprintf("failed to list artifacts: %v", err), http.StatusInternalServerError)
				slog.Error("get ticket status: list artifacts failed", "run_id", uuid.UUID(run.ID.Bytes).String(), "job_id", uuid.UUID(job.ID.Bytes).String(), "err", err)
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

			// Use job's step_index directly for ordering (float, but expose as int for API).
			stepIndex := int(job.StepIndex)

			// Attempts/MaxAttempts are currently fixed at 1; future retries must
			// update these counters without changing StepIndex semantics.
			summary.Stages[uuid.UUID(job.ID.Bytes).String()] = modsapi.StageStatus{
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

// createJobsFromSpec parses the run spec and creates jobs for the execution pipeline.
// Jobs are created with float step_index for ordered execution with dynamic insertion support.
//
// Default job layout:
//   - pre-gate  (step_index=1000): Pre-mod validation/gate
//   - mod-0     (step_index=2000): First mod execution
//   - post-gate (step_index=3000): Post-mod validation
//
// For multi-step runs (mods[] array), creates one mod job per entry.
// Healing jobs can be inserted dynamically between existing jobs using midpoint calculation.
func createJobsFromSpec(ctx context.Context, st store.Store, runID pgtype.UUID, spec []byte) error {
	// Parse spec to detect multi-step vs single-step.
	var specMap map[string]interface{}
	if len(spec) > 0 && json.Valid(spec) {
		if err := json.Unmarshal(spec, &specMap); err != nil {
			// Invalid JSON; fallback to single mod job.
			return createSingleModJob(ctx, st, runID, "")
		}
	}

	// Check for mods[] array (multi-step run).
	if mods, ok := specMap["mods"].([]interface{}); ok && len(mods) > 0 {
		// Multi-step run: create pre-gate, one job per mod, and post-gate.
		// Server-driven scheduling: first job (pre-gate) is 'pending', rest are 'created'.
		// Pre-gate job - pending (ready to be claimed immediately)
		if err := createJobWithIndex(ctx, st, runID, "pre-gate", "pre_gate", 1000, "", store.JobStatusPending); err != nil {
			return fmt.Errorf("create pre-gate job: %w", err)
		}

		// Mod jobs: step_index starts at 2000, increment by 1000
		// All mod jobs start as 'created' - server will schedule them after prior job completes.
		for i, modInterface := range mods {
			modImage := ""
			if modMap, ok := modInterface.(map[string]interface{}); ok {
				if img, ok := modMap["image"].(string); ok {
					modImage = strings.TrimSpace(img)
				}
			}
			jobName := fmt.Sprintf("mod-%d", i)
			stepIndex := float64(2000 + i*1000)
			if err := createJobWithIndex(ctx, st, runID, jobName, "mod", stepIndex, modImage, store.JobStatusCreated); err != nil {
				return fmt.Errorf("create mod job %d: %w", i, err)
			}
		}

		// Post-gate job: after all mods - starts as 'created'
		postGateIndex := float64(2000 + len(mods)*1000)
		if err := createJobWithIndex(ctx, st, runID, "post-gate", "post_gate", postGateIndex, "", store.JobStatusCreated); err != nil {
			return fmt.Errorf("create post-gate job: %w", err)
		}
		return nil
	}

	// Single-step run: create pre-gate, single mod, and post-gate.
	modImage := ""
	if mod, ok := specMap["mod"].(map[string]interface{}); ok {
		if img, ok := mod["image"].(string); ok {
			modImage = strings.TrimSpace(img)
		}
	} else if img, ok := specMap["image"].(string); ok {
		modImage = strings.TrimSpace(img)
	}
	return createSingleModJob(ctx, st, runID, modImage)
}

// createSingleModJob creates the standard 3-job pipeline: pre-gate, mod-0, post-gate.
// Server-driven scheduling: first job (pre-gate) is 'pending', rest are 'created'.
func createSingleModJob(ctx context.Context, st store.Store, runID pgtype.UUID, modImage string) error {
	// Pre-gate is pending (ready to claim), others are created (wait for server to schedule).
	if err := createJobWithIndex(ctx, st, runID, "pre-gate", "pre_gate", 1000, "", store.JobStatusPending); err != nil {
		return fmt.Errorf("create pre-gate job: %w", err)
	}
	if err := createJobWithIndex(ctx, st, runID, "mod-0", "mod", 2000, modImage, store.JobStatusCreated); err != nil {
		return fmt.Errorf("create mod job: %w", err)
	}
	if err := createJobWithIndex(ctx, st, runID, "post-gate", "post_gate", 3000, "", store.JobStatusCreated); err != nil {
		return fmt.Errorf("create post-gate job: %w", err)
	}
	return nil
}

// createJobWithIndex creates a job with the given step_index, status, and metadata.
// modType identifies the job phase ("pre_gate", "mod", "post_gate", "heal").
// status should be JobStatusPending for the first job, JobStatusCreated for others.
func createJobWithIndex(ctx context.Context, st store.Store, runID pgtype.UUID, name, modType string, stepIndex float64, modImage string, status store.JobStatus) error {
	// Create the job with step_index and status.
	_, err := st.CreateJob(ctx, store.CreateJobParams{
		RunID:     runID,
		Name:      name,
		Status:    status,
		ModType:   modType,
		ModImage:  modImage,
		StepIndex: stepIndex,
		Meta:      []byte(`{}`),
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
