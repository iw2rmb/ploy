package step

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func baseManifestForHydra(t *testing.T) contracts.StepManifest {
	t.Helper()
	return contracts.StepManifest{
		ID:    types.StepID("step-hydra"),
		Name:  "With Hydra Mounts",
		Image: "alpine:3",
		Inputs: []contracts.StepInput{{
			Name:        "src",
			MountPath:   "/workspace",
			Mode:        contracts.StepInputModeReadWrite,
			SnapshotCID: types.CID("bafy123"),
		}},
	}
}

// ---------------------------------------------------------------------------
// Single-type Hydra mount tests (table-driven)
// ---------------------------------------------------------------------------

func TestBuildContainerSpec_HydraSingleMount(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(m *contracts.StepManifest) string // returns outDir if needed
		wantTarget string
		wantSrcSfx string // expected suffix of mount source path
		wantRO     bool
		negTarget  string // mount target that must NOT exist
	}{
		{
			name: "in mounted read-only",
			setup: func(m *contracts.StepManifest) string {
				m.In = []string{"abcdef0:/in/config.json"}
				return ""
			},
			wantTarget: "/in/config.json",
			wantSrcSfx: filepath.Join("abcdef0", "content"),
			wantRO:     true,
		},
		{
			name: "out seeded into outDir not separate mount",
			setup: func(m *contracts.StepManifest) string {
				m.Out = []string{"bbbbbbb:/out/results"}
				return t.TempDir()
			},
			wantTarget: "/out",
			negTarget:  "/out/results",
		},
		{
			name: "home mount default rw",
			setup: func(m *contracts.StepManifest) string {
				m.Home = []string{"ccccccc:.codex/auth.json"}
				return ""
			},
			wantTarget: "/home/user/.codex/auth.json",
			wantSrcSfx: filepath.Join("ccccccc", "content"),
			wantRO:     false,
		},
		{
			name: "home mount with :ro",
			setup: func(m *contracts.StepManifest) string {
				m.Home = []string{"ddddddd:.config/app.toml:ro"}
				return ""
			},
			wantTarget: "/home/user/.config/app.toml",
			wantSrcSfx: filepath.Join("ddddddd", "content"),
			wantRO:     true,
		},
		{
			name: "home uses HOME env override",
			setup: func(m *contracts.StepManifest) string {
				m.Envs = map[string]string{"HOME": "/root"}
				m.Home = []string{"ccccccc:.codex/auth.json"}
				return ""
			},
			wantTarget: "/root/.codex/auth.json",
			wantSrcSfx: filepath.Join("ccccccc", "content"),
		},
		{
			name: "ca mount read-only",
			setup: func(m *contracts.StepManifest) string {
				m.CA = []string{"eeeeeee"}
				return ""
			},
			wantTarget: "/etc/ploy/ca/eeeeeee",
			wantSrcSfx: filepath.Join("eeeeeee", "content"),
			wantRO:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stagingDir := t.TempDir()
			manifest := baseManifestForHydra(t)
			outDir := tt.setup(&manifest)

			spec, err := buildContainerSpec(
				types.RunID("run-"+tt.name), types.JobID("job-"+tt.name),
				manifest, "/ws", outDir, "", stagingDir,
			)
			if err != nil {
				t.Fatalf("buildContainerSpec error: %v", err)
			}

			var found bool
			for _, m := range spec.Mounts {
				if m.Target == tt.wantTarget {
					found = true
					if tt.wantSrcSfx != "" && !strings.HasSuffix(m.Source, tt.wantSrcSfx) {
						t.Errorf("mount %s: source = %q, want suffix %q", tt.wantTarget, m.Source, tt.wantSrcSfx)
					}
					if tt.wantSrcSfx != "" && m.ReadOnly != tt.wantRO {
						t.Errorf("mount %s: readOnly = %v, want %v", tt.wantTarget, m.ReadOnly, tt.wantRO)
					}
				}
			}
			if !found {
				t.Fatalf("mount %s not found in %+v", tt.wantTarget, spec.Mounts)
			}

			if tt.negTarget != "" {
				for _, m := range spec.Mounts {
					if m.Target == tt.negTarget {
						t.Fatalf("unexpected separate mount for %s", tt.negTarget)
					}
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Edge cases (table-driven)
// ---------------------------------------------------------------------------

func TestBuildContainerSpec_HydraEdgeCases(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(m *contracts.StepManifest) (outDir, stagingDir string)
		wantErr    string
		wantMounts int // expected mount count when no error
	}{
		{
			name: "skipped without staging dir",
			setup: func(m *contracts.StepManifest) (string, string) {
				m.In = []string{"abcdef0:/in/config.json"}
				m.CA = []string{"bbbbbbb"}
				return "", ""
			},
			wantMounts: 1,
		},
		{
			name: "no hydra fields valid",
			setup: func(m *contracts.StepManifest) (string, string) {
				return "", t.TempDir()
			},
			wantMounts: 1,
		},
		{
			name: "out requires outDir",
			setup: func(m *contracts.StepManifest) (string, string) {
				m.Out = []string{"fff0000:/out/results"}
				return "", t.TempDir()
			},
			wantErr: "outDir required",
		},
		{
			name: "out invalid entry rejected",
			setup: func(m *contracts.StepManifest) (string, string) {
				m.Out = []string{"not-a-valid-entry"}
				return t.TempDir(), t.TempDir()
			},
			wantErr: "", // any non-nil error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := baseManifestForHydra(t)
			outDir, stagingDir := tt.setup(&manifest)

			spec, err := buildContainerSpec(
				types.RunID("run-edge"), types.JobID("job-edge"),
				manifest, "/ws", outDir, "", stagingDir,
			)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error = %q, want containing %q", err, tt.wantErr)
				}
				return
			}
			if tt.name == "out invalid entry rejected" {
				if err == nil {
					t.Fatal("expected error for invalid out entry, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(spec.Mounts) != tt.wantMounts {
				t.Fatalf("got %d mounts, want %d: %+v", len(spec.Mounts), tt.wantMounts, spec.Mounts)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Mixed mount plan (all four Hydra entry types together)
// ---------------------------------------------------------------------------

func TestBuildContainerSpec_HydraMixedMountPlan(t *testing.T) {
	stagingDir := t.TempDir()
	outDir := t.TempDir()

	manifest := baseManifestForHydra(t)
	manifest.CA = []string{"aaa0000"}
	manifest.In = []string{"bbb1111:/in/data.json"}
	manifest.Out = []string{"ccc2222:/out/results"}
	manifest.Home = []string{"ddd3333:.config/app.toml:ro"}

	spec, err := buildContainerSpec(
		types.RunID("run-mixed"), types.JobID("job-mixed"),
		manifest, "/ws", outDir, "", stagingDir,
	)
	if err != nil {
		t.Fatalf("buildContainerSpec error: %v", err)
	}

	type wantMount struct {
		target   string
		readOnly bool
		source   string
	}
	wants := []wantMount{
		{target: "/workspace", readOnly: false, source: "/ws"},
		{target: "/out", readOnly: false, source: outDir},
		{target: "/etc/ploy/ca/aaa0000", readOnly: true, source: filepath.Join(stagingDir, "aaa0000", "content")},
		{target: "/in/data.json", readOnly: true, source: filepath.Join(stagingDir, "bbb1111", "content")},
		{target: "/home/user/.config/app.toml", readOnly: true, source: filepath.Join(stagingDir, "ddd3333", "content")},
	}

	for _, w := range wants {
		var found bool
		for _, m := range spec.Mounts {
			if m.Target == w.target {
				found = true
				if m.Source != w.source {
					t.Errorf("mount %s: source = %q, want %q", w.target, m.Source, w.source)
				}
				if m.ReadOnly != w.readOnly {
					t.Errorf("mount %s: readOnly = %v, want %v", w.target, m.ReadOnly, w.readOnly)
				}
			}
		}
		if !found {
			t.Errorf("mount %s not found in spec mounts", w.target)
		}
	}

	for _, m := range spec.Mounts {
		if m.Target == "/out/results" {
			t.Errorf("unexpected separate mount for /out/results; should be covered by /out")
		}
	}

	if len(spec.Mounts) != 5 {
		t.Errorf("got %d mounts, want 5: %+v", len(spec.Mounts), spec.Mounts)
	}
}

// ---------------------------------------------------------------------------
// SeedOutDirFromStaging
// ---------------------------------------------------------------------------

func TestSeedOutDirFromStaging_ContainmentCheck(t *testing.T) {
	stagingDir := t.TempDir()
	outDir := t.TempDir()

	hash := "abc0000"
	contentDir := filepath.Join(stagingDir, hash, "content")
	if err := os.MkdirAll(contentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contentDir, "ok.txt"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	manifest := contracts.StepManifest{
		Out: []string{hash + ":/out/results"},
	}

	if err := SeedOutDirFromStaging(manifest, stagingDir, outDir); err != nil {
		t.Fatalf("SeedOutDirFromStaging error: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(outDir, "results", "ok.txt"))
	if err != nil {
		t.Fatalf("seeded content missing: %v", err)
	}
	if string(got) != "data" {
		t.Errorf("content = %q, want %q", got, "data")
	}
}

func TestSeedOutDirFromStaging(t *testing.T) {
	stagingDir := t.TempDir()
	outDir := t.TempDir()

	hash := "abc0000"
	contentDir := filepath.Join(stagingDir, hash, "content")
	if err := os.MkdirAll(contentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contentDir, "result.txt"), []byte("output"), 0o644); err != nil {
		t.Fatal(err)
	}

	manifest := contracts.StepManifest{
		Out: []string{hash + ":/out/data"},
	}

	if err := SeedOutDirFromStaging(manifest, stagingDir, outDir); err != nil {
		t.Fatalf("SeedOutDirFromStaging error: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(outDir, "data", "result.txt"))
	if err != nil {
		t.Fatalf("seeded content missing: %v", err)
	}
	if string(got) != "output" {
		t.Errorf("seeded content = %q, want %q", got, "output")
	}
}

func TestSeedInDirFromStaging(t *testing.T) {
	stagingDir := t.TempDir()
	inDir := t.TempDir()

	fileHash := "abc0001"
	dirHash := "abc0002"

	fileContentPath := filepath.Join(stagingDir, fileHash, "content")
	if err := os.MkdirAll(filepath.Dir(fileContentPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fileContentPath, []byte("yaml"), 0o644); err != nil {
		t.Fatal(err)
	}

	dirContentPath := filepath.Join(stagingDir, dirHash, "content")
	if err := os.MkdirAll(dirContentPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirContentPath, "route.yaml"), []byte("route"), 0o644); err != nil {
		t.Fatal(err)
	}

	manifest := contracts.StepManifest{
		In: []string{
			fileHash + ":/in/amata.yaml",
			dirHash + ":/in/amata",
		},
	}

	if err := SeedInDirFromStaging(manifest, stagingDir, inDir); err != nil {
		t.Fatalf("SeedInDirFromStaging error: %v", err)
	}

	fileData, err := os.ReadFile(filepath.Join(inDir, "amata.yaml"))
	if err != nil {
		t.Fatalf("seeded file content missing: %v", err)
	}
	if string(fileData) != "yaml" {
		t.Errorf("seeded file content = %q, want %q", fileData, "yaml")
	}

	dirData, err := os.ReadFile(filepath.Join(inDir, "amata", "route.yaml"))
	if err != nil {
		t.Fatalf("seeded dir content missing: %v", err)
	}
	if string(dirData) != "route" {
		t.Errorf("seeded dir content = %q, want %q", dirData, "route")
	}
}
