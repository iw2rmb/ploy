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
		// NodeID and StageID are asserted below via path match
		if r.URL.Path != "/v1/nodes/test-node/stage/stage-1/artifact" {
			t.Fatalf("path = %s, want /v1/nodes/test-node/stage/stage-1/artifact", r.URL.Path)
		}

		var payload map[string]any
		dec := json.NewDecoder(r.Body)
		if err := dec.Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		// Expect run_id and name
		if payload["run_id"] != "run-1" {
			t.Fatalf("run_id = %v, want run-1", payload["run_id"])
		}
		if payload["name"] != "mod-out" {
			t.Fatalf("name = %v, want mod-out", payload["name"])
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

	cfg := Config{ServerURL: server.URL, NodeID: "test-node"}
	if err := uploadOutDirIfPresent(context.Background(), cfg, "run-1", "stage-1", outDir); err != nil {
		t.Fatalf("uploadOutDirIfPresent error: %v", err)
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

	cfg := Config{ServerURL: server.URL, NodeID: "n"}
	if err := uploadOutDirIfPresent(context.Background(), cfg, "run-1", "stage-1", outDir); err != nil {
		t.Fatalf("uploadOutDirIfPresent error: %v", err)
	}
}
