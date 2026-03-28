package nodeagent

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
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
	tests := []struct {
		name        string
		diffContent string
		summary     types.DiffSummary
		serverCode  int
		wantErr     bool
	}{
		{
			name:        "successful upload",
			diffContent: "diff --git a/file.txt b/file.txt\n--- a/file.txt\n+++ b/file.txt\n@@ -1 +1 @@\n-old line\n+new line\n",
			summary: types.NewDiffSummaryBuilder().
				ExitCode(0).
				Timings(0, 0, 0, 1000).
				MustBuild(),
			serverCode: http.StatusCreated,
		},
		{
			name:        "empty diff",
			diffContent: "",
			summary: types.NewDiffSummaryBuilder().
				ExitCode(0).
				MustBuild(),
			serverCode: http.StatusCreated,
		},
		{
			name:        "server error",
			diffContent: "diff content",
			summary:     types.NewDiffSummaryBuilder().MustBuild(),
			serverCode:  http.StatusInternalServerError,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, _ := newDiffUploadServer(t, "test-run-id", "test-job-id",
				withDiffStatus(tt.serverCode))

			uploader := newTestUploader(t, server.URL)
			err := uploader.UploadDiff(context.Background(), "test-run-id", "test-job-id",
				[]byte(tt.diffContent), tt.summary)
			checkErr(t, tt.wantErr, err)
		})
	}
}

func TestDiffUploader_Compression(t *testing.T) {
	diffContent := "diff --git a/file.txt b/file.txt\n--- a/file.txt\n+++ b/file.txt\n@@ -1 +1 @@\n-old line\n+new line\n"

	server, calls := newDiffUploadServer(t, "test-run-id", "test-job-id")
	uploader := newTestUploader(t, server.URL)

	err := uploader.UploadDiff(context.Background(), "test-run-id", "test-job-id",
		[]byte(diffContent), types.NewDiffSummaryBuilder().MustBuild())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(*calls) == 0 {
		t.Fatal("no upload calls recorded")
	}

	// Verify the patch is valid gzip containing the original diff.
	gz, err := gzip.NewReader(bytes.NewReader((*calls)[0].Patch))
	if err != nil {
		t.Fatalf("patch is not valid gzip: %v", err)
	}
	defer func() { _ = gz.Close() }()

	decompressed, err := io.ReadAll(gz)
	if err != nil {
		t.Fatalf("failed to decompress: %v", err)
	}
	if string(decompressed) != diffContent {
		t.Errorf("decompressed content mismatch:\ngot:  %s\nwant: %s", decompressed, diffContent)
	}
}

// TestBearerToken_TrimsWhitespace verifies that the bearer token read from
// file is trimmed of leading/trailing whitespace before being used in
// the Authorization header.
func TestBearerToken_TrimsWhitespace(t *testing.T) {
	tests := []struct {
		name          string
		tokenContent  string
		expectedToken string
	}{
		{"trailing newline", "tok\n", "tok"},
		{"trailing CRLF", "tok\r\n", "tok"},
		{"leading and trailing whitespace", "  tok  \n", "tok"},
		{"multiple trailing newlines", "tok\n\n\n", "tok"},
		{"clean token", "tok", "tok"},
		{"internal spaces preserved", "tok with spaces\n", "tok with spaces"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokenPath := filepath.Join(t.TempDir(), "bearer-token")
			if err := os.WriteFile(tokenPath, []byte(tt.tokenContent), 0600); err != nil {
				t.Fatalf("write token file: %v", err)
			}
			t.Setenv("PLOY_NODE_BEARER_TOKEN_PATH", tokenPath)

			var capturedAuthHeader string
			server := newCaptureAuthServer(t, &capturedAuthHeader)
			uploader := newTestUploader(t, server.URL)

			_ = uploader.UploadDiff(context.Background(), "test-run-id", "test-job-id",
				[]byte("diff"), types.NewDiffSummaryBuilder().MustBuild())

			expectedHeader := "Bearer " + tt.expectedToken
			if capturedAuthHeader != expectedHeader {
				t.Errorf("Authorization header mismatch:\ngot:  %q\nwant: %q",
					capturedAuthHeader, expectedHeader)
			}
		})
	}
}

// --- Upload size cap tests ---

func TestUploader_SizeCap(t *testing.T) {
	tests := []struct {
		name   string
		upload func(context.Context, *baseUploader) error
	}{
		{
			name: "diff exceeding size cap",
			upload: func(ctx context.Context, u *baseUploader) error {
				return u.UploadDiff(ctx, "test-run-id", "test-job-id",
					incompressibleBytes(MaxUploadSize+1),
					types.NewDiffSummaryBuilder().MustBuild())
			},
		},
		{
			name: "artifact exceeding size cap",
			upload: func(ctx context.Context, u *baseUploader) error {
				f := filepath.Join(t.TempDir(), "large.bin")
				if err := os.WriteFile(f, incompressibleBytes(MaxUploadSize+1), 0600); err != nil {
					t.Fatalf("write large file: %v", err)
				}
				_, _, err := u.UploadArtifact(ctx, "test-run-id", "test-job-id", []string{f}, "")
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newUncalledServer(t)
			uploader := newTestUploader(t, server.URL)

			err := tt.upload(context.Background(), uploader)
			if err == nil {
				t.Fatal("expected error for oversized upload but got none")
			}
			if !strings.Contains(err.Error(), "exceeds size cap") {
				t.Fatalf("unexpected error, want size cap: %v", err)
			}
		})
	}
}

// --- ArtifactUploader tests ---

func TestArtifactUploader_UploadArtifact(t *testing.T) {
	tests := []struct {
		name       string
		files      map[string]string // filename -> content; nil means no files
		serverCode int
		wantErr    bool
		verify     func(t *testing.T, calls []artifactUploadCall)
	}{
		{
			name:       "success",
			files:      map[string]string{"test1.txt": "content1", "test2.txt": "content2"},
			serverCode: http.StatusCreated,
			verify: func(t *testing.T, calls []artifactUploadCall) {
				t.Helper()
				if len(calls) != 1 {
					t.Fatalf("expected 1 upload call, got %d", len(calls))
				}
				if calls[0].Name != "test-bundle" {
					t.Errorf("expected name 'test-bundle', got %q", calls[0].Name)
				}
				if len(calls[0].Bundle) == 0 {
					t.Error("expected non-empty bundle")
				}
			},
		},
		{
			name: "empty paths",
		},
		{
			name:       "server error",
			files:      map[string]string{"test.txt": "content"},
			serverCode: http.StatusInternalServerError,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp files from declarative map.
			var paths []string
			if tt.files != nil {
				tmpDir := t.TempDir()
				for name, content := range tt.files {
					p := filepath.Join(tmpDir, name)
					if err := os.WriteFile(p, []byte(content), 0600); err != nil {
						t.Fatalf("create test file: %v", err)
					}
					paths = append(paths, p)
				}
			}

			var serverURL string
			var calls *[]artifactUploadCall
			if tt.serverCode != 0 {
				srv, c := newArtifactUploadServer(t, "test-run-id", "test-job-id",
					withArtifactStatus(tt.serverCode))
				serverURL = srv.URL
				calls = c
			} else {
				serverURL = "http://localhost:8443"
			}

			uploader := newTestUploader(t, serverURL)
			_, _, err := uploader.UploadArtifact(context.Background(),
				"test-run-id", "test-job-id", paths, "test-bundle")
			checkErr(t, tt.wantErr, err)

			if tt.verify != nil && calls != nil {
				tt.verify(t, *calls)
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
	_, err := createTarGzBundle([]string{"/nonexistent/file.txt"})
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

// TestCreateTarGzBundle_SymlinkPreserved verifies that symlinks in a directory
// are archived as symlinks (TypeSymlink header), not as regular files with
// followed content. External symlinks are skipped for security.
func TestCreateTarGzBundle_SymlinkPreserved(t *testing.T) {
	tmpDir := t.TempDir()

	workspaceDir := filepath.Join(tmpDir, "workspace")
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}

	regularFile := filepath.Join(workspaceDir, "regular.txt")
	regularContent := []byte("regular file content")
	if err := os.WriteFile(regularFile, regularContent, 0o600); err != nil {
		t.Fatalf("write regular file: %v", err)
	}

	// Symlink pointing outside workspace — should be skipped.
	if err := os.Symlink("/etc/hosts", filepath.Join(workspaceDir, "link_to_external")); err != nil {
		t.Fatalf("create external symlink: %v", err)
	}

	// Symlink pointing to internal file — should be preserved as symlink.
	if err := os.Symlink("regular.txt", filepath.Join(workspaceDir, "link_to_internal")); err != nil {
		t.Fatalf("create internal symlink: %v", err)
	}

	bundleBytes, err := createTarGzBundle([]string{workspaceDir})
	if err != nil {
		t.Fatalf("create bundle: %v", err)
	}

	entries := tarEntriesFromBundle(t, bundleBytes)

	// Regular file archived correctly.
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

	// External symlink skipped for security.
	if _, ok = entries[filepath.Join("workspace", "link_to_external")]; ok {
		t.Error("link_to_external should NOT be in archive — external symlinks should be skipped")
	}

	// Internal symlink preserved as TypeSymlink.
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

// --- helpers local to this file ---

// newCaptureAuthServer returns a test server that captures the Authorization
// header and responds with 201. Used by bearer token tests.
func newCaptureAuthServer(t *testing.T, dst *string) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*dst = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(ts.Close)
	return ts
}
