package main

import (
    "bytes"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
)

func TestJobsListRequiresTicketAndPrintsJobs(t *testing.T) {
    t.Helper()
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.Method == http.MethodGet && r.URL.Path == "/v1/jobs" {
            if r.URL.Query().Get("ticket") != "mods-123" {
                http.Error(w, "ticket required", http.StatusBadRequest)
                return
            }
            _ = json.NewEncoder(w).Encode(map[string]any{"jobs": []map[string]any{{"id": "job-1", "ticket": "mods-123", "step_id": "plan", "state": "completed"}}})
            return
        }
        http.NotFound(w, r)
    }))
    defer server.Close()

    t.Setenv("PLOY_CONTROL_PLANE_URL", server.URL)

    // Missing ticket should error
    if err := execute([]string{"jobs", "ls"}, &bytes.Buffer{}); err == nil {
        t.Fatal("expected error when --ticket missing")
    }

    buf := &bytes.Buffer{}
    err := execute([]string{"jobs", "ls", "--ticket", "mods-123"}, buf)
    if err != nil {
        t.Fatalf("jobs ls error: %v", err)
    }
    if !bytes.Contains(buf.Bytes(), []byte("job-1")) {
        t.Fatalf("expected job id in output, got %q", buf.String())
    }
}
