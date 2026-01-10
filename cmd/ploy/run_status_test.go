package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRunStatusPrintsSummary(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/runs/batch-123" {
			now := time.Now()
			resp := struct {
				ID        string    `json:"id"`
				Status    string    `json:"status"`
				ModID     string    `json:"mod_id"`
				SpecID    string    `json:"spec_id"`
				CreatedAt time.Time `json:"created_at"`
				Counts    *struct {
					Total         int32  `json:"total"`
					Queued        int32  `json:"queued"`
					Running       int32  `json:"running"`
					Success       int32  `json:"success"`
					Fail          int32  `json:"fail"`
					Cancelled     int32  `json:"cancelled"`
					DerivedStatus string `json:"derived_status"`
				} `json:"repo_counts,omitempty"`
			}{
				ID:        "batch-123",
				Status:    "running",
				ModID:     "mod-123",
				SpecID:    "spec-123",
				CreatedAt: now,
				Counts: &struct {
					Total         int32  `json:"total"`
					Queued        int32  `json:"queued"`
					Running       int32  `json:"running"`
					Success       int32  `json:"success"`
					Fail          int32  `json:"fail"`
					Cancelled     int32  `json:"cancelled"`
					DerivedStatus string `json:"derived_status"`
				}{
					Total:         5,
					Queued:        1,
					Running:       2,
					Success:       2,
					Fail:          0,
					Cancelled:     0,
					DerivedStatus: "running",
				},
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
	err := executeCmd([]string{"run", "status", "batch-123"}, &buf)
	if err != nil {
		t.Fatalf("run status error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Run: batch-123") {
		t.Fatalf("expected output to contain run id; got %q", out)
	}
	if !strings.Contains(out, "Repo Counts") {
		t.Fatalf("expected output to contain Repo Counts; got %q", out)
	}
}
