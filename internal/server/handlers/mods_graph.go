package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/graph"
)

// getModGraphHandler returns an HTTP handler that fetches the workflow graph
// for a Mods ticket by ID.
//
// GET /v1/mods/{id}/graph — Returns WorkflowGraph with jobs as nodes and
// dependencies derived from step_index ordering.
//
// This endpoint is intended for debugging and visualization purposes. The
// graph view is materialized from existing jobs rows without additional
// persistence. Response includes:
//   - nodes: All jobs as graph nodes with type, status, step_index, etc.
//   - root_ids: Entry-point nodes (typically pre-gate)
//   - leaf_ids: Terminal nodes (typically post-gate)
//   - linear: Whether the graph is a simple linear chain
//
// Example response:
//
//	{
//	  "run_id": "abc-123",
//	  "nodes": {
//	    "job-1": {"id": "job-1", "name": "pre-gate", "type": "pre_gate", ...},
//	    "job-2": {"id": "job-2", "name": "mod-0", "type": "mod", ...}
//	  },
//	  "root_ids": ["job-1"],
//	  "leaf_ids": ["job-3"],
//	  "linear": true
//	}
//
// Run and job IDs are now KSUID-backed strings; no UUID parsing is performed.
func getModGraphHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse the ticket ID from the URL path parameter.
		// Run IDs are KSUID strings; treated as opaque identifiers.
		ticketIDStr := strings.TrimSpace(r.PathValue("id"))
		if ticketIDStr == "" {
			http.Error(w, "ticket id is required", http.StatusBadRequest)
			return
		}

		// Verify the run exists before fetching jobs using string ID directly.
		// No UUID parsing needed; store accepts KSUID strings.
		_, err := st.GetRun(r.Context(), ticketIDStr)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "ticket not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get ticket: %v", err), http.StatusInternalServerError)
			slog.Error("get mod graph: fetch run failed", "ticket_id", ticketIDStr, "err", err)
			return
		}

		// Fetch all jobs for the run, ordered by step_index.
		// Uses string run ID directly.
		jobs, err := st.ListJobsByRun(r.Context(), ticketIDStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to list jobs: %v", err), http.StatusInternalServerError)
			slog.Error("get mod graph: list jobs failed", "ticket_id", ticketIDStr, "err", err)
			return
		}

		// Build the workflow graph from jobs.
		// The graph materializes nodes from jobs and computes edges from
		// step_index ordering. runID is now a string (KSUID).
		workflowGraph := graph.BuildFromJobs(ticketIDStr, jobs)

		// Return the graph as JSON.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(workflowGraph); err != nil {
			slog.Error("get mod graph: encode response failed", "err", err)
		}

		slog.Debug("mod graph fetched",
			"ticket_id", ticketIDStr,
			"node_count", workflowGraph.NodeCount(),
			"linear", workflowGraph.Linear,
		)
	}
}
