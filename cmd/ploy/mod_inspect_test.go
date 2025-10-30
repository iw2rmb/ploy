package main

import (
    "bytes"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"

    modsapi "github.com/iw2rmb/ploy/internal/mods/api"
)

func TestModInspectPrintsSummary(t *testing.T) {
    t.Helper()
    ticket := "ticket-11"
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.Method == http.MethodGet && r.URL.Path == "/v1/mods/"+ticket {
            _ = json.NewEncoder(w).Encode(modsapi.TicketStatusResponse{Ticket: modsapi.TicketSummary{TicketID: ticket, State: modsapi.TicketStateRunning}})
            return
        }
        http.NotFound(w, r)
    }))
    defer server.Close()

    t.Setenv("PLOY_CONTROL_PLANE_URL", server.URL)
    buf := &bytes.Buffer{}
    err := execute([]string{"mod", "inspect", ticket}, buf)
    if err != nil {
        t.Fatalf("mod inspect error: %v", err)
    }
    out := buf.String()
    if out == "" || !bytes.Contains([]byte(out), []byte(ticket)) {
        t.Fatalf("expected summary output to include ticket id; got %q", out)
    }
}

