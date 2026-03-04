package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/testutil/clienv"
)

// TestRunStartCallsControlPlane validates `ploy run start <run-id>` calls the API.
// Not parallel because useServerDescriptor uses t.Setenv.
func TestRunStartCallsControlPlane(t *testing.T) {
	var called bool
	runID := domaintypes.NewRunID()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/runs/"+runID.String()+"/start" {
			called = true

			resp := struct {
				RunID       domaintypes.RunID `json:"run_id"`
				Started     int               `json:"started"`
				AlreadyDone int               `json:"already_done"`
				Pending     int               `json:"pending"`
			}{
				RunID:       runID,
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

	clienv.UseServerDescriptor(t, server.URL)

	var buf bytes.Buffer
	err := executeCmd([]string{"run", "start", runID.String()}, &buf)
	if err != nil {
		t.Fatalf("run start error: %v", err)
	}
	if !called {
		t.Fatalf("expected POST /v1/runs/%s/start to be called", runID.String())
	}

	output := buf.String()
	if !strings.Contains(output, runID.String()) {
		t.Errorf("output should contain %s: %s", runID.String(), output)
	}
	if !strings.Contains(output, "started 3") {
		t.Errorf("output should contain started 3: %s", output)
	}
	if !strings.Contains(output, "1 already done") {
		t.Errorf("output should contain 1 already done: %s", output)
	}
}
