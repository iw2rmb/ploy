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

func TestExtractTmpBundle_Valid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		entries   map[string][]byte
		wantFiles map[string]string
		wantDirs  []string
	}{
		{
			name: "flat files",
			entries: map[string][]byte{
				"config.json": []byte(`{"k":"v"}`),
				"secret.txt":  []byte("s3cr3t"),
			},
			wantFiles: map[string]string{
				"config.json": `{"k":"v"}`,
				"secret.txt":  "s3cr3t",
			},
		},
		{
			name: "nested directory",
			entries: map[string][]byte{
				"scripts/":        nil,
				"scripts/run.sh":  []byte("#!/bin/sh\necho hello"),
				"scripts/prep.sh": []byte("#!/bin/sh\necho prep"),
			},
			wantFiles: map[string]string{
				"scripts/run.sh":  "#!/bin/sh\necho hello",
				"scripts/prep.sh": "#!/bin/sh\necho prep",
			},
			wantDirs: []string{"scripts"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			data := buildTestTarGz(t, tt.entries, nil)
			stagingDir := t.TempDir()
			if err := extractTmpBundle(data, stagingDir); err != nil {
				t.Fatalf("extractTmpBundle error: %v", err)
			}
			for _, dir := range tt.wantDirs {
				if _, err := os.Stat(filepath.Join(stagingDir, dir)); err != nil {
					t.Fatalf("dir %q not created: %v", dir, err)
				}
			}
			for path, want := range tt.wantFiles {
				got, err := os.ReadFile(filepath.Join(stagingDir, path))
				if err != nil {
					t.Errorf("read %q: %v", path, err)
					continue
				}
				if string(got) != want {
					t.Errorf("%q: got %q, want %q", path, got, want)
				}
			}
		})
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

// --- materializeHydraResources integration: download + verify + extract ---

func TestHydraResources_Materialization(t *testing.T) {
	t.Parallel()

	validArchive := buildTestTarGz(t, map[string][]byte{
		"config.json": []byte(`{"key":"value"}`),
	}, nil)
	validHash := digestOf(validArchive)[:12] // short hash prefix

	badDigestArchive := buildTestTarGz(t, map[string][]byte{
		"file.txt": []byte("data"),
	}, nil)
	badHash := "000000000000"

	tests := []struct {
		name      string
		handler   http.HandlerFunc
		manifest  contracts.StepManifest
		bundleMap map[string]string
		wantErr   bool
		assertFn  func(t *testing.T, stagingDir string)
	}{
		{
			name: "download verify extract via CA entry",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/gzip")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(validArchive)
			},
			manifest:  contracts.StepManifest{CA: []string{validHash}},
			bundleMap: map[string]string{validHash: "bun-test-123"},
			assertFn: func(t *testing.T, stagingDir string) {
				t.Helper()
				got, err := os.ReadFile(filepath.Join(stagingDir, validHash, "config.json"))
				if err != nil {
					t.Fatalf("read config.json: %v", err)
				}
				if string(got) != `{"key":"value"}` {
					t.Errorf("config.json: got %q", got)
				}
			},
		},
		{
			name: "digest mismatch rejected",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(badDigestArchive)
			},
			manifest:  contracts.StepManifest{In: []string{badHash + ":/in/data"}},
			bundleMap: map[string]string{badHash: "bun-bad-digest"},
			wantErr:   true,
		},
		{
			name: "server error rejected",
			handler: func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "not found", http.StatusNotFound)
			},
			manifest:  contracts.StepManifest{Out: []string{"abc1234567ab:/out/result"}},
			bundleMap: map[string]string{"abc1234567ab": "bun-missing"},
			wantErr:   true,
		},
		{
			name: "missing bundle map entry rejected",
			handler: func(w http.ResponseWriter, r *http.Request) {
				t.Fatal("handler should not be called")
			},
			manifest:  contracts.StepManifest{CA: []string{"deadbeef1234"}},
			bundleMap: map[string]string{}, // no mapping
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(tt.handler)
			defer srv.Close()

			rc := newTestController(t, newAgentConfig(srv.URL))
			stagingDir := t.TempDir()
			err := rc.materializeHydraResources(t.Context(), tt.manifest, tt.bundleMap, stagingDir)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("materializeHydraResources error: %v", err)
			}
			if tt.assertFn != nil {
				tt.assertFn(t, stagingDir)
			}
		})
	}
}
