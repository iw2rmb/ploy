package httpserver_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/node/logstream"
)

func TestModsHTTPSubmitStatusLifecycle(t *testing.T) {
	t.Parallel()

	fixture := newModsServerFixture(t)
	ticketID := "mod-http-1"

	submit := postJSON(t, fixture.server.URL+"/v1/mods", map[string]any{
		"ticket_id": ticketID,
		"submitter": "cli",
		"stages": []any{
			map[string]any{"id": "plan"},
		},
	})
	ticket, ok := submit["ticket"].(map[string]any)
	if !ok {
		t.Fatalf("expected ticket in submit response")
	}
	if got, _ := ticket["ticket_id"].(string); got != ticketID {
		t.Fatalf("unexpected ticket id %q", got)
	}

	statusURL := fmt.Sprintf("%s/v1/mods/%s", fixture.server.URL, ticketID)
	status := getJSON(t, statusURL)
	statusTicket, ok := status["ticket"].(map[string]any)
	if !ok {
		t.Fatalf("expected ticket block in status response")
	}
	stages, ok := statusTicket["stages"].(map[string]any)
	if !ok || len(stages) != 1 {
		t.Fatalf("expected stages map in status response, got %+v", statusTicket["stages"])
	}
	stage, ok := stages["plan"].(map[string]any)
	if !ok {
		t.Fatalf("expected plan stage in status response")
	}
	if state, _ := stage["state"].(string); state != "queued" {
		t.Fatalf("expected stage queued, got %q", state)
	}
	if jobID, _ := stage["current_job_id"].(string); strings.TrimSpace(jobID) == "" {
		t.Fatalf("expected current job id on queued stage")
	}

	cancelStatus, _ := postJSONStatus(t, fmt.Sprintf("%s/v1/mods/%s/cancel", fixture.server.URL, ticketID), map[string]any{})
	if cancelStatus != http.StatusAccepted {
		t.Fatalf("expected cancel status 202, got %d", cancelStatus)
	}

	cancelled := getJSON(t, statusURL)
	cancelStages := cancelled["ticket"].(map[string]any)["stages"].(map[string]any)
	cancelStage := cancelStages["plan"].(map[string]any)
	if state, _ := cancelStage["state"].(string); state != "cancelled" {
		t.Fatalf("expected stage cancelled, got %q", state)
	}

	resume := postJSON(t, fmt.Sprintf("%s/v1/mods/%s/resume", fixture.server.URL, ticketID), map[string]any{})
	resumeTicket := resume["ticket"].(map[string]any)
	resumeStages := resumeTicket["stages"].(map[string]any)
	resumeStage := resumeStages["plan"].(map[string]any)
	if state, _ := resumeStage["state"].(string); state != "queued" {
		t.Fatalf("expected stage queued after resume, got %q", state)
	}
	if jobID, _ := resumeStage["current_job_id"].(string); strings.TrimSpace(jobID) == "" {
		t.Fatalf("expected new job id after resume")
	}

	legacy := getJSON(t, fmt.Sprintf("%s/v1/mods/tickets/%s", fixture.server.URL, ticketID))
	legacyTicket := legacy["ticket"].(map[string]any)
	if got, _ := legacyTicket["ticket_id"].(string); got != ticketID {
		t.Fatalf("legacy endpoint returned wrong ticket id %q", got)
	}
}

func TestModsLogsEndpoints(t *testing.T) {
	t.Parallel()

	fixture := newModsServerFixture(t)
	ticketID := "mod-logs-1"

	fixture.streams.Ensure(ticketID)
	if err := fixture.streams.PublishLog(context.Background(), ticketID, logstream.LogRecord{
		Timestamp: "2025-10-24T10:00:00Z",
		Stream:    "stdout",
		Line:      "starting stage",
	}); err != nil {
		t.Fatalf("publish log: %v", err)
	}

	logs := getJSON(t, fmt.Sprintf("%s/v1/mods/%s/logs", fixture.server.URL, ticketID))
	events, ok := logs["events"].([]any)
	if !ok || len(events) == 0 {
		t.Fatalf("expected snapshot events in logs response")
	}
	first, ok := events[0].(map[string]any)
	if !ok {
		t.Fatalf("expected event map in logs snapshot")
	}
	if evtType, _ := first["type"].(string); evtType != "log" {
		t.Fatalf("expected first snapshot event log, got %q", evtType)
	}
	payload, ok := first["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected event payload map")
	}
	if line, _ := payload["line"].(string); line != "starting stage" {
		t.Fatalf("unexpected log line %q", line)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	eventCh := make(chan sseEvent, 4)
	errCh := make(chan error, 1)
	go func() {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/v1/mods/%s/logs/stream", fixture.server.URL, ticketID), nil)
		if err != nil {
			errCh <- err
			return
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			errCh <- err
			return
		}
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			errCh <- fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
			return
		}
		defer func() {
			_ = resp.Body.Close()
		}()
		reader := bufio.NewReader(resp.Body)
		for {
			evt, err := readSSEEvent(reader)
			if err != nil {
				errCh <- err
				return
			}
			eventCh <- evt
			if evt.Type == "done" {
				return
			}
		}
	}()

	time.Sleep(50 * time.Millisecond)

	if err := fixture.streams.PublishLog(context.Background(), ticketID, logstream.LogRecord{
		Timestamp: "2025-10-24T10:00:01Z",
		Stream:    "stderr",
		Line:      "warning: retry",
	}); err != nil {
		t.Fatalf("publish follow-up log: %v", err)
	}
	if err := fixture.streams.PublishRetention(context.Background(), ticketID, logstream.RetentionHint{
		Retained: true,
		TTL:      "72h",
		Expires:  "2025-10-27T10:00:00Z",
		Bundle:   "bafy-log-bundle",
	}); err != nil {
		t.Fatalf("publish retention: %v", err)
	}
	if err := fixture.streams.PublishStatus(context.Background(), ticketID, logstream.Status{Status: "completed"}); err != nil {
		t.Fatalf("publish status: %v", err)
	}

	wantOrder := []string{"log", "log", "retention", "done"}
	for i := 0; i < len(wantOrder); i++ {
		select {
		case evt := <-eventCh:
			if evt.Type != wantOrder[i] {
				t.Fatalf("expected event %s at position %d, got %s", wantOrder[i], i, evt.Type)
			}
		case err := <-errCh:
			t.Fatalf("stream error: %v", err)
		case <-ctx.Done():
			t.Fatalf("timed out waiting for event %d", i)
		}
	}
}

func TestModsEventsStream(t *testing.T) {
	t.Parallel()

	fixture := newModsServerFixture(t)
	ticketID := "mod-events-1"

	postJSON(t, fixture.server.URL+"/v1/mods", map[string]any{
		"ticket_id": ticketID,
		"stages": []any{
			map[string]any{"id": "plan"},
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	eventCh := make(chan sseEvent, 6)
	errCh := make(chan error, 1)

	go func() {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/v1/mods/%s/events", fixture.server.URL, ticketID), nil)
		if err != nil {
			errCh <- err
			return
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			errCh <- err
			return
		}
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			errCh <- fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
			return
		}
		defer func() {
			_ = resp.Body.Close()
		}()
		reader := bufio.NewReader(resp.Body)
		for {
			evt, err := readSSEEvent(reader)
			if err != nil {
				errCh <- err
				return
			}
			eventCh <- evt
		}
	}()

	var initial sseEvent
	select {
	case evt := <-eventCh:
		initial = evt
	case err := <-errCh:
		t.Fatalf("initial stream error: %v", err)
	case <-ctx.Done():
		t.Fatalf("timeout waiting for initial mods event")
	}
	if initial.Type != "ticket" {
		t.Fatalf("expected initial ticket event, got %s", initial.Type)
	}
	var initialPayload map[string]any
	if err := json.Unmarshal([]byte(initial.Data), &initialPayload); err != nil {
		t.Fatalf("decode initial payload: %v", err)
	}
	if state, _ := initialPayload["state"].(string); state == "" {
		t.Fatalf("expected ticket state in initial payload")
	}

	cancelStatus, _ := postJSONStatus(t, fmt.Sprintf("%s/v1/mods/%s/cancel", fixture.server.URL, ticketID), map[string]any{})
	if cancelStatus != http.StatusAccepted {
		t.Fatalf("expected cancel status 202, got %d", cancelStatus)
	}

	cancelled := false
	timeout := time.After(4 * time.Second)
	for !cancelled {
		select {
		case evt := <-eventCh:
			if evt.Type != "ticket" {
				continue
			}
			var payload map[string]any
			if err := json.Unmarshal([]byte(evt.Data), &payload); err != nil {
				t.Fatalf("decode ticket payload: %v", err)
			}
			if state, _ := payload["state"].(string); state == "cancelled" {
				cancelled = true
			}
		case err := <-errCh:
			t.Fatalf("stream error: %v", err)
		case <-timeout:
			t.Fatalf("timed out waiting for cancelled ticket event")
		}
	}
}
