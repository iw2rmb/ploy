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
		image      string
		env        map[string]string
		wantTarget string
		wantSource string
	}{
		{
			name:  "sbom stack gradle mounts gradle lane",
			image: "ghcr.io/iw2rmb/ploy/sbom-gradle:latest",
			env: map[string]string{
				contracts.PLOYStackLanguageEnv: "java",
				contracts.PLOYStackToolEnv:     "gradle",
				contracts.PLOYStackReleaseEnv:  "17",
			},
			wantTarget: BuildGateGradleUserHomeDir,
			wantSource: filepath.Join(cacheRoot, "java", "gradle", "17"),
		},
		{
			name:  "hook stack maven mounts maven lane",
			image: "ghcr.io/example/hook:latest",
			env: map[string]string{
				contracts.PLOYStackLanguageEnv: "java",
				contracts.PLOYStackToolEnv:     "maven",
				contracts.PLOYStackReleaseEnv:  "21",
			},
			wantTarget: BuildGateMavenUserHomeDir,
			wantSource: filepath.Join(cacheRoot, "java", "maven", "21"),
		},
		{
			name:  "mig stack gradle mounts gradle lane",
			image: "ghcr.io/example/mig:latest",
			env: map[string]string{
				contracts.PLOYStackLanguageEnv: "java",
				contracts.PLOYStackToolEnv:     "gradle",
				contracts.PLOYStackReleaseEnv:  "11",
			},
			wantTarget: BuildGateGradleUserHomeDir,
			wantSource: filepath.Join(cacheRoot, "java", "gradle", "11"),
		},
		{
			name:  "heal stack maven mounts maven lane",
			image: "ghcr.io/iw2rmb/ploy/orw-cli-gradle:latest",
			env: map[string]string{
				contracts.PLOYStackLanguageEnv: "java",
				contracts.PLOYStackToolEnv:     "maven",
				contracts.PLOYStackReleaseEnv:  "17",
			},
			wantTarget: BuildGateMavenUserHomeDir,
			wantSource: filepath.Join(cacheRoot, "java", "maven", "17"),
		},
		{
			name:  "missing release uses unknown-release lane",
			image: "ghcr.io/example/mig:latest",
			env: map[string]string{
				contracts.PLOYStackLanguageEnv: "java",
				contracts.PLOYStackToolEnv:     "maven",
			},
			wantTarget: BuildGateMavenUserHomeDir,
			wantSource: filepath.Join(cacheRoot, "java", "maven", "unknown-release"),
		},
		{
			name:  "sbom image without stack env does not mount",
			image: "ghcr.io/iw2rmb/ploy/sbom-gradle:latest",
		},
		{
			name:  "orw image without stack env does not mount",
			image: "ghcr.io/iw2rmb/ploy/orw-cli-maven:latest",
		},
		{
			name:  "non-java stack does not mount",
			image: "ghcr.io/example/mig:latest",
			env: map[string]string{
				contracts.PLOYStackLanguageEnv: "go",
				contracts.PLOYStackToolEnv:     "go",
				contracts.PLOYStackReleaseEnv:  "1.25",
			},
		},
		{
			name:  "unsupported java tool does not mount",
			image: "ghcr.io/example/mig:latest",
			env: map[string]string{
				contracts.PLOYStackLanguageEnv: "java",
				contracts.PLOYStackToolEnv:     "ant",
				contracts.PLOYStackReleaseEnv:  "17",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := contracts.StepManifest{
				ID:    types.StepID("step-java-cache"),
				Name:  "Java cache mount",
				Image: tt.image,
				Envs:  tt.env,
				Inputs: []contracts.StepInput{{
					Name:        "src",
					MountPath:   "/workspace",
					Mode:        contracts.StepInputModeReadWrite,
					SnapshotCID: types.CID("bafy123"),
				}},
			}

			spec, err := buildContainerSpec(types.RunID("run-java-cache"), types.JobID("job-java-cache"), manifest, "/tmp/ws", "", "", "")
			if err != nil {
				t.Fatalf("buildContainerSpec error: %v", err)
			}

			if tt.wantTarget == "" {
				for _, mount := range spec.Mounts {
					if mount.Target == BuildGateGradleUserHomeDir || mount.Target == BuildGateMavenUserHomeDir {
						t.Fatalf("unexpected java cache mount for case %q: %+v", tt.name, mount)
					}
				}
				return
			}

			var found bool
			for _, mount := range spec.Mounts {
				if mount.Target != tt.wantTarget {
					continue
				}
				found = true
				if mount.Source != tt.wantSource {
					t.Fatalf("cache mount source=%q, want %q", mount.Source, tt.wantSource)
				}
				if mount.ReadOnly {
					t.Fatalf("cache mount must be writable: %+v", mount)
				}
			}
			if !found {
				t.Fatalf("expected cache mount %q -> %q in %+v", tt.wantSource, tt.wantTarget, spec.Mounts)
			}
		})
	}
}
