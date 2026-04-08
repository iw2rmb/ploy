package step

import (
	"path/filepath"
	"strings"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestBuildContainerSpec_MountsSBOMGradleCache(t *testing.T) {
	cacheRoot := t.TempDir()
	t.Setenv("PLOY_BUILDGATE_CACHE_ROOT", cacheRoot)

	manifest := contracts.StepManifest{
		ID:    types.StepID("step-sbom-cache"),
		Name:  "SBOM Gradle cache mount",
		Image: "ghcr.io/iw2rmb/ploy/sbom-gradle:latest",
		Inputs: []contracts.StepInput{{
			Name:        "src",
			MountPath:   "/workspace",
			Mode:        contracts.StepInputModeReadWrite,
			SnapshotCID: types.CID("bafy123"),
		}},
	}

	spec, err := buildContainerSpec(types.RunID("run-sbom"), types.JobID("job-sbom"), manifest, "/tmp/ws", "", "", "")
	if err != nil {
		t.Fatalf("buildContainerSpec error: %v", err)
	}

	var found bool
	wantSource := filepath.Join(cacheRoot, "java", "gradle", "17")
	for _, mount := range spec.Mounts {
		if mount.Target != BuildGateGradleUserHomeDir {
			continue
		}
		found = true
		if mount.Source != wantSource {
			t.Fatalf("sbom cache mount source=%q, want %q", mount.Source, wantSource)
		}
		if mount.ReadOnly {
			t.Fatalf("sbom cache mount must be writable: %+v", mount)
		}
	}
	if !found {
		t.Fatalf("expected sbom cache mount to %q in %+v", BuildGateGradleUserHomeDir, spec.Mounts)
	}
}

func TestBuildContainerSpec_DoesNotMountSBOMGradleCacheForOtherImages(t *testing.T) {
	cacheRoot := t.TempDir()
	t.Setenv("PLOY_BUILDGATE_CACHE_ROOT", cacheRoot)

	manifest := contracts.StepManifest{
		ID:    types.StepID("step-sbom-cache-off"),
		Name:  "No SBOM Gradle cache mount",
		Image: "ghcr.io/iw2rmb/ploy/sbom-maven:latest",
		Inputs: []contracts.StepInput{{
			Name:        "src",
			MountPath:   "/workspace",
			Mode:        contracts.StepInputModeReadWrite,
			SnapshotCID: types.CID("bafy123"),
		}},
	}

	spec, err := buildContainerSpec(types.RunID("run-sbom-off"), types.JobID("job-sbom-off"), manifest, "/tmp/ws", "", "", "")
	if err != nil {
		t.Fatalf("buildContainerSpec error: %v", err)
	}

	for _, mount := range spec.Mounts {
		if mount.Target == BuildGateGradleUserHomeDir && strings.Contains(strings.ToLower(mount.Source), "gradle") {
			t.Fatalf("unexpected sbom-gradle cache mount for non-gradle sbom image: %+v", mount)
		}
	}
}
