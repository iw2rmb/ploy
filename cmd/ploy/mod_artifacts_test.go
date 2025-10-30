package main

import (
    "bytes"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"

    modsapi "github.com/iw2rmb/ploy/internal/mods/api"
)

func TestModArtifactsListsStageArtifacts(t *testing.T) {
    t.Helper()
    ticket := "ticket-artifacts"
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.Method == http.MethodGet && r.URL.Path == "/v1/mods/"+ticket {
            _ = json.NewEncoder(w).Encode(modsapi.TicketStatusResponse{Ticket: modsapi.TicketSummary{
                TicketID: ticket,
                State:    modsapi.TicketStateSucceeded,
                Stages: map[string]modsapi.StageStatus{
                    "plan": {StageID: "plan", State: modsapi.StageStateSucceeded, Artifacts: map[string]string{"diff": "bafy-diff"}},
                    "exec": {StageID: "exec", State: modsapi.StageStateSucceeded, Artifacts: map[string]string{"logs": "bafy-logs"}},
                },
            }})
            return
        }
        http.NotFound(w, r)
    }))
    defer server.Close()

    t.Setenv("PLOY_CONTROL_PLANE_URL", server.URL)
    buf := &bytes.Buffer{}
    err := execute([]string{"mod", "artifacts", ticket}, buf)
    if err != nil {
        t.Fatalf("mod artifacts error: %v", err)
    }
    out := buf.String()
    if !bytes.Contains([]byte(out), []byte("plan")) || !bytes.Contains([]byte(out), []byte("exec")) {
        t.Fatalf("expected stage names in output; got %q", out)
    }
}

