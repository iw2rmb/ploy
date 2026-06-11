package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	migsapi "github.com/iw2rmb/ploy/internal/migs/api"
	"github.com/iw2rmb/ploy/internal/testutil/assertx"
	"github.com/iw2rmb/ploy/internal/testutil/clienv"
)

func TestMigArtifactsListsStageArtifacts(t *testing.T) {
	runID := domaintypes.NewRunID().String()
	stageA := domaintypes.NewJobID()
	stageB := domaintypes.NewJobID()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/runs/"+runID+"/status" {
			// Return RunSummary directly — the canonical response shape.
			_ = json.NewEncoder(w).Encode(migsapi.RunSummary{
				RunID: domaintypes.RunID(runID),
				State: migsapi.RunStateSucceeded,
				Stages: map[domaintypes.JobID]migsapi.StageStatus{
					stageA: {State: migsapi.StageStateSucceeded, Artifacts: map[string]string{"diff": "bafy-diff"}},
					stageB: {State: migsapi.StageStateSucceeded, Artifacts: map[string]string{"logs": "bafy-logs"}},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	clienv.UseControlPlaneEnv(t, server.URL)
	out := clienv.RunExpectOK(t, executeCmd, []string{"mig", "artifacts", runID})
	assertx.Contains(t, out, stageA.String())
	assertx.Contains(t, out, stageB.String())
	assertx.Contains(t, out, "diff: bafy-diff")
	assertx.Contains(t, out, "logs: bafy-logs")
}
