package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestJobsRetryPostsToControlPlane(t *testing.T) {
	t.Helper()
	var called bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/jobs/job-77/retry" && r.URL.Query().Get("ticket") == "mods-rt" {
			called = true
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"status":"accepted"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	t.Setenv("PLOY_CONTROL_PLANE_URL", server.URL)
	buf := &bytes.Buffer{}
	err := execute([]string{"jobs", "retry", "--ticket", "mods-rt", "job-77"}, buf)
	if err != nil {
		t.Fatalf("jobs retry error: %v", err)
	}
	if !called {
		t.Fatalf("expected /v1/jobs/{id}/retry to be called")
	}
}
