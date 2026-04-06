package step

import (
	"path/filepath"
	"strings"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestBuildContainerSpec_MountsORWCacheForStandaloneImages(t *testing.T) {
	cacheRoot := t.TempDir()
	t.Setenv(orwCacheRootEnv, cacheRoot)

	tests := []struct {
		name  string
		image string
	}{
		{name: "maven lane", image: "ghcr.io/iw2rmb/ploy/orw-cli-maven:latest"},
		{name: "gradle lane", image: "ghcr.io/iw2rmb/ploy/orw-cli-gradle:latest"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := contracts.StepManifest{
				ID:    types.StepID("step-orw-cache"),
				Name:  "ORW cache mount",
				Image: tt.image,
				Inputs: []contracts.StepInput{{
					Name:        "src",
					MountPath:   "/workspace",
					Mode:        contracts.StepInputModeReadWrite,
					SnapshotCID: types.CID("bafy123"),
				}},
			}

			spec, err := buildContainerSpec(types.RunID("run-orw"), types.JobID("job-orw"), manifest, "/tmp/ws", "", "", "")
			if err != nil {
				t.Fatalf("buildContainerSpec error: %v", err)
			}

			var found bool
			for _, mount := range spec.Mounts {
				if mount.Target != orwMavenUserHomeDir {
					continue
				}
				found = true
				if !strings.HasPrefix(mount.Source, filepath.Clean(cacheRoot)+string(filepath.Separator)) {
					t.Fatalf("orw cache mount source=%q, want under %q", mount.Source, cacheRoot)
				}
				if mount.ReadOnly {
					t.Fatalf("orw cache mount must be writable: %+v", mount)
				}
			}
			if !found {
				t.Fatalf("expected ORW cache mount to %q in %+v", orwMavenUserHomeDir, spec.Mounts)
			}
		})
	}
}

func TestBuildContainerSpec_DoesNotMountORWCacheForNonORWImage(t *testing.T) {
	cacheRoot := t.TempDir()
	t.Setenv(orwCacheRootEnv, cacheRoot)

	manifest := contracts.StepManifest{
		ID:    types.StepID("step-non-orw-cache"),
		Name:  "No ORW cache mount",
		Image: "alpine:3.20",
		Inputs: []contracts.StepInput{{
			Name:        "src",
			MountPath:   "/workspace",
			Mode:        contracts.StepInputModeReadWrite,
			SnapshotCID: types.CID("bafy123"),
		}},
	}

	spec, err := buildContainerSpec(types.RunID("run-non-orw"), types.JobID("job-non-orw"), manifest, "/tmp/ws", "", "", "")
	if err != nil {
		t.Fatalf("buildContainerSpec error: %v", err)
	}

	for _, mount := range spec.Mounts {
		if mount.Target == orwMavenUserHomeDir {
			t.Fatalf("unexpected ORW cache mount for non-ORW image: %+v", mount)
		}
	}
}
