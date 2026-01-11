package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// TestRunStopCallsControlPlane validates `ploy run stop <run-id>` calls the API.
// Not parallel because useServerDescriptor uses t.Setenv.
func TestRunStopCallsControlPlane(t *testing.T) {
	var called bool
	runID := domaintypes.NewRunID()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/runs/"+runID.String()+"/cancel" {
			called = true

			now := time.Now()
			resp := struct {
				ID        string    `json:"id"`
				Status    string    `json:"status"`
				CreatedAt time.Time `json:"created_at"`
				Counts    *struct {
					Total     int32 `json:"total"`
					Cancelled int32 `json:"cancelled"`
				} `json:"repo_counts,omitempty"`
			}{
				ID:        runID.String(),
				Status:    "Cancelled",
				CreatedAt: now,
				Counts: &struct {
					Total     int32 `json:"total"`
					Cancelled int32 `json:"cancelled"`
				}{Total: 5, Cancelled: 3},
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
	err := executeCmd([]string{"run", "stop", runID.String()}, &buf)
	if err != nil {
		t.Fatalf("run stop error: %v", err)
	}
	if !called {
		t.Fatalf("expected POST /v1/runs/%s/cancel to be called", runID.String())
	}

	output := buf.String()
	if !strings.Contains(output, runID.String()) {
		t.Errorf("output should contain %s: %s", runID.String(), output)
	}
	if !strings.Contains(output, "stopped") {
		t.Errorf("output should contain stopped: %s", output)
	}
	if !strings.Contains(output, "Cancelled 3") {
		t.Errorf("output should contain Cancelled 3: %s", output)
	}
}
