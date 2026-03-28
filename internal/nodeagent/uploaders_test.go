package nodeagent

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

// --- DiffUploader tests ---

func TestDiffUploader_UploadDiff(t *testing.T) {
	// Helper to create test summaries using the builder pattern.
	// DiffSummary is now json.RawMessage-backed, so we use the builder.
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
			summary: types.NewDiffSummaryBuilder().
				ExitCode(0).
				Timings(0, 0, 0, 1000).
				MustBuild(),
			wantStatusCode: http.StatusCreated,
			wantErr:        false,
		},
		{
			name:        "empty diff",
			diffContent: "",
			summary: types.NewDiffSummaryBuilder().
				ExitCode(0).
				MustBuild(),
			wantStatusCode: http.StatusCreated,
			wantErr:        false,
		},
		{
			name:           "server error",
			diffContent:    "diff content",
			summary:        types.NewDiffSummaryBuilder().MustBuild(),
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

			uploader := newTestUploader(t, server.URL)

			// Upload diff with job-scoped endpoint.
			ctx := context.Background()
			err := uploader.UploadDiff(ctx, "test-run-id", "test-job-id", []byte(tt.diffContent), tt.summary)

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
	// Create incompressible data (> MaxUploadSize) so gzip stays > MaxUploadSize.
	rnd := make([]byte, MaxUploadSize+1)
	rand.New(rand.NewSource(1)).Read(rnd)

	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	uploader := newTestUploader(t, server.URL)

	ctx := context.Background()
	err := uploader.UploadDiff(ctx, "test-run-id", "test-job-id", rnd, types.NewDiffSummaryBuilder().MustBuild())
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

	uploader := newTestUploader(t, server.URL)

	ctx := context.Background()
	err := uploader.UploadDiff(ctx, "test-run-id", "test-job-id", []byte(diffContent), types.NewDiffSummaryBuilder().MustBuild())
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

// TestBearerToken_TrimsWhitespace verifies that the bearer token read from
// file is trimmed of leading/trailing whitespace before being used in
// the Authorization header. Token files commonly have trailing newlines
// (e.g., from text editors or "echo tok > file") which would corrupt headers.
func TestBearerToken_TrimsWhitespace(t *testing.T) {
	tests := []struct {
		name          string
		tokenContent  string // Raw file contents (may include whitespace/newlines)
		expectedToken string // Expected token after trimming
	}{
		{
			name:          "trailing newline",
			tokenContent:  "tok\n",
			expectedToken: "tok",
		},
		{
			name:          "trailing CRLF",
			tokenContent:  "tok\r\n",
			expectedToken: "tok",
		},
		{
			name:          "leading and trailing whitespace",
			tokenContent:  "  tok  \n",
			expectedToken: "tok",
		},
		{
			name:          "multiple trailing newlines",
			tokenContent:  "tok\n\n\n",
			expectedToken: "tok",
		},
		{
			name:          "clean token (no whitespace)",
			tokenContent:  "tok",
			expectedToken: "tok",
		},
		{
			name:          "token with internal spaces preserved",
			tokenContent:  "tok with spaces\n",
			expectedToken: "tok with spaces",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temp directory and token file with the test content.
			tmpDir := t.TempDir()
			tokenPath := filepath.Join(tmpDir, "bearer-token")
			if err := os.WriteFile(tokenPath, []byte(tt.tokenContent), 0600); err != nil {
				t.Fatalf("failed to write token file: %v", err)
			}

			// Override the bearer token path for this test.
			t.Setenv("PLOY_NODE_BEARER_TOKEN_PATH", tokenPath)

			// Capture the Authorization header sent by the HTTP client.
			var capturedAuthHeader string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedAuthHeader = r.Header.Get("Authorization")
				w.WriteHeader(http.StatusCreated)
			}))
			defer server.Close()

			uploader := newTestUploader(t, server.URL)

			// Make a request to trigger the Authorization header.
			ctx := context.Background()
			_ = uploader.UploadDiff(ctx, "test-run-id", "test-job-id", []byte("diff"), types.NewDiffSummaryBuilder().MustBuild())

			// Verify the Authorization header is correctly trimmed.
			expectedHeader := "Bearer " + tt.expectedToken
			if capturedAuthHeader != expectedHeader {
				t.Errorf("Authorization header mismatch:\ngot:  %q\nwant: %q", capturedAuthHeader, expectedHeader)
			}
		})
	}
}

// --- ArtifactUploader tests ---

func TestArtifactUploader_UploadArtifact(t *testing.T) {
	tests := []struct {
		name       string
		setupFiles func(t *testing.T) []string // returns file paths to upload
		serverCode int                          // 0 means no server needed
		wantErr    bool
		verify     func(t *testing.T, payload map[string]interface{})
	}{
		{
			name: "success",
			setupFiles: func(t *testing.T) []string {
				t.Helper()
				tmpDir := t.TempDir()
				file1 := filepath.Join(tmpDir, "test1.txt")
				file2 := filepath.Join(tmpDir, "test2.txt")
				if err := os.WriteFile(file1, []byte("content1"), 0600); err != nil {
					t.Fatalf("create test file: %v", err)
				}
				if err := os.WriteFile(file2, []byte("content2"), 0600); err != nil {
					t.Fatalf("create test file: %v", err)
				}
				return []string{file1, file2}
			},
			serverCode: http.StatusCreated,
			wantErr:    false,
			verify: func(t *testing.T, payload map[string]interface{}) {
				t.Helper()
				if _, exists := payload["run_id"]; exists {
					t.Error("run_id should not be in payload (it's in URL)")
				}
				if payload["name"] != "test-bundle" {
					t.Errorf("expected name 'test-bundle', got %v", payload["name"])
				}
				if _, exists := payload["bundle"]; !exists {
					t.Error("expected bundle field in payload")
				}
			},
		},
		{
			name: "empty paths",
			setupFiles: func(t *testing.T) []string {
				return []string{}
			},
			wantErr: false,
		},
		{
			name: "server error",
			setupFiles: func(t *testing.T) []string {
				t.Helper()
				tmpDir := t.TempDir()
				file1 := filepath.Join(tmpDir, "test.txt")
				if err := os.WriteFile(file1, []byte("content"), 0600); err != nil {
					t.Fatalf("create test file: %v", err)
				}
				return []string{file1}
			},
			serverCode: http.StatusInternalServerError,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths := tt.setupFiles(t)

			var serverURL string
			var receivedPayload map[string]interface{}

			if tt.serverCode != 0 {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method != http.MethodPost {
						t.Errorf("expected POST, got %s", r.Method)
					}
					expectedPath := "/v1/runs/test-run-id/jobs/test-job-id/artifact"
					if r.URL.Path != expectedPath {
						t.Errorf("expected path %s, got %s", expectedPath, r.URL.Path)
					}
					body, err := io.ReadAll(r.Body)
					if err != nil {
						t.Fatalf("read request body: %v", err)
					}
					if err := json.Unmarshal(body, &receivedPayload); err != nil {
						t.Fatalf("unmarshal request: %v", err)
					}
					w.WriteHeader(tt.serverCode)
					if tt.serverCode == http.StatusCreated {
						_, _ = w.Write([]byte(`{"artifact_bundle_id":"test-id"}`))
					}
				}))
				defer server.Close()
				serverURL = server.URL
			} else {
				// No server needed — use dummy URL.
				serverURL = "http://localhost:8443"
			}

			uploader := newTestUploader(t, serverURL)

			ctx := context.Background()
			_, _, err := uploader.UploadArtifact(ctx, "test-run-id", "test-job-id", paths, "test-bundle")

			if tt.wantErr && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tt.verify != nil {
				tt.verify(t, receivedPayload)
			}
		})
	}
}

// --- CreateTarGzBundle tests ---

func TestCreateTarGzBundle_Files(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string // filename -> content
	}{
		{
			name:  "single file",
			files: map[string]string{"test.txt": "hello world"},
		},
		{
			name:  "multiple files",
			files: map[string]string{"file1.txt": "content1", "file2.txt": "content2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			var paths []string
			for name, content := range tt.files {
				p := filepath.Join(tmpDir, name)
				if err := os.WriteFile(p, []byte(content), 0600); err != nil {
					t.Fatalf("create test file: %v", err)
				}
				paths = append(paths, p)
			}

			bundleBytes, err := createTarGzBundle(paths)
			if err != nil {
				t.Fatalf("create bundle: %v", err)
			}
			if len(bundleBytes) == 0 {
				t.Fatal("expected non-empty bundle")
			}

			entries := tarEntriesFromBundle(t, bundleBytes)

			for name, content := range tt.files {
				entry, ok := entries[name]
				if !ok {
					t.Errorf("expected %s in archive, got keys: %v", name, mapKeys(entries))
					continue
				}
				if entry.Typeflag != tar.TypeReg {
					t.Errorf("%s: expected TypeReg, got %d", name, entry.Typeflag)
				}
				if !bytes.Equal(entry.Content, []byte(content)) {
					t.Errorf("%s: content mismatch: got %q, want %q", name, entry.Content, content)
				}
			}
		})
	}
}

func TestCreateTarGzBundle_DirectoryRecursive(t *testing.T) {
	tmpDir := t.TempDir()
	// Create directory tree: rootDir/{a.txt, sub/b.txt}
	rootDir := filepath.Join(tmpDir, "root")
	if err := os.MkdirAll(filepath.Join(rootDir, "sub"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	aContent := []byte("A")
	bContent := []byte("B")
	if err := os.WriteFile(filepath.Join(rootDir, "a.txt"), aContent, 0o600); err != nil {
		t.Fatalf("write a: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rootDir, "sub", "b.txt"), bContent, 0o600); err != nil {
		t.Fatalf("write b: %v", err)
	}

	bundleBytes, err := createTarGzBundle([]string{rootDir})
	if err != nil {
		t.Fatalf("create bundle: %v", err)
	}

	entries := tarEntriesFromBundle(t, bundleBytes)

	wantA := filepath.Join("root", "a.txt")
	wantB := filepath.Join("root", "sub", "b.txt")
	if !bytes.Equal(entries[wantA].Content, aContent) {
		t.Fatalf("content %s mismatch", wantA)
	}
	if !bytes.Equal(entries[wantB].Content, bContent) {
		t.Fatalf("content %s mismatch", wantB)
	}
}

func TestCreateTarGzBundleFromEntries_CustomArchiveRoot(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(filepath.Join(srcDir, "nested"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "nested", "candidate.json"), []byte(`{"schema_version":1}`), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	bundleBytes, err := createTarGzBundleFromEntries([]ArtifactBundleEntry{{
		SourcePath:  srcDir,
		ArchivePath: "out",
	}})
	if err != nil {
		t.Fatalf("create bundle: %v", err)
	}

	entries := tarEntriesFromBundle(t, bundleBytes)

	if _, ok := entries["out/nested/candidate.json"]; !ok {
		t.Fatalf("expected out/nested/candidate.json in archive, got %v", mapKeys(entries))
	}
}

func TestCreateTarGzBundle_NonExistentFile(t *testing.T) {
	// Try to bundle a non-existent file.
	_, err := createTarGzBundle([]string{"/nonexistent/file.txt"})
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

// TestCreateTarGzBundle_SymlinkPreserved verifies that symlinks in a directory
// are archived as symlinks (TypeSymlink header), not as regular files with
// followed content. This is a security-critical behavior to prevent symlink-based
// exfiltration of files outside the workspace.
func TestCreateTarGzBundle_SymlinkPreserved(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a directory structure:
	//   workspace/
	//     regular.txt        -> regular file with known content
	//     link_to_external   -> symlink pointing to /etc/hosts (external path)
	//     link_to_internal   -> symlink pointing to regular.txt (internal path)
	workspaceDir := filepath.Join(tmpDir, "workspace")
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}

	// Create a regular file inside workspace.
	regularFile := filepath.Join(workspaceDir, "regular.txt")
	regularContent := []byte("regular file content")
	if err := os.WriteFile(regularFile, regularContent, 0o600); err != nil {
		t.Fatalf("write regular file: %v", err)
	}

	// Create a symlink pointing to an external file (e.g., /etc/hosts).
	// This tests that we don't read/exfiltrate external file contents.
	externalLink := filepath.Join(workspaceDir, "link_to_external")
	if err := os.Symlink("/etc/hosts", externalLink); err != nil {
		t.Fatalf("create external symlink: %v", err)
	}

	// Create a symlink pointing to the internal regular file.
	internalLink := filepath.Join(workspaceDir, "link_to_internal")
	if err := os.Symlink("regular.txt", internalLink); err != nil {
		t.Fatalf("create internal symlink: %v", err)
	}

	// Bundle the workspace directory.
	bundleBytes, err := createTarGzBundle([]string{workspaceDir})
	if err != nil {
		t.Fatalf("create bundle: %v", err)
	}

	entries := tarEntriesFromBundle(t, bundleBytes)

	// Verify the regular file is archived correctly.
	regularEntry, ok := entries[filepath.Join("workspace", "regular.txt")]
	if !ok {
		t.Fatal("regular.txt not found in archive")
	}
	if regularEntry.Typeflag != tar.TypeReg {
		t.Errorf("regular.txt: expected TypeReg (%d), got %d", tar.TypeReg, regularEntry.Typeflag)
	}
	if !bytes.Equal(regularEntry.Content, regularContent) {
		t.Errorf("regular.txt content mismatch")
	}

	// External symlinks pointing outside the workspace should be SKIPPED for security.
	// This prevents data exfiltration via symlinks to sensitive files like /etc/hosts.
	_, ok = entries[filepath.Join("workspace", "link_to_external")]
	if ok {
		t.Error("link_to_external should NOT be in archive - external symlinks should be skipped for security")
	}

	// Verify the internal symlink is also archived as a symlink.
	internalEntry, ok := entries[filepath.Join("workspace", "link_to_internal")]
	if !ok {
		t.Fatal("link_to_internal not found in archive")
	}
	if internalEntry.Typeflag != tar.TypeSymlink {
		t.Errorf("link_to_internal: expected TypeSymlink (%d), got %d", tar.TypeSymlink, internalEntry.Typeflag)
	}
	if internalEntry.Linkname != "regular.txt" {
		t.Errorf("link_to_internal: expected linkname 'regular.txt', got %q", internalEntry.Linkname)
	}
}

func TestArtifactUploader_SizeCap(t *testing.T) {
	// Create a large file that will exceed the 10 MiB cap when bundled.
	tmpDir := t.TempDir()
	largeFile := filepath.Join(tmpDir, "large.txt")
	// Use deterministic pseudo-random bytes so gzip doesn't meaningfully compress.
	largeContent := make([]byte, MaxUploadSize+1)
	rand.New(rand.NewSource(1)).Read(largeContent)
	if err := os.WriteFile(largeFile, largeContent, 0600); err != nil {
		t.Fatalf("create large file: %v", err)
	}

	uploader := newTestUploader(t, "http://localhost:8443")

	ctx := context.Background()
	_, _, err := uploader.UploadArtifact(ctx, "test-run-id", "test-job-id", []string{largeFile}, "")
	if err == nil {
		t.Error("expected error for oversized bundle")
	}
}

// mapKeys returns the keys of a tarEntry map for diagnostic output.
func mapKeys(m map[string]tarEntry) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
