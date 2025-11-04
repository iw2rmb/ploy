package events

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
)

// FuzzPublishTicketRoundTrip ensures arbitrary ticket payloads marshal/unmarshal without panicking
// and that the hub stores "ticket"-typed events. Runs only with `-fuzz`.
func FuzzPublishTicketRoundTrip(f *testing.F) {
	svc, err := New(Options{BufferSize: 2, HistorySize: 8})
	if err != nil {
		f.Fatalf("new service: %v", err)
	}
	// Seed a few interesting values.
	f.Add(" ", "", uint8(0))
	f.Add("run-123", "ticket-xyz", uint8(1))
	f.Add("run-123", "ticket-xyz", uint8(255))

	f.Fuzz(func(t *testing.T, runID, ticketID string, stateByte uint8) {
		// Map byte to a valid ticket state.
		states := []modsapi.TicketState{
			modsapi.TicketStatePending,
			modsapi.TicketStateRunning,
			modsapi.TicketStateSucceeded,
			modsapi.TicketStateFailed,
			modsapi.TicketStateCancelled,
		}
		state := states[int(stateByte)%len(states)]

		payload := modsapi.TicketSummary{TicketID: ticketID, State: state}
		ctx := context.Background()

		err := svc.PublishTicket(ctx, runID, payload)
		if strings.TrimSpace(runID) == "" {
			if err == nil {
				t.Fatalf("expected error for empty runID")
			}
			return
		}
		if err != nil {
			t.Fatalf("publish ticket: %v", err)
		}
		snap := svc.Hub().Snapshot(runID)
		if len(snap) == 0 {
			t.Fatalf("expected at least one event in snapshot")
		}
		if got := strings.ToLower(snap[0].Type); got != "ticket" {
			t.Fatalf("unexpected event type: %s", got)
		}
		var out modsapi.TicketSummary
		if err := json.Unmarshal(snap[0].Data, &out); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
	})
}
