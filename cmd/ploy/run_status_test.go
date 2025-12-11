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
				Name      *string   `json:"name,omitempty"`
				Status    string    `json:"status"`
				RepoURL   string    `json:"repo_url"`
				BaseRef   string    `json:"base_ref"`
				TargetRef string    `json:"target_ref"`
				CreatedAt time.Time `json:"created_at"`
				Counts    *struct {
					Total         int32  `json:"total"`
					Pending       int32  `json:"pending"`
					Running       int32  `json:"running"`
					Succeeded     int32  `json:"succeeded"`
					Failed        int32  `json:"failed"`
					Skipped       int32  `json:"skipped"`
					Cancelled     int32  `json:"cancelled"`
					DerivedStatus string `json:"derived_status"`
				} `json:"repo_counts,omitempty"`
			}{
				ID:      "batch-123",
				Status:  "running",
				RepoURL: "https://github.com/org/repo.git",
				BaseRef: "main", TargetRef: "feature",
				CreatedAt: now,
				Counts: &struct {
					Total         int32  `json:"total"`
					Pending       int32  `json:"pending"`
					Running       int32  `json:"running"`
					Succeeded     int32  `json:"succeeded"`
					Failed        int32  `json:"failed"`
					Skipped       int32  `json:"skipped"`
					Cancelled     int32  `json:"cancelled"`
					DerivedStatus string `json:"derived_status"`
				}{
					Total:         5,
					Pending:       1,
					Running:       2,
					Succeeded:     2,
					Failed:        0,
					Skipped:       0,
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
