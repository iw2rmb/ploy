package jobs

import (
    "bytes"
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "net/url"
    "testing"
)

func TestInspectListRetryCommands(t *testing.T) {
    mux := http.NewServeMux()
    mux.HandleFunc("/v1/jobs", func(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodGet || r.URL.Query().Get("ticket") == "" {
            http.Error(w, "bad", http.StatusBadRequest)
            return
        }
        _ = json.NewEncoder(w).Encode(map[string]any{"jobs": []map[string]any{{"id": "j1", "step_id": "build", "state": "failed"}, {"id": "j2", "step_id": "test", "state": "running"}}})
    })
    mux.HandleFunc("/v1/jobs/j1", func(w http.ResponseWriter, r *http.Request) {
        _ = json.NewEncoder(w).Encode(map[string]any{"id": "j1", "state": "failed", "step_id": "build"})
    })
    mux.HandleFunc("/v1/jobs/j1/retry", func(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodPost || r.URL.Query().Get("ticket") == "" {
            http.Error(w, "bad", http.StatusBadRequest)
            return
        }
        w.WriteHeader(http.StatusAccepted)
    })
    srv := httptest.NewServer(mux)
    defer srv.Close()
    base, _ := url.Parse(srv.URL)

    // List
    var out bytes.Buffer
    if err := (ListCommand{Client: srv.Client(), BaseURL: base, Ticket: "t" , Output: &out}).Run(context.Background()); err != nil {
        t.Fatalf("ls err=%v", err)
    }
    if out.Len() == 0 {
        t.Fatalf("expected list output")
    }

    // Inspect
    out.Reset()
    if err := (InspectCommand{Client: srv.Client(), BaseURL: base, Ticket: "t", JobID: "j1", Output: &out}).Run(context.Background()); err != nil {
        t.Fatalf("inspect err=%v", err)
    }
    if out.Len() == 0 {
        t.Fatalf("expected inspect output")
    }

    // Retry
    out.Reset()
    if err := (RetryCommand{Client: srv.Client(), BaseURL: base, Ticket: "t", JobID: "j1", Output: &out}).Run(context.Background()); err != nil {
        t.Fatalf("retry err=%v", err)
    }
    if out.Len() == 0 {
        t.Fatalf("expected retry output")
    }
}

