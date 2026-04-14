package nodeagent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/domain/types"
	containertypes "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/system"
	"github.com/moby/moby/client"
)

type fakeFreeBytes struct {
	values       []int64
	valuesByPath map[string][]int64
	errAt        int
	err          error
	calls        int
	paths        []string
	callsByPath  map[string]int
}

func (f *fakeFreeBytes) read(path string) (int64, error) {
	f.calls++
	f.paths = append(f.paths, path)
	if f.errAt > 0 && f.calls == f.errAt {
		return 0, f.err
	}
	if f.callsByPath == nil {
		f.callsByPath = map[string]int{}
	}
	if seq, ok := f.valuesByPath[path]; ok {
		if len(seq) == 0 {
			return 0, nil
		}
		idx := f.callsByPath[path]
		f.callsByPath[path] = idx + 1
		if idx >= len(seq) {
			idx = len(seq) - 1
		}
		return seq[idx], nil
	}
	if len(f.values) == 0 {
		return 0, nil
	}
	idx := f.calls - 1
	if idx >= len(f.values) {
		idx = len(f.values) - 1
	}
	return f.values[idx], nil
}

func TestDockerPreClaimCleanup_EnoughCapacitySkipsCleanup(t *testing.T) {
	t.Parallel()

	workspaceRoot := t.TempDir()
	fakeDocker := &fakeDockerClient{
		infoResult: client.SystemInfoResult{Info: system.Info{DockerRootDir: "/var/lib/docker"}},
	}
	fb := &fakeFreeBytes{
		valuesByPath: map[string][]int64{
			"/var/lib/docker": {minDockerFreeBytes + 1},
			workspaceRoot:     {minDockerFreeBytes + 1},
		},
	}

	cleanup := &dockerPreClaimCleanup{docker: fakeDocker, freeBytes: fb.read, workspaceRoot: workspaceRoot}

	ok, err := cleanup.EnsureCapacity(context.Background())
	if err != nil {
		t.Fatalf("EnsureCapacity() error = %v", err)
	}
	if !ok {
		t.Fatal("EnsureCapacity() ok = false, want true")
	}
	if fakeDocker.listCalls != 0 {
		t.Fatalf("ContainerList calls = %d, want 0", fakeDocker.listCalls)
	}
	if len(fakeDocker.removedIDs) != 0 {
		t.Fatalf("removed containers = %v, want none", fakeDocker.removedIDs)
	}
}

func TestDockerPreClaimCleanup_FiltersAndRemovesFIFO(t *testing.T) {
	t.Parallel()

	low := minDockerFreeBytes - 1
	workspaceRoot := t.TempDir()
	fakeDocker := &fakeDockerClient{
		infoResult: client.SystemInfoResult{Info: system.Info{DockerRootDir: "/var/lib/docker"}},
		listResult: client.ContainerListResult{Items: []containertypes.Summary{
			{ID: "unmanaged-stopped", Created: 1, State: containertypes.ContainerState("exited")},
			{ID: "managed-running", Created: 0, State: containertypes.StateRunning, Labels: map[string]string{types.LabelRunID: "run-1"}},
			{ID: "c20", Created: 20, State: containertypes.ContainerState("exited"), Labels: map[string]string{types.LabelRunID: "run-2"}},
			{ID: "b10", Created: 10, State: containertypes.ContainerState("exited"), Labels: map[string]string{types.LabelRunID: "run-3"}},
			{ID: "a10", Created: 10, State: containertypes.ContainerState("dead"), Labels: map[string]string{types.LabelJobID: "job-1"}},
		}},
	}
	fb := &fakeFreeBytes{
		valuesByPath: map[string][]int64{
			"/var/lib/docker": {low, low, low, minDockerFreeBytes + 1},
			workspaceRoot:     {minDockerFreeBytes + 1, minDockerFreeBytes + 1, minDockerFreeBytes + 1, minDockerFreeBytes + 1},
		},
	}

	cleanup := &dockerPreClaimCleanup{docker: fakeDocker, freeBytes: fb.read, workspaceRoot: workspaceRoot}

	ok, err := cleanup.EnsureCapacity(context.Background())
	if err != nil {
		t.Fatalf("EnsureCapacity() error = %v", err)
	}
	if !ok {
		t.Fatal("EnsureCapacity() ok = false, want true")
	}
	if fakeDocker.listCalls != 1 {
		t.Fatalf("ContainerList calls = %d, want 1", fakeDocker.listCalls)
	}
	if got, want := fakeDocker.removedIDs, []string{"a10", "b10", "c20"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("removed order = %v, want %v", got, want)
	}
	if len(fb.paths) == 0 {
		t.Fatal("expected free-bytes probes, got none")
	}
	var seenDocker, seenWorkspace bool
	for _, path := range fb.paths {
		if path == "/var/lib/docker" {
			seenDocker = true
		}
		if path == workspaceRoot {
			seenWorkspace = true
		}
	}
	if !seenDocker || !seenWorkspace {
		t.Fatalf("free bytes paths must include docker and workspace roots, got %v", fb.paths)
	}
}

func TestDockerPreClaimCleanup_ExhaustedContainersReturnsFalse(t *testing.T) {
	t.Parallel()

	low := minDockerFreeBytes - 1
	workspaceRoot := t.TempDir()
	fakeDocker := &fakeDockerClient{
		infoResult: client.SystemInfoResult{Info: system.Info{DockerRootDir: "/var/lib/docker"}},
		listResult: client.ContainerListResult{Items: []containertypes.Summary{
			{ID: "old-1", Created: 1, State: containertypes.ContainerState("exited"), Labels: map[string]string{types.LabelRunID: "run-1"}},
			{ID: "old-2", Created: 2, State: containertypes.ContainerState("dead"), Labels: map[string]string{types.LabelJobID: "job-2"}},
		}},
	}
	fb := &fakeFreeBytes{
		valuesByPath: map[string][]int64{
			"/var/lib/docker": {low, low, low},
			workspaceRoot:     {minDockerFreeBytes + 1, minDockerFreeBytes + 1, minDockerFreeBytes + 1},
		},
	}

	cleanup := &dockerPreClaimCleanup{docker: fakeDocker, freeBytes: fb.read, workspaceRoot: workspaceRoot}

	ok, err := cleanup.EnsureCapacity(context.Background())
	if err != nil {
		t.Fatalf("EnsureCapacity() error = %v", err)
	}
	if ok {
		t.Fatal("EnsureCapacity() ok = true, want false")
	}
	if got, want := fakeDocker.removedIDs, []string{"old-1", "old-2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("removed containers = %v, want %v", got, want)
	}
}

func TestDockerPreClaimCleanup_InfoError(t *testing.T) {
	t.Parallel()

	cleanup := &dockerPreClaimCleanup{
		docker: &fakeDockerClient{infoErr: errors.New("boom")},
		freeBytes: func(string) (int64, error) {
			return minDockerFreeBytes, nil
		},
		workspaceRoot: t.TempDir(),
	}

	ok, err := cleanup.EnsureCapacity(context.Background())
	if err == nil {
		t.Fatal("EnsureCapacity() error = nil, want non-nil")
	}
	if ok {
		t.Fatal("EnsureCapacity() ok = true, want false")
	}
}

func TestDockerPreClaimCleanup_EmptyDockerRootDir(t *testing.T) {
	t.Parallel()

	cleanup := &dockerPreClaimCleanup{
		docker: &fakeDockerClient{infoResult: client.SystemInfoResult{Info: system.Info{DockerRootDir: ""}}},
		freeBytes: func(string) (int64, error) {
			return minDockerFreeBytes, nil
		},
		workspaceRoot: t.TempDir(),
	}

	ok, err := cleanup.EnsureCapacity(context.Background())
	if err == nil {
		t.Fatal("EnsureCapacity() error = nil, want non-nil")
	}
	if ok {
		t.Fatal("EnsureCapacity() ok = true, want false")
	}
}

func TestDockerPreClaimCleanup_FreeBytesError(t *testing.T) {
	t.Parallel()

	cleanup := &dockerPreClaimCleanup{
		docker: &fakeDockerClient{infoResult: client.SystemInfoResult{Info: system.Info{DockerRootDir: "/var/lib/docker"}}},
		freeBytes: func(string) (int64, error) {
			return 0, errors.New("statfs failed")
		},
		workspaceRoot: t.TempDir(),
	}

	ok, err := cleanup.EnsureCapacity(context.Background())
	if err == nil {
		t.Fatal("EnsureCapacity() error = nil, want non-nil")
	}
	if ok {
		t.Fatal("EnsureCapacity() ok = true, want false")
	}
}

func TestDockerPreClaimCleanup_RemoveError(t *testing.T) {
	t.Parallel()

	low := minDockerFreeBytes - 1
	workspaceRoot := t.TempDir()
	fakeDocker := &fakeDockerClient{
		infoResult: client.SystemInfoResult{Info: system.Info{DockerRootDir: "/var/lib/docker"}},
		listResult: client.ContainerListResult{Items: []containertypes.Summary{{
			ID:      "old-1",
			Created: 1,
			State:   containertypes.ContainerState("exited"),
			Labels:  map[string]string{types.LabelRunID: "run-1"},
		}}},
		removeErrByID: map[string]error{"old-1": errors.New("remove failed")},
	}
	fb := &fakeFreeBytes{
		valuesByPath: map[string][]int64{
			"/var/lib/docker": {low},
			workspaceRoot:     {minDockerFreeBytes + 1},
		},
	}

	cleanup := &dockerPreClaimCleanup{docker: fakeDocker, freeBytes: fb.read, workspaceRoot: workspaceRoot}

	ok, err := cleanup.EnsureCapacity(context.Background())
	if err == nil {
		t.Fatal("EnsureCapacity() error = nil, want non-nil")
	}
	if ok {
		t.Fatal("EnsureCapacity() ok = true, want false")
	}
	if got, want := fakeDocker.removedIDs, []string{"old-1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("removed containers = %v, want %v", got, want)
	}
}

func TestDockerPreClaimCleanup_EvictsOldestStickyWorkspaceFirst(t *testing.T) {
	t.Parallel()

	workspaceRoot := t.TempDir()
	oldest := filepath.Join(workspaceRoot, "run-old", "repos", "repo-a", "workspace")
	newer := filepath.Join(workspaceRoot, "run-new", "repos", "repo-b", "workspace")
	if err := os.MkdirAll(oldest, 0o755); err != nil {
		t.Fatalf("mkdir oldest workspace: %v", err)
	}
	if err := os.MkdirAll(newer, 0o755); err != nil {
		t.Fatalf("mkdir newer workspace: %v", err)
	}
	oldTime := time.Now().Add(-2 * time.Hour)
	newTime := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(oldest, oldTime, oldTime); err != nil {
		t.Fatalf("chtimes oldest: %v", err)
	}
	if err := os.Chtimes(newer, newTime, newTime); err != nil {
		t.Fatalf("chtimes newer: %v", err)
	}

	fakeDocker := &fakeDockerClient{
		infoResult: client.SystemInfoResult{Info: system.Info{DockerRootDir: "/var/lib/docker"}},
	}
	fb := &fakeFreeBytes{
		valuesByPath: map[string][]int64{
			"/var/lib/docker": {minDockerFreeBytes + 1, minDockerFreeBytes + 1},
			workspaceRoot:     {minDockerFreeBytes - 1, minDockerFreeBytes + 1},
		},
	}
	cleanup := &dockerPreClaimCleanup{docker: fakeDocker, freeBytes: fb.read, workspaceRoot: workspaceRoot}

	ok, err := cleanup.EnsureCapacity(context.Background())
	if err != nil {
		t.Fatalf("EnsureCapacity() error = %v", err)
	}
	if !ok {
		t.Fatal("EnsureCapacity() ok = false, want true")
	}
	if _, err := os.Stat(oldest); !os.IsNotExist(err) {
		t.Fatalf("oldest workspace should be evicted, stat err=%v", err)
	}
	if _, err := os.Stat(newer); err != nil {
		t.Fatalf("newer workspace should remain, stat err=%v", err)
	}
	if fakeDocker.listCalls != 0 {
		t.Fatalf("ContainerList calls = %d, want 0", fakeDocker.listCalls)
	}
}
