package step

import (
	"path/filepath"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestBuildContainerSpec_JavaToolCacheMountsFromStackEnv(t *testing.T) {
	cacheRoot := t.TempDir()
	t.Setenv(buildGateCacheRootEnv, cacheRoot)

	tests := []struct {
		name       string
		env        map[string]string
		wantMounts map[string]string
	}{
		{
			name: "java stack mounts both caches",
			env: map[string]string{
				contracts.PLOYStackLanguageEnv: "java",
				contracts.PLOYStackReleaseEnv:  "17",
			},
			wantMounts: map[string]string{
				BuildGateGradleUserHomeDir: filepath.Join(cacheRoot, "java", "gradle", "17"),
				BuildGateMavenUserHomeDir:  filepath.Join(cacheRoot, "java", "maven", "17"),
			},
		},
		{
			name: "non-java stack mounts no java caches",
			env: map[string]string{
				contracts.PLOYStackLanguageEnv: "go",
				contracts.PLOYStackReleaseEnv:  "1.25",
			},
		},
		{
			name: "missing stack env mounts no java caches",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := contracts.StepManifest{
				ID:    types.StepID("step-java-cache"),
				Name:  "Java cache mount",
				Image: "ghcr.io/example/mig:latest",
				Envs:  tt.env,
				Inputs: []contracts.StepInput{{
					Name:        "src",
					MountPath:   "/workspace",
					Mode:        contracts.StepInputModeReadWrite,
					SnapshotCID: types.CID("bafy123"),
				}},
			}

			spec, err := buildContainerSpec(types.RunID("run-java-cache"), types.JobID("job-java-cache"), manifest, "/tmp/ws", "", "", "", "")
			if err != nil {
				t.Fatalf("buildContainerSpec error: %v", err)
			}

			gotMounts := map[string]ContainerMount{}
			for _, mount := range spec.Mounts {
				if mount.Target != BuildGateGradleUserHomeDir && mount.Target != BuildGateMavenUserHomeDir {
					continue
				}
				gotMounts[mount.Target] = mount
			}

			if len(tt.wantMounts) == 0 {
				if len(gotMounts) != 0 {
					t.Fatalf("unexpected java cache mounts: %+v", gotMounts)
				}
				return
			}

			if len(gotMounts) != len(tt.wantMounts) {
				t.Fatalf("java cache mount count=%d, want %d (%+v)", len(gotMounts), len(tt.wantMounts), gotMounts)
			}
			for target, wantSource := range tt.wantMounts {
				gotMount, ok := gotMounts[target]
				if !ok {
					t.Fatalf("missing java cache mount target %q", target)
				}
				if gotMount.Source != wantSource {
					t.Fatalf("cache mount source=%q, want %q", gotMount.Source, wantSource)
				}
				if gotMount.ReadOnly {
					t.Fatalf("cache mount must be writable: %+v", gotMount)
				}
			}
		})
	}
}
