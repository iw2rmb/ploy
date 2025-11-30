package nodeagent

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestDiffUploader_UploadDiff(t *testing.T) {
	tests := []struct {
		name           string
		diffContent    string
		summary        types.DiffSummary
		wantStatusCode int
		wantErr        bool
	}{
		{
			name:        "successful upload",
			diffContent: "diff --git a/file.txt b/file.txt\n--- a/file.txt\n+++ b/file.txt\n@@ -1 +1 @@\n-old line\n+new line\n",
			summary: types.DiffSummary{
				"exit_code": 0,
				"timings": map[string]interface{}{
					"total_duration_ms": 1000,
				},
			},
			wantStatusCode: http.StatusCreated,
			wantErr:        false,
		},
		{
			name:        "empty diff",
			diffContent: "",
			summary: types.DiffSummary{
				"exit_code": 0,
			},
			wantStatusCode: http.StatusCreated,
			wantErr:        false,
		},
		{
			name:           "server error",
			diffContent:    "diff content",
			summary:        types.DiffSummary{},
			wantStatusCode: http.StatusInternalServerError,
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test server.
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request method and path.
				if r.Method != http.MethodPost {
					t.Errorf("expected POST request, got %s", r.Method)
				}

				// Verify URL path uses job-scoped endpoint.
				expectedPath := "/v1/runs/test-run-id/jobs/test-job-id/diff"
				if r.URL.Path != expectedPath {
					t.Errorf("expected path %s, got %s", expectedPath, r.URL.Path)
				}

				// Verify content type.
				if ct := r.Header.Get("Content-Type"); ct != "application/json" {
					t.Errorf("expected Content-Type application/json, got %s", ct)
				}

				// Decode and verify payload.
				var payload map[string]interface{}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Errorf("failed to decode payload: %v", err)
				}

				// Verify run_id is NOT in payload (it's in the URL path).
				if _, ok := payload["run_id"]; ok {
					t.Error("run_id should not be in payload (it's in URL)")
				}

				// Verify patch is present and gzipped.
				if patchData, ok := payload["patch"]; ok {
					// JSON unmarshals []byte as base64, but we need to verify it's gzipped.
					// For this test, we'll just check it's present.
					if patchData == nil {
						t.Error("patch is nil")
					}
				} else {
					t.Error("patch not present in payload")
				}

				// Verify summary is present.
				if _, ok := payload["summary"]; !ok {
					t.Error("summary not present in payload")
				}

				w.WriteHeader(tt.wantStatusCode)
				if tt.wantStatusCode == http.StatusCreated {
					_ = json.NewEncoder(w).Encode(map[string]string{"diff_id": "test-diff-id"})
				}
			}))
			defer server.Close()

			// Create uploader with test config.
			cfg := Config{
				ServerURL: server.URL,
				NodeID:    "test-node-id",
				HTTP: HTTPConfig{
					TLS: TLSConfig{
						Enabled: false,
					},
				},
			}

			uploader, err := NewDiffUploader(cfg)
			if err != nil {
				t.Fatalf("failed to create uploader: %v", err)
			}

			// Upload diff with job-scoped endpoint.
			ctx := context.Background()
			err = uploader.UploadDiff(ctx, "test-run-id", "test-job-id", []byte(tt.diffContent), tt.summary)

			if tt.wantErr && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestDiffUploader_SizeLimit(t *testing.T) {
	// Create incompressible data (~2 MiB of random bytes) so gzip stays > 1 MiB.
	// This should trigger the client-side size cap before any HTTP call.
	rnd := make([]byte, 2<<20)
	if _, err := io.ReadFull(bytes.NewReader(func() []byte {
		// Fill with pseudo-random looking bytes deterministically for test speed.
		// Avoid crypto/rand to keep tests fast and hermetic.
		b := make([]byte, len(rnd))
		var x uint64 = 0x9e3779b97f4a7c15
		for i := range b {
			// xorshift-style generator
			x ^= x << 13
			x ^= x >> 7
			x ^= x << 17
			b[i] = byte(x)
		}
		return b
	}()), rnd); err != nil {
		t.Fatalf("failed to make incompressible data: %v", err)
	}

	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	cfg := Config{
		ServerURL: server.URL,
		NodeID:    "test-node-id",
		HTTP: HTTPConfig{
			TLS: TLSConfig{Enabled: false},
		},
	}

	uploader, err := NewDiffUploader(cfg)
	if err != nil {
		t.Fatalf("failed to create uploader: %v", err)
	}

	ctx := context.Background()
	err = uploader.UploadDiff(ctx, "test-run-id", "test-job-id", rnd, types.DiffSummary{})
	if err == nil {
		t.Fatal("expected error for oversized diff but got none")
	}
	if !strings.Contains(err.Error(), "exceeds size cap") {
		t.Fatalf("unexpected error, want size cap: %v", err)
	}
	if called {
		t.Fatal("server should not have been called when size cap triggers")
	}
}

func TestDiffUploader_Compression(t *testing.T) {
	diffContent := "diff --git a/file.txt b/file.txt\n--- a/file.txt\n+++ b/file.txt\n@@ -1 +1 @@\n-old line\n+new line\n"

	var receivedGzipped []byte

	// Create a test server that captures the gzipped payload.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read the raw body first.
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read body: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Decode the JSON payload.
		var payload map[string]json.RawMessage
		if err := json.Unmarshal(bodyBytes, &payload); err != nil {
			t.Errorf("failed to decode payload: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Extract the patch field (it's base64-encoded in JSON).
		if patchRaw, ok := payload["patch"]; ok {
			// Decode the base64-encoded patch.
			var patchBytes []byte
			if err := json.Unmarshal(patchRaw, &patchBytes); err != nil {
				t.Errorf("failed to decode patch: %v", err)
			} else {
				receivedGzipped = patchBytes
			}
		}

		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"diff_id": "test-diff-id"})
	}))
	defer server.Close()

	cfg := Config{
		ServerURL: server.URL,
		NodeID:    "test-node-id",
		HTTP: HTTPConfig{
			TLS: TLSConfig{
				Enabled: false,
			},
		},
	}

	uploader, err := NewDiffUploader(cfg)
	if err != nil {
		t.Fatalf("failed to create uploader: %v", err)
	}

	ctx := context.Background()
	err = uploader.UploadDiff(ctx, "test-run-id", "test-job-id", []byte(diffContent), types.DiffSummary{})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Verify that the content was gzipped by decompressing it.
	if len(receivedGzipped) > 0 {
		gzReader, err := gzip.NewReader(bytes.NewReader(receivedGzipped))
		if err != nil {
			t.Errorf("failed to create gzip reader: %v", err)
			return
		}
		defer func() { _ = gzReader.Close() }()

		decompressed, err := io.ReadAll(gzReader)
		if err != nil {
			t.Errorf("failed to decompress: %v", err)
			return
		}

		if string(decompressed) != diffContent {
			t.Errorf("decompressed content mismatch:\ngot:  %s\nwant: %s", string(decompressed), diffContent)
		}
	} else {
		t.Error("no gzipped data received")
	}
}
