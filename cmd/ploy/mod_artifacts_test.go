package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
)

func TestModArtifactsListsStageArtifacts(t *testing.T) {
	t.Helper()
	runID := "run-artifacts"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/mods/"+runID {
			// Return RunSummary directly — the canonical response shape.
			_ = json.NewEncoder(w).Encode(modsapi.RunSummary{
				RunID: domaintypes.RunID(runID),
				State: modsapi.RunStateSucceeded,
				Stages: map[string]modsapi.StageStatus{
					"plan": {State: modsapi.StageStateSucceeded, Artifacts: map[string]string{"diff": "bafy-diff"}},
					"exec": {State: modsapi.StageStateSucceeded, Artifacts: map[string]string{"logs": "bafy-logs"}},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)
	buf := &bytes.Buffer{}
	err := executeCmd([]string{"mod", "artifacts", runID}, buf)
	if err != nil {
		t.Fatalf("mod artifacts error: %v", err)
	}
	out := buf.String()
	if !bytes.Contains([]byte(out), []byte("plan")) || !bytes.Contains([]byte(out), []byte("exec")) {
		t.Fatalf("expected stage names in output; got %q", out)
	}
}
