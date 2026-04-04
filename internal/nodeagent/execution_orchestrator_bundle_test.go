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
	"context"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/step"
)

// spyContainerRuntime captures the ContainerSpec from Create for test assertions.
type spyContainerRuntime struct {
	capturedSpec step.ContainerSpec
}

func (s *spyContainerRuntime) Create(_ context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
	s.capturedSpec = spec
	return "spy-handle", nil
}

func (s *spyContainerRuntime) Start(_ context.Context, _ step.ContainerHandle) error {
	return nil
}

func (s *spyContainerRuntime) Wait(_ context.Context, _ step.ContainerHandle) (step.ContainerResult, error) {
	return step.ContainerResult{ExitCode: 0}, nil
}

func (s *spyContainerRuntime) Logs(_ context.Context, _ step.ContainerHandle) ([]byte, error) {
	return nil, nil
}

func (s *spyContainerRuntime) Remove(_ context.Context, _ step.ContainerHandle) error {
	return nil
}

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

// --- extractBundle tests ---


func TestExtractBundle_Valid(t *testing.T) {
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
			if err := extractBundle(data, stagingDir); err != nil {
				t.Fatalf("extractBundle error: %v", err)
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

func TestExtractBundle_RejectsUnsafeEntryType(t *testing.T) {
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
			if err := extractBundle(data, stagingDir); err == nil {
				t.Fatalf("expected error for %s entry, got nil", tt.name)
			}
		})
	}
}

func TestExtractBundle_RejectsUnsafePath(t *testing.T) {
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
			if err := extractBundle(buf.Bytes(), stagingDir); err == nil {
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

// ---------------------------------------------------------------------------
// Mixed in/out/home/ca materialization and mount planning integration
// ---------------------------------------------------------------------------

func TestHydraResources_MixedMaterializationAndMountPlanning(t *testing.T) {
	t.Parallel()

	// Build distinct archives for each entry type, using the "content" root
	// that buildSourceArchive produces.
	caArchive := buildTestTarGz(t, map[string][]byte{
		"content": []byte("-----BEGIN CERTIFICATE-----\nFAKE\n-----END CERTIFICATE-----"),
	}, nil)
	caHash := digestOf(caArchive)[:12]

	inArchive := buildTestTarGz(t, map[string][]byte{
		"content": []byte(`{"config":"value"}`),
	}, nil)
	inHash := digestOf(inArchive)[:12]

	outArchive := buildTestTarGz(t, map[string][]byte{
		"content/":        nil,
		"content/seed.md": []byte("seed content"),
	}, nil)
	outHash := digestOf(outArchive)[:12]

	homeArchive := buildTestTarGz(t, map[string][]byte{
		"content": []byte(`{"auth":"token"}`),
	}, nil)
	homeHash := digestOf(homeArchive)[:12]

	archives := map[string][]byte{
		caHash:   caArchive,
		inHash:   inArchive,
		outHash:  outArchive,
		homeHash: homeArchive,
	}

	bundleMap := map[string]string{
		caHash:   "bun-ca",
		inHash:   "bun-in",
		outHash:  "bun-out",
		homeHash: "bun-home",
	}

	// Serve bundles by bundleID.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := filepath.Base(r.URL.Path)
		for hash, bid := range bundleMap {
			if parts == bid {
				w.Header().Set("Content-Type", "application/gzip")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(archives[hash])
				return
			}
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	manifest := contracts.StepManifest{
		CA:        []string{caHash},
		In:        []string{inHash + ":/in/config.json"},
		Out:       []string{outHash + ":/out/results"},
		Home:      []string{homeHash + ":.auth.json:ro"},
		BundleMap: bundleMap,
	}

	rc := newTestController(t, newAgentConfig(srv.URL))
	stagingDir := t.TempDir()

	// 1. Materialize all 4 entry types in a single call.
	if err := rc.materializeHydraResources(t.Context(), manifest, bundleMap, stagingDir); err != nil {
		t.Fatalf("materializeHydraResources: %v", err)
	}

	// 2. Verify all unique hashes were staged.
	for _, hash := range []string{caHash, inHash, outHash, homeHash} {
		if _, err := os.Stat(filepath.Join(stagingDir, hash)); err != nil {
			t.Errorf("staging dir for hash %s missing: %v", hash, err)
		}
	}

	// 3. Verify staged content at the content subdirectory level.
	gotCA, err := os.ReadFile(filepath.Join(stagingDir, caHash, "content"))
	if err != nil {
		t.Fatalf("read CA content: %v", err)
	}
	if !bytes.Contains(gotCA, []byte("CERTIFICATE")) {
		t.Errorf("CA content missing CERTIFICATE marker: %q", gotCA)
	}

	gotIn, err := os.ReadFile(filepath.Join(stagingDir, inHash, "content"))
	if err != nil {
		t.Fatalf("read In content: %v", err)
	}
	if string(gotIn) != `{"config":"value"}` {
		t.Errorf("In content = %q", gotIn)
	}

	gotOutSeed, err := os.ReadFile(filepath.Join(stagingDir, outHash, "content", "seed.md"))
	if err != nil {
		t.Fatalf("read Out seed content: %v", err)
	}
	if string(gotOutSeed) != "seed content" {
		t.Errorf("Out seed = %q", gotOutSeed)
	}

	gotHome, err := os.ReadFile(filepath.Join(stagingDir, homeHash, "content"))
	if err != nil {
		t.Fatalf("read Home content: %v", err)
	}
	if string(gotHome) != `{"auth":"token"}` {
		t.Errorf("Home content = %q", gotHome)
	}

	// 4. Verify hash deduplication: same hash should not be materialized twice.
	hashes := collectUniqueHashes(manifest)
	seen := make(map[string]bool)
	for _, h := range hashes {
		if seen[h] {
			t.Errorf("duplicate hash %s in collectUniqueHashes result", h)
		}
		seen[h] = true
	}
	if len(hashes) != 4 {
		t.Errorf("expected 4 unique hashes, got %d: %v", len(hashes), hashes)
	}

	// 5. Execute out mount planning via SeedOutDirFromStaging and verify
	//    seeded content at the correct destination-relative path.
	outDir := t.TempDir()
	if err := step.SeedOutDirFromStaging(manifest, stagingDir, outDir); err != nil {
		t.Fatalf("SeedOutDirFromStaging: %v", err)
	}
	seededOut, err := os.ReadFile(filepath.Join(outDir, "results", "seed.md"))
	if err != nil {
		t.Fatalf("seeded out content missing: %v", err)
	}
	if string(seededOut) != "seed content" {
		t.Errorf("seeded out = %q, want %q", seededOut, "seed content")
	}

	// 6. Assert mount source layout for in/ca/home matches buildContainerSpec
	//    expectations: each mount source is stagingDir/<hash>/content.
	type wantMount struct {
		hash     string
		field    string
		target   string
		readOnly bool
	}
	wantMounts := []wantMount{
		{hash: caHash, field: "ca", target: "/etc/ploy/ca/" + caHash, readOnly: true},
		{hash: inHash, field: "in", target: "/in/config.json", readOnly: true},
		{hash: homeHash, field: "home", target: "/home/user/.auth.json", readOnly: true},
	}
	for _, w := range wantMounts {
		contentPath := filepath.Join(stagingDir, w.hash, "content")
		if _, statErr := os.Stat(contentPath); statErr != nil {
			t.Errorf("%s mount source %s missing: %v", w.field, contentPath, statErr)
		}
	}

	// 7. Build a full manifest and run through Runner with spy runtime to
	//    verify mount planning produces correct targets and modes.
	spy := &spyContainerRuntime{}
	fullManifest := contracts.StepManifest{
		ID:    types.StepID("step-integration"),
		Name:  "Mixed Hydra Integration",
		Image: "alpine:3",
		Inputs: []contracts.StepInput{{
			Name:        "src",
			MountPath:   "/workspace",
			Mode:        contracts.StepInputModeReadWrite,
			SnapshotCID: types.CID("bafy-test"),
		}},
		CA:        manifest.CA,
		In:        manifest.In,
		Out:       manifest.Out,
		Home:      manifest.Home,
		BundleMap: bundleMap,
	}

	runner := &step.Runner{Containers: spy}
	outDir2 := t.TempDir()
	_, runErr := runner.Run(t.Context(), step.Request{
		RunID:      types.RunID("run-integration"),
		JobID:      types.JobID("job-integration"),
		Manifest:   fullManifest,
		Workspace:  t.TempDir(),
		OutDir:     outDir2,
		StagingDir: stagingDir,
	})
	if runErr != nil {
		t.Fatalf("Runner.Run: %v", runErr)
	}

	// Verify that SeedOutDirFromStaging was executed during Run.
	seeded2, err := os.ReadFile(filepath.Join(outDir2, "results", "seed.md"))
	if err != nil {
		t.Fatalf("Runner seeded out content missing: %v", err)
	}
	if string(seeded2) != "seed content" {
		t.Errorf("Runner seeded out = %q, want %q", seeded2, "seed content")
	}

	// Assert mount targets and modes from captured container spec.
	for _, w := range wantMounts {
		var found bool
		for _, m := range spy.capturedSpec.Mounts {
			if m.Target == w.target {
				found = true
				wantSrc := filepath.Join(stagingDir, w.hash, "content")
				if m.Source != wantSrc {
					t.Errorf("mount %s (%s): source = %q, want %q", w.target, w.field, m.Source, wantSrc)
				}
				if m.ReadOnly != w.readOnly {
					t.Errorf("mount %s (%s): readOnly = %v, want %v", w.target, w.field, m.ReadOnly, w.readOnly)
				}
			}
		}
		if !found {
			t.Errorf("mount %s (%s) not found in spec mounts: %+v", w.target, w.field, spy.capturedSpec.Mounts)
		}
	}

	// Assert /out mount target and mode from outDir (not staging-based).
	var foundOut bool
	for _, m := range spy.capturedSpec.Mounts {
		if m.Target == "/out" {
			foundOut = true
			if m.Source != outDir2 {
				t.Errorf("mount /out (out): source = %q, want %q", m.Source, outDir2)
			}
			if m.ReadOnly {
				t.Errorf("mount /out (out): readOnly = true, want false")
			}
		}
	}
	if !foundOut {
		t.Errorf("mount /out (out) not found in spec mounts: %+v", spy.capturedSpec.Mounts)
	}
}
