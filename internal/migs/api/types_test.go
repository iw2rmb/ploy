package api

import (
	"encoding/json"
	"strings"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// TestJSONRoundTrip ensures typed IDs in the Mods API marshal/unmarshal as JSON strings.
// Verifies that RunSummary.RunID marshals as "run_id" in JSON for wire compatibility.
func TestJSONRoundTrip(t *testing.T) {
	runID := domaintypes.NewRunID()
	stageKey := domaintypes.NewJobID()
	jobID := domaintypes.NewJobID()
	nextID := domaintypes.NewJobID()

	in := RunSummary{
		RunID: runID,
		State: RunStateRunning,
		Stages: map[domaintypes.JobID]StageStatus{
			stageKey: {State: StageStateQueued, CurrentJobID: jobID, NextID: &nextID},
		},
	}

	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// Expect JSON to contain plain strings for ids.
	// RunID marshals as "run_id" to maintain wire format compatibility.
	js := string(b)
	for _, want := range []string{
		"\"run_id\":\"" + runID.String() + "\"",
		"\"current_job_id\":\"" + jobID.String() + "\"",
		"\"next_id\":\"" + nextID.String() + "\"",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("expected json to contain %s; got %s", want, js)
		}
	}

	var out RunSummary
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.RunID != in.RunID {
		t.Fatalf("run id roundtrip mismatch: %v vs %v", out.RunID, in.RunID)
	}
	if out.Stages[stageKey].CurrentJobID != in.Stages[stageKey].CurrentJobID {
		t.Fatalf("job id roundtrip mismatch: %v vs %v", out.Stages[stageKey].CurrentJobID, in.Stages[stageKey].CurrentJobID)
	}
	if out.Stages[stageKey].NextID == nil || in.Stages[stageKey].NextID == nil || *out.Stages[stageKey].NextID != *in.Stages[stageKey].NextID {
		t.Fatalf("next id roundtrip mismatch: %v vs %v", out.Stages[stageKey].NextID, in.Stages[stageKey].NextID)
	}
}
