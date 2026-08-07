package nodeagent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/step"
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

func TestConcurrentSameImageJobsUseDistinctRuntimeStorage(t *testing.T) {
	cacheHome := t.TempDir()
	nodeCache := filepath.Join(t.TempDir(), "node-cache")
	nodeConfig := filepath.Join(t.TempDir(), "job-config")
	t.Setenv("PLOYD_CACHE_HOME", cacheHome)
	t.Setenv(nodeCacheRootEnv, nodeCache)
	t.Setenv(nodeJobConfigRootEnv, nodeConfig)

	runID := types.NewRunID()
	if err := ensureRunDirectories(runID); err != nil {
		t.Fatalf("ensureRunDirectories() error = %v", err)
	}

	type runtimeJob struct {
		id        types.JobID
		dirs      JobDirectories
		mounts    step.JobMounts
		workspace string
	}
	jobs := make([]runtimeJob, 0, 2)
	for range 2 {
		jobID := types.NewJobID()
		dirs := jobDirectories(runID, jobID)
		if err := ensureJobDirectories(dirs); err != nil {
			t.Fatalf("ensureJobDirectories() error = %v", err)
		}
		mounts, err := jobMounts(dirs, runID, types.JobTypeMig)
		if err != nil {
			t.Fatalf("jobMounts() error = %v", err)
		}
		jobs = append(jobs, runtimeJob{id: jobID, dirs: dirs, mounts: mounts, workspace: t.TempDir()})
	}

	specs := make(chan step.ContainerSpec, len(jobs))
	release := make(chan struct{})
	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, spec step.ContainerSpec) (step.ContainerHandle, error) {
			specs <- spec
			return step.ContainerHandle(spec.Labels[types.LabelJobID]), nil
		},
		waitFn: func(ctx context.Context, handle step.ContainerHandle) (step.ContainerResult, error) {
			select {
			case <-release:
				return step.ContainerResult{ContainerID: string(handle)}, nil
			case <-ctx.Done():
				return step.ContainerResult{}, ctx.Err()
			}
		},
	}
	runner := &step.Runner{Containers: runtime}
	manifest := contracts.StepManifest{
		ID:      "same-image-job",
		Image:   "example/same-image:latest",
		Command: []string{"true"},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	errCh := make(chan error, len(jobs))
	for _, job := range jobs {
		job := job
		go func() {
			_, err := runner.Run(ctx, step.Request{
				RunID:     runID,
				JobID:     job.id,
				Manifest:  manifest,
				Workspace: job.workspace,
				JobMounts: job.mounts,
			})
			errCh <- err
		}()
	}

	captured := make([]step.ContainerSpec, 0, len(jobs))
	for range jobs {
		select {
		case spec := <-specs:
			captured = append(captured, spec)
		case <-ctx.Done():
			t.Fatal("concurrent jobs did not both reach container creation")
		}
	}
	close(release)
	for range jobs {
		if err := <-errCh; err != nil {
			t.Fatalf("Runner.Run() error = %v", err)
		}
	}

	expected := make(map[string]JobDirectories, len(jobs))
	for _, job := range jobs {
		expected[job.id.String()] = job.dirs
	}
	sources := map[string][]string{"cache": nil, "home": nil, "tmp": nil}
	for _, spec := range captured {
		if spec.Image != manifest.Image {
			t.Fatalf("container image = %q, want %q", spec.Image, manifest.Image)
		}
		dirs, ok := expected[spec.Labels[types.LabelJobID]]
		if !ok {
			t.Fatalf("container job label = %q, want one of the concurrent jobs", spec.Labels[types.LabelJobID])
		}
		wantMounts := map[string]struct {
			target string
			source string
		}{
			"cache": {target: "/ploy/cache/job", source: dirs.Cache},
			"home":  {target: "/root", source: dirs.Home},
			"tmp":   {target: "/tmp", source: dirs.Tmp},
		}
		for name, want := range wantMounts {
			var got string
			for _, mount := range spec.Mounts {
				if mount.Target == want.target {
					got = mount.Source
					break
				}
			}
			if got != want.source {
				t.Fatalf("%s mount source = %q, want %q", name, got, want.source)
			}
			sources[name] = append(sources[name], got)
		}
	}
	for name, mounted := range sources {
		if len(mounted) != 2 || mounted[0] == mounted[1] {
			t.Errorf("concurrent jobs share %s mount source: %v", name, mounted)
		}
	}
	if jobs[0].mounts.Staging == jobs[1].mounts.Staging {
		t.Errorf("concurrent jobs share staging source %q", jobs[0].mounts.Staging)
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
