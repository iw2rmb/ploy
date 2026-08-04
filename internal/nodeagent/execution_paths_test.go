package nodeagent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestJobDirectoriesUseUniversalLayout(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	runID := types.NewRunID()
	jobID := types.NewJobID()
	wantRunRoot := filepath.Join(cacheHome, "runs", runID.String())
	if got := runDir(runID); got != wantRunRoot {
		t.Fatalf("runDir() = %q, want %q", got, wantRunRoot)
	}
	if strings.Contains(runDir(runID), filepath.Join("ploy", "run")) {
		t.Fatalf("run cache path uses old ploy/run layout: %s", runDir(runID))
	}
	if got := workspaceDir(runID); got != filepath.Join(wantRunRoot, "workspace") {
		t.Fatalf("workspaceDir() = %q", got)
	}
	if got := runShareDir(runID); got != filepath.Join(wantRunRoot, "share") {
		t.Fatalf("runShareDir() = %q", got)
	}
	if got := runRuntimeShareDir(runID); got != filepath.Join(wantRunRoot, "runtime-share") {
		t.Fatalf("runRuntimeShareDir() = %q", got)
	}

	dirs := jobDirectories(runID, jobID)
	wantJobRoot := filepath.Join(wantRunRoot, "jobs", jobID.String())
	if dirs.Root != wantJobRoot {
		t.Fatalf("job root = %q, want %q", dirs.Root, wantJobRoot)
	}
	want := map[string]string{
		"cache":                  dirs.Cache,
		"home":                   dirs.Home,
		"in":                     dirs.In,
		"out":                    dirs.Out,
		"staging":                dirs.Staging,
		"tmp":                    dirs.Tmp,
		"stdout.log":             dirs.Stdout,
		"stderr.log":             dirs.Stderr,
		"diff.patch":             dirs.Diff,
		"container.inspect.json": dirs.ContainerInspect,
	}
	for name, got := range want {
		if got != filepath.Join(wantJobRoot, name) {
			t.Errorf("job path %s = %q", name, got)
		}
	}
}

func TestJobMountsUseConfiguredGenericNodeRoots(t *testing.T) {
	cacheHome := t.TempDir()
	nodeCache := filepath.Join(t.TempDir(), "node-cache")
	nodeConfig := filepath.Join(t.TempDir(), "job-config")
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)
	t.Setenv(nodeCacheRootEnv, nodeCache)
	t.Setenv(nodeJobConfigRootEnv, nodeConfig)

	runID := types.RunID("run_mounts")
	jobID := types.JobID("job_mounts")
	dirs := jobDirectories(runID, jobID)
	got, err := jobMounts(dirs, runID, types.JobTypePostGate)
	if err != nil {
		t.Fatalf("jobMounts() error = %v", err)
	}

	if got.Cache != dirs.Cache || got.Home != dirs.Home || got.In != dirs.In || got.Out != dirs.Out || got.Staging != dirs.Staging || got.Tmp != dirs.Tmp {
		t.Fatalf("jobMounts() job paths do not match JobDirectories: %+v", got)
	}
	if got.Share != runShareDir(runID) || got.RuntimeShare != runRuntimeShareDir(runID) {
		t.Fatalf("jobMounts() run paths = share %q runtime %q", got.Share, got.RuntimeShare)
	}
	if got.NodeCache != nodeCache {
		t.Fatalf("jobMounts().NodeCache = %q, want %q", got.NodeCache, nodeCache)
	}
	if got.CommonConfig != filepath.Join(nodeConfig, "common") {
		t.Fatalf("jobMounts().CommonConfig = %q", got.CommonConfig)
	}
	if got.JobConfig != filepath.Join(nodeConfig, "post_gate") {
		t.Fatalf("jobMounts().JobConfig = %q", got.JobConfig)
	}
	if got.JobType != types.JobTypePostGate {
		t.Fatalf("jobMounts().JobType = %q", got.JobType)
	}
}

func TestJobMountsRequireGenericNodeRoots(t *testing.T) {
	tests := []struct {
		name    string
		missing string
	}{
		{name: "node cache", missing: nodeCacheRootEnv},
		{name: "node job config", missing: nodeJobConfigRootEnv},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(nodeCacheRootEnv, filepath.Join(t.TempDir(), "node-cache"))
			t.Setenv(nodeJobConfigRootEnv, filepath.Join(t.TempDir(), "job-config"))
			t.Setenv(test.missing, "")
			_, err := jobMounts(jobDirectories(types.RunID("run_mounts"), types.JobID("job_mounts")), types.RunID("run_mounts"), types.JobTypeMig)
			if err == nil || !strings.Contains(err.Error(), test.missing+" is required") {
				t.Fatalf("jobMounts() error = %v, want missing %s", err, test.missing)
			}
		})
	}
}

func TestConcurrentJobsHaveDistinctOwnedDirectories(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	runID := types.NewRunID()
	first := jobDirectories(runID, types.NewJobID())
	second := jobDirectories(runID, types.NewJobID())
	for name, paths := range map[string][2]string{
		"cache":   {first.Cache, second.Cache},
		"home":    {first.Home, second.Home},
		"in":      {first.In, second.In},
		"out":     {first.Out, second.Out},
		"staging": {first.Staging, second.Staging},
		"tmp":     {first.Tmp, second.Tmp},
	} {
		if paths[0] == paths[1] {
			t.Errorf("concurrent jobs share %s directory %q", name, paths[0])
		}
	}
}

func TestEnsureAndCleanupJobDirectoriesPreserveDurableArtifacts(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)

	runID := types.NewRunID()
	dirs := jobDirectories(runID, types.NewJobID())
	if err := ensureRunDirectories(runID); err != nil {
		t.Fatalf("ensureRunDirectories() error = %v", err)
	}
	if err := ensureJobDirectories(dirs); err != nil {
		t.Fatalf("ensureJobDirectories() error = %v", err)
	}
	for _, dir := range []string{dirs.Cache, dirs.Home, dirs.In, dirs.Out, dirs.Staging, dirs.Tmp, runShareDir(runID), runRuntimeShareDir(runID)} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("expected directory %s, info=%v err=%v", dir, info, err)
		}
	}

	for _, path := range []string{
		filepath.Join(dirs.Cache, "cache.bin"),
		filepath.Join(dirs.Home, "home.txt"),
		filepath.Join(dirs.Staging, "staged.bin"),
		filepath.Join(dirs.Tmp, "tmp.bin"),
		filepath.Join(dirs.In, "input.txt"),
		filepath.Join(dirs.Out, "output.txt"),
		dirs.Stdout,
		dirs.Stderr,
		dirs.Diff,
		dirs.ContainerInspect,
	} {
		if err := os.WriteFile(path, []byte("data"), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	if err := cleanupJobRuntime(dirs); err != nil {
		t.Fatalf("cleanupJobRuntime() error = %v", err)
	}
	for _, dir := range []string{dirs.Cache, dirs.Home, dirs.Staging, dirs.Tmp} {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("runtime directory remains after cleanup: %s", dir)
		}
	}
	for _, path := range []string{dirs.In, dirs.Out, dirs.Stdout, dirs.Stderr, dirs.Diff, dirs.ContainerInspect} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("durable artifact was removed: %s: %v", path, err)
		}
	}
}
