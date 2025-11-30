package api

import (
	"encoding/json"
	"strings"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// TestJSONRoundTrip ensures typed IDs in the Mods API marshal/unmarshal as JSON strings.
func TestJSONRoundTrip(t *testing.T) {
	in := TicketSummary{
		TicketID: domaintypes.TicketID("t-123"),
		State:    TicketStateRunning,
		Stages: map[string]StageStatus{
			"s1": {StageID: domaintypes.StageID("s1"), State: StageStateQueued, CurrentJobID: domaintypes.JobID("job-1")},
		},
	}

	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// Expect JSON to contain plain strings for ids.
	js := string(b)
	for _, want := range []string{"\"ticket_id\":\"t-123\"", "\"stage_id\":\"s1\"", "\"current_job_id\":\"job-1\""} {
		if !strings.Contains(js, want) {
			t.Fatalf("expected json to contain %s; got %s", want, js)
		}
	}

	var out TicketSummary
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.TicketID != in.TicketID {
		t.Fatalf("ticket id roundtrip mismatch: %v vs %v", out.TicketID, in.TicketID)
	}
	got := out.Stages["s1"].StageID
	if got != in.Stages["s1"].StageID {
		t.Fatalf("stage id roundtrip mismatch: %v vs %v", got, in.Stages["s1"].StageID)
	}
	if out.Stages["s1"].CurrentJobID != in.Stages["s1"].CurrentJobID {
		t.Fatalf("job id roundtrip mismatch: %v vs %v", out.Stages["s1"].CurrentJobID, in.Stages["s1"].CurrentJobID)
	}
}
