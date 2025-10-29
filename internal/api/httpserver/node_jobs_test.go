package httpserver_test

import (
    "encoding/json"
    "net/http/httptest"
    "testing"

    "github.com/iw2rmb/ploy/internal/api/httpserver"
    "github.com/iw2rmb/ploy/internal/node/jobs"
    "github.com/iw2rmb/ploy/internal/node/logstream"
)

// TestNodeJobsEndpoints validates list and detail JSON payloads.
func TestNodeJobsEndpoints(t *testing.T) {
    t.Helper()

    cfg := loadConfig(t, `
control_plane:
  endpoint: https://control.example.com
  ca: /etc/ploy/pki/ca.pem
  certificate: /etc/ploy/pki/node.pem
  key: /etc/ploy/pki/node-key.pem
runtime:
  plugins:
    - name: local
      module: internal
`)

    hub := logstream.NewHub(logstream.Options{})
    tracker := jobs.NewStore(jobs.Options{Capacity: 8})
    tracker.Start("abc")
    tracker.Start("xyz")
    tracker.Complete("abc", jobs.StateFailed, "boom")

    server, err := httpserver.New(httpserver.Options{
        Config:  cfg,
        Streams: hub,
        Status:  &stubStatus{},
        Jobs:    httpserver.NewJobProvider(tracker),
    })
    if err != nil {
        t.Fatalf("New() error = %v", err)
    }

    app := server.App()

    // List
    req := httptest.NewRequest("GET", "/v1/node/jobs", nil)
    resp, err := app.Test(req, 1000)
    if err != nil {
        t.Fatalf("list: app.Test() error = %v", err)
    }
    defer resp.Body.Close()
    if resp.StatusCode != 200 {
        t.Fatalf("unexpected status %d", resp.StatusCode)
    }
    var list []map[string]any
    if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
        t.Fatalf("decode list: %v", err)
    }
    if len(list) != 2 {
        t.Fatalf("list len=%d want 2", len(list))
    }

    // Detail
    req = httptest.NewRequest("GET", "/v1/node/jobs/abc", nil)
    resp, err = app.Test(req, 1000)
    if err != nil {
        t.Fatalf("detail: app.Test() error = %v", err)
    }
    defer resp.Body.Close()
    if resp.StatusCode != 200 {
        t.Fatalf("unexpected status %d", resp.StatusCode)
    }
    var detail map[string]any
    if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
        t.Fatalf("decode detail: %v", err)
    }
    if detail["id"] != "abc" || detail["state"] != string(jobs.StateFailed) {
        t.Fatalf("unexpected detail: %+v", detail)
    }
}
