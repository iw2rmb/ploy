package nodeagent

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

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
	uploader, err := NewArtifactUploader(cfg)
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
	uploader, err := NewArtifactUploader(cfg)
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
	uploader, err := NewArtifactUploader(cfg)
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
	// Create a large file that will exceed the 1 MiB cap when bundled.
	tmpDir := t.TempDir()
	largeFile := filepath.Join(tmpDir, "large.txt")
	// Create a 2 MiB file (will exceed 1 MiB cap even when compressed).
	largeContent := make([]byte, 2<<20)
	for i := range largeContent {
		largeContent[i] = byte(i % 256)
	}
	if err := os.WriteFile(largeFile, largeContent, 0600); err != nil {
		t.Fatalf("create large file: %v", err)
	}

	cfg := Config{
		ServerURL: "http://localhost:8443",
		NodeID:    testNodeID,
	}
	uploader, err := NewArtifactUploader(cfg)
	if err != nil {
		t.Fatalf("create uploader: %v", err)
	}

	ctx := context.Background()
	_, _, err = uploader.UploadArtifact(ctx, "test-run-id", "test-job-id", []string{largeFile}, "")
	if err == nil {
		t.Error("expected error for oversized bundle")
	}
}
