package step

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
		setup      func(m *contracts.StepManifest) (outDir, tmpDir string)
		wantTarget string
		wantSrcSfx string // expected suffix of mount source path
		wantRO     bool
		negTarget  string // mount target that must NOT exist
	}{
		{
			name: "in mounted read-only",
			setup: func(m *contracts.StepManifest) (string, string) {
				m.In = []string{"abcdef0:/in/config.json"}
				return "", ""
			},
			wantTarget: "/in/config.json",
			wantSrcSfx: filepath.Join("abcdef0", "content"),
			wantRO:     true,
		},
		{
			name: "out seeded into outDir not separate mount",
			setup: func(m *contracts.StepManifest) (string, string) {
				m.Out = []string{"bbbbbbb:/out/results"}
				return t.TempDir(), ""
			},
			wantTarget: "/out",
			negTarget:  "/out/results",
		},
		{
			name: "home mount default rw",
			setup: func(m *contracts.StepManifest) (string, string) {
				m.Home = []string{"ccccccc:.codex/auth.json"}
				return "", ""
			},
			wantTarget: "/root/.codex/auth.json",
			wantSrcSfx: filepath.Join("ccccccc", "content"),
			wantRO:     false,
		},
		{
			name: "home mount with :ro",
			setup: func(m *contracts.StepManifest) (string, string) {
				m.Home = []string{"ddddddd:.config/app.toml:ro"}
				return "", ""
			},
			wantTarget: "/root/.config/app.toml",
			wantSrcSfx: filepath.Join("ddddddd", "content"),
			wantRO:     true,
		},
		{
			name: "home uses HOME env override",
			setup: func(m *contracts.StepManifest) (string, string) {
				m.Envs = map[string]string{"HOME": "/root"}
				m.Home = []string{"ccccccc:.codex/auth.json"}
				return "", ""
			},
			wantTarget: "/root/.codex/auth.json",
			wantSrcSfx: filepath.Join("ccccccc", "content"),
		},
		{
			name: "tmp uses single writable mount",
			setup: func(m *contracts.StepManifest) (string, string) {
				m.Tmp = []string{"eeeeeee:/tmp/ploy/lib.jar"}
				return "", t.TempDir()
			},
			wantTarget: "/tmp",
			wantRO:     false,
			negTarget:  "/tmp/ploy/lib.jar",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stagingDir := t.TempDir()
			manifest := baseManifestForHydra(t)
			outDir, tmpDir := tt.setup(&manifest)

			spec, err := buildContainerSpec(
				types.RunID("run-"+tt.name), types.JobID("job-"+tt.name),
				manifest, "/ws", outDir, "", "", tmpDir, stagingDir,
			)
			if err != nil {
				t.Fatalf("buildContainerSpec error: %v", err)
			}

			m, ok := findMount(spec.Mounts, tt.wantTarget)
			if !ok {
				t.Fatalf("mount %s not found in %+v", tt.wantTarget, spec.Mounts)
			}
			if tt.wantSrcSfx != "" {
				if !strings.HasSuffix(m.Source, tt.wantSrcSfx) {
					t.Errorf("mount %s: source = %q, want suffix %q", tt.wantTarget, m.Source, tt.wantSrcSfx)
				}
				if m.ReadOnly != tt.wantRO {
					t.Errorf("mount %s: readOnly = %v, want %v", tt.wantTarget, m.ReadOnly, tt.wantRO)
				}
			} else if m.ReadOnly != tt.wantRO {
				t.Errorf("mount %s: readOnly = %v, want %v", tt.wantTarget, m.ReadOnly, tt.wantRO)
			}

			if tt.negTarget != "" {
				requireNoMount(t, spec.Mounts, tt.negTarget)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Edge cases (table-driven)
// ---------------------------------------------------------------------------

func TestBuildContainerSpec_HydraEdgeCases(t *testing.T) {
	tests := []struct {
		name            string
		setup           func(m *contracts.StepManifest) (outDir, tmpDir, stagingDir string)
		wantErr         bool
		wantErrContains string // substring; empty means "any error" when wantErr is true
		wantMounts      int    // expected mount count when no error
	}{
		{
			name: "skipped without staging dir",
			setup: func(m *contracts.StepManifest) (string, string, string) {
				m.In = []string{"abcdef0:/in/config.json"}
				return "", "", ""
			},
			wantMounts: 1,
		},
		{
			name: "no hydra fields valid",
			setup: func(m *contracts.StepManifest) (string, string, string) {
				return "", "", t.TempDir()
			},
			wantMounts: 1,
		},
		{
			name: "out requires outDir",
			setup: func(m *contracts.StepManifest) (string, string, string) {
				m.Out = []string{"fff0000:/out/results"}
				return "", "", t.TempDir()
			},
			wantErr:         true,
			wantErrContains: "outDir required",
		},
		{
			name: "out invalid entry rejected",
			setup: func(m *contracts.StepManifest) (string, string, string) {
				m.Out = []string{"not-a-valid-entry"}
				return t.TempDir(), "", t.TempDir()
			},
			wantErr: true,
		},
		{
			name: "tmp requires tmpDir",
			setup: func(m *contracts.StepManifest) (string, string, string) {
				m.Tmp = []string{"abcdef0:/tmp/tool.jar"}
				return "", "", t.TempDir()
			},
			wantErr:         true,
			wantErrContains: "tmpDir required",
		},
		{
			name: "tmp invalid entry rejected",
			setup: func(m *contracts.StepManifest) (string, string, string) {
				m.Tmp = []string{"abcdef0:/var/tmp/tool.jar"}
				return "", t.TempDir(), t.TempDir()
			},
			wantErr:         true,
			wantErrContains: "destination must start with /tmp/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := baseManifestForHydra(t)
			outDir, tmpDir, stagingDir := tt.setup(&manifest)

			spec, err := buildContainerSpec(
				types.RunID("run-edge"), types.JobID("job-edge"),
				manifest, "/ws", outDir, "", "", tmpDir, stagingDir,
			)

			if tt.wantErr {
				requireErrContains(t, err, tt.wantErrContains)
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
// Mixed mount plan (all three Hydra entry types together)
// ---------------------------------------------------------------------------

func TestBuildContainerSpec_HydraMixedMountPlan(t *testing.T) {
	stagingDir := t.TempDir()
	outDir := t.TempDir()
	tmpDir := t.TempDir()

	manifest := baseManifestForHydra(t)
	manifest.In = []string{"bbb1111:/in/data.json"}
	manifest.Out = []string{"ccc2222:/out/results"}
	manifest.Home = []string{"ddd3333:.config/app.toml:ro"}
	manifest.Tmp = []string{"eee4444:/tmp/ploy/tool.jar"}

	spec, err := buildContainerSpec(
		types.RunID("run-mixed"), types.JobID("job-mixed"),
		manifest, "/ws", outDir, "", "", tmpDir, stagingDir,
	)
	if err != nil {
		t.Fatalf("buildContainerSpec error: %v", err)
	}

	requireMount(t, spec.Mounts, "/workspace", "/ws", false)
	requireMount(t, spec.Mounts, "/out", outDir, false)
	requireMount(t, spec.Mounts, "/tmp", tmpDir, false)
	requireMount(t, spec.Mounts, "/in/data.json", filepath.Join(stagingDir, "bbb1111", "content"), true)
	requireMount(t, spec.Mounts, "/root/.config/app.toml", filepath.Join(stagingDir, "ddd3333", "content"), true)
	requireNoMount(t, spec.Mounts, "/out/results")
	requireNoMount(t, spec.Mounts, "/tmp/ploy/tool.jar")

	if len(spec.Mounts) != 5 {
		t.Errorf("got %d mounts, want 5: %+v", len(spec.Mounts), spec.Mounts)
	}
}

// ---------------------------------------------------------------------------
// SeedOutDirFromStaging
// ---------------------------------------------------------------------------

func TestSeedOutDirFromStaging(t *testing.T) {
	tests := []struct {
		name     string
		hash     string
		fileName string
		body     string
		outEntry string // value placed into manifest.Out (without hash prefix)
		wantRel  string // path under outDir where content should appear
	}{
		{
			name:     "nested target path",
			hash:     "abc0000",
			fileName: "ok.txt",
			body:     "data",
			outEntry: "/out/results",
			wantRel:  filepath.Join("results", "ok.txt"),
		},
		{
			name:     "flat target path",
			hash:     "abc0000",
			fileName: "result.txt",
			body:     "output",
			outEntry: "/out/data",
			wantRel:  filepath.Join("data", "result.txt"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stagingDir := t.TempDir()
			outDir := t.TempDir()

			contentDir := filepath.Join(stagingDir, tt.hash, "content")
			if err := os.MkdirAll(contentDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(contentDir, tt.fileName), []byte(tt.body), 0o644); err != nil {
				t.Fatal(err)
			}

			manifest := contracts.StepManifest{Out: []string{tt.hash + ":" + tt.outEntry}}
			if err := SeedOutDirFromStaging(manifest, stagingDir, outDir); err != nil {
				t.Fatalf("SeedOutDirFromStaging error: %v", err)
			}

			got, err := os.ReadFile(filepath.Join(outDir, tt.wantRel))
			if err != nil {
				t.Fatalf("seeded content missing: %v", err)
			}
			if string(got) != tt.body {
				t.Errorf("content = %q, want %q", got, tt.body)
			}
		})
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
	if err := os.Chmod(fileContentPath, 0o751); err != nil {
		t.Fatal(err)
	}
	fileModTime := time.Unix(1_704_444_444, 0).UTC()
	if err := os.Chtimes(fileContentPath, fileModTime, fileModTime); err != nil {
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
	fileInfo, err := os.Stat(filepath.Join(inDir, "amata.yaml"))
	if err != nil {
		t.Fatalf("stat seeded file: %v", err)
	}
	if got, want := fileInfo.Mode().Perm(), os.FileMode(0o751); got != want {
		t.Fatalf("seeded file mode = %o, want %o", got, want)
	}
	if !fileInfo.ModTime().UTC().Equal(fileModTime) {
		t.Fatalf("seeded file modtime = %s, want %s", fileInfo.ModTime().UTC(), fileModTime)
	}

	dirData, err := os.ReadFile(filepath.Join(inDir, "amata", "route.yaml"))
	if err != nil {
		t.Fatalf("seeded dir content missing: %v", err)
	}
	if string(dirData) != "route" {
		t.Errorf("seeded dir content = %q, want %q", dirData, "route")
	}
}

func TestSeedTmpDirFromStaging(t *testing.T) {
	stagingDir := t.TempDir()
	tmpDir := t.TempDir()

	fileHash := "abc0003"
	contentPath := filepath.Join(stagingDir, fileHash, "content")
	if err := os.MkdirAll(filepath.Dir(contentPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(contentPath, []byte("jar"), 0o644); err != nil {
		t.Fatal(err)
	}

	manifest := contracts.StepManifest{
		Tmp: []string{fileHash + ":/tmp/ploy/lib/tool.jar"},
	}

	if err := SeedTmpDirFromStaging(manifest, stagingDir, tmpDir); err != nil {
		t.Fatalf("SeedTmpDirFromStaging error: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(tmpDir, "ploy", "lib", "tool.jar"))
	if err != nil {
		t.Fatalf("seeded tmp content missing: %v", err)
	}
	if string(got) != "jar" {
		t.Errorf("seeded tmp content = %q, want %q", got, "jar")
	}
}
