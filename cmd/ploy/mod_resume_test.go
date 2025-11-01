package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestModResumeCallsControlPlane(t *testing.T) {
	t.Helper()
	var called bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/mods/ticket-9/resume" {
			called = true
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"state":"running"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	t.Setenv("PLOY_CONTROL_PLANE_URL", server.URL)
	buf := &bytes.Buffer{}
	err := execute([]string{"mod", "resume", "ticket-9"}, buf)
	if err != nil {
		t.Fatalf("mod resume error: %v", err)
	}
	if !called {
		t.Fatalf("expected /v1/mods/{ticket}/resume to be called")
	}
}
