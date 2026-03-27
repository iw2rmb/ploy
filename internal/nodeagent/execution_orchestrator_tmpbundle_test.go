package nodeagent

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// buildTestTarGz creates a minimal tar.gz archive with the given entries for testing.
// Each entry is map[archivePath]content; directories have nil content.
func buildTestTarGz(t *testing.T, entries map[string][]byte, typeflagOverrides map[string]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	for name, content := range entries {
		var typeflag byte = tar.TypeReg
		if content == nil {
			typeflag = tar.TypeDir
		}
		if override, ok := typeflagOverrides[name]; ok {
			typeflag = override
		}
		hdr := &tar.Header{
			Name:     name,
			Typeflag: typeflag,
			Size:     int64(len(content)),
			Mode:     0o644,
		}
		// Symlinks and hardlinks carry no file data.
		if typeflag == tar.TypeSymlink || typeflag == tar.TypeLink {
			hdr.Size = 0
			hdr.Linkname = "link-target"
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write tar header: %v", err)
		}
		if hdr.Size > 0 && len(content) > 0 {
			if _, err := tw.Write(content); err != nil {
				t.Fatalf("write tar content: %v", err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buf.Bytes()
}

func digestOf(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// --- extractTmpBundle tests ---

func TestExtractTmpBundle_ValidFiles(t *testing.T) {
	t.Parallel()

	data := buildTestTarGz(t, map[string][]byte{
		"config.json": []byte(`{"k":"v"}`),
		"secret.txt":  []byte("s3cr3t"),
	}, nil)

	stagingDir := t.TempDir()
	if err := extractTmpBundle(data, stagingDir); err != nil {
		t.Fatalf("extractTmpBundle error: %v", err)
	}

	for name, want := range map[string][]byte{
		"config.json": []byte(`{"k":"v"}`),
		"secret.txt":  []byte("s3cr3t"),
	} {
		got, err := os.ReadFile(filepath.Join(stagingDir, name))
		if err != nil {
			t.Errorf("read %q: %v", name, err)
			continue
		}
		if string(got) != string(want) {
			t.Errorf("%q: got %q, want %q", name, got, want)
		}
	}
}

func TestExtractTmpBundle_ValidDirectory(t *testing.T) {
	t.Parallel()

	data := buildTestTarGz(t, map[string][]byte{
		"scripts/":        nil,
		"scripts/run.sh":  []byte("#!/bin/sh\necho hello"),
		"scripts/prep.sh": []byte("#!/bin/sh\necho prep"),
	}, nil)

	stagingDir := t.TempDir()
	if err := extractTmpBundle(data, stagingDir); err != nil {
		t.Fatalf("extractTmpBundle error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(stagingDir, "scripts")); err != nil {
		t.Fatalf("scripts dir not created: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(stagingDir, "scripts", "run.sh"))
	if err != nil {
		t.Fatalf("read scripts/run.sh: %v", err)
	}
	if string(got) != "#!/bin/sh\necho hello" {
		t.Errorf("scripts/run.sh: got %q", got)
	}
}

func TestExtractTmpBundle_RejectsUnsafeEntryType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		typeflag byte
	}{
		{name: "symlink", typeflag: tar.TypeSymlink},
		{name: "hardlink", typeflag: tar.TypeLink},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			data := buildTestTarGz(t, map[string][]byte{
				"evil.sh": []byte("data"),
			}, map[string]byte{"evil.sh": tt.typeflag})

			stagingDir := t.TempDir()
			if err := extractTmpBundle(data, stagingDir); err == nil {
				t.Fatalf("expected error for %s entry, got nil", tt.name)
			}
		})
	}
}

func TestExtractTmpBundle_RejectsUnsafePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
	}{
		{name: "absolute_path", path: "/etc/passwd"},
		{name: "traversal_parent", path: "../escape"},
		{name: "traversal_nested", path: "foo/../../etc/passwd"},
		{name: "duplicate_path", path: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			gw := gzip.NewWriter(&buf)
			tw := tar.NewWriter(gw)

			if tt.name == "duplicate_path" {
				for range 2 {
					_ = tw.WriteHeader(&tar.Header{Name: "config.json", Typeflag: tar.TypeReg, Size: 2, Mode: 0o644})
					_, _ = tw.Write([]byte("{}"))
				}
			} else {
				_ = tw.WriteHeader(&tar.Header{Name: tt.path, Typeflag: tar.TypeReg, Size: 4, Mode: 0o644})
				_, _ = tw.Write([]byte("data"))
			}

			_ = tw.Close()
			_ = gw.Close()

			stagingDir := t.TempDir()
			if err := extractTmpBundle(buf.Bytes(), stagingDir); err == nil {
				t.Fatalf("expected error for %s, got nil", tt.name)
			}
		})
	}
}

func TestVerifyBundleDigest(t *testing.T) {
	t.Parallel()

	data := []byte("hello bundle")
	correctDigest := digestOf(data)

	tests := []struct {
		name    string
		data    []byte
		digest  string
		wantErr bool
	}{
		{name: "match", data: data, digest: correctDigest},
		{name: "mismatch", data: data, digest: "deadbeef", wantErr: true},
		{name: "case_insensitive", data: []byte("case test"), digest: upper(digestOf([]byte("case test")))},
		{name: "sha256_prefix", data: []byte("server prefix test"), digest: "sha256:" + digestOf([]byte("server prefix test"))},
		{name: "sha256_prefix_mismatch", data: []byte("server prefix test"), digest: "sha256:deadbeef", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := verifyBundleDigest(tt.data, tt.digest)
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func upper(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'a' && c <= 'f' {
			b[i] = c - 32
		}
	}
	return string(b)
}

// --- materializeTmpBundle integration: download + verify + extract ---

func TestTmpBundle_Materialization_DownloadVerifyExtract(t *testing.T) {
	t.Parallel()

	archiveData := buildTestTarGz(t, map[string][]byte{
		"config.json": []byte(`{"key":"value"}`),
	}, nil)
	digest := digestOf(archiveData)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(archiveData)
	}))
	defer srv.Close()

	rc := newTestController(t, newTestConfig(srv.URL))

	bundle := &contracts.TmpBundleRef{
		BundleID: "bun-test-123",
		CID:      "bafy123",
		Digest:   digest,
		Entries:  []string{"config.json"},
	}

	stagingDir := t.TempDir()
	if err := rc.materializeTmpBundle(t.Context(), bundle, stagingDir); err != nil {
		t.Fatalf("materializeTmpBundle error: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(stagingDir, "config.json"))
	if err != nil {
		t.Fatalf("read config.json: %v", err)
	}
	if string(got) != `{"key":"value"}` {
		t.Errorf("config.json: got %q", got)
	}
}

func TestTmpBundle_Materialization_DigestMismatchRejected(t *testing.T) {
	t.Parallel()

	archiveData := buildTestTarGz(t, map[string][]byte{
		"file.txt": []byte("data"),
	}, nil)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(archiveData)
	}))
	defer srv.Close()

	rc := newTestController(t, newTestConfig(srv.URL))

	bundle := &contracts.TmpBundleRef{
		BundleID: "bun-bad-digest",
		CID:      "bafy123",
		Digest:   "0000000000000000000000000000000000000000000000000000000000000000",
		Entries:  []string{"file.txt"},
	}

	stagingDir := t.TempDir()
	if err := rc.materializeTmpBundle(t.Context(), bundle, stagingDir); err == nil {
		t.Fatal("expected digest mismatch error, got nil")
	}
}

func TestTmpBundle_Materialization_ServerErrorRejected(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	rc := newTestController(t, newTestConfig(srv.URL))

	bundle := &contracts.TmpBundleRef{
		BundleID: "bun-missing",
		CID:      "bafy123",
		Digest:   "abc",
		Entries:  []string{"file.txt"},
	}

	stagingDir := t.TempDir()
	if err := rc.materializeTmpBundle(t.Context(), bundle, stagingDir); err == nil {
		t.Fatal("expected error for 404 response, got nil")
	}
}
