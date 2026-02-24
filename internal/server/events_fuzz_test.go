package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	modsapi "github.com/iw2rmb/ploy/internal/migs/api"
)

// FuzzPublishRunRoundTrip ensures arbitrary run payloads marshal/unmarshal without panicking
// and that the hub stores "run"-typed events. Runs only with `-fuzz`.
func FuzzPublishRunRoundTrip(f *testing.F) {
	svc, err := NewEventsService(EventsOptions{BufferSize: 2, HistorySize: 8})
	if err != nil {
		f.Fatalf("new service: %v", err)
	}
	// Seed a few interesting values. The second parameter is the RunID field value.
	f.Add(" ", "", uint8(0))
	f.Add(domaintypes.NewRunID().String(), domaintypes.NewRunID().String(), uint8(1))
	f.Add(domaintypes.NewRunID().String(), domaintypes.NewRunID().String(), uint8(255))

	f.Fuzz(func(t *testing.T, streamRunID, payloadRunID string, stateByte uint8) {
		// Map byte to a valid run state.
		states := []modsapi.RunState{
			modsapi.RunStatePending,
			modsapi.RunStateRunning,
			modsapi.RunStateSucceeded,
			modsapi.RunStateFailed,
			modsapi.RunStateCancelled,
		}
		state := states[int(stateByte)%len(states)]

		// Build payload using renamed RunID field.
		payloadID := domaintypes.RunID(payloadRunID)
		payload := modsapi.RunSummary{RunID: payloadID, State: state}
		ctx := context.Background()

		err := svc.PublishRun(ctx, domaintypes.RunID(streamRunID), payload)
		if strings.TrimSpace(streamRunID) == "" {
			if err == nil {
				t.Fatalf("expected error for empty runID")
			}
			return
		}
		if _, idErr := payloadID.MarshalText(); idErr != nil {
			// Payload contains an invalid RunID, so JSON marshaling should fail.
			if err == nil {
				t.Fatalf("expected error for invalid payload runID")
			}
			return
		}
		if err != nil {
			t.Fatalf("publish run: %v", err)
		}
		snap := svc.Hub().Snapshot(domaintypes.RunID(streamRunID))
		if len(snap) == 0 {
			t.Fatalf("expected at least one event in snapshot")
		}
		if snap[0].Type != domaintypes.SSEEventRun {
			t.Fatalf("unexpected event type: %s", snap[0].Type)
		}
		var out modsapi.RunSummary
		if err := json.Unmarshal(snap[0].Data, &out); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
	})
}
