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

			// Create uploader with test config.
			cfg := Config{
				ServerURL: server.URL,
				NodeID:    testNodeID,
				HTTP: HTTPConfig{
					TLS: TLSConfig{
						Enabled: false,
					},
				},
			}

			uploader, err := newBaseUploader(cfg)
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
	// Create incompressible data (> MaxUploadSize) so gzip stays > MaxUploadSize.
	// This should trigger the client-side size cap before any HTTP call.
	rnd := make([]byte, MaxUploadSize+1)
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
		NodeID:    testNodeID,
		HTTP: HTTPConfig{
			TLS: TLSConfig{Enabled: false},
		},
	}

	uploader, err := newBaseUploader(cfg)
	if err != nil {
		t.Fatalf("failed to create uploader: %v", err)
	}

	ctx := context.Background()
	err = uploader.UploadDiff(ctx, "test-run-id", "test-job-id", rnd, types.NewDiffSummaryBuilder().MustBuild())
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
		NodeID:    testNodeID,
		HTTP: HTTPConfig{
			TLS: TLSConfig{
				Enabled: false,
			},
		},
	}

	uploader, err := newBaseUploader(cfg)
	if err != nil {
		t.Fatalf("failed to create uploader: %v", err)
	}

	ctx := context.Background()
	err = uploader.UploadDiff(ctx, "test-run-id", "test-job-id", []byte(diffContent), types.NewDiffSummaryBuilder().MustBuild())
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

			cfg := Config{
				ServerURL: server.URL,
				NodeID:    testNodeID,
				HTTP: HTTPConfig{
					TLS: TLSConfig{Enabled: false},
				},
			}

			uploader, err := newBaseUploader(cfg)
			if err != nil {
				t.Fatalf("failed to create uploader: %v", err)
			}

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

func TestArtifactUploader_UploadArtifact_Success(t *testing.T) {
	// Create temporary test files.
	tmpDir := t.TempDir()
	file1 := filepath.Join(tmpDir, "test1.txt")
	if err := os.WriteFile(file1, []byte("content1"), 0600); err != nil {
		t.Fatalf("create test file: %v", err)
	}
	file2 := filepath.Join(tmpDir, "test2.txt")
	if err := os.WriteFile(file2, []byte("content2"), 0600); err != nil {
		t.Fatalf("create test file: %v", err)
	}

	// Setup mock server.
	var receivedPayload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		// Verify URL path uses job-scoped endpoint.
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

		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"artifact_bundle_id":"test-id"}`))
	}))
	defer server.Close()

	// Create uploader.
	cfg := Config{
		ServerURL: server.URL,
		NodeID:    testNodeID,
	}
	uploader, err := newBaseUploader(cfg)
	if err != nil {
		t.Fatalf("create uploader: %v", err)
	}

	// Upload artifact.
	ctx := context.Background()
	_, _, err = uploader.UploadArtifact(ctx, "test-run-id", "test-job-id", []string{file1, file2}, "test-bundle")
	if err != nil {
		t.Fatalf("upload artifact: %v", err)
	}

	// Verify run_id is NOT in payload (it's in the URL path).
	if _, exists := receivedPayload["run_id"]; exists {
		t.Error("run_id should not be in payload (it's in URL)")
	}
	if receivedPayload["name"] != "test-bundle" {
		t.Errorf("expected name 'test-bundle', got %v", receivedPayload["name"])
	}

	// Verify bundle exists and is non-empty.
	// The bundle field should be present in the payload.
	if _, exists := receivedPayload["bundle"]; !exists {
		t.Error("expected bundle field in payload")
	}
}

func TestArtifactUploader_UploadArtifact_EmptyPaths(t *testing.T) {
	cfg := Config{
		ServerURL: "http://localhost:8443",
		NodeID:    testNodeID,
	}
	uploader, err := newBaseUploader(cfg)
	if err != nil {
		t.Fatalf("create uploader: %v", err)
	}

	ctx := context.Background()
	_, _, err = uploader.UploadArtifact(ctx, "test-run-id", "test-job-id", []string{}, "test-bundle")
	if err != nil {
		t.Errorf("expected no error for empty paths, got %v", err)
	}
}

func TestArtifactUploader_UploadArtifact_ServerError(t *testing.T) {
	// Setup mock server that returns error.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	}))
	defer server.Close()

	// Create temporary test file.
	tmpDir := t.TempDir()
	file1 := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(file1, []byte("content"), 0600); err != nil {
		t.Fatalf("create test file: %v", err)
	}

	cfg := Config{
		ServerURL: server.URL,
		NodeID:    testNodeID,
	}
	uploader, err := newBaseUploader(cfg)
	if err != nil {
		t.Fatalf("create uploader: %v", err)
	}

	ctx := context.Background()
	_, _, err = uploader.UploadArtifact(ctx, "test-run-id", "test-job-id", []string{file1}, "")
	if err == nil {
		t.Error("expected error when server returns 500")
	}
}

func TestCreateTarGzBundle_SingleFile(t *testing.T) {
	// Create temporary test file.
	tmpDir := t.TempDir()
	file1 := filepath.Join(tmpDir, "test.txt")
	content := []byte("hello world")
	if err := os.WriteFile(file1, content, 0600); err != nil {
		t.Fatalf("create test file: %v", err)
	}

	// Create bundle.
	bundleBytes, err := createTarGzBundle([]string{file1})
	if err != nil {
		t.Fatalf("create bundle: %v", err)
	}

	if len(bundleBytes) == 0 {
		t.Error("expected non-empty bundle")
	}

	// Verify we can decompress and read the tar.
	gzReader, err := gzip.NewReader(bytes.NewReader(bundleBytes))
	if err != nil {
		t.Fatalf("create gzip reader: %v", err)
	}
	defer func() {
		_ = gzReader.Close()
	}()

	tarReader := tar.NewReader(gzReader)
	header, err := tarReader.Next()
	if err != nil {
		t.Fatalf("read tar header: %v", err)
	}

	if header.Name != "test.txt" {
		t.Errorf("expected name 'test.txt', got %s", header.Name)
	}

	readContent, err := io.ReadAll(tarReader)
	if err != nil {
		t.Fatalf("read tar content: %v", err)
	}

	if !bytes.Equal(readContent, content) {
		t.Errorf("content mismatch: expected %q, got %q", content, readContent)
	}
}

func TestCreateTarGzBundle_MultipleFiles(t *testing.T) {
	// Create temporary test files.
	tmpDir := t.TempDir()
	file1 := filepath.Join(tmpDir, "file1.txt")
	file2 := filepath.Join(tmpDir, "file2.txt")
	if err := os.WriteFile(file1, []byte("content1"), 0600); err != nil {
		t.Fatalf("create test file: %v", err)
	}
	if err := os.WriteFile(file2, []byte("content2"), 0600); err != nil {
		t.Fatalf("create test file: %v", err)
	}

	// Create bundle.
	bundleBytes, err := createTarGzBundle([]string{file1, file2})
	if err != nil {
		t.Fatalf("create bundle: %v", err)
	}

	// Verify we can decompress and read the tar.
	gzReader, err := gzip.NewReader(bytes.NewReader(bundleBytes))
	if err != nil {
		t.Fatalf("create gzip reader: %v", err)
	}
	defer func() {
		_ = gzReader.Close()
	}()

	tarReader := tar.NewReader(gzReader)

	// Read first file.
	header1, err := tarReader.Next()
	if err != nil {
		t.Fatalf("read first tar header: %v", err)
	}
	if header1.Name != "file1.txt" {
		t.Errorf("expected name 'file1.txt', got %s", header1.Name)
	}

	// Read second file.
	header2, err := tarReader.Next()
	if err != nil {
		t.Fatalf("read second tar header: %v", err)
	}
	if header2.Name != "file2.txt" {
		t.Errorf("expected name 'file2.txt', got %s", header2.Name)
	}
}

func TestCreateTarGzBundle_DirectoryRecursive(t *testing.T) {
	tmpDir := t.TempDir()
	// Create directory tree: rootDir/{a.txt, sub/b.txt}
	rootDir := filepath.Join(tmpDir, "root")
	if err := os.MkdirAll(filepath.Join(rootDir, "sub"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	aPath := filepath.Join(rootDir, "a.txt")
	bPath := filepath.Join(rootDir, "sub", "b.txt")
	aContent := []byte("A")
	bContent := []byte("B")
	if err := os.WriteFile(aPath, aContent, 0o600); err != nil {
		t.Fatalf("write a: %v", err)
	}
	if err := os.WriteFile(bPath, bContent, 0o600); err != nil {
		t.Fatalf("write b: %v", err)
	}

	bundleBytes, err := createTarGzBundle([]string{rootDir})
	if err != nil {
		t.Fatalf("create bundle: %v", err)
	}

	gzReader, err := gzip.NewReader(bytes.NewReader(bundleBytes))
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer func() {
		_ = gzReader.Close()
	}()
	tr := tar.NewReader(gzReader)

	got := map[string][]byte{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar read: %v", err)
		}
		// Skip dir headers when reading content
		if hdr.FileInfo().IsDir() {
			continue
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatalf("read file: %v", err)
		}
		got[hdr.Name] = data
	}

	// Expect names include the base directory name
	wantA := filepath.Join("root", "a.txt")
	wantB := filepath.Join("root", "sub", "b.txt")
	if !bytes.Equal(got[wantA], aContent) {
		t.Fatalf("content %s mismatch", wantA)
	}
	if !bytes.Equal(got[wantB], bContent) {
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

	gzReader, err := gzip.NewReader(bytes.NewReader(bundleBytes))
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer func() { _ = gzReader.Close() }()
	tr := tar.NewReader(gzReader)

	names := map[string]struct{}{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar read: %v", err)
		}
		names[hdr.Name] = struct{}{}
	}

	if _, ok := names["out/nested/candidate.json"]; !ok {
		t.Fatalf("expected out/nested/candidate.json in archive, got %v", names)
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

	// Decompress and inspect the tar archive.
	gzReader, err := gzip.NewReader(bytes.NewReader(bundleBytes))
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer func() { _ = gzReader.Close() }()

	tr := tar.NewReader(gzReader)

	// Track what we find in the archive.
	type entryInfo struct {
		typeflag byte
		linkname string
		content  []byte
	}
	entries := make(map[string]entryInfo)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar read: %v", err)
		}

		var content []byte
		if hdr.Typeflag == tar.TypeReg {
			content, err = io.ReadAll(tr)
			if err != nil {
				t.Fatalf("read content: %v", err)
			}
		}

		entries[hdr.Name] = entryInfo{
			typeflag: hdr.Typeflag,
			linkname: hdr.Linkname,
			content:  content,
		}
	}

	// Verify the regular file is archived correctly.
	regularEntry, ok := entries[filepath.Join("workspace", "regular.txt")]
	if !ok {
		t.Fatal("regular.txt not found in archive")
	}
	if regularEntry.typeflag != tar.TypeReg {
		t.Errorf("regular.txt: expected TypeReg (%d), got %d", tar.TypeReg, regularEntry.typeflag)
	}
	if !bytes.Equal(regularEntry.content, regularContent) {
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
	if internalEntry.typeflag != tar.TypeSymlink {
		t.Errorf("link_to_internal: expected TypeSymlink (%d), got %d", tar.TypeSymlink, internalEntry.typeflag)
	}
	if internalEntry.linkname != "regular.txt" {
		t.Errorf("link_to_internal: expected linkname 'regular.txt', got %q", internalEntry.linkname)
	}
}

func TestArtifactUploader_SizeCap(t *testing.T) {
	// Create a large file that will exceed the 10 MiB cap when bundled.
	tmpDir := t.TempDir()
	largeFile := filepath.Join(tmpDir, "large.txt")
	// Use deterministic pseudo-random bytes so gzip doesn't meaningfully compress.
	largeContent := make([]byte, MaxUploadSize+1)
	if _, err := rand.New(rand.NewSource(1)).Read(largeContent); err != nil {
		t.Fatalf("generate large file content: %v", err)
	}
	if err := os.WriteFile(largeFile, largeContent, 0600); err != nil {
		t.Fatalf("create large file: %v", err)
	}

	cfg := Config{
		ServerURL: "http://localhost:8443",
		NodeID:    testNodeID,
	}
	uploader, err := newBaseUploader(cfg)
	if err != nil {
		t.Fatalf("create uploader: %v", err)
	}

	ctx := context.Background()
	_, _, err = uploader.UploadArtifact(ctx, "test-run-id", "test-job-id", []string{largeFile}, "")
	if err == nil {
		t.Error("expected error for oversized bundle")
	}
}
