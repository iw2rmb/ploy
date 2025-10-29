package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "net/http"
    "net/http/httptest"
    "os"
    "path/filepath"
    "strings"
    "testing"
    "time"

    modsapi "github.com/iw2rmb/ploy/internal/mods/api"
)

// End-to-end happy path for 3.1: submit, follow events, download artifacts.
func TestModRunFollowStreamsAndDownloadsArtifacts(t *testing.T) {
    t.Helper()

    ticketID := "mods-follow-test"
    artifactCID := "bafy-artifact-test"

    // Minimal control-plane emulator.
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        switch {
        case r.Method == http.MethodPost && r.URL.Path == "/v1/mods":
            var req modsapi.TicketSubmitRequest
            _ = json.NewDecoder(r.Body).Decode(&req)
            if req.TicketID != ticketID {
                t.Fatalf("unexpected ticket id %q", req.TicketID)
            }
            w.WriteHeader(http.StatusAccepted)
            _ = json.NewEncoder(w).Encode(modsapi.TicketSubmitResponse{Ticket: modsapi.TicketSummary{TicketID: req.TicketID, State: modsapi.TicketStateRunning}})

        case r.Method == http.MethodGet && r.URL.Path == fmt.Sprintf("/v1/mods/%s/events", ticketID):
            // SSE stream: ticket running -> ticket succeeded
            w.Header().Set("Content-Type", "text/event-stream")
            fl, ok := w.(http.Flusher)
            if !ok { t.Fatalf("no flusher") }
            // running
            _, _ = w.Write([]byte("event: ticket\n"))
            data, _ := json.Marshal(modsapi.TicketSummary{TicketID: ticketID, State: modsapi.TicketStateRunning})
            _, _ = w.Write([]byte("data: "))
            _, _ = w.Write(data)
            _, _ = w.Write([]byte("\n\n"))
            fl.Flush()
            time.Sleep(5 * time.Millisecond)
            // succeeded
            _, _ = w.Write([]byte("event: ticket\n"))
            data2, _ := json.Marshal(modsapi.TicketSummary{TicketID: ticketID, State: modsapi.TicketStateSucceeded})
            _, _ = w.Write([]byte("data: "))
            _, _ = w.Write(data2)
            _, _ = w.Write([]byte("\n\n"))
            fl.Flush()

        case r.Method == http.MethodGet && r.URL.Path == fmt.Sprintf("/v1/mods/%s", ticketID):
            w.Header().Set("Content-Type", "application/json")
            _ = json.NewEncoder(w).Encode(modsapi.TicketStatusResponse{Ticket: modsapi.TicketSummary{
                TicketID: ticketID,
                State:    modsapi.TicketStateSucceeded,
                Stages: map[string]modsapi.StageStatus{
                    "plan": {StageID: "plan", State: modsapi.StageStateSucceeded, Artifacts: map[string]string{"diff": artifactCID}},
                },
            }})

        case r.Method == http.MethodGet && r.URL.Path == "/v1/artifacts":
            if q := r.URL.Query().Get("cid"); q != artifactCID {
                t.Fatalf("unexpected artifact lookup cid: %q", q)
            }
            w.Header().Set("Content-Type", "application/json")
            _, _ = w.Write([]byte(`{"artifacts":[{"id":"artifact-1","cid":"` + artifactCID + `","digest":"sha256:deadbeef","name":"plan-diff.tar.gz","size":10}]}`))

        case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/artifacts/artifact-1"):
            // Download bytes
            _, _ = w.Write([]byte("artifact-bytes"))

        default:
            t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
        }
    }))
    defer server.Close()

    t.Setenv(controlPlaneURLEnv, server.URL)

    dir := t.TempDir()
    buf := &bytes.Buffer{}
    args := []string{"--tenant", "acme", "--ticket", ticketID, "--follow", "--artifact-dir", dir}
    if err := executeModRun(args, buf); err != nil {
        t.Fatalf("executeModRun error: %v", err)
    }

    // Output should at least acknowledge submission and success.
    out := buf.String()
    if !strings.Contains(out, "submitted") {
        t.Fatalf("expected submission message, got: %s", out)
    }
    if !strings.Contains(strings.ToLower(out), "succeeded") {
        t.Fatalf("expected success in output, got: %s", out)
    }

    // An artifact should be written and a manifest.json produced.
    files, err := os.ReadDir(dir)
    if err != nil { t.Fatal(err) }
    var hasManifest, hasArtifact bool
    for _, f := range files {
        if f.Name() == "manifest.json" { hasManifest = true }
        if strings.Contains(f.Name(), "deadbeef") || strings.Contains(f.Name(), artifactCID) { hasArtifact = true }
    }
    if !hasManifest { t.Fatalf("manifest.json not found in %s; files=%v", dir, list(dir)) }
    if !hasArtifact { t.Fatalf("artifact file not found in %s; files=%v", dir, list(dir)) }
}

func list(dir string) []string {
    entries, _ := os.ReadDir(dir)
    out := make([]string, 0, len(entries))
    for _, e := range entries { out = append(out, filepath.Join(dir, e.Name())) }
    return out
}
