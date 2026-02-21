package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/events"
	"github.com/iw2rmb/ploy/internal/store"
)

// createNodeEventsHandler appends structured events/log frames to DB with SSE fanout.
func createNodeEventsHandler(st store.Store, eventsService *events.Service) http.HandlerFunc {
	const maxRequestSize = 1 << 20 // 1 MiB
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract node id from path parameter.
		nodeID, err := ParseNodeIDParam(r, "id")
		if err != nil {
			httpErr(w, http.StatusBadRequest, "%s", err)
			return
		}

		// Check payload size before reading body.
		if r.ContentLength > maxRequestSize {
			httpErr(w, http.StatusRequestEntityTooLarge, "payload exceeds 1 MiB size cap")
			return
		}

		// Decode request body with strict validation.
		// Uses domain types (RunID, JobID) for type-safe request parsing.
		var req struct {
			RunID  domaintypes.RunID `json:"run_id"` // Run ID (KSUID-backed)
			Events []struct {
				JobID   *domaintypes.JobID     `json:"job_id,omitempty"` // Job ID (KSUID-backed, optional)
				Time    *string                `json:"time,omitempty"`
				Level   string                 `json:"level"`
				Message string                 `json:"message"`
				Meta    map[string]interface{} `json:"meta,omitempty"`
			} `json:"events"`
		}

		if err := DecodeJSON(w, r, &req, maxRequestSize); err != nil {
			return
		}

		// Validate run_id is present using domain type's IsZero method.
		if req.RunID.IsZero() {
			httpErr(w, http.StatusBadRequest, "run_id is required")
			return
		}

		// Validate events array is not empty.
		if len(req.Events) == 0 {
			httpErr(w, http.StatusBadRequest, "events array is required and must not be empty")
			return
		}

		// Check if the node exists before processing.
		_, err = st.GetNode(r.Context(), nodeID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				httpErr(w, http.StatusNotFound, "node not found")
				return
			}
			httpErr(w, http.StatusInternalServerError, "failed to check node: %v", err)
			slog.Error("node events: check failed", "node_id", nodeID, "err", err)
			return
		}

		// Process and persist each event.
		count := 0
		for i, evt := range req.Events {
			// Validate required fields.
			if strings.TrimSpace(evt.Level) == "" {
				httpErr(w, http.StatusBadRequest, "events[%d]: level is required", i)
				return
			}
			if strings.TrimSpace(evt.Message) == "" {
				httpErr(w, http.StatusBadRequest, "events[%d]: message is required", i)
				return
			}

			// Parse job_id if provided.
			var jobID *domaintypes.JobID
			if evt.JobID != nil && !evt.JobID.IsZero() {
				jobID = evt.JobID
			}

			// Parse event time if provided, otherwise use server time.
			eventTime := time.Now().UTC()
			if evt.Time != nil && strings.TrimSpace(*evt.Time) != "" {
				parsedTime, err := time.Parse(time.RFC3339, strings.TrimSpace(*evt.Time))
				if err != nil {
					httpErr(w, http.StatusBadRequest, "events[%d]: invalid time format: %v", i, err)
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
				httpErr(w, http.StatusBadRequest, "events[%d]: failed to marshal meta: %v", i, err)
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
				httpErr(w, http.StatusInternalServerError, "failed to create event: %v", err)
				slog.Error("node events: create failed", "node_id", nodeID.String(), "run_id", req.RunID.String(), "index", i, "err", err)
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
			"node_id", nodeID.String(),
			"run_id", req.RunID.String(),
			"count", count,
		)
	}
}
