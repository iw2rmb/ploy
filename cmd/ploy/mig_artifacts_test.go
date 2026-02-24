package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	modsapi "github.com/iw2rmb/ploy/internal/migs/api"
)

func TestModArtifactsListsStageArtifacts(t *testing.T) {
	t.Helper()
	runID := domaintypes.NewRunID().String()
	stageA := domaintypes.NewJobID()
	stageB := domaintypes.NewJobID()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/runs/"+runID+"/status" {
			// Return RunSummary directly — the canonical response shape.
			_ = json.NewEncoder(w).Encode(modsapi.RunSummary{
				RunID: domaintypes.RunID(runID),
				State: modsapi.RunStateSucceeded,
				Stages: map[domaintypes.JobID]modsapi.StageStatus{
					stageA: {State: modsapi.StageStateSucceeded, Artifacts: map[string]string{"diff": "bafy-diff"}},
					stageB: {State: modsapi.StageStateSucceeded, Artifacts: map[string]string{"logs": "bafy-logs"}},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)
	buf := &bytes.Buffer{}
	err := executeCmd([]string{"mig", "artifacts", runID}, buf)
	if err != nil {
		t.Fatalf("mod artifacts error: %v", err)
	}
	out := buf.String()
	if !bytes.Contains([]byte(out), []byte(stageA.String())) || !bytes.Contains([]byte(out), []byte(stageB.String())) {
		t.Fatalf("expected stage IDs in output; got %q", out)
	}
	if !bytes.Contains([]byte(out), []byte("diff: bafy-diff")) || !bytes.Contains([]byte(out), []byte("logs: bafy-logs")) {
		t.Fatalf("expected artifact entries in output; got %q", out)
	}
}
