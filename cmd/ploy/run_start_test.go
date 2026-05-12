package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/testutil/assertx"
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

	out := clienv.RunExpectOK(t, executeCmd, []string{"run", "start", runID.String()})
	if !called {
		t.Fatalf("expected POST /v1/runs/%s/start to be called", runID.String())
	}
	assertx.Contains(t, out, runID.String())
	assertx.Contains(t, out, "started 3")
	assertx.Contains(t, out, "1 already done")
}
