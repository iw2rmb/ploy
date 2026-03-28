package nodeagent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestUploadOutDirIfPresent_UploadsWhenFilesExist(t *testing.T) {
	// Prepare an outDir with a file
	outDir := t.TempDir()
	f := filepath.Join(outDir, "report.log")
	if err := os.WriteFile(f, []byte("hello"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	// Mock control-plane server to capture the upload
	var seen bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Expect artifact endpoint
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		// Job-scoped artifact endpoint: /v1/runs/{run_id}/jobs/{job_id}/artifact
		if r.URL.Path != "/v1/runs/run-1/jobs/stage-1/artifact" {
			t.Fatalf("path = %s, want /v1/runs/run-1/jobs/stage-1/artifact", r.URL.Path)
		}

		var payload map[string]any
		dec := json.NewDecoder(r.Body)
		if err := dec.Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		// run_id is now in URL path, not payload
		// Expect name to be set
		if payload["name"] != "mig-out" {
			t.Fatalf("name = %v, want mig-out", payload["name"])
		}
		// Expect bundle present (base64-encoded string after JSON)
		if b64, ok := payload["bundle"].(string); !ok || len(b64) == 0 {
			t.Fatalf("bundle missing or empty")
		}
		seen = true
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"artifact_bundle_id":"x"}`))
	}))
	defer server.Close()

	cfg := Config{ServerURL: server.URL, NodeID: testNodeID, HTTP: HTTPConfig{TLS: TLSConfig{Enabled: false}}}
	controller := newTestController(t, cfg)
	if err := controller.uploadOutDirBundle(context.Background(), "run-1", "stage-1", outDir, "mig-out"); err != nil {
		t.Fatalf("uploadOutDir error: %v", err)
	}
	if !seen {
		t.Fatalf("expected artifact upload to be sent")
	}
}

func TestUploadOutDirIfPresent_SkipsWhenEmpty(t *testing.T) {
	outDir := t.TempDir()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected request to %s", r.URL.Path)
	}))
	defer server.Close()

	cfg := Config{ServerURL: server.URL, NodeID: "n", HTTP: HTTPConfig{TLS: TLSConfig{Enabled: false}}}
	controller := newTestController(t, cfg)
	if err := controller.uploadOutDirBundle(context.Background(), "run-1", "stage-1", outDir, "mig-out"); err != nil {
		t.Fatalf("uploadOutDir error: %v", err)
	}
}
