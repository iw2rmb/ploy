package mods

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestRemoteStartReturnsExecutionID verifies that the helper returns the execution id from the controller
func TestRemoteStartReturnsExecutionID(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/mods", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, `{"execution_id":"tf-test1234"}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := &http.Client{Timeout: 2 * time.Second}
	id, err := remoteStart(srv.URL+"/v1", []byte("id: test\n"), false, client)
	if err != nil {
		t.Fatalf("remoteStart error: %v", err)
	}
	if id != "tf-test1234" {
		t.Fatalf("unexpected id: %s", id)
	}
}

// TestExecuteRemoteTransflowPrintsExecutionID ensures the CLI prints the id and completes when status becomes completed
func TestExecuteRemoteModsPrintsExecutionID(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/mods", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, `{"execution_id":"tf-abc123"}`)
	})
	mux.HandleFunc("/v1/mods/tf-abc123/status", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Minimal successful status payload
		resp := map[string]any{
			"id":     "tf-abc123",
			"status": "completed",
			"result": map[string]any{"artifacts": map[string]any{}},
		}
		_ = json.NewEncoder(w).Encode(resp)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Create a temporary config file
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "tf.yaml")
	if err := os.WriteFile(cfgPath, []byte("id: test\n"), 0644); err != nil {
		t.Fatalf("write cfg: %v", err)
	}

	// Capture stdout
	var buf bytes.Buffer
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, r)
		close(done)
	}()

	// Execute
	err := executeRemoteMod(srv.URL+"/v1", cfgPath, false, true, false, "text")

	// Restore stdout
	_ = w.Close()
	os.Stdout = old
	<-done

	if err != nil {
		t.Fatalf("executeRemoteMod error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Execution ID: tf-abc123") {
		t.Fatalf("expected Execution ID in output, got: %s", out)
	}
	if !strings.Contains(out, "ploy mod watch -id tf-abc123") {
		t.Fatalf("expected watch hint in output, got: %s", out)
	}
}

// TestExecuteRemoteTransflowWatch attaches SSE watch and returns quickly
func TestExecuteRemoteModsWatch(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/mods", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, `{"execution_id":"tf-watch1"}`)
	})
	// SSE endpoint returns immediate end
	mux.HandleFunc("/v1/mods/tf-watch1/logs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "event: init\n")
		_, _ = io.WriteString(w, "data: {\"id\":\"tf-watch1\",\"message\":\"SSE connected\"}\n\n")
		_, _ = io.WriteString(w, "event: end\n")
		_, _ = io.WriteString(w, "data: {\"status\":\"completed\"}\n\n")
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "tf.yaml")
	if err := os.WriteFile(cfgPath, []byte("id: test\n"), 0644); err != nil {
		t.Fatalf("write cfg: %v", err)
	}

	if err := executeRemoteMod(srv.URL+"/v1", cfgPath, false, false, true, "text"); err != nil {
		t.Fatalf("watch mode failed: %v", err)
	}
}

// TestExecuteRemoteTransflowWatchAcceptsCharset verifies SSE watch accepts Content-Type with charset
func TestExecuteRemoteModsWatchAcceptsCharset(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/mods", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, `{"execution_id":"tf-watch2"}`)
	})
	// SSE endpoint returns with charset on content type
	mux.HandleFunc("/v1/mods/tf-watch2/logs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "event: init\n")
		_, _ = io.WriteString(w, "data: {\"id\":\"tf-watch2\",\"message\":\"SSE connected\"}\n\n")
		_, _ = io.WriteString(w, "event: end\n")
		_, _ = io.WriteString(w, "data: {\"status\":\"completed\"}\n\n")
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "tf.yaml")
	if err := os.WriteFile(cfgPath, []byte("id: test\n"), 0644); err != nil {
		t.Fatalf("write cfg: %v", err)
	}

	if err := executeRemoteMod(srv.URL+"/v1", cfgPath, false, false, true, "text"); err != nil {
		t.Fatalf("watch mode failed with charset content-type: %v", err)
	}
}

// TestExecuteRemoteTransflowJSONOutputsExecutionID ensures --output=json prints a single JSON with execution_id
func TestExecuteRemoteModsJSONOutputsExecutionID(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/mods", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, `{"execution_id":"tf-json1"}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "tf.yaml")
	if err := os.WriteFile(cfgPath, []byte("id: test\n"), 0644); err != nil {
		t.Fatalf("write cfg: %v", err)
	}

	// Capture stdout
	var buf bytes.Buffer
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	done := make(chan struct{})
	go func() { _, _ = io.Copy(&buf, r); close(done) }()

	err := executeRemoteMod(srv.URL+"/v1", cfgPath, false, true, false, "json")

	_ = w.Close()
	os.Stdout = old
	<-done

	if err != nil {
		t.Fatalf("executeRemoteMod json error: %v", err)
	}
	out := strings.TrimSpace(buf.String())
	if !strings.Contains(out, `"execution_id":"tf-json1"`) {
		t.Fatalf("expected json with execution_id, got: %s", out)
	}
}
