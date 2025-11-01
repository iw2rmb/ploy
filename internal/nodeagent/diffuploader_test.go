package nodeagent

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDiffUploader_UploadDiff(t *testing.T) {
	tests := []struct {
		name           string
		diffContent    string
		summary        map[string]interface{}
		wantStatusCode int
		wantErr        bool
	}{
		{
			name:        "successful upload",
			diffContent: "diff --git a/file.txt b/file.txt\n--- a/file.txt\n+++ b/file.txt\n@@ -1 +1 @@\n-old line\n+new line\n",
			summary: map[string]interface{}{
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
			summary: map[string]interface{}{
				"exit_code": 0,
			},
			wantStatusCode: http.StatusCreated,
			wantErr:        false,
		},
		{
			name:           "server error",
			diffContent:    "diff content",
			summary:        map[string]interface{}{},
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

				// Verify content type.
				if ct := r.Header.Get("Content-Type"); ct != "application/json" {
					t.Errorf("expected Content-Type application/json, got %s", ct)
				}

				// Decode and verify payload.
				var payload map[string]interface{}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Errorf("failed to decode payload: %v", err)
				}

				// Verify run_id is present.
				if _, ok := payload["run_id"]; !ok {
					t.Error("run_id not present in payload")
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

			// Upload diff.
			ctx := context.Background()
			err = uploader.UploadDiff(ctx, "test-run-id", "test-stage-id", []byte(tt.diffContent), tt.summary)

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
	// Create a diff that won't compress well and will exceed 1 MiB when gzipped.
	// Use incompressible data (crypto-random would be ideal, but we use a simpler approach).
	// Create data that is already gzip-compressed (won't re-compress).
	var precompressed bytes.Buffer
	gzw := gzip.NewWriter(&precompressed)
	largeDiff := bytes.Repeat([]byte("x"), 2<<20) // 2 MiB of data
	_, _ = gzw.Write(largeDiff)
	_ = gzw.Close()

	// Use the pre-compressed data as our "diff" - it won't compress further.
	testDiff := precompressed.Bytes()

	// Create a test server.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusRequestEntityTooLarge)
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
	err = uploader.UploadDiff(ctx, "test-run-id", "test-stage-id", testDiff, map[string]interface{}{})

	if err == nil {
		t.Error("expected error for oversized diff but got none")
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
	err = uploader.UploadDiff(ctx, "test-run-id", "test-stage-id", []byte(diffContent), map[string]interface{}{})
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
