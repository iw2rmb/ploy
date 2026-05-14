package step

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	units "github.com/docker/go-units"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// TestDockerGateExecutor_Mounts consolidates the mount-shape assertions for the
// docker gate executor: each row builds a workspace, spec, and context, then
// expects either a specific mount to be present or a specific target absent.
func TestDockerGateExecutor_Mounts(t *testing.T) {
	type expectMount struct {
		source   string
		target   string
		readOnly bool
	}
	tests := []struct {
		name string
		// build returns the workspace path, spec, ctx, expected mount (may be zero), and an
		// optional substring whose presence in any mount target should fail the test.
		build       func(t *testing.T) (workspace string, spec *contracts.StepGateSpec, ctx context.Context, want expectMount, absentTargetSubstr string)
		expectMount bool
	}{
		{
			name: "unix docker host mounts socket",
			build: func(t *testing.T) (string, *contracts.StepGateSpec, context.Context, expectMount, string) {
				socketDir := t.TempDir()
				socketPath := filepath.Join(socketDir, "docker.sock")
				if err := os.WriteFile(socketPath, []byte("mock socket"), 0o600); err != nil {
					t.Fatalf("write docker socket placeholder: %v", err)
				}
				spec := &contracts.StepGateSpec{
					Enabled: true,
					Env:     map[string]string{"DOCKER_HOST": "unix://" + socketPath},
				}
				return createMavenWorkspace(t, "17"), spec, context.Background(),
					expectMount{source: socketPath, target: socketPath, readOnly: false}, ""
			},
			expectMount: true,
		},
		{
			name: "tcp docker host omits socket",
			build: func(t *testing.T) (string, *contracts.StepGateSpec, context.Context, expectMount, string) {
				spec := &contracts.StepGateSpec{
					Enabled: true,
					Env:     map[string]string{"DOCKER_HOST": "tcp://prep-dind:2375"},
				}
				return createMavenWorkspace(t, "17"), spec, context.Background(),
					expectMount{}, "docker.sock"
			},
		},
		{
			name: "out dir mounted writable",
			build: func(t *testing.T) (string, *contracts.StepGateSpec, context.Context, expectMount, string) {
				workspace := createMavenWorkspace(t, "17")
				return workspace, &contracts.StepGateSpec{Enabled: true}, context.Background(),
					expectMount{source: filepath.Join(workspace, BuildGateWorkspaceOutDir), target: BuildGateContainerOutDir}, ""
			},
			expectMount: true,
		},
		{
			name: "in dir mounted when present",
			build: func(t *testing.T) (string, *contracts.StepGateSpec, context.Context, expectMount, string) {
				workspace := createMavenWorkspace(t, "17")
				inDir := filepath.Join(workspace, BuildGateWorkspaceInDir)
				if err := os.MkdirAll(inDir, 0o755); err != nil {
					t.Fatalf("MkdirAll(%q): %v", inDir, err)
				}
				return workspace, &contracts.StepGateSpec{Enabled: true}, context.Background(),
					expectMount{source: inDir, target: BuildGateContainerInDir}, ""
			},
			expectMount: true,
		},
		{
			name: "share dir mounted when provided via context",
			build: func(t *testing.T) (string, *contracts.StepGateSpec, context.Context, expectMount, string) {
				shareDir := t.TempDir()
				return createMavenWorkspace(t, "17"), &contracts.StepGateSpec{Enabled: true},
					WithGateShareDir(context.Background(), shareDir),
					expectMount{source: shareDir, target: containerShareDir}, ""
			},
			expectMount: true,
		},
		{
			name: "gradle workspace mounts native cache",
			build: func(t *testing.T) (string, *contracts.StepGateSpec, context.Context, expectMount, string) {
				cacheRoot, err := resolveBuildGateCacheRoot()
				if err != nil {
					t.Fatalf("resolveBuildGateCacheRoot() error: %v", err)
				}
				return createGradleWorkspace(t, "17"), &contracts.StepGateSpec{Enabled: true}, context.Background(),
					expectMount{source: filepath.Join(cacheRoot, "java", "gradle", "17"), target: BuildGateGradleUserHomeDir}, ""
			},
			expectMount: true,
		},
		{
			name: "maven workspace mounts native cache",
			build: func(t *testing.T) (string, *contracts.StepGateSpec, context.Context, expectMount, string) {
				cacheRoot, err := resolveBuildGateCacheRoot()
				if err != nil {
					t.Fatalf("resolveBuildGateCacheRoot() error: %v", err)
				}
				return createMavenWorkspace(t, "17"), &contracts.StepGateSpec{Enabled: true}, context.Background(),
					expectMount{source: filepath.Join(cacheRoot, "java", "maven", "17"), target: BuildGateMavenUserHomeDir}, ""
			},
			expectMount: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rt := &testContainerRuntime{}
			executor := NewDockerGateExecutor(rt)
			workspace, spec, ctx, want, absent := tt.build(t)

			if _, err := executor.Execute(ctx, spec, workspace); err != nil {
				t.Fatalf("Execute() unexpected error: %v", err)
			}
			if !rt.createCalled {
				t.Fatal("expected Create to be called")
			}

			if tt.expectMount {
				requireMount(t, rt.captured.Mounts, want.target, want.source, want.readOnly)
			}
			if absent != "" {
				for _, m := range rt.captured.Mounts {
					if strings.Contains(m.Target, absent) {
						t.Fatalf("unexpected mount target containing %q: %+v", absent, rt.captured.Mounts)
					}
				}
			}
		})
	}
}

func TestResolveBuildGateCacheRoot_UsesOverrideEnv(t *testing.T) {
	override := filepath.Join(t.TempDir(), "gate-cache")
	t.Setenv(buildGateCacheRootEnv, override)
	got, err := resolveBuildGateCacheRoot()
	if err != nil {
		t.Fatalf("resolveBuildGateCacheRoot() error: %v", err)
	}
	if got != override {
		t.Fatalf("resolveBuildGateCacheRoot()=%q, want %q", got, override)
	}
	info, err := os.Stat(override)
	if err != nil {
		t.Fatalf("expected override dir to be created: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("expected override path to be a directory: %q", override)
	}
}

func TestDockerGateExecutor_LimitEnvParsing(t *testing.T) {
	memHuman, err := units.RAMInBytes("1GiB")
	if err != nil {
		t.Fatalf("RAMInBytes(1GiB) error: %v", err)
	}
	diskHuman, err := units.RAMInBytes("5GiB")
	if err != nil {
		t.Fatalf("RAMInBytes(5GiB) error: %v", err)
	}

	testCases := []struct {
		name       string
		memEnv     string
		cpuEnv     string
		diskEnv    string
		wantMem    int64
		wantNano   int64
		wantDisk   int64
		wantDiskOp string
	}{
		{
			name:       "numeric_limits",
			memEnv:     "2048",
			cpuEnv:     "250",
			diskEnv:    "4096",
			wantMem:    2048,
			wantNano:   250 * 1_000_000,
			wantDisk:   4096,
			wantDiskOp: "4096",
		},
		{
			name:       "human_size_limits",
			memEnv:     "1GiB",
			cpuEnv:     "500",
			diskEnv:    "5GiB",
			wantMem:    memHuman,
			wantNano:   500 * 1_000_000,
			wantDisk:   diskHuman,
			wantDiskOp: "5GiB",
		},
		{
			name:       "invalid_values_fall_back_to_zero",
			memEnv:     "not-a-size",
			cpuEnv:     "not-a-number",
			diskEnv:    "bad-size",
			wantMem:    0,
			wantNano:   0,
			wantDisk:   0,
			wantDiskOp: "bad-size",
		},
		{
			name:       "integer_fallback_for_bytes",
			memEnv:     "-512",
			cpuEnv:     "100",
			diskEnv:    "-1234",
			wantMem:    -512,
			wantNano:   100 * 1_000_000,
			wantDisk:   -1234,
			wantDiskOp: "-1234",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(buildGateLimitMemoryEnv, tc.memEnv)
			t.Setenv(buildGateLimitCPUEnv, tc.cpuEnv)
			t.Setenv(buildGateLimitDiskEnv, tc.diskEnv)

			rt := &testContainerRuntime{}
			executor := NewDockerGateExecutor(rt)
			workspace := createMavenWorkspace(t, "17")

			spec := &contracts.StepGateSpec{Enabled: true}
			if _, err := executor.Execute(context.Background(), spec, workspace); err != nil {
				t.Fatalf("Execute() unexpected error: %v", err)
			}
			if !rt.createCalled {
				t.Fatal("expected Create to be called")
			}

			if rt.captured.LimitMemoryBytes != tc.wantMem {
				t.Fatalf("LimitMemoryBytes=%d, want %d", rt.captured.LimitMemoryBytes, tc.wantMem)
			}
			if rt.captured.LimitNanoCPUs != tc.wantNano {
				t.Fatalf("LimitNanoCPUs=%d, want %d", rt.captured.LimitNanoCPUs, tc.wantNano)
			}
			if rt.captured.LimitDiskBytes != tc.wantDisk {
				t.Fatalf("LimitDiskBytes=%d, want %d", rt.captured.LimitDiskBytes, tc.wantDisk)
			}
			if rt.captured.StorageSizeOpt != tc.wantDiskOp {
				t.Fatalf("StorageSizeOpt=%q, want %q", rt.captured.StorageSizeOpt, tc.wantDiskOp)
			}
		})
	}
}
