package mods

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// Event represents a telemetry event for runner progress
type Event struct {
	Phase   string    `json:"phase,omitempty"`
	Step    string    `json:"step,omitempty"`
	Level   string    `json:"level,omitempty"`
	Message string    `json:"message,omitempty"`
	Time    time.Time `json:"ts,omitempty"`
	JobName string    `json:"job_name,omitempty"`
	AllocID string    `json:"alloc_id,omitempty"`
}

// EventReporter publishes events to an external sink (e.g., controller API)
type EventReporter interface {
	Report(ctx context.Context, ev Event) error
}

// ControllerEventReporter posts events to the controller's /v1/mods/:id/events endpoint
type ControllerEventReporter struct {
	endpoint string
	execID   string
	client   *http.Client
}

// NewControllerEventReporter creates a reporter targeting the given controller URL (with /v1 suffix) and execution ID
func NewControllerEventReporter(controllerURL, executionID string) EventReporter {
	endpoint := strings.TrimRight(controllerURL, "/") + "/mods/" + strings.TrimLeft(executionID, "/") + "/events"
	return &ControllerEventReporter{
		endpoint: endpoint,
		execID:   executionID,
		client:   &http.Client{Timeout: 5 * time.Second},
	}
}

func (r *ControllerEventReporter) Report(ctx context.Context, ev Event) error {
	payload := map[string]interface{}{
		"phase":   ev.Phase,
		"step":    ev.Step,
		"level":   ev.Level,
		"message": ev.Message,
	}
	if !ev.Time.IsZero() {
		payload["ts"] = ev.Time
	}
	if ev.JobName != "" {
		payload["job_name"] = ev.JobName
	}
	if ev.AllocID != "" {
		payload["alloc_id"] = ev.AllocID
	}
	b, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.client.Do(req)
	if err != nil {
		// Best-effort: do not fail runner due to telemetry
		return nil
	}
	_ = resp.Body.Close()
	return nil
}
