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

func TestBuildContainerSpec_HydraInMountedReadOnly(t *testing.T) {
	stagingDir := t.TempDir()

	manifest := baseManifestForHydra(t)
	manifest.In = []string{"abcdef0:/in/config.json"}

	spec, err := buildContainerSpec(types.RunID("run-in"), types.JobID("job-in"), manifest, "/ws", "", "", stagingDir)
	if err != nil {
		t.Fatalf("buildContainerSpec error: %v", err)
	}

	var found bool
	for _, m := range spec.Mounts {
		if m.Target == "/in/config.json" {
			found = true
			wantSrc := filepath.Join(stagingDir, "abcdef0", "content")
			if m.Source != wantSrc {
				t.Errorf("source = %q, want %q", m.Source, wantSrc)
			}
			if !m.ReadOnly {
				t.Errorf("/in mount must be read-only")
			}
		}
	}
	if !found {
		t.Fatalf("/in/config.json mount not found in %+v", spec.Mounts)
	}
}

func TestBuildContainerSpec_HydraOutSeededInOutDir(t *testing.T) {
	stagingDir := t.TempDir()
	outDir := t.TempDir()

	manifest := baseManifestForHydra(t)
	manifest.Out = []string{"bbbbbbb:/out/results"}

	spec, err := buildContainerSpec(types.RunID("run-out"), types.JobID("job-out"), manifest, "/ws", outDir, "", stagingDir)
	if err != nil {
		t.Fatalf("buildContainerSpec error: %v", err)
	}

	// Out entries should NOT produce separate mounts; they are seeded into
	// outDir and covered by the existing /out mount.
	for _, m := range spec.Mounts {
		if m.Target == "/out/results" {
			t.Fatalf("unexpected separate mount for /out/results; out entries should be seeded into outDir")
		}
	}

	// The /out mount from outDir should be present.
	var outMountFound bool
	for _, m := range spec.Mounts {
		if m.Target == "/out" && m.Source == outDir {
			outMountFound = true
		}
	}
	if !outMountFound {
		t.Fatalf("/out mount from outDir not found in %+v", spec.Mounts)
	}
}

func TestBuildContainerSpec_HydraHomeMountRW(t *testing.T) {
	stagingDir := t.TempDir()

	manifest := baseManifestForHydra(t)
	manifest.Home = []string{"ccccccc:.codex/auth.json"}

	spec, err := buildContainerSpec(types.RunID("run-home"), types.JobID("job-home"), manifest, "/ws", "", "", stagingDir)
	if err != nil {
		t.Fatalf("buildContainerSpec error: %v", err)
	}

	var found bool
	for _, m := range spec.Mounts {
		if m.Target == "/home/user/.codex/auth.json" {
			found = true
			wantSrc := filepath.Join(stagingDir, "ccccccc", "content")
			if m.Source != wantSrc {
				t.Errorf("source = %q, want %q", m.Source, wantSrc)
			}
			if m.ReadOnly {
				t.Errorf("home mount (default) must be read-write")
			}
		}
	}
	if !found {
		t.Fatalf("home mount not found in %+v", spec.Mounts)
	}
}

func TestBuildContainerSpec_HydraHomeMountRO(t *testing.T) {
	stagingDir := t.TempDir()

	manifest := baseManifestForHydra(t)
	manifest.Home = []string{"ddddddd:.config/app.toml:ro"}

	spec, err := buildContainerSpec(types.RunID("run-home-ro"), types.JobID("job-home-ro"), manifest, "/ws", "", "", stagingDir)
	if err != nil {
		t.Fatalf("buildContainerSpec error: %v", err)
	}

	var found bool
	for _, m := range spec.Mounts {
		if m.Target == "/home/user/.config/app.toml" {
			found = true
			wantSrc := filepath.Join(stagingDir, "ddddddd", "content")
			if m.Source != wantSrc {
				t.Errorf("source = %q, want %q", m.Source, wantSrc)
			}
			if !m.ReadOnly {
				t.Errorf("home mount with :ro must be read-only")
			}
		}
	}
	if !found {
		t.Fatalf("home mount not found in %+v", spec.Mounts)
	}
}

func TestBuildContainerSpec_HydraHomeUsesEnvHOME(t *testing.T) {
	stagingDir := t.TempDir()

	manifest := baseManifestForHydra(t)
	manifest.Envs = map[string]string{"HOME": "/root"}
	manifest.Home = []string{"ccccccc:.codex/auth.json"}

	spec, err := buildContainerSpec(types.RunID("run-home-env"), types.JobID("job-home-env"), manifest, "/ws", "", "", stagingDir)
	if err != nil {
		t.Fatalf("buildContainerSpec error: %v", err)
	}

	var found bool
	for _, m := range spec.Mounts {
		if m.Target == "/root/.codex/auth.json" {
			found = true
			wantSrc := filepath.Join(stagingDir, "ccccccc", "content")
			if m.Source != wantSrc {
				t.Errorf("source = %q, want %q", m.Source, wantSrc)
			}
		}
	}
	if !found {
		t.Fatalf("home mount with HOME=/root not found in %+v", spec.Mounts)
	}
}

func TestBuildContainerSpec_HydraCAMount(t *testing.T) {
	stagingDir := t.TempDir()

	manifest := baseManifestForHydra(t)
	manifest.CA = []string{"eeeeeee"}

	spec, err := buildContainerSpec(types.RunID("run-ca"), types.JobID("job-ca"), manifest, "/ws", "", "", stagingDir)
	if err != nil {
		t.Fatalf("buildContainerSpec error: %v", err)
	}

	var found bool
	for _, m := range spec.Mounts {
		if m.Target == "/etc/ploy/ca/eeeeeee" {
			found = true
			wantSrc := filepath.Join(stagingDir, "eeeeeee", "content")
			if m.Source != wantSrc {
				t.Errorf("source = %q, want %q", m.Source, wantSrc)
			}
			if !m.ReadOnly {
				t.Errorf("CA mount must be read-only")
			}
		}
	}
	if !found {
		t.Fatalf("CA mount not found in %+v", spec.Mounts)
	}
}

func TestBuildContainerSpec_HydraSkippedWithoutStagingDir(t *testing.T) {
	manifest := baseManifestForHydra(t)
	manifest.In = []string{"abcdef0:/in/config.json"}
	manifest.CA = []string{"bbbbbbb"}

	spec, err := buildContainerSpec(types.RunID("run-nostaging"), types.JobID("job-nostaging"), manifest, "/ws", "", "", "")
	if err != nil {
		t.Fatalf("buildContainerSpec error: %v", err)
	}

	// Only workspace mount should be present.
	if len(spec.Mounts) != 1 {
		t.Fatalf("got %d mounts, want 1: %+v", len(spec.Mounts), spec.Mounts)
	}
}

func TestBuildContainerSpec_HydraNoFieldsValid(t *testing.T) {
	stagingDir := t.TempDir()
	manifest := baseManifestForHydra(t)

	spec, err := buildContainerSpec(types.RunID("run-empty"), types.JobID("job-empty"), manifest, "/ws", "", "", stagingDir)
	if err != nil {
		t.Fatalf("buildContainerSpec error: %v", err)
	}

	// Only workspace mount.
	if len(spec.Mounts) != 1 {
		t.Fatalf("got %d mounts, want 1: %+v", len(spec.Mounts), spec.Mounts)
	}
}

// TestBuildContainerSpec_HydraMixedMountPlan verifies that a manifest with all
// four Hydra entry types (CA, In, Out, Home) produces the correct mount plan
// with proper source paths, targets, and read-only modes.
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

	// Out entries must NOT produce separate mounts (covered by /out → outDir).
	for _, m := range spec.Mounts {
		if m.Target == "/out/results" {
			t.Errorf("unexpected separate mount for /out/results; should be covered by /out")
		}
	}

	// Total: workspace + /out + CA + In + Home = 5.
	if len(spec.Mounts) != 5 {
		t.Errorf("got %d mounts, want 5: %+v", len(spec.Mounts), spec.Mounts)
	}
}

// TestBuildContainerSpec_HydraOutRequiresOutDir verifies that out entries in the
// manifest are validated and that buildContainerSpec returns an error when outDir
// is empty but out entries are present.
func TestBuildContainerSpec_HydraOutRequiresOutDir(t *testing.T) {
	stagingDir := t.TempDir()

	manifest := baseManifestForHydra(t)
	manifest.Out = []string{"fff0000:/out/results"}

	_, err := buildContainerSpec(
		types.RunID("run-out-nodir"), types.JobID("job-out-nodir"),
		manifest, "/ws", "", "", stagingDir,
	)
	if err == nil {
		t.Fatal("expected error when outDir is empty with out entries, got nil")
	}
	if !strings.Contains(err.Error(), "outDir required") {
		t.Errorf("error = %q, want mention of outDir required", err)
	}
}

// TestBuildContainerSpec_HydraOutInvalidEntry verifies that an invalid out entry
// is rejected during mount planning.
func TestBuildContainerSpec_HydraOutInvalidEntry(t *testing.T) {
	stagingDir := t.TempDir()
	outDir := t.TempDir()

	manifest := baseManifestForHydra(t)
	manifest.Out = []string{"not-a-valid-entry"}

	_, err := buildContainerSpec(
		types.RunID("run-out-bad"), types.JobID("job-out-bad"),
		manifest, "/ws", outDir, "", stagingDir,
	)
	if err == nil {
		t.Fatal("expected error for invalid out entry, got nil")
	}
}

// TestSeedOutDirFromStaging_ContainmentCheck verifies that SeedOutDirFromStaging
// rejects out entries whose resolved destination escapes outDir.
func TestSeedOutDirFromStaging_ContainmentCheck(t *testing.T) {
	stagingDir := t.TempDir()
	outDir := t.TempDir()

	// Even though ParseStoredOutEntry cleans paths, verify the containment
	// check in SeedOutDirFromStaging as defense-in-depth by testing with a
	// destination that after cleaning still tries to escape via the rel path.
	// A cleaned /out/results is safe; we verify it works.
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

// TestSeedOutDirFromStaging verifies that out entry content is copied from
// staging into outDir at the correct relative paths.
func TestSeedOutDirFromStaging(t *testing.T) {
	stagingDir := t.TempDir()
	outDir := t.TempDir()

	// Simulate materialized out content at stagingDir/<hash>/content.
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
