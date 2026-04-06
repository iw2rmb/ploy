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

func TestDockerGateExecutor_MountsDockerSocketForUnixDockerHost(t *testing.T) {
	t.Parallel()

	executor, rt, _ := newDockerGateTestHarness(t)

	socketDir := t.TempDir()
	socketPath := filepath.Join(socketDir, "docker.sock")
	if err := os.WriteFile(socketPath, []byte("mock socket"), 0o600); err != nil {
		t.Fatalf("write docker socket placeholder: %v", err)
	}

	workspace := createMavenWorkspace(t, "17")
	spec := &contracts.StepGateSpec{
		Enabled: true,
		GateProfile: &contracts.BuildGateProfileOverride{
			Command: contracts.CommandSpec{Shell: "echo prep-gate"},
			Env: map[string]string{
				"DOCKER_HOST": "unix://" + socketPath,
			},
		},
	}

	_, err := executor.Execute(context.Background(), spec, workspace)
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}
	if !rt.createCalled {
		t.Fatal("expected Create to be called")
	}

	found := false
	for _, mount := range rt.captured.Mounts {
		if mount.Source == socketPath && mount.Target == socketPath {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected docker socket mount for %q, got mounts=%+v", socketPath, rt.captured.Mounts)
	}
}

func TestDockerGateExecutor_DoesNotMountDockerSocketForTCPDockerHost(t *testing.T) {
	t.Parallel()

	executor, rt, workspace := newDockerGateTestHarness(t)
	spec := &contracts.StepGateSpec{
		Enabled: true,
		GateProfile: &contracts.BuildGateProfileOverride{
			Command: contracts.CommandSpec{Shell: "echo prep-gate"},
			Env: map[string]string{
				"DOCKER_HOST": "tcp://prep-dind:2375",
			},
		},
	}

	_, err := executor.Execute(context.Background(), spec, workspace)
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}
	if !rt.createCalled {
		t.Fatal("expected Create to be called")
	}
	for _, mount := range rt.captured.Mounts {
		if strings.Contains(mount.Target, "docker.sock") {
			t.Fatalf("expected no docker socket mounts for tcp docker host, got mounts=%+v", rt.captured.Mounts)
		}
	}
}

func TestDockerGateExecutor_MountsOutDir(t *testing.T) {
	t.Parallel()

	executor, rt, workspace := newDockerGateTestHarness(t)
	spec := &contracts.StepGateSpec{Enabled: true}

	_, err := executor.Execute(context.Background(), spec, workspace)
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}
	if !rt.createCalled {
		t.Fatal("expected Create to be called")
	}

	wantSource := filepath.Join(workspace, BuildGateWorkspaceOutDir)
	found := false
	for _, mount := range rt.captured.Mounts {
		if mount.Source == wantSource && mount.Target == BuildGateContainerOutDir && !mount.ReadOnly {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected /out mount %q -> %q in mounts=%+v", wantSource, BuildGateContainerOutDir, rt.captured.Mounts)
	}
}

func TestDockerGateExecutor_MountsGradleNativeCacheDir(t *testing.T) {
	t.Parallel()

	rt := &testContainerRuntime{}
	executor := NewDockerGateExecutor(rt)
	workspace := createGradleWorkspace(t, "17")
	spec := &contracts.StepGateSpec{Enabled: true}

	_, err := executor.Execute(context.Background(), spec, workspace)
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}
	if !rt.createCalled {
		t.Fatal("expected Create to be called")
	}

	cacheRoot, err := resolveBuildGateCacheRoot()
	if err != nil {
		t.Fatalf("resolveBuildGateCacheRoot() error: %v", err)
	}
	wantSource := filepath.Join(cacheRoot, "java", "gradle", "17")
	found := false
	for _, mount := range rt.captured.Mounts {
		if mount.Source == wantSource && mount.Target == BuildGateGradleUserHomeDir && !mount.ReadOnly {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected gradle cache mount %q -> %q in mounts=%+v", wantSource, BuildGateGradleUserHomeDir, rt.captured.Mounts)
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

func TestDockerGateExecutor_MountsMavenNativeCacheDir(t *testing.T) {
	t.Parallel()

	rt := &testContainerRuntime{}
	executor := NewDockerGateExecutor(rt)
	workspace := createMavenWorkspace(t, "17")
	spec := &contracts.StepGateSpec{Enabled: true}

	_, err := executor.Execute(context.Background(), spec, workspace)
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}
	if !rt.createCalled {
		t.Fatal("expected Create to be called")
	}

	cacheRoot, err := resolveBuildGateCacheRoot()
	if err != nil {
		t.Fatalf("resolveBuildGateCacheRoot() error: %v", err)
	}
	wantSource := filepath.Join(cacheRoot, "java", "maven", "17")
	found := false
	for _, mount := range rt.captured.Mounts {
		if mount.Source == wantSource && mount.Target == BuildGateMavenUserHomeDir && !mount.ReadOnly {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected maven cache mount %q -> %q in mounts=%+v", wantSource, BuildGateMavenUserHomeDir, rt.captured.Mounts)
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
		tc := tc
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

func TestDockerGateExecutor_GradleCommandOmitsFailFast(t *testing.T) {
	rt := &testContainerRuntime{}
	executor := NewDockerGateExecutor(rt)

	tmpDir := createGradleWorkspace(t, "17")

	spec := &contracts.StepGateSpec{
		Enabled: true,
	}

	if _, err := executor.Execute(context.Background(), spec, tmpDir); err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}

	if !rt.createCalled {
		t.Fatal("expected Create to be called")
	}
	if len(rt.captured.Command) != 3 {
		t.Fatalf("expected 3-element command, got %v", rt.captured.Command)
	}

	cmd := rt.captured.Command[2]
	if !strings.Contains(cmd, "gradle -q --stacktrace --build-cache") {
		t.Fatalf("expected gradle command with -q --stacktrace --build-cache, got %q", cmd)
	}
	if strings.Contains(cmd, "--fail-fast") {
		t.Fatalf("expected gradle command not to contain --fail-fast, got %q", cmd)
	}
	if !strings.Contains(cmd, "test -p /workspace") {
		t.Fatalf("expected gradle command to run tests in /workspace, got %q", cmd)
	}
}

func TestDockerGateExecutor_GradleCommandUsesWrapperWhenSpecified(t *testing.T) {
	rt := &testContainerRuntime{}
	executor := NewDockerGateExecutor(rt)

	tmpDir := createGradleWorkspaceWithWrapper(t, "17")

	spec := &contracts.StepGateSpec{
		Enabled: true,
	}

	if _, err := executor.Execute(context.Background(), spec, tmpDir); err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}

	if !rt.createCalled {
		t.Fatal("expected Create to be called")
	}
	if len(rt.captured.Command) != 3 {
		t.Fatalf("expected 3-element command, got %v", rt.captured.Command)
	}

	cmd := rt.captured.Command[2]
	if !strings.Contains(cmd, "./gradlew -q --stacktrace --build-cache") {
		t.Fatalf("expected gradle wrapper command with -q --stacktrace --build-cache, got %q", cmd)
	}
	if strings.Contains(cmd, "--fail-fast") {
		t.Fatalf("expected gradle command not to contain --fail-fast, got %q", cmd)
	}
	if !strings.Contains(cmd, "test -p /workspace") {
		t.Fatalf("expected gradle command to run tests in /workspace, got %q", cmd)
	}
}
