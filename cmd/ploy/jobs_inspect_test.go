package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestJobsInspectRequiresArgsAndPrintsSummary(t *testing.T) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/jobs/job-55" {
			if r.URL.Query().Get("ticket") == "mods-xyz" {
				_, _ = w.Write([]byte(`{"id":"job-55","ticket":"mods-xyz","step_id":"plan","state":"running"}`))
				return
			}
			http.Error(w, "ticket required", http.StatusBadRequest)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	t.Setenv("PLOY_CONTROL_PLANE_URL", server.URL)

	// Missing args
	if err := execute([]string{"jobs", "inspect"}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected error when args missing")
	}

	buf := &bytes.Buffer{}
	err := execute([]string{"jobs", "inspect", "--ticket", "mods-xyz", "job-55"}, buf)
	if err != nil {
		t.Fatalf("jobs inspect error: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("job-55")) {
		t.Fatalf("expected job id in output, got %q", buf.String())
	}
}
