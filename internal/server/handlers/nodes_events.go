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
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/server/events"
	"github.com/iw2rmb/ploy/internal/store"
)

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
				JobID   *string                `json:"job_id,omitempty"`
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

		// Validate events array is not empty.
		if len(req.Events) == 0 {
			http.Error(w, "events array is required and must not be empty", http.StatusBadRequest)
			return
		}

		// Check if the node exists before processing.
		var err error
		_, err = st.GetNode(r.Context(), nodeIDStr)
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

			// Parse job_id if provided.
			var jobID *string
			if evt.JobID != nil && strings.TrimSpace(*evt.JobID) != "" {
				jobIDStr := strings.TrimSpace(*evt.JobID)
				jobID = &jobIDStr
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
				RunID: req.RunID,
				JobID: jobID,
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
