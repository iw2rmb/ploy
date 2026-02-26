package nodeagent

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
	containertypes "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/system"
	"github.com/moby/moby/client"
)

type fakeClaimCleanupDockerClient struct {
	infoResult client.SystemInfoResult
	infoErr    error

	listResult client.ContainerListResult
	listErr    error
	listCalls  int

	removeErrByID map[string]error
	removedIDs    []string
}

func (f *fakeClaimCleanupDockerClient) Info(context.Context, client.InfoOptions) (client.SystemInfoResult, error) {
	if f.infoErr != nil {
		return client.SystemInfoResult{}, f.infoErr
	}
	return f.infoResult, nil
}

func (f *fakeClaimCleanupDockerClient) ContainerList(context.Context, client.ContainerListOptions) (client.ContainerListResult, error) {
	f.listCalls++
	if f.listErr != nil {
		return client.ContainerListResult{}, f.listErr
	}
	return f.listResult, nil
}

func (f *fakeClaimCleanupDockerClient) ContainerRemove(_ context.Context, containerID string, _ client.ContainerRemoveOptions) (client.ContainerRemoveResult, error) {
	f.removedIDs = append(f.removedIDs, containerID)
	if err, ok := f.removeErrByID[containerID]; ok && err != nil {
		return client.ContainerRemoveResult{}, err
	}
	return client.ContainerRemoveResult{}, nil
}

type fakeFreeBytes struct {
	values []int64
	errAt  int
	err    error
	calls  int
	paths  []string
}

func (f *fakeFreeBytes) read(path string) (int64, error) {
	f.calls++
	f.paths = append(f.paths, path)
	if f.errAt > 0 && f.calls == f.errAt {
		return 0, f.err
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

	fakeDocker := &fakeClaimCleanupDockerClient{
		infoResult: client.SystemInfoResult{Info: system.Info{DockerRootDir: "/var/lib/docker"}},
	}
	fb := &fakeFreeBytes{values: []int64{minDockerFreeBytes + 1}}

	cleanup := &dockerPreClaimCleanup{docker: fakeDocker, freeBytes: fb.read}

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
	fakeDocker := &fakeClaimCleanupDockerClient{
		infoResult: client.SystemInfoResult{Info: system.Info{DockerRootDir: "/var/lib/docker"}},
		listResult: client.ContainerListResult{Items: []containertypes.Summary{
			{ID: "unmanaged-stopped", Created: 1, State: containertypes.ContainerState("exited")},
			{ID: "managed-running", Created: 0, State: containertypes.StateRunning, Labels: map[string]string{types.LabelRunID: "run-1"}},
			{ID: "c20", Created: 20, State: containertypes.ContainerState("exited"), Labels: map[string]string{types.LabelRunID: "run-2"}},
			{ID: "b10", Created: 10, State: containertypes.ContainerState("exited"), Labels: map[string]string{types.LabelRunID: "run-3"}},
			{ID: "a10", Created: 10, State: containertypes.ContainerState("dead"), Labels: map[string]string{types.LabelJobID: "job-1"}},
		}},
	}
	fb := &fakeFreeBytes{values: []int64{low, low, low, minDockerFreeBytes + 1}}

	cleanup := &dockerPreClaimCleanup{docker: fakeDocker, freeBytes: fb.read}

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
	for _, path := range fb.paths {
		if path != "/var/lib/docker" {
			t.Fatalf("free bytes path = %q, want /var/lib/docker", path)
		}
	}
}

func TestDockerPreClaimCleanup_ExhaustedContainersReturnsFalse(t *testing.T) {
	t.Parallel()

	low := minDockerFreeBytes - 1
	fakeDocker := &fakeClaimCleanupDockerClient{
		infoResult: client.SystemInfoResult{Info: system.Info{DockerRootDir: "/var/lib/docker"}},
		listResult: client.ContainerListResult{Items: []containertypes.Summary{
			{ID: "old-1", Created: 1, State: containertypes.ContainerState("exited"), Labels: map[string]string{types.LabelRunID: "run-1"}},
			{ID: "old-2", Created: 2, State: containertypes.ContainerState("dead"), Labels: map[string]string{types.LabelJobID: "job-2"}},
		}},
	}
	fb := &fakeFreeBytes{values: []int64{low, low, low}}

	cleanup := &dockerPreClaimCleanup{docker: fakeDocker, freeBytes: fb.read}

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
		docker: &fakeClaimCleanupDockerClient{infoErr: errors.New("boom")},
		freeBytes: func(string) (int64, error) {
			return minDockerFreeBytes, nil
		},
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
		docker: &fakeClaimCleanupDockerClient{infoResult: client.SystemInfoResult{Info: system.Info{DockerRootDir: ""}}},
		freeBytes: func(string) (int64, error) {
			return minDockerFreeBytes, nil
		},
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
		docker: &fakeClaimCleanupDockerClient{infoResult: client.SystemInfoResult{Info: system.Info{DockerRootDir: "/var/lib/docker"}}},
		freeBytes: func(string) (int64, error) {
			return 0, errors.New("statfs failed")
		},
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
	fakeDocker := &fakeClaimCleanupDockerClient{
		infoResult: client.SystemInfoResult{Info: system.Info{DockerRootDir: "/var/lib/docker"}},
		listResult: client.ContainerListResult{Items: []containertypes.Summary{{
			ID:      "old-1",
			Created: 1,
			State:   containertypes.ContainerState("exited"),
			Labels:  map[string]string{types.LabelRunID: "run-1"},
		}}},
		removeErrByID: map[string]error{"old-1": errors.New("remove failed")},
	}
	fb := &fakeFreeBytes{values: []int64{low}}

	cleanup := &dockerPreClaimCleanup{docker: fakeDocker, freeBytes: fb.read}

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
