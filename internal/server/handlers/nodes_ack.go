package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
	"github.com/iw2rmb/ploy/internal/server/events"
	"github.com/iw2rmb/ploy/internal/store"
)

// ackRunStartHandler acknowledges that a node has started executing a job.
// Since jobs now transition directly to 'running' on claim, this handler
// primarily serves for backward compatibility and SSE event publishing.
func ackRunStartHandler(st store.Store, eventsService *events.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract node id from path parameter.
		nodeIDStr := r.PathValue("id")
		if strings.TrimSpace(nodeIDStr) == "" {
			http.Error(w, "id path parameter is required", http.StatusBadRequest)
			return
		}

		// Decode request body to get run_id and job_id.
		var req struct {
			RunID domaintypes.RunID `json:"run_id"`
			JobID domaintypes.JobID `json:"job_id"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
			return
		}

		// Validate run_id is present.
		if req.RunID.IsZero() {
			http.Error(w, "run_id is required", http.StatusBadRequest)
			return
		}

		// Validate job_id is present.
		if req.JobID.IsZero() {
			http.Error(w, "job_id is required", http.StatusBadRequest)
			return
		}

		var err error
		// Verify node exists before attempting to acknowledge.
		_, err = st.GetNode(r.Context(), nodeIDStr)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "node not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to check node: %v", err), http.StatusInternalServerError)
			slog.Error("ack job start: node check failed", "node_id", nodeIDStr, "err", err)
			return
		}

		// Verify run exists.
		run, err := st.GetRun(r.Context(), req.RunID.String())
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "run not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to check run: %v", err), http.StatusInternalServerError)
			slog.Error("ack job start: run check failed", "run_id", req.RunID, "err", err)
			return
		}

		// Get the job and verify it belongs to the run and is assigned to this node.
		job, err := st.GetJob(r.Context(), req.JobID.String())
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "job not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get job: %v", err), http.StatusInternalServerError)
			slog.Error("ack job start: get job failed", "job_id", req.JobID, "err", err)
			return
		}

		// Verify the job belongs to the specified run.
		if job.RunID != req.RunID.String() {
			http.Error(w, "job does not belong to this run", http.StatusBadRequest)
			return
		}

		// Verify the job is assigned to the requesting node.
		if job.NodeID == nil || *job.NodeID != nodeIDStr {
			http.Error(w, "job not assigned to this node", http.StatusForbidden)
			return
		}

		// Verify the job is in 'running' status.
		// Jobs now transition directly to 'running' on claim, so this is expected.
		if job.Status != store.JobStatusRunning {
			http.Error(w, fmt.Sprintf("job status is %s, expected running", job.Status), http.StatusConflict)
			return
		}

		// Job is already 'running' from the claim, no status update needed.

		// Transition run status to 'running' if it's still queued or assigned.
		// AckRunStart is idempotent and only updates if status allows transition.
		_ = st.AckRunStart(r.Context(), req.RunID.String())

		slog.Info("job start acknowledged",
			"run_id", req.RunID,
			"job_id", req.JobID,
			"job_name", job.Name,
			"node_id", nodeIDStr,
			"status", "running",
		)

		// Publish running event to SSE hub.
		if eventsService != nil {
			// Use RunID field to describe the run.
			runSummary := modsapi.RunSummary{
				RunID:      domaintypes.RunID(req.RunID.String()),
				State:      modsapi.RunStateRunning,
				Repository: run.RepoUrl,
				CreatedAt:  run.CreatedAt.Time,
				UpdatedAt:  time.Now().UTC(),
				Stages:     make(map[string]modsapi.StageStatus),
			}
			if err := eventsService.PublishRun(r.Context(), req.RunID.String(), runSummary); err != nil {
				slog.Error("ack job start: publish run event failed", "run_id", req.RunID, "err", err)
			}
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
