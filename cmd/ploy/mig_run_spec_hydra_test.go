package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Authoring entry parsers
// ---------------------------------------------------------------------------

func TestParseAuthoringInEntry(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input   string
		wantSrc string
		wantDst string
		wantErr string
	}{
		{input: "/tmp/data.txt:/in/data.txt", wantSrc: "/tmp/data.txt", wantDst: "/in/in/data.txt"},
		{input: "relative/path:/in/nested/file.json", wantSrc: "relative/path", wantDst: "/in/in/nested/file.json"},
		{input: "/tmp/data.txt:data.txt", wantSrc: "/tmp/data.txt", wantDst: "/in/data.txt"},
		{input: "/tmp/data.txt:./nested/data.txt", wantSrc: "/tmp/data.txt", wantDst: "/in/nested/data.txt"},
		{input: "/tmp/data.txt:/out/data.txt", wantSrc: "/tmp/data.txt", wantDst: "/in/out/data.txt"},
		{input: "/tmp/data.txt:/in/../etc/passwd", wantSrc: "/tmp/data.txt", wantDst: "/in/etc/passwd"},
		{input: "no-colon-at-all", wantErr: "expected src:dst format"},
		{input: ":/in/dst", wantErr: "source is empty"},
		{input: "/src:", wantErr: "destination is empty"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			src, dst, err := parseAuthoringInEntry(tt.input)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if src != tt.wantSrc {
				t.Errorf("src = %q, want %q", src, tt.wantSrc)
			}
			if dst != tt.wantDst {
				t.Errorf("dst = %q, want %q", dst, tt.wantDst)
			}
		})
	}
}

func TestParseAuthoringOutEntry(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input   string
		wantSrc string
		wantDst string
		wantErr string
	}{
		{input: "/tmp/out.txt:/out/result.txt", wantSrc: "/tmp/out.txt", wantDst: "/out/out/result.txt"},
		{input: "/tmp/out.txt:result.txt", wantSrc: "/tmp/out.txt", wantDst: "/out/result.txt"},
		{input: "/tmp/out.txt:/in/result.txt", wantSrc: "/tmp/out.txt", wantDst: "/out/in/result.txt"},
		{input: "/tmp/out.txt:/out/../etc/shadow", wantSrc: "/tmp/out.txt", wantDst: "/out/etc/shadow"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			src, dst, err := parseAuthoringOutEntry(tt.input)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if src != tt.wantSrc {
				t.Errorf("src = %q, want %q", src, tt.wantSrc)
			}
			if dst != tt.wantDst {
				t.Errorf("dst = %q, want %q", dst, tt.wantDst)
			}
		})
	}
}

func TestParseAuthoringHomeEntry(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input   string
		wantSrc string
		wantDst string
		wantRO  bool
		wantErr string
	}{
		{input: "/tmp/cfg:.config/app.toml", wantSrc: "/tmp/cfg", wantDst: ".config/app.toml"},
		{input: "/tmp/cfg:.config/app.toml:ro", wantSrc: "/tmp/cfg", wantDst: ".config/app.toml", wantRO: true},
		{input: "/tmp/cfg:/absolute/path", wantSrc: "/tmp/cfg", wantDst: "absolute/path"},
		{input: "/tmp/cfg:../escape", wantErr: "path traversal not allowed"},
		{input: "just-a-string", wantErr: "expected src:dst format"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			src, dst, ro, err := parseAuthoringHomeEntry(tt.input)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if src != tt.wantSrc {
				t.Errorf("src = %q, want %q", src, tt.wantSrc)
			}
			if dst != tt.wantDst {
				t.Errorf("dst = %q, want %q", dst, tt.wantDst)
			}
			if ro != tt.wantRO {
				t.Errorf("readOnly = %v, want %v", ro, tt.wantRO)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Canonical detection
// ---------------------------------------------------------------------------

func TestIsAlreadyCanonical(t *testing.T) {
	t.Parallel()
	tests := []struct {
		field string
		entry string
		want  bool
	}{
		{field: "ca", entry: "abcdef1234ab", want: true},
		{field: "ca", entry: "abcdef1234abcdef1234abcdef1234abcdef1234abcdef1234abcdef1234abcd", want: true},
		{field: "ca", entry: "/tmp/cert.pem", want: false},
		{field: "ca", entry: "relative/cert.pem", want: false},
		{field: "in", entry: "abcdef1234ab:/in/config.txt", want: true},
		{field: "in", entry: "/tmp/data.txt:/in/data.txt", want: false},
		{field: "out", entry: "abcdef1234ab:/out/result.txt", want: true},
		{field: "home", entry: "abcdef1234ab:.config/app.toml", want: true},
		{field: "home", entry: "abcdef1234ab:.config/app.toml:ro", want: true},
		{field: "in", entry: "no-colon", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.field+"/"+tt.entry, func(t *testing.T) {
			t.Parallel()
			got := isAlreadyCanonical(tt.field, tt.entry)
			if got != tt.want {
				t.Errorf("isAlreadyCanonical(%q, %q) = %v, want %v", tt.field, tt.entry, got, tt.want)
			}
		})
	}
}

func TestHasAuthoringEntries(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		block map[string]any
		want  bool
	}{
		{name: "no hydra fields", block: map[string]any{"image": "test:latest"}, want: false},
		{name: "ca with file path", block: map[string]any{"ca": []any{"/tmp/cert.pem"}}, want: true},
		{name: "ca already canonical", block: map[string]any{"ca": []any{"abcdef1234ab"}}, want: false},
		{name: "in with authoring entry", block: map[string]any{"in": []any{"/tmp/data.txt:/in/data.txt"}}, want: true},
		{name: "in already canonical", block: map[string]any{"in": []any{"abcdef1234ab:/in/data.txt"}}, want: false},
		{name: "mixed canonical and authoring", block: map[string]any{"in": []any{"abcdef1234ab:/in/a.txt", "/tmp/b.txt:/in/b.txt"}}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := hasAuthoringEntries(tt.block)
			if got != tt.want {
				t.Errorf("hasAuthoringEntries() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Archive and hash
// ---------------------------------------------------------------------------

func TestBuildSourceArchive_FileProducesDeterministicHash(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	f1 := filepath.Join(tmpDir, "a.txt")
	f2 := filepath.Join(tmpDir, "b.txt")
	writeFile(t, f1, "hello")
	writeFile(t, f2, "hello")

	arch1, err := buildSourceArchive(f1)
	if err != nil {
		t.Fatalf("buildSourceArchive(%s): %v", f1, err)
	}
	arch2, err := buildSourceArchive(f2)
	if err != nil {
		t.Fatalf("buildSourceArchive(%s): %v", f2, err)
	}

	hash1 := computeArchiveShortHash(arch1)
	hash2 := computeArchiveShortHash(arch2)
	if hash1 != hash2 {
		t.Errorf("identical content produced different hashes: %s vs %s", hash1, hash2)
	}
	if len(hash1) != shortHashLen {
		t.Errorf("short hash length = %d, want %d", len(hash1), shortHashLen)
	}
}

func TestBuildSourceArchive_DirProducesDeterministicHash(t *testing.T) {
	t.Parallel()
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	for _, d := range []string{dir1, dir2} {
		writeFile(t, filepath.Join(d, "a.txt"), "alpha")
		writeFile(t, filepath.Join(d, "b.txt"), "beta")
	}

	arch1, err := buildSourceArchive(dir1)
	if err != nil {
		t.Fatal(err)
	}
	arch2, err := buildSourceArchive(dir2)
	if err != nil {
		t.Fatal(err)
	}

	hash1 := computeArchiveShortHash(arch1)
	hash2 := computeArchiveShortHash(arch2)
	if hash1 != hash2 {
		t.Errorf("identical dir content produced different hashes: %s vs %s", hash1, hash2)
	}
}

func TestBuildSourceArchive_PreservesModeAndModTime(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dir := filepath.Join(tmpDir, "scripts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	execPath := filepath.Join(dir, "build.sh")
	plainPath := filepath.Join(dir, "config.json")
	writeFile(t, execPath, "#!/usr/bin/env bash\necho ok\n")
	writeFile(t, plainPath, `{"ok":true}`)

	if err := os.Chmod(execPath, 0o751); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(plainPath, 0o640); err != nil {
		t.Fatal(err)
	}

	execModTime := time.Unix(1_701_111_111, 0).UTC()
	plainModTime := time.Unix(1_702_222_222, 0).UTC()
	if err := os.Chtimes(execPath, execModTime, execModTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(plainPath, plainModTime, plainModTime); err != nil {
		t.Fatal(err)
	}

	archiveBytes, err := buildSourceArchive(dir)
	if err != nil {
		t.Fatal(err)
	}
	headers := readTarHeaders(t, archiveBytes)

	execHeader, ok := headers["content/build.sh"]
	if !ok {
		t.Fatalf("missing header for content/build.sh")
	}
	if got, want := execHeader.FileInfo().Mode().Perm(), os.FileMode(0o751); got != want {
		t.Fatalf("content/build.sh mode = %o, want %o", got, want)
	}
	if !execHeader.ModTime.Equal(execModTime) {
		t.Fatalf("content/build.sh modtime = %s, want %s", execHeader.ModTime.UTC(), execModTime)
	}

	plainHeader, ok := headers["content/config.json"]
	if !ok {
		t.Fatalf("missing header for content/config.json")
	}
	if got, want := plainHeader.FileInfo().Mode().Perm(), os.FileMode(0o640); got != want {
		t.Fatalf("content/config.json mode = %o, want %o", got, want)
	}
	if !plainHeader.ModTime.Equal(plainModTime) {
		t.Fatalf("content/config.json modtime = %s, want %s", plainHeader.ModTime.UTC(), plainModTime)
	}
}

func TestComputeArchiveShortHash(t *testing.T) {
	t.Parallel()
	data := []byte("test data for hashing")
	hash := computeArchiveShortHash(data)
	if len(hash) != shortHashLen {
		t.Errorf("short hash length = %d, want %d", len(hash), shortHashLen)
	}

	full := sha256.Sum256(data)
	want := hex.EncodeToString(full[:])[:shortHashLen]
	if hash != want {
		t.Errorf("hash = %q, want %q", hash, want)
	}
}

// ---------------------------------------------------------------------------
// Mock server for upload
// ---------------------------------------------------------------------------

func newMockBundleServer(t *testing.T) (*httptest.Server, *url.URL, *http.Client, *int) {
	t.Helper()
	var uploadCount int
	seenCIDs := make(map[string]string)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/spec-bundles" {
			if r.Method == http.MethodHead {
				cid := r.URL.Query().Get("cid")
				if bundleID, ok := seenCIDs[cid]; ok {
					w.Header().Set("X-Bundle-ID", bundleID)
					w.WriteHeader(http.StatusOK)
				} else {
					w.WriteHeader(http.StatusNotFound)
				}
				return
			}
			if r.Method == http.MethodPost {
				uploadCount++
				data, _ := io.ReadAll(r.Body)
				h := sha256.Sum256(data)
				hexHash := hex.EncodeToString(h[:])
				cid := "bafy" + hexHash[:32]
				digest := "sha256:" + hexHash
				bundleID := "bundle-" + hex.EncodeToString(h[:8])
				seenCIDs[cid] = bundleID
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"bundle_id": bundleID,
					"cid":       cid,
					"digest":    digest,
				})
				return
			}
		}
		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	return srv, u, srv.Client(), &uploadCount
}

// compileHydraSpec is a test helper that calls compileHydraRecordsInPlace and
// fails the test on error.
func compileHydraSpec(t *testing.T, base *url.URL, client *http.Client, spec map[string]any, tmpDir string) {
	t.Helper()
	if err := compileHydraRecordsInPlace(context.Background(), base, client, spec, tmpDir); err != nil {
		t.Fatalf("compileHydraRecordsInPlace: %v", err)
	}
}

// assertCanonicalHash checks that s is a valid short hash (or starts with one
// before the first colon for in/out/home entries).
func assertCanonicalHash(t *testing.T, s string) string {
	t.Helper()
	hash := strings.SplitN(s, ":", 2)[0]
	if !shortHashPattern.MatchString(hash) {
		t.Errorf("%q: hash segment %q does not match short hash pattern", s, hash)
	}
	return hash
}

// ---------------------------------------------------------------------------
// Compile Hydra records: single-field (ca/in/out/home)
// ---------------------------------------------------------------------------

func TestCompileHydraRecordsInPlace_SingleField(t *testing.T) {
	tests := []struct {
		name        string
		fileName    string
		content     string
		field       string // "ca", "in", "out", "home"
		entrySuffix string // appended to file path for in/out/home
		wantSuffix  string // expected suffix on compiled entry
	}{
		{
			name:     "ca entries",
			fileName: "cert.pem",
			content:  "-----BEGIN CERTIFICATE-----\ntest\n-----END CERTIFICATE-----\n",
			field:    "ca",
		},
		{
			name:        "in entries",
			fileName:    "config.json",
			content:     `{"key": "value"}`,
			field:       "in",
			entrySuffix: ":config.json",
			wantSuffix:  ":/in/config.json",
		},
		{
			name:        "out entries",
			fileName:    "template.txt",
			content:     "output template",
			field:       "out",
			entrySuffix: ":result.txt",
			wantSuffix:  ":/out/result.txt",
		},
		{
			name:        "home entries",
			fileName:    "config.toml",
			content:     "[app]\nkey = true\n",
			field:       "home",
			entrySuffix: ":/.config/app.toml:ro",
			wantSuffix:  ":.config/app.toml:ro",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, base, client, _ := newMockBundleServer(t)
			tmpDir := t.TempDir()
			filePath := filepath.Join(tmpDir, tt.fileName)
			writeFile(t, filePath, tt.content)

			authEntry := filePath + tt.entrySuffix
			spec := map[string]any{
				"steps": []any{
					map[string]any{
						"image":  "docker.io/test/mig:latest",
						tt.field: []any{authEntry},
					},
				},
			}

			compileHydraSpec(t, base, client, spec, tmpDir)

			step0 := spec["steps"].([]any)[0].(map[string]any)
			entries := step0[tt.field].([]any)
			if len(entries) != 1 {
				t.Fatalf("expected 1 %s entry, got %d", tt.field, len(entries))
			}
			entry := entries[0].(string)
			assertCanonicalHash(t, entry)
			if tt.wantSuffix != "" && !strings.HasSuffix(entry, tt.wantSuffix) {
				t.Errorf("%s[0] = %q, expected suffix %q", tt.field, entry, tt.wantSuffix)
			}
		})
	}
}

func TestCompileHydraRecordsInPlace_InDirectoryRelativeDestination(t *testing.T) {
	_, base, client, _ := newMockBundleServer(t)
	tmpDir := t.TempDir()

	amataDir := filepath.Join(tmpDir, "amata")
	if err := os.MkdirAll(amataDir, 0o755); err != nil {
		t.Fatalf("mkdir amata dir: %v", err)
	}
	writeFile(t, filepath.Join(amataDir, "workflow.yaml"), "version: amata/v1\n")
	writeFile(t, filepath.Join(amataDir, "schema.json"), `{"type":"object"}`)

	spec := map[string]any{
		"steps": []any{
			map[string]any{
				"image": "docker.io/test/mig:latest",
				"in": []any{
					"./amata:amata",
				},
			},
		},
	}

	compileHydraSpec(t, base, client, spec, tmpDir)
	step0 := spec["steps"].([]any)[0].(map[string]any)
	entry := step0["in"].([]any)[0].(string)
	assertCanonicalHash(t, entry)
	if !strings.HasSuffix(entry, ":/in/amata") {
		t.Fatalf("in[0] = %q, want suffix %q", entry, ":/in/amata")
	}
}

// ---------------------------------------------------------------------------
// Edge cases: skip canonical, nil entries, missing server
// ---------------------------------------------------------------------------

func TestCompileHydraRecordsInPlace_SkipsCanonicalEntries(t *testing.T) {
	_, base, client, uploadCount := newMockBundleServer(t)

	spec := map[string]any{
		"steps": []any{
			map[string]any{
				"image": "docker.io/test/mig:latest",
				"ca":    []any{"abcdef1234ab"},
				"in":    []any{"abcdef1234ab:/in/config.json"},
			},
		},
	}

	compileHydraSpec(t, base, client, spec, "")

	if *uploadCount != 0 {
		t.Errorf("upload count = %d, want 0 (all entries canonical)", *uploadCount)
	}
	step0 := spec["steps"].([]any)[0].(map[string]any)
	if step0["ca"].([]any)[0] != "abcdef1234ab" {
		t.Errorf("ca[0] changed from canonical: %v", step0["ca"].([]any)[0])
	}
}

func TestCompileHydraRecordsInPlace_NilWhenNoEntries(t *testing.T) {
	t.Parallel()
	spec := map[string]any{
		"steps": []any{
			map[string]any{"image": "docker.io/test/mig:latest"},
		},
	}
	if err := compileHydraRecordsInPlace(context.Background(), nil, nil, spec, ""); err != nil {
		t.Fatalf("compileHydraRecordsInPlace: %v", err)
	}
}

func TestCompileHydraRecordsInPlace_ErrorsWithoutServer(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	writeFile(t, filepath.Join(tmpDir, "cert.pem"), "cert")

	spec := map[string]any{
		"steps": []any{
			map[string]any{
				"image": "docker.io/test/mig:latest",
				"ca":    []any{filepath.Join(tmpDir, "cert.pem")},
			},
		},
	}

	err := compileHydraRecordsInPlace(context.Background(), nil, nil, spec, tmpDir)
	if err == nil {
		t.Fatal("expected error when base URL is nil with authoring entries")
	}
	if !strings.Contains(err.Error(), "no server base URL") {
		t.Errorf("error = %q, expected to mention missing server URL", err)
	}
}

// ---------------------------------------------------------------------------
// Upload deduplication
// ---------------------------------------------------------------------------

func TestCompileHydraRecordsInPlace_UploadDedup(t *testing.T) {
	_, base, client, uploadCount := newMockBundleServer(t)

	tmpDir := t.TempDir()
	writeFile(t, filepath.Join(tmpDir, "a.txt"), "same-content")
	writeFile(t, filepath.Join(tmpDir, "b.txt"), "same-content")

	spec := map[string]any{
		"steps": []any{
			map[string]any{
				"image": "docker.io/test/mig:latest",
				"in": []any{
					filepath.Join(tmpDir, "a.txt") + ":/in/a.txt",
					filepath.Join(tmpDir, "b.txt") + ":/in/b.txt",
				},
			},
		},
	}

	compileHydraSpec(t, base, client, spec, tmpDir)

	step0 := spec["steps"].([]any)[0].(map[string]any)
	in := step0["in"].([]any)
	hash1 := assertCanonicalHash(t, in[0].(string))
	hash2 := assertCanonicalHash(t, in[1].(string))
	if hash1 != hash2 {
		t.Errorf("identical content produced different hashes: %s vs %s", hash1, hash2)
	}
	if *uploadCount != 1 {
		t.Errorf("upload count = %d, want 1 (identical content deduped in-process)", *uploadCount)
	}
}

func TestCompileHydraRecordsInPlace_RepeatedRunSkipsUpload(t *testing.T) {
	_, base, client, uploadCount := newMockBundleServer(t)

	tmpDir := t.TempDir()
	writeFile(t, filepath.Join(tmpDir, "cert.pem"), "repeated-run-cert")

	makeSpec := func() map[string]any {
		return map[string]any{
			"steps": []any{
				map[string]any{
					"image": "docker.io/test/mig:latest",
					"ca":    []any{filepath.Join(tmpDir, "cert.pem")},
				},
			},
		}
	}

	// First compile: probe misses, uploads.
	spec1 := makeSpec()
	compileHydraSpec(t, base, client, spec1, tmpDir)
	if *uploadCount != 1 {
		t.Fatalf("after first compile: upload count = %d, want 1", *uploadCount)
	}
	hash1 := spec1["steps"].([]any)[0].(map[string]any)["ca"].([]any)[0].(string)

	// Second compile: probe hits, no new upload.
	spec2 := makeSpec()
	compileHydraSpec(t, base, client, spec2, tmpDir)
	if *uploadCount != 1 {
		t.Errorf("after second compile: upload count = %d, want 1", *uploadCount)
	}
	hash2 := spec2["steps"].([]any)[0].(map[string]any)["ca"].([]any)[0].(string)

	if hash1 != hash2 {
		t.Errorf("repeated run produced different hashes: %s vs %s", hash1, hash2)
	}
}

// ---------------------------------------------------------------------------
// bundle_map emission
// ---------------------------------------------------------------------------

func TestCompileHydraRecordsInPlace_EmitsBundleMap(t *testing.T) {
	_, base, client, _ := newMockBundleServer(t)

	tmpDir := t.TempDir()
	writeFile(t, filepath.Join(tmpDir, "ca.pem"), "ca-cert")
	writeFile(t, filepath.Join(tmpDir, "config.json"), "in-data")
	writeFile(t, filepath.Join(tmpDir, "seed.txt"), "out-data")
	writeFile(t, filepath.Join(tmpDir, "auth.json"), "home-data")

	spec := map[string]any{
		"steps": []any{
			map[string]any{
				"image": "alpine:3",
				"ca":    []any{filepath.Join(tmpDir, "ca.pem")},
				"in":    []any{filepath.Join(tmpDir, "config.json") + ":/in/config.json"},
				"out":   []any{filepath.Join(tmpDir, "seed.txt") + ":/out/seed.txt"},
				"home":  []any{filepath.Join(tmpDir, "auth.json") + ":.auth.json"},
			},
		},
	}

	compileHydraSpec(t, base, client, spec, tmpDir)

	bm, ok := spec["bundle_map"]
	if !ok {
		t.Fatal("bundle_map not emitted into spec")
	}
	bundleMap, ok := bm.(map[string]string)
	if !ok {
		t.Fatalf("bundle_map type = %T, want map[string]string", bm)
	}

	step0 := spec["steps"].([]any)[0].(map[string]any)
	hashes := []string{
		step0["ca"].([]any)[0].(string),
		strings.SplitN(step0["in"].([]any)[0].(string), ":", 2)[0],
		strings.SplitN(step0["out"].([]any)[0].(string), ":", 2)[0],
		strings.SplitN(step0["home"].([]any)[0].(string), ":", 2)[0],
	}

	for _, h := range hashes {
		if _, exists := bundleMap[h]; !exists {
			t.Errorf("bundle_map missing entry for hash %q", h)
		}
	}
	for hash, bundleID := range bundleMap {
		if bundleID == "" {
			t.Errorf("bundle_map[%q] is empty", hash)
		}
	}
}

// ---------------------------------------------------------------------------
// bundle_map: mixed canonical/authoring preservation
// ---------------------------------------------------------------------------

func TestCompileHydraRecordsInPlace_MixedCanonicalAndAuthoring_PreservesBundleMap(t *testing.T) {
	tests := []struct {
		name      string
		bundleMap any // map[string]string or map[string]any (JSON-unmarshaled)
	}{
		{
			name: "typed map[string]string",
			bundleMap: map[string]string{
				"aabbcc112233": "bundle-existing-ca",
				"ddeeff445566": "bundle-existing-in",
			},
		},
		{
			name: "JSON-unmarshaled map[string]any",
			bundleMap: map[string]any{
				"aabbcc112233": "bundle-existing-ca",
				"ddeeff445566": "bundle-existing-in",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, base, client, uploadCount := newMockBundleServer(t)

			tmpDir := t.TempDir()
			writeFile(t, filepath.Join(tmpDir, "new.json"), `{"new":"data"}`)

			spec := map[string]any{
				"steps": []any{
					map[string]any{
						"image": "alpine:3",
						"ca":    []any{"aabbcc112233"},
						"in": []any{
							"ddeeff445566:/in/old.json",
							filepath.Join(tmpDir, "new.json") + ":/in/new.json",
						},
					},
				},
				"bundle_map": tt.bundleMap,
			}

			compileHydraSpec(t, base, client, spec, tmpDir)

			bm, ok := spec["bundle_map"].(map[string]string)
			if !ok {
				t.Fatalf("bundle_map type = %T, want map[string]string", spec["bundle_map"])
			}

			// Existing canonical entries must retain their mappings.
			if got := bm["aabbcc112233"]; got != "bundle-existing-ca" {
				t.Errorf("bundle_map[aabbcc112233] = %q, want %q", got, "bundle-existing-ca")
			}
			if got := bm["ddeeff445566"]; got != "bundle-existing-in" {
				t.Errorf("bundle_map[ddeeff445566] = %q, want %q", got, "bundle-existing-in")
			}

			// Newly compiled entry must also be present.
			step0 := spec["steps"].([]any)[0].(map[string]any)
			newEntry := step0["in"].([]any)[1].(string)
			newHash := strings.SplitN(newEntry, ":", 2)[0]
			if _, exists := bm[newHash]; !exists {
				t.Errorf("bundle_map missing entry for newly compiled hash %q", newHash)
			}

			if *uploadCount != 1 {
				t.Errorf("upload count = %d, want 1", *uploadCount)
			}
		})
	}
}

func readTarHeaders(t *testing.T, archiveBytes []byte) map[string]*tar.Header {
	t.Helper()

	gr, err := gzip.NewReader(bytes.NewReader(archiveBytes))
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	headers := make(map[string]*tar.Header)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar next: %v", err)
		}
		copied := *hdr
		headers[hdr.Name] = &copied
	}
	return headers
}
