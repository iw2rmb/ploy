package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestModCancelCallsControlPlane(t *testing.T) {
	t.Helper()
	var called bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/mods/run-7/cancel" {
			called = true
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"state":"cancelling"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)
	buf := &bytes.Buffer{}
	err := executeCmd([]string{"mod", "cancel", "--run", "run-7", "--reason", "cleanup"}, buf)
	if err != nil {
		t.Fatalf("mod cancel error: %v", err)
	}
	if !called {
		t.Fatalf("expected /v1/mods/{id}/cancel to be called")
	}
}
