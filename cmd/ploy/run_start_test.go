package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRunStartCallsControlPlane validates `ploy run start <run-id>` calls the API.
// Not parallel because useServerDescriptor uses t.Setenv.
func TestRunStartCallsControlPlane(t *testing.T) {
	var called bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/runs/run-789/start" {
			called = true

			resp := struct {
				RunID       string `json:"run_id"`
				Started     int    `json:"started"`
				AlreadyDone int    `json:"already_done"`
				Pending     int    `json:"pending"`
			}{
				RunID:       "run-789",
				Started:     3,
				AlreadyDone: 1,
				Pending:     0,
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)

	var buf bytes.Buffer
	err := executeCmd([]string{"run", "start", "run-789"}, &buf)
	if err != nil {
		t.Fatalf("run start error: %v", err)
	}
	if !called {
		t.Fatal("expected POST /v1/runs/run-789/start to be called")
	}

	output := buf.String()
	if !strings.Contains(output, "run-789") {
		t.Errorf("output should contain run-789: %s", output)
	}
	if !strings.Contains(output, "started 3") {
		t.Errorf("output should contain started 3: %s", output)
	}
	if !strings.Contains(output, "1 already done") {
		t.Errorf("output should contain 1 already done: %s", output)
	}
}
