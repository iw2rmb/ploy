package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/iw2rmb/ploy/internal/testutil/clienv"
)

func TestRunCancelCallsControlPlane(t *testing.T) {
	t.Helper()
	var called bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/runs/run-7/cancel" {
			called = true
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"state":"cancelling"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	clienv.UseServerDescriptor(t, server.URL)
	buf := &bytes.Buffer{}
	err := executeCmd([]string{"run", "cancel", "run-7", "--reason", "cleanup"}, buf)
	if err != nil {
		t.Fatalf("run cancel error: %v", err)
	}
	if !called {
		t.Fatalf("expected /v1/runs/{id}/cancel to be called")
	}
}
