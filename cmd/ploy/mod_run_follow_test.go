package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	modsapi "github.com/iw2rmb/ploy/internal/mods/api"
)

// End-to-end happy path for 3.1: submit, follow events, download artifacts.
func TestModRunFollowStreamsAndDownloadsArtifacts(t *testing.T) {
	t.Helper()

	runID := "mods-follow-test"
	artifactCID := "bafy-artifact-test"

	// Minimal control-plane emulator.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/mods":
			var req modsapi.RunSubmitRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			// Server returns 201 Created with canonical submit response.
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(struct {
				RunID  domaintypes.RunID `json:"run_id"`
				Status string            `json:"status"`
			}{RunID: domaintypes.RunID(runID), Status: "running"})

		case r.Method == http.MethodGet && r.URL.Path == fmt.Sprintf("/v1/runs/%s/events", runID):
			// SSE stream: run running -> run succeeded
			w.Header().Set("Content-Type", "text/event-stream")
			fl, ok := w.(http.Flusher)
			if !ok {
				t.Fatalf("no flusher")
			}
			// running
			_, _ = w.Write([]byte("event: run\n"))
			data, _ := json.Marshal(modsapi.RunSummary{
				RunID: domaintypes.RunID(runID),
				State: modsapi.RunStateRunning,
			})
			_, _ = w.Write([]byte("data: "))
			_, _ = w.Write(data)
			_, _ = w.Write([]byte("\n\n"))
			fl.Flush()
			time.Sleep(5 * time.Millisecond)
			// succeeded
			_, _ = w.Write([]byte("event: run\n"))
			data2, _ := json.Marshal(modsapi.RunSummary{
				RunID: domaintypes.RunID(runID),
				State: modsapi.RunStateSucceeded,
			})
			_, _ = w.Write([]byte("data: "))
			_, _ = w.Write(data2)
			_, _ = w.Write([]byte("\n\n"))
			fl.Flush()

		case r.Method == http.MethodGet && r.URL.Path == fmt.Sprintf("/v1/runs/%s/status", runID):
			// Return RunSummary directly — the canonical response shape.
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(modsapi.RunSummary{
				RunID: domaintypes.RunID(runID),
				State: modsapi.RunStateSucceeded,
				Stages: map[string]modsapi.StageStatus{
					"plan": {State: modsapi.StageStateSucceeded, Artifacts: map[string]string{"diff": artifactCID}},
				},
			})

		case r.Method == http.MethodGet && r.URL.Path == "/v1/artifacts":
			if q := r.URL.Query().Get("cid"); q != artifactCID {
				t.Fatalf("unexpected artifact lookup cid: %q", q)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"artifacts":[{"id":"artifact-1","cid":"` + artifactCID + `","digest":"sha256:deadbeef","name":"plan-diff.tar.gz","size":10}]}`))

		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/artifacts/artifact-1"):
			// Download bytes
			_, _ = w.Write([]byte("artifact-bytes"))

		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)

	dir := t.TempDir()
	buf := &bytes.Buffer{}
	args := []string{"--follow", "--artifact-dir", dir}
	if err := executeModRun(args, buf); err != nil {
		t.Fatalf("executeModRun error: %v", err)
	}

	// Output should at least acknowledge submission and success.
	out := buf.String()
	if !strings.Contains(out, "submitted") {
		t.Fatalf("expected submission message, got: %s", out)
	}
	if !strings.Contains(strings.ToLower(out), "succeeded") {
		t.Fatalf("expected success in output, got: %s", out)
	}

	// An artifact should be written and a manifest.json produced.
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var hasManifest, hasArtifact bool
	for _, f := range files {
		if f.Name() == "manifest.json" {
			hasManifest = true
		}
		if strings.Contains(f.Name(), "deadbeef") || strings.Contains(f.Name(), artifactCID) {
			hasArtifact = true
		}
	}
	if !hasManifest {
		t.Fatalf("manifest.json not found in %s; files=%v", dir, list(dir))
	}
	if !hasArtifact {
		t.Fatalf("artifact file not found in %s; files=%v", dir, list(dir))
	}
}

func list(dir string) []string {
	entries, _ := os.ReadDir(dir)
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, filepath.Join(dir, e.Name()))
	}
	return out
}

// TestModRunFollowStreamsUnifiedLogs verifies that mod run --follow renders
// enriched log events using the shared log printer alongside run/stage updates.
// This test covers the unified log streaming wired in via ROADMAP line 32.
func TestModRunFollowStreamsUnifiedLogs(t *testing.T) {
	runID := "mods-unified-logs-test"

	// Control-plane emulator that sends run, stage, and log events.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/mods":
			// Server returns 201 Created with canonical submit response.
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(struct {
				RunID  domaintypes.RunID `json:"run_id"`
				Status string            `json:"status"`
			}{RunID: domaintypes.RunID(runID), Status: "running"})

		case r.Method == http.MethodGet && r.URL.Path == fmt.Sprintf("/v1/runs/%s/events", runID):
			// SSE stream with run, stage, and log events.
			w.Header().Set("Content-Type", "text/event-stream")
			fl, ok := w.(http.Flusher)
			if !ok {
				t.Fatalf("no flusher")
			}

			// Run running event.
			_, _ = w.Write([]byte("event: run\n"))
			runData, _ := json.Marshal(modsapi.RunSummary{
				RunID: domaintypes.RunID(runID),
				State: modsapi.RunStateRunning,
			})
			_, _ = w.Write([]byte("data: "))
			_, _ = w.Write(runData)
			_, _ = w.Write([]byte("\n\n"))
			fl.Flush()

			// Enriched log event with node_id, mod_type, step_index, job_id.
			_, _ = w.Write([]byte("event: log\n"))
			logData := `{"timestamp":"2025-10-22T10:00:00Z","stream":"stdout","line":"Step started","node_id":"node-abc123","job_id":"job-def456","mod_type":"mod","step_index":2000}`
			_, _ = w.Write([]byte("data: " + logData + "\n\n"))
			fl.Flush()

			// Another log without enriched fields (backward compatibility).
			_, _ = w.Write([]byte("event: log\n"))
			logData2 := `{"timestamp":"2025-10-22T10:00:01Z","stream":"stderr","line":"Warning message"}`
			_, _ = w.Write([]byte("data: " + logData2 + "\n\n"))
			fl.Flush()

			time.Sleep(5 * time.Millisecond)

			// Run succeeded event.
			_, _ = w.Write([]byte("event: run\n"))
			runData2, _ := json.Marshal(modsapi.RunSummary{
				RunID: domaintypes.RunID(runID),
				State: modsapi.RunStateSucceeded,
			})
			_, _ = w.Write([]byte("data: "))
			_, _ = w.Write(runData2)
			_, _ = w.Write([]byte("\n\n"))
			fl.Flush()

		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)

	buf := &bytes.Buffer{}
	args := []string{"--follow"}
	if err := executeModRun(args, buf); err != nil {
		t.Fatalf("executeModRun error: %v", err)
	}

	out := buf.String()

	// Verify run state messages are present.
	if !strings.Contains(out, "submitted") {
		t.Errorf("expected submission message, got: %s", out)
	}
	if !strings.Contains(strings.ToLower(out), "succeeded") {
		t.Errorf("expected success in output, got: %s", out)
	}

	// Verify enriched log line is rendered with context fields (structured format).
	// Expected format: "2025-10-22T10:00:00Z stdout node=node-abc123 mod=mod step=2000 job=job-def456 Step started"
	if !strings.Contains(out, "node=node-abc123") {
		t.Errorf("expected enriched log with node_id, got: %s", out)
	}
	if !strings.Contains(out, "mod=mod") {
		t.Errorf("expected enriched log with mod_type, got: %s", out)
	}
	if !strings.Contains(out, "step=2000") {
		t.Errorf("expected enriched log with step_index, got: %s", out)
	}
	if !strings.Contains(out, "job=job-def456") {
		t.Errorf("expected enriched log with job_id, got: %s", out)
	}
	if !strings.Contains(out, "Step started") {
		t.Errorf("expected log message content, got: %s", out)
	}

	// Verify basic log without enriched fields is also rendered.
	if !strings.Contains(out, "Warning message") {
		t.Errorf("expected basic log message, got: %s", out)
	}
}

// TestModRunFollowRawLogFormat verifies that --log-format raw renders logs
// as message-only (no timestamps or context fields).
func TestModRunFollowRawLogFormat(t *testing.T) {
	runID := "mods-raw-format-test"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/mods":
			// Server returns 201 Created with canonical submit response.
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(struct {
				RunID  domaintypes.RunID `json:"run_id"`
				Status string            `json:"status"`
			}{RunID: domaintypes.RunID(runID), Status: "running"})

		case r.Method == http.MethodGet && r.URL.Path == fmt.Sprintf("/v1/runs/%s/events", runID):
			w.Header().Set("Content-Type", "text/event-stream")
			fl, ok := w.(http.Flusher)
			if !ok {
				t.Fatalf("no flusher")
			}

			// Run running.
			_, _ = w.Write([]byte("event: run\n"))
			runData, _ := json.Marshal(modsapi.RunSummary{
				RunID: domaintypes.RunID(runID),
				State: modsapi.RunStateRunning,
			})
			_, _ = w.Write([]byte("data: "))
			_, _ = w.Write(runData)
			_, _ = w.Write([]byte("\n\n"))
			fl.Flush()

			// Log event with enriched fields.
			_, _ = w.Write([]byte("event: log\n"))
			logData := `{"timestamp":"2025-10-22T10:00:00Z","stream":"stdout","line":"Raw log line","node_id":"node-xyz","job_id":"job-999","mod_type":"gate","step_index":100}`
			_, _ = w.Write([]byte("data: " + logData + "\n\n"))
			fl.Flush()

			// Run succeeded.
			_, _ = w.Write([]byte("event: run\n"))
			runData2, _ := json.Marshal(modsapi.RunSummary{
				RunID: domaintypes.RunID(runID),
				State: modsapi.RunStateSucceeded,
			})
			_, _ = w.Write([]byte("data: "))
			_, _ = w.Write(runData2)
			_, _ = w.Write([]byte("\n\n"))
			fl.Flush()

		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	useServerDescriptor(t, server.URL)

	buf := &bytes.Buffer{}
	args := []string{"--follow", "--log-format", "raw"}
	if err := executeModRun(args, buf); err != nil {
		t.Fatalf("executeModRun error: %v", err)
	}

	out := buf.String()

	// Verify raw log line is present (message only).
	if !strings.Contains(out, "Raw log line") {
		t.Errorf("expected raw log message, got: %s", out)
	}

	// In raw mode, enriched context fields should NOT appear in the log output.
	// Note: They may still appear in run/stage output, so check specifically
	// that the log line itself doesn't have the structured prefix.
	// The raw line "Raw log line" should appear without "node=" prefix on the same line.
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		if strings.Contains(line, "Raw log line") && strings.Contains(line, "node=") {
			t.Errorf("raw format should not include node= context, got line: %s", line)
		}
	}
}
