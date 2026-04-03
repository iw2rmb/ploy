package main

import (
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
		{input: "/tmp/data.txt:/in/data.txt", wantSrc: "/tmp/data.txt", wantDst: "/in/data.txt"},
		{input: "relative/path:/in/nested/file.json", wantSrc: "relative/path", wantDst: "/in/nested/file.json"},
		{input: "/tmp/data.txt:/out/data.txt", wantErr: "destination must start with /in/"},
		{input: "/tmp/data.txt:data.txt", wantErr: "destination must start with /in/"},
		{input: "/tmp/data.txt:/in/../etc/passwd", wantErr: "path traversal not allowed"},
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
		{input: "/tmp/out.txt:/out/result.txt", wantSrc: "/tmp/out.txt", wantDst: "/out/result.txt"},
		{input: "/tmp/out.txt:/in/result.txt", wantErr: "destination must start with /out/"},
		{input: "/tmp/out.txt:/out/../etc/shadow", wantErr: "path traversal not allowed"},
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
		input      string
		wantSrc    string
		wantDst    string
		wantRO     bool
		wantErr    string
	}{
		{input: "/tmp/cfg:.config/app.toml", wantSrc: "/tmp/cfg", wantDst: ".config/app.toml"},
		{input: "/tmp/cfg:.config/app.toml:ro", wantSrc: "/tmp/cfg", wantDst: ".config/app.toml", wantRO: true},
		{input: "/tmp/cfg:/absolute/path", wantErr: "destination must be relative"},
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
		// ca: bare short hash → canonical
		{field: "ca", entry: "abcdef1234ab", want: true},
		{field: "ca", entry: "abcdef1234abcdef1234abcdef1234abcdef1234abcdef1234abcdef1234abcd", want: true},
		// ca: file path → not canonical
		{field: "ca", entry: "/tmp/cert.pem", want: false},
		{field: "ca", entry: "relative/cert.pem", want: false},
		// in: shortHash:/in/dst → canonical
		{field: "in", entry: "abcdef1234ab:/in/config.txt", want: true},
		// in: file path → not canonical
		{field: "in", entry: "/tmp/data.txt:/in/data.txt", want: false},
		// out: shortHash:/out/dst → canonical
		{field: "out", entry: "abcdef1234ab:/out/result.txt", want: true},
		// home: shortHash:dst → canonical
		{field: "home", entry: "abcdef1234ab:.config/app.toml", want: true},
		// home: shortHash:dst:ro → canonical (first segment before : is hash)
		{field: "home", entry: "abcdef1234ab:.config/app.toml:ro", want: true},
		// Empty or no colon
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
		{
			name:  "no hydra fields",
			block: map[string]any{"image": "test:latest"},
			want:  false,
		},
		{
			name:  "ca with file path",
			block: map[string]any{"ca": []any{"/tmp/cert.pem"}},
			want:  true,
		},
		{
			name:  "ca already canonical",
			block: map[string]any{"ca": []any{"abcdef1234ab"}},
			want:  false,
		},
		{
			name:  "in with authoring entry",
			block: map[string]any{"in": []any{"/tmp/data.txt:/in/data.txt"}},
			want:  true,
		},
		{
			name:  "in already canonical",
			block: map[string]any{"in": []any{"abcdef1234ab:/in/data.txt"}},
			want:  false,
		},
		{
			name:  "mixed canonical and authoring",
			block: map[string]any{"in": []any{"abcdef1234ab:/in/a.txt", "/tmp/b.txt:/in/b.txt"}},
			want:  true,
		},
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
	if err := os.WriteFile(f1, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f2, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

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
		if err := os.WriteFile(filepath.Join(d, "a.txt"), []byte("alpha"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "b.txt"), []byte("beta"), 0o644); err != nil {
			t.Fatal(err)
		}
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

func TestComputeArchiveShortHash(t *testing.T) {
	t.Parallel()
	data := []byte("test data for hashing")
	hash := computeArchiveShortHash(data)
	if len(hash) != shortHashLen {
		t.Errorf("short hash length = %d, want %d", len(hash), shortHashLen)
	}

	// Verify it matches expected SHA256 prefix.
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
	// seenCIDs maps CID → bundleID for probe dedup and X-Bundle-ID header.
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

// ---------------------------------------------------------------------------
// Compile Hydra records integration
// ---------------------------------------------------------------------------

func TestCompileHydraRecordsInPlace_CAEntries(t *testing.T) {
	_, base, client, uploadCount := newMockBundleServer(t)

	tmpDir := t.TempDir()
	certFile := filepath.Join(tmpDir, "cert.pem")
	if err := os.WriteFile(certFile, []byte("-----BEGIN CERTIFICATE-----\ntest\n-----END CERTIFICATE-----\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	spec := map[string]any{
		"steps": []any{
			map[string]any{
				"image": "docker.io/test/mig:latest",
				"ca":    []any{certFile},
			},
		},
	}

	if err := compileHydraRecordsInPlace(context.Background(), base, client, spec, tmpDir); err != nil {
		t.Fatalf("compileHydraRecordsInPlace: %v", err)
	}

	steps := spec["steps"].([]any)
	step0 := steps[0].(map[string]any)
	ca := step0["ca"].([]any)
	if len(ca) != 1 {
		t.Fatalf("expected 1 ca entry, got %d", len(ca))
	}
	hash, ok := ca[0].(string)
	if !ok {
		t.Fatalf("expected ca[0] to be string, got %T", ca[0])
	}
	if !shortHashPattern.MatchString(hash) {
		t.Errorf("ca[0] = %q, expected short hash pattern", hash)
	}
	if len(hash) != shortHashLen {
		t.Errorf("ca[0] hash length = %d, want %d", len(hash), shortHashLen)
	}
	if *uploadCount != 1 {
		t.Errorf("upload count = %d, want 1", *uploadCount)
	}
}

func TestCompileHydraRecordsInPlace_InEntries(t *testing.T) {
	_, base, client, _ := newMockBundleServer(t)

	tmpDir := t.TempDir()
	dataFile := filepath.Join(tmpDir, "config.json")
	if err := os.WriteFile(dataFile, []byte(`{"key": "value"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	spec := map[string]any{
		"steps": []any{
			map[string]any{
				"image": "docker.io/test/mig:latest",
				"in":    []any{dataFile + ":/in/config.json"},
			},
		},
	}

	if err := compileHydraRecordsInPlace(context.Background(), base, client, spec, tmpDir); err != nil {
		t.Fatalf("compileHydraRecordsInPlace: %v", err)
	}

	step0 := spec["steps"].([]any)[0].(map[string]any)
	in := step0["in"].([]any)
	if len(in) != 1 {
		t.Fatalf("expected 1 in entry, got %d", len(in))
	}
	entry := in[0].(string)
	if !strings.HasSuffix(entry, ":/in/config.json") {
		t.Errorf("in[0] = %q, expected suffix :/in/config.json", entry)
	}
	// Verify the hash prefix.
	parts := strings.SplitN(entry, ":", 2)
	if !shortHashPattern.MatchString(parts[0]) {
		t.Errorf("in[0] hash segment = %q, expected short hash", parts[0])
	}
}

func TestCompileHydraRecordsInPlace_OutEntries(t *testing.T) {
	_, base, client, _ := newMockBundleServer(t)

	tmpDir := t.TempDir()
	outFile := filepath.Join(tmpDir, "template.txt")
	if err := os.WriteFile(outFile, []byte("output template"), 0o644); err != nil {
		t.Fatal(err)
	}

	spec := map[string]any{
		"steps": []any{
			map[string]any{
				"image": "docker.io/test/mig:latest",
				"out":   []any{outFile + ":/out/result.txt"},
			},
		},
	}

	if err := compileHydraRecordsInPlace(context.Background(), base, client, spec, tmpDir); err != nil {
		t.Fatalf("compileHydraRecordsInPlace: %v", err)
	}

	step0 := spec["steps"].([]any)[0].(map[string]any)
	out := step0["out"].([]any)
	entry := out[0].(string)
	if !strings.HasSuffix(entry, ":/out/result.txt") {
		t.Errorf("out[0] = %q, expected suffix :/out/result.txt", entry)
	}
}

func TestCompileHydraRecordsInPlace_HomeEntries(t *testing.T) {
	_, base, client, _ := newMockBundleServer(t)

	tmpDir := t.TempDir()
	homeFile := filepath.Join(tmpDir, "config.toml")
	if err := os.WriteFile(homeFile, []byte("[app]\nkey = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	spec := map[string]any{
		"steps": []any{
			map[string]any{
				"image": "docker.io/test/mig:latest",
				"home":  []any{homeFile + ":.config/app.toml:ro"},
			},
		},
	}

	if err := compileHydraRecordsInPlace(context.Background(), base, client, spec, tmpDir); err != nil {
		t.Fatalf("compileHydraRecordsInPlace: %v", err)
	}

	step0 := spec["steps"].([]any)[0].(map[string]any)
	home := step0["home"].([]any)
	entry := home[0].(string)
	if !strings.HasSuffix(entry, ":.config/app.toml:ro") {
		t.Errorf("home[0] = %q, expected suffix :.config/app.toml:ro", entry)
	}
	// Verify hash prefix.
	parts := strings.SplitN(entry, ":", 2)
	if !shortHashPattern.MatchString(parts[0]) {
		t.Errorf("home[0] hash segment = %q, expected short hash", parts[0])
	}
}

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

	if err := compileHydraRecordsInPlace(context.Background(), base, client, spec, ""); err != nil {
		t.Fatalf("compileHydraRecordsInPlace: %v", err)
	}

	// No uploads should happen for already-canonical entries.
	if *uploadCount != 0 {
		t.Errorf("upload count = %d, want 0 (all entries canonical)", *uploadCount)
	}

	step0 := spec["steps"].([]any)[0].(map[string]any)
	ca := step0["ca"].([]any)
	if ca[0] != "abcdef1234ab" {
		t.Errorf("ca[0] changed from canonical: %v", ca[0])
	}
}

func TestCompileHydraRecordsInPlace_NilWhenNoEntries(t *testing.T) {
	t.Parallel()
	spec := map[string]any{
		"steps": []any{
			map[string]any{
				"image": "docker.io/test/mig:latest",
			},
		},
	}

	// Should return nil without needing base/client.
	if err := compileHydraRecordsInPlace(context.Background(), nil, nil, spec, ""); err != nil {
		t.Fatalf("compileHydraRecordsInPlace: %v", err)
	}
}

func TestCompileHydraRecordsInPlace_ErrorsWithoutServer(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	certFile := filepath.Join(tmpDir, "cert.pem")
	if err := os.WriteFile(certFile, []byte("cert"), 0o644); err != nil {
		t.Fatal(err)
	}

	spec := map[string]any{
		"steps": []any{
			map[string]any{
				"image": "docker.io/test/mig:latest",
				"ca":    []any{certFile},
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

func TestCompileHydraRecordsInPlace_BuildGateRouter(t *testing.T) {
	_, base, client, _ := newMockBundleServer(t)

	tmpDir := t.TempDir()
	routerCA := filepath.Join(tmpDir, "router-ca.pem")
	if err := os.WriteFile(routerCA, []byte("router-cert"), 0o644); err != nil {
		t.Fatal(err)
	}

	spec := map[string]any{
		"steps": []any{
			map[string]any{"image": "docker.io/test/mig:latest"},
		},
		"build_gate": map[string]any{
			"router": map[string]any{
				"image": "docker.io/test/router:latest",
				"ca":    []any{routerCA},
			},
		},
	}

	if err := compileHydraRecordsInPlace(context.Background(), base, client, spec, tmpDir); err != nil {
		t.Fatalf("compileHydraRecordsInPlace: %v", err)
	}

	router := spec["build_gate"].(map[string]any)["router"].(map[string]any)
	ca := router["ca"].([]any)
	hash := ca[0].(string)
	if !shortHashPattern.MatchString(hash) {
		t.Errorf("router ca[0] = %q, expected short hash", hash)
	}
}

func TestCompileHydraRecordsInPlace_BuildGateHealing(t *testing.T) {
	_, base, client, _ := newMockBundleServer(t)

	tmpDir := t.TempDir()
	healingIn := filepath.Join(tmpDir, "healing-config.json")
	if err := os.WriteFile(healingIn, []byte(`{"mode":"auto"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	spec := map[string]any{
		"steps": []any{
			map[string]any{"image": "docker.io/test/mig:latest"},
		},
		"build_gate": map[string]any{
			"healing": map[string]any{
				"by_error_kind": map[string]any{
					"infra": map[string]any{
						"image": "docker.io/test/healer:latest",
						"in":    []any{healingIn + ":/in/healing-config.json"},
					},
				},
			},
		},
	}

	if err := compileHydraRecordsInPlace(context.Background(), base, client, spec, tmpDir); err != nil {
		t.Fatalf("compileHydraRecordsInPlace: %v", err)
	}

	infra := spec["build_gate"].(map[string]any)["healing"].(map[string]any)["by_error_kind"].(map[string]any)["infra"].(map[string]any)
	in := infra["in"].([]any)
	entry := in[0].(string)
	if !strings.Contains(entry, ":/in/healing-config.json") {
		t.Errorf("healing in[0] = %q, expected to contain :/in/healing-config.json", entry)
	}
}

// ---------------------------------------------------------------------------
// Upload deduplication: identical content should produce same hash
// ---------------------------------------------------------------------------

func TestCompileHydraRecordsInPlace_UploadDedup(t *testing.T) {
	_, base, client, uploadCount := newMockBundleServer(t)

	tmpDir := t.TempDir()
	file1 := filepath.Join(tmpDir, "a.txt")
	file2 := filepath.Join(tmpDir, "b.txt")
	// Identical content in both files.
	if err := os.WriteFile(file1, []byte("same-content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file2, []byte("same-content"), 0o644); err != nil {
		t.Fatal(err)
	}

	spec := map[string]any{
		"steps": []any{
			map[string]any{
				"image": "docker.io/test/mig:latest",
				"in": []any{
					file1 + ":/in/a.txt",
					file2 + ":/in/b.txt",
				},
			},
		},
	}

	if err := compileHydraRecordsInPlace(context.Background(), base, client, spec, tmpDir); err != nil {
		t.Fatalf("compileHydraRecordsInPlace: %v", err)
	}

	step0 := spec["steps"].([]any)[0].(map[string]any)
	in := step0["in"].([]any)
	entry1 := in[0].(string)
	entry2 := in[1].(string)

	// Both should have the same hash prefix (identical content).
	hash1 := strings.SplitN(entry1, ":", 2)[0]
	hash2 := strings.SplitN(entry2, ":", 2)[0]
	if hash1 != hash2 {
		t.Errorf("identical content produced different hashes: %s vs %s", hash1, hash2)
	}

	// In-process cache: identical content within one compile pass triggers only 1 upload.
	if *uploadCount != 1 {
		t.Errorf("upload count = %d, want 1 (identical content deduped in-process)", *uploadCount)
	}
}

func TestCompileHydraRecordsInPlace_RepeatedRunSkipsUpload(t *testing.T) {
	_, base, client, uploadCount := newMockBundleServer(t)

	tmpDir := t.TempDir()
	certFile := filepath.Join(tmpDir, "cert.pem")
	if err := os.WriteFile(certFile, []byte("repeated-run-cert"), 0o644); err != nil {
		t.Fatal(err)
	}

	makeSpec := func() map[string]any {
		return map[string]any{
			"steps": []any{
				map[string]any{
					"image": "docker.io/test/mig:latest",
					"ca":    []any{certFile},
				},
			},
		}
	}

	// First compile: probe misses, uploads.
	spec1 := makeSpec()
	if err := compileHydraRecordsInPlace(context.Background(), base, client, spec1, tmpDir); err != nil {
		t.Fatalf("first compile: %v", err)
	}
	if *uploadCount != 1 {
		t.Fatalf("after first compile: upload count = %d, want 1", *uploadCount)
	}
	hash1 := spec1["steps"].([]any)[0].(map[string]any)["ca"].([]any)[0].(string)

	// Second compile (simulates repeated run): probe hits, no new upload.
	spec2 := makeSpec()
	if err := compileHydraRecordsInPlace(context.Background(), base, client, spec2, tmpDir); err != nil {
		t.Fatalf("second compile: %v", err)
	}
	if *uploadCount != 1 {
		t.Errorf("after second compile: upload count = %d, want 1 (repeated run should skip upload)", *uploadCount)
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
	certFile := filepath.Join(tmpDir, "ca.pem")
	inFile := filepath.Join(tmpDir, "config.json")
	outFile := filepath.Join(tmpDir, "seed.txt")
	homeFile := filepath.Join(tmpDir, "auth.json")

	if err := os.WriteFile(certFile, []byte("ca-cert"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inFile, []byte("in-data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outFile, []byte("out-data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(homeFile, []byte("home-data"), 0o644); err != nil {
		t.Fatal(err)
	}

	spec := map[string]any{
		"steps": []any{
			map[string]any{
				"image": "alpine:3",
				"ca":    []any{certFile},
				"in":    []any{inFile + ":/in/config.json"},
				"out":   []any{outFile + ":/out/seed.txt"},
				"home":  []any{homeFile + ":.auth.json"},
			},
		},
	}

	if err := compileHydraRecordsInPlace(context.Background(), base, client, spec, tmpDir); err != nil {
		t.Fatalf("compileHydraRecordsInPlace: %v", err)
	}

	bm, ok := spec["bundle_map"]
	if !ok {
		t.Fatal("bundle_map not emitted into spec")
	}
	bundleMap, ok := bm.(map[string]string)
	if !ok {
		t.Fatalf("bundle_map type = %T, want map[string]string", bm)
	}

	step0 := spec["steps"].([]any)[0].(map[string]any)
	// Collect all shortHashes used in canonical entries.
	caHash := step0["ca"].([]any)[0].(string)
	inHash := strings.SplitN(step0["in"].([]any)[0].(string), ":", 2)[0]
	outHash := strings.SplitN(step0["out"].([]any)[0].(string), ":", 2)[0]
	homeHash := strings.SplitN(step0["home"].([]any)[0].(string), ":", 2)[0]

	for _, h := range []string{caHash, inHash, outHash, homeHash} {
		if _, exists := bundleMap[h]; !exists {
			t.Errorf("bundle_map missing entry for hash %q", h)
		}
	}

	// Every bundleID should be non-empty.
	for hash, bundleID := range bundleMap {
		if bundleID == "" {
			t.Errorf("bundle_map[%q] is empty", hash)
		}
	}
}
