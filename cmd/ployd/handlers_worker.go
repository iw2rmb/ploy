package main

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

	"github.com/iw2rmb/ploy/internal/api/events"
	"github.com/iw2rmb/ploy/internal/store"
)

// heartbeatHandler updates node heartbeat and resource snapshot.
func heartbeatHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract node id from path parameter.
		nodeIDStr := r.PathValue("id")
		if strings.TrimSpace(nodeIDStr) == "" {
			http.Error(w, "id path parameter is required", http.StatusBadRequest)
			return
		}

		// Parse and validate node_id.
		nodeUUID, err := uuid.Parse(nodeIDStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid id: %v", err), http.StatusBadRequest)
			return
		}

		// Decode request body.
		var req struct {
			CPUFreeMilli  float64 `json:"cpu_free_millis"`
			CPUTotalMilli float64 `json:"cpu_total_millis"`
			MemFreeMB     float64 `json:"mem_free_mb"`
			MemTotalMB    float64 `json:"mem_total_mb"`
			DiskFreeMB    float64 `json:"disk_free_mb"`
			DiskTotalMB   float64 `json:"disk_total_mb"`
			Version       string  `json:"version,omitempty"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
			return
		}

		// Check if the node exists before attempting to update.
		_, err = st.GetNode(r.Context(), pgtype.UUID{
			Bytes: nodeUUID,
			Valid: true,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "node not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to check node: %v", err), http.StatusInternalServerError)
			slog.Error("heartbeat: check failed", "node_id", nodeIDStr, "err", err)
			return
		}

		// Convert MB to bytes (1 MB = 1048576 bytes).
		const mbToBytes = 1048576

		// Update node heartbeat with resource snapshot.
		err = st.UpdateNodeHeartbeat(r.Context(), store.UpdateNodeHeartbeatParams{
			ID: pgtype.UUID{
				Bytes: nodeUUID,
				Valid: true,
			},
			LastHeartbeat: pgtype.Timestamptz{
				Time:  time.Now().UTC(),
				Valid: true,
			},
			CpuTotalMillis: int32(req.CPUTotalMilli),
			CpuFreeMillis:  int32(req.CPUFreeMilli),
			MemTotalBytes:  int64(req.MemTotalMB * mbToBytes),
			MemFreeBytes:   int64(req.MemFreeMB * mbToBytes),
			DiskTotalBytes: int64(req.DiskTotalMB * mbToBytes),
			DiskFreeBytes:  int64(req.DiskFreeMB * mbToBytes),
			Version:        strings.TrimSpace(req.Version),
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to update heartbeat: %v", err), http.StatusInternalServerError)
			slog.Error("heartbeat: update failed", "node_id", nodeIDStr, "err", err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
		slog.Debug("heartbeat updated",
			"node_id", nodeIDStr,
			"cpu_free_millis", req.CPUFreeMilli,
			"mem_free_mb", req.MemFreeMB,
			"disk_free_mb", req.DiskFreeMB,
			"version", req.Version,
		)
	}
}

// claimRunHandler allows nodes to claim a queued run for execution.
// Returns the assigned run or 204 No Content if no runs are available.
func claimRunHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract node id from path parameter.
		nodeIDStr := r.PathValue("id")
		if strings.TrimSpace(nodeIDStr) == "" {
			http.Error(w, "id path parameter is required", http.StatusBadRequest)
			return
		}

		// Parse and validate node_id.
		nodeUUID, err := uuid.Parse(nodeIDStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid id: %v", err), http.StatusBadRequest)
			return
		}

		// Verify node exists before attempting to claim a run.
		_, err = st.GetNode(r.Context(), pgtype.UUID{
			Bytes: nodeUUID,
			Valid: true,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "node not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to check node: %v", err), http.StatusInternalServerError)
			slog.Error("claim run: node check failed", "node_id", nodeIDStr, "err", err)
			return
		}

		// Attempt to claim a run using FOR UPDATE SKIP LOCKED.
		run, err := st.ClaimRun(r.Context(), pgtype.UUID{
			Bytes: nodeUUID,
			Valid: true,
		})
		if err != nil {
			// No queued runs available is a valid state; return 204 No Content.
			if errors.Is(err, pgx.ErrNoRows) {
				w.WriteHeader(http.StatusNoContent)
				slog.Debug("claim run: no queued runs available", "node_id", nodeIDStr)
				return
			}
			http.Error(w, fmt.Sprintf("failed to claim run: %v", err), http.StatusInternalServerError)
			slog.Error("claim run: database error", "node_id", nodeIDStr, "err", err)
			return
		}

		// Build response with claimed run details.
		resp := struct {
			ID        string  `json:"id"`
			ModID     string  `json:"mod_id"`
			Status    string  `json:"status"`
			NodeID    string  `json:"node_id"`
			BaseRef   string  `json:"base_ref"`
			TargetRef string  `json:"target_ref"`
			CommitSha *string `json:"commit_sha,omitempty"`
			StartedAt string  `json:"started_at"`
			CreatedAt string  `json:"created_at"`
		}{
			ID:        uuid.UUID(run.ID.Bytes).String(),
			ModID:     uuid.UUID(run.ModID.Bytes).String(),
			Status:    string(run.Status),
			NodeID:    uuid.UUID(run.NodeID.Bytes).String(),
			BaseRef:   run.BaseRef,
			TargetRef: run.TargetRef,
			CommitSha: run.CommitSha,
			// Use RFC3339 for consistency with other API responses.
			StartedAt: run.StartedAt.Time.Format(time.RFC3339),
			CreatedAt: run.CreatedAt.Time.Format(time.RFC3339),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("claim run: encode response failed", "err", err)
		}

		slog.Info("run claimed",
			"run_id", resp.ID,
			"node_id", nodeIDStr,
			"mod_id", resp.ModID,
			"status", resp.Status,
		)
	}
}

// ackRunStartHandler acknowledges that a node has started executing a run.
// Transitions run status from 'assigned' to 'running'.
func ackRunStartHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract node id from path parameter.
		nodeIDStr := r.PathValue("id")
		if strings.TrimSpace(nodeIDStr) == "" {
			http.Error(w, "id path parameter is required", http.StatusBadRequest)
			return
		}

		// Parse and validate node_id.
		nodeUUID, err := uuid.Parse(nodeIDStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid id: %v", err), http.StatusBadRequest)
			return
		}

		// Decode request body to get run_id.
		var req struct {
			RunID string `json:"run_id"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
			return
		}

		// Validate run_id is present.
		if strings.TrimSpace(req.RunID) == "" {
			http.Error(w, "run_id is required", http.StatusBadRequest)
			return
		}

		// Parse and validate run_id.
		runUUID, err := uuid.Parse(req.RunID)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid run_id: %v", err), http.StatusBadRequest)
			return
		}

		// Verify node exists before attempting to acknowledge.
		_, err = st.GetNode(r.Context(), pgtype.UUID{
			Bytes: nodeUUID,
			Valid: true,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "node not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to check node: %v", err), http.StatusInternalServerError)
			slog.Error("ack run start: node check failed", "node_id", nodeIDStr, "err", err)
			return
		}

		// Verify run exists and is assigned to this node.
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
			slog.Error("ack run start: run check failed", "run_id", req.RunID, "err", err)
			return
		}

		// Verify the run is assigned to the requesting node.
		if !run.NodeID.Valid || uuid.UUID(run.NodeID.Bytes) != nodeUUID {
			http.Error(w, "run not assigned to this node", http.StatusForbidden)
			return
		}

		// Verify the run is in 'assigned' status before transitioning to 'running'.
		if run.Status != store.RunStatusAssigned {
			http.Error(w, fmt.Sprintf("run status is %s, expected assigned", run.Status), http.StatusConflict)
			return
		}

		// Transition run status from 'assigned' to 'running'.
		err = st.AckRunStart(r.Context(), pgtype.UUID{
			Bytes: runUUID,
			Valid: true,
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to acknowledge run start: %v", err), http.StatusInternalServerError)
			slog.Error("ack run start: update failed", "run_id", req.RunID, "node_id", nodeIDStr, "err", err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
		slog.Info("run start acknowledged",
			"run_id", req.RunID,
			"node_id", nodeIDStr,
			"status", "running",
		)
	}
}

// completeRunHandler marks a run as completed with terminal status and stats.
// Sets finished_at timestamp and populates runs.stats field.
func completeRunHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract node id from path parameter.
		nodeIDStr := r.PathValue("id")
		if strings.TrimSpace(nodeIDStr) == "" {
			http.Error(w, "id path parameter is required", http.StatusBadRequest)
			return
		}

		// Parse and validate node_id.
		nodeUUID, err := uuid.Parse(nodeIDStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid id: %v", err), http.StatusBadRequest)
			return
		}

		// Decode request body to get run_id, status, reason, and stats.
		var req struct {
			RunID  string          `json:"run_id"`
			Status string          `json:"status"`
			Reason *string         `json:"reason,omitempty"`
			Stats  json.RawMessage `json:"stats,omitempty"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
			return
		}

		// Validate run_id is present.
		if strings.TrimSpace(req.RunID) == "" {
			http.Error(w, "run_id is required", http.StatusBadRequest)
			return
		}

		// Parse and validate run_id.
		runUUID, err := uuid.Parse(req.RunID)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid run_id: %v", err), http.StatusBadRequest)
			return
		}

		// Validate status is a terminal state.
		if strings.TrimSpace(req.Status) == "" {
			http.Error(w, "status is required", http.StatusBadRequest)
			return
		}

		// Normalize status to match RunStatus enum values.
		normalizedStatus := store.RunStatus(strings.ToLower(strings.TrimSpace(req.Status)))

		// Validate that status is a terminal state (succeeded, failed, or canceled).
		if normalizedStatus != store.RunStatusSucceeded &&
			normalizedStatus != store.RunStatusFailed &&
			normalizedStatus != store.RunStatusCanceled {
			http.Error(w, fmt.Sprintf("status must be succeeded, failed, or canceled, got %s", req.Status), http.StatusBadRequest)
			return
		}

		// Verify node exists before attempting to complete the run.
		_, err = st.GetNode(r.Context(), pgtype.UUID{
			Bytes: nodeUUID,
			Valid: true,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "node not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to check node: %v", err), http.StatusInternalServerError)
			slog.Error("complete run: node check failed", "node_id", nodeIDStr, "err", err)
			return
		}

		// Verify run exists and is assigned to this node.
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
			slog.Error("complete run: run check failed", "run_id", req.RunID, "err", err)
			return
		}

		// Verify the run is assigned to the requesting node.
		if !run.NodeID.Valid || uuid.UUID(run.NodeID.Bytes) != nodeUUID {
			http.Error(w, "run not assigned to this node", http.StatusForbidden)
			return
		}

		// Verify the run is in 'running' status before transitioning to terminal state.
		if run.Status != store.RunStatusRunning {
			http.Error(w, fmt.Sprintf("run status is %s, expected running", run.Status), http.StatusConflict)
			return
		}

		// Prepare stats field (default to empty JSON object if not provided).
		statsBytes := []byte("{}")
		if len(req.Stats) > 0 {
			// Validate that stats is valid JSON.
			if !json.Valid(req.Stats) {
				http.Error(w, "stats field must be valid JSON", http.StatusBadRequest)
				return
			}
			statsBytes = req.Stats
		}

		// Update run completion: set status, reason, finished_at (server-side now()), and stats.
		err = st.UpdateRunCompletion(r.Context(), store.UpdateRunCompletionParams{
			ID: pgtype.UUID{
				Bytes: runUUID,
				Valid: true,
			},
			Status: normalizedStatus,
			Reason: req.Reason,
			Stats:  statsBytes,
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to complete run: %v", err), http.StatusInternalServerError)
			slog.Error("complete run: update failed", "run_id", req.RunID, "node_id", nodeIDStr, "err", err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
		slog.Info("run completed",
			"run_id", req.RunID,
			"node_id", nodeIDStr,
			"status", req.Status,
			"has_reason", req.Reason != nil,
			"stats_size", len(statsBytes),
		)
	}
}

// createNodeEventsHandler appends structured events/log frames to DB with SSE fanout.
func createNodeEventsHandler(st store.Store, eventsService *events.Service) http.HandlerFunc {
	const maxRequestSize = 1 << 20 // 1 MiB
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract node id from path parameter.
		nodeIDStr := r.PathValue("id")
		if strings.TrimSpace(nodeIDStr) == "" {
			http.Error(w, "id path parameter is required", http.StatusBadRequest)
			return
		}

		// Parse and validate node_id.
		nodeUUID, err := uuid.Parse(nodeIDStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid id: %v", err), http.StatusBadRequest)
			return
		}

		// Check payload size before reading body.
		if r.ContentLength > maxRequestSize {
			http.Error(w, "payload exceeds 1 MiB size cap", http.StatusRequestEntityTooLarge)
			return
		}

		// Limit request body to 1 MiB to prevent memory exhaustion.
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestSize)

		// Decode request body.
		var req struct {
			RunID  string `json:"run_id"`
			Events []struct {
				StageID *string                `json:"stage_id,omitempty"`
				Time    *string                `json:"time,omitempty"`
				Level   string                 `json:"level"`
				Message string                 `json:"message"`
				Meta    map[string]interface{} `json:"meta,omitempty"`
			} `json:"events"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			// Return 413 when MaxBytesReader trips the size cap.
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				http.Error(w, "payload exceeds 1 MiB size cap", http.StatusRequestEntityTooLarge)
				return
			}
			http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
			return
		}

		// Validate run_id is present.
		if strings.TrimSpace(req.RunID) == "" {
			http.Error(w, "run_id is required", http.StatusBadRequest)
			return
		}

		// Validate run_id is a valid UUID.
		runUUID, err := uuid.Parse(req.RunID)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid run_id: %v", err), http.StatusBadRequest)
			return
		}

		// Validate events array is not empty.
		if len(req.Events) == 0 {
			http.Error(w, "events array is required and must not be empty", http.StatusBadRequest)
			return
		}

		// Check if the node exists before processing.
		_, err = st.GetNode(r.Context(), pgtype.UUID{
			Bytes: nodeUUID,
			Valid: true,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "node not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to check node: %v", err), http.StatusInternalServerError)
			slog.Error("node events: check failed", "node_id", nodeIDStr, "err", err)
			return
		}

		// Process and persist each event.
		count := 0
		for i, evt := range req.Events {
			// Validate required fields.
			if strings.TrimSpace(evt.Level) == "" {
				http.Error(w, fmt.Sprintf("events[%d]: level is required", i), http.StatusBadRequest)
				return
			}
			if strings.TrimSpace(evt.Message) == "" {
				http.Error(w, fmt.Sprintf("events[%d]: message is required", i), http.StatusBadRequest)
				return
			}

			// Parse stage_id if provided.
			var stageID pgtype.UUID
			if evt.StageID != nil && strings.TrimSpace(*evt.StageID) != "" {
				stageUUID, err := uuid.Parse(*evt.StageID)
				if err != nil {
					http.Error(w, fmt.Sprintf("events[%d]: invalid stage_id: %v", i, err), http.StatusBadRequest)
					return
				}
				stageID = pgtype.UUID{
					Bytes: stageUUID,
					Valid: true,
				}
			}

			// Parse event time if provided, otherwise use server time.
			eventTime := time.Now().UTC()
			if evt.Time != nil && strings.TrimSpace(*evt.Time) != "" {
				parsedTime, err := time.Parse(time.RFC3339, strings.TrimSpace(*evt.Time))
				if err != nil {
					http.Error(w, fmt.Sprintf("events[%d]: invalid time format: %v", i, err), http.StatusBadRequest)
					return
				}
				eventTime = parsedTime.UTC()
			}

			// Prepare meta field (default to empty JSON object if nil).
			meta := evt.Meta
			if meta == nil {
				meta = map[string]interface{}{}
			}

			// Marshal meta to JSON.
			metaBytes, err := json.Marshal(meta)
			if err != nil {
				http.Error(w, fmt.Sprintf("events[%d]: failed to marshal meta: %v", i, err), http.StatusBadRequest)
				return
			}

			// Create event params.
			// Normalize level to lowercase for consistency in SSE streams.
			level := strings.ToLower(strings.TrimSpace(evt.Level))

			params := store.CreateEventParams{
				RunID: pgtype.UUID{
					Bytes: runUUID,
					Valid: true,
				},
				StageID: stageID,
				Time: pgtype.Timestamptz{
					Time:  eventTime,
					Valid: true,
				},
				Level:   level,
				Message: evt.Message,
				Meta:    metaBytes,
			}

			// Persist event to DB and fan out to SSE.
			_, err = eventsService.CreateAndPublishEvent(r.Context(), params)
			if err != nil {
				http.Error(w, fmt.Sprintf("failed to create event: %v", err), http.StatusInternalServerError)
				slog.Error("node events: create failed", "node_id", nodeIDStr, "run_id", req.RunID, "index", i, "err", err)
				return
			}

			count++
		}

		// Return success response with count.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(map[string]interface{}{"count": count}); err != nil {
			slog.Error("node events: encode response failed", "err", err)
		}

		slog.Debug("node events created",
			"node_id", nodeIDStr,
			"run_id", req.RunID,
			"count", count,
		)
	}
}
