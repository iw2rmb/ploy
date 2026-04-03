package step

import (
	"path/filepath"
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
			if m.Source != filepath.Join(stagingDir, "abcdef0") {
				t.Errorf("source = %q, want %q", m.Source, filepath.Join(stagingDir, "abcdef0"))
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

func TestBuildContainerSpec_HydraOutMountedReadWrite(t *testing.T) {
	stagingDir := t.TempDir()

	manifest := baseManifestForHydra(t)
	manifest.Out = []string{"bbbbbbb:/out/results"}

	spec, err := buildContainerSpec(types.RunID("run-out"), types.JobID("job-out"), manifest, "/ws", "", "", stagingDir)
	if err != nil {
		t.Fatalf("buildContainerSpec error: %v", err)
	}

	var found bool
	for _, m := range spec.Mounts {
		if m.Target == "/out/results" {
			found = true
			if m.Source != filepath.Join(stagingDir, "bbbbbbb") {
				t.Errorf("source = %q, want %q", m.Source, filepath.Join(stagingDir, "bbbbbbb"))
			}
			if m.ReadOnly {
				t.Errorf("/out mount must be read-write")
			}
		}
	}
	if !found {
		t.Fatalf("/out/results mount not found in %+v", spec.Mounts)
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
			if m.Source != filepath.Join(stagingDir, "ccccccc") {
				t.Errorf("source = %q, want %q", m.Source, filepath.Join(stagingDir, "ccccccc"))
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
			if !m.ReadOnly {
				t.Errorf("home mount with :ro must be read-only")
			}
		}
	}
	if !found {
		t.Fatalf("home mount not found in %+v", spec.Mounts)
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
			if m.Source != filepath.Join(stagingDir, "eeeeeee") {
				t.Errorf("source = %q, want %q", m.Source, filepath.Join(stagingDir, "eeeeeee"))
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
