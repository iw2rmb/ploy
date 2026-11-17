package nodeagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
)

type cleanupDockerFixture struct {
	containers    []container.InspectResponse
	inspectErr    error
	removed       []string
	removeOptions []client.ContainerRemoveOptions
	imageItems    []image.Summary
	imageRemoved  []string
	imageOptions  []client.ImageRemoveOptions
	imageInspect  func(string) (client.ImageInspectResult, error)
	builds        []client.BuildCachePruneOptions
	volumes       []client.VolumePruneOptions
}

func (f *cleanupDockerFixture) ContainerList(context.Context, client.ContainerListOptions) (client.ContainerListResult, error) {
	result := client.ContainerListResult{}
	for _, c := range f.containers {
		result.Items = append(result.Items, container.Summary{ID: c.ID})
	}
	return result, nil
}
func (f *cleanupDockerFixture) ContainerInspect(_ context.Context, id string, _ client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	if f.inspectErr != nil {
		return client.ContainerInspectResult{}, f.inspectErr
	}
	for _, c := range f.containers {
		if c.ID == id {
			if c.Mounts == nil {
				c.Mounts = []container.MountPoint{}
			}
			raw, _ := json.Marshal(c)
			return client.ContainerInspectResult{Container: c, Raw: raw}, nil
		}
	}
	return client.ContainerInspectResult{}, fmt.Errorf("missing container %s", id)
}

// Missing mount inspection cannot prove that retained data has no container references.
func TestCleanupContainerInspectionRequiresMounts(t *testing.T) {
	for _, tc := range []struct {
		name, mounts string
		wantErr      bool
	}{
		{"empty array", `,"Mounts":[]`, false},
		{"missing", "", true},
		{"null", `,"Mounts":null`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if strings.HasSuffix(req.URL.Path, "/containers/json") {
					_, _ = fmt.Fprint(w, `[{"Id":"probe"}]`)
					return
				}
				_, _ = fmt.Fprintf(w, `{"Id":"probe","Config":{},"State":{"Status":"exited","Running":false,"Restarting":false,"Paused":false,"Pid":0}%s}`, tc.mounts)
			}))
			defer server.Close()
			docker, err := client.New(client.WithHost(server.URL), client.WithAPIVersion("1.52"))
			if err != nil {
				t.Fatal(err)
			}
			defer docker.Close()
			_, err = inspectCleanupContainers(context.Background(), docker)
			if (err != nil) != tc.wantErr {
				t.Fatalf("inspection error=%v, want error=%v", err, tc.wantErr)
			}
		})
	}
}
func (f *cleanupDockerFixture) ContainerRemove(_ context.Context, id string, options client.ContainerRemoveOptions) (client.ContainerRemoveResult, error) {
	f.removed = append(f.removed, id)
	f.removeOptions = append(f.removeOptions, options)
	return client.ContainerRemoveResult{}, nil
}
func (f *cleanupDockerFixture) ImageList(context.Context, client.ImageListOptions) (client.ImageListResult, error) {
	return client.ImageListResult{Items: f.imageItems}, nil
}
func (f *cleanupDockerFixture) ImageInspect(_ context.Context, ref string, _ ...client.ImageInspectOption) (client.ImageInspectResult, error) {
	if f.imageInspect != nil {
		return f.imageInspect(ref)
	}
	return client.ImageInspectResult{InspectResponse: image.InspectResponse{ID: ref}}, nil
}
func (f *cleanupDockerFixture) ImageRemove(_ context.Context, ref string, options client.ImageRemoveOptions) (client.ImageRemoveResult, error) {
	f.imageRemoved = append(f.imageRemoved, ref)
	f.imageOptions = append(f.imageOptions, options)
	return client.ImageRemoveResult{}, nil
}
func (f *cleanupDockerFixture) BuildCachePrune(_ context.Context, options client.BuildCachePruneOptions) (client.BuildCachePruneResult, error) {
	f.builds = append(f.builds, options)
	return client.BuildCachePruneResult{}, nil
}
func (f *cleanupDockerFixture) VolumePrune(_ context.Context, options client.VolumePruneOptions) (client.VolumePruneResult, error) {
	f.volumes = append(f.volumes, options)
	return client.VolumePruneResult{}, nil
}

// Activity inspection accepts only accounted inactive leases and stopped job writers.
func TestCleanupActivityPreservesUnknownActivity(t *testing.T) {
	inactive, active := false, true
	for _, tc := range []struct {
		name          string
		state         container.State
		tasks, leases map[string]bool
		cache         []cleanupBuildCacheRecord
		cacheErr      error
		wantErr       bool
	}{
		{name: "stopped", state: container.State{Status: container.StateExited}},
		{name: "running", state: container.State{Status: container.StateRunning, Running: true}, wantErr: true},
		{name: "created job", state: container.State{Status: container.StateCreated}},
		{name: "created job with task", state: container.State{Status: container.StateCreated}, tasks: map[string]bool{"job": true}, wantErr: true},
		{name: "stale pid", state: container.State{Status: container.StateExited, Pid: 42}, wantErr: true},
		{name: "remaining task", state: container.State{Status: container.StateExited}, tasks: map[string]bool{"job": true}, wantErr: true},
		{name: "container lease", state: container.State{Status: container.StateExited}, leases: map[string]bool{"job": true}},
		{name: "inactive BuildKit", state: container.State{Status: container.StateExited}, leases: map[string]bool{"cache-view": true, "cache-variants": true}, cache: []cleanupBuildCacheRecord{{ID: "cache", InUse: &inactive}}},
		{name: "active BuildKit", state: container.State{Status: container.StateExited}, leases: map[string]bool{"cache": true}, cache: []cleanupBuildCacheRecord{{ID: "cache", InUse: &active}}, wantErr: true},
		{name: "active overrides inactive", state: container.State{Status: container.StateExited}, leases: map[string]bool{"cache": true}, cache: []cleanupBuildCacheRecord{{ID: "cache", InUse: &inactive}, {ID: "cache", InUse: &active}}, wantErr: true},
		{name: "unknown lease", state: container.State{Status: container.StateExited}, leases: map[string]bool{"pull": true}, cache: []cleanupBuildCacheRecord{}, wantErr: true},
		{name: "missing InUse", state: container.State{Status: container.StateExited}, leases: map[string]bool{"cache": true}, cache: []cleanupBuildCacheRecord{{ID: "cache"}}, wantErr: true},
		{name: "inspection failure", state: container.State{Status: container.StateExited}, leases: map[string]bool{"cache": true}, cacheErr: errors.New("unavailable"), wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			containers := []container.InspectResponse{{ID: "job", State: &tc.state, Config: &container.Config{Labels: map[string]string{types.LabelJobID: "job"}}}}
			err := checkCleanupActivity(context.Background(), containers, tc.tasks, tc.leases, func(context.Context) ([]cleanupBuildCacheRecord, error) { return tc.cache, tc.cacheErr })
			if (err != nil) != tc.wantErr {
				t.Fatalf("error=%v, want error=%v", err, tc.wantErr)
			}
		})
	}
}

// Container cleanup retains creation-age filtering, anonymous volumes, and opt-in volume pruning.
func TestCleanupDockerPreservesDeletionPolicy(t *testing.T) {
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	for _, volumes := range []bool{false, true} {
		t.Run(fmt.Sprint(volumes), func(t *testing.T) {
			f := &cleanupDockerFixture{}
			for _, tc := range []struct {
				id     string
				status container.ContainerState
				age    time.Duration
			}{
				{"old exited", container.StateExited, 25 * time.Hour}, {"old created", container.StateCreated, 25 * time.Hour}, {"old dead", container.StateDead, 25 * time.Hour},
				{"boundary", container.StateExited, 24 * time.Hour}, {"recent", container.StateExited, time.Hour}, {"running", container.StateRunning, 25 * time.Hour},
			} {
				f.containers = append(f.containers, container.InspectResponse{ID: tc.id, State: &container.State{Status: tc.status}, Created: now.Add(-tc.age).Format(time.RFC3339Nano)})
			}
			remaining, err := pruneCleanupDocker(context.Background(), f, f.containers, now, 24*time.Hour, "24h", volumes)
			if err != nil {
				t.Fatal(err)
			}
			if len(remaining) != 3 || remaining[0].ID != "boundary" || remaining[1].ID != "recent" || remaining[2].ID != "running" {
				t.Fatalf("remaining %+v", remaining)
			}
			if !reflect.DeepEqual(f.removed, []string{"old exited", "old created", "old dead"}) {
				t.Fatalf("removed %v", f.removed)
			}
			for _, o := range f.removeOptions {
				if !o.RemoveVolumes || o.Force {
					t.Fatalf("remove options %+v", o)
				}
			}
			if len(f.builds) != 1 || !f.builds[0].All || !f.builds[0].Filters["until"]["24h"] {
				t.Fatalf("build options %+v", f.builds)
			}
			if (len(f.volumes) == 1) != volumes {
				t.Fatalf("volume calls %d", len(f.volumes))
			}
		})
	}
}

func cleanupTestRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PLOYD_CACHE_HOME", root)
	return root
}

func cleanupTestMountInfo(t *testing.T, root string) string {
	t.Helper()
	path := filepath.Join(root, "mountinfo")
	if err := os.WriteFile(path, []byte("1 0 0:1 / / rw - rootfs rootfs rw\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

// Eligible runs lose only workspace and runtime-share; server identity changes preserve retained data.
func TestCleanupRunDataUsesServerEligibilityAndIdentity(t *testing.T) {
	for _, tc := range []struct {
		name     string
		eligible bool
		change   bool
		unknown  bool
		wantErr  bool
		removed  bool
	}{
		{"eligible", true, false, false, false, true}, {"inside retention", false, false, false, false, false},
		{"changed resume", true, true, false, true, false}, {"unknown completion", true, false, true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := cleanupTestRoot(t)
			mountInfo := cleanupTestMountInfo(t, root)
			id := types.NewRunID()
			for _, name := range []string{"workspace", "runtime-share", "jobs", "share"} {
				if err := os.MkdirAll(filepath.Join(runDir(id), name), 0700); err != nil {
					t.Fatal(err)
				}
			}
			finished := time.Now().Add(-48 * time.Hour)
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				calls++
				run := types.RunSummary{ID: id, MigID: types.NewMigID(), SpecID: types.NewSpecID(), Attempt: 1, Status: types.RunStatusSuccess, FinishedAt: &finished, CleanupEligible: tc.eligible}
				if tc.change && calls > 1 {
					run.ResumeCount = 1
				}
				if tc.unknown {
					run.FinishedAt = nil
				}
				if err := json.NewEncoder(w).Encode(run); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			r := &runController{cfg: Config{ServerURL: server.URL}, uploader: &baseUploader{client: server.Client()}}
			err := r.pruneExpiredRunData(context.Background(), nil, mountInfo)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error=%v, want=%v", err, tc.wantErr)
			}
			for _, name := range []string{"workspace", "runtime-share", "jobs", "share"} {
				_, err := os.Stat(filepath.Join(runDir(id), name))
				wantRemoved := tc.removed && (name == "workspace" || name == "runtime-share")
				if os.IsNotExist(err) != wantRemoved {
					t.Fatalf("%s error=%v, removed=%v", name, err, wantRemoved)
				}
			}
		})
	}
}

// Unsafe paths and host or container mounts prevent deletion, including stopped-container references.
func TestCleanupPathRejectsUnsafeReferences(t *testing.T) {
	root := cleanupTestRoot(t)
	path := filepath.Join(root, "run", "workspace")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	dangling := filepath.Join(root, "dangling")
	if err := os.Symlink(filepath.Join(path, "gone"), dangling); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name       string
		mounts     []string
		containers []container.InspectResponse
		wantErr    bool
	}{
		{name: "clear"}, {name: "mounted child", mounts: []string{filepath.Join(path, "nested")}, wantErr: true}, {name: "mounted run", mounts: []string{filepath.Dir(path)}, wantErr: true},
		{name: "container parent", containers: []container.InspectResponse{{ID: "other", Config: &container.Config{}, Mounts: []container.MountPoint{{Type: mount.TypeBind, Source: root}}}}, wantErr: true},
		{name: "container host root", containers: []container.InspectResponse{{ID: "other", Config: &container.Config{}, Mounts: []container.MountPoint{{Type: mount.TypeBind, Source: "/"}}}}, wantErr: true},
		{name: "container missing child", containers: []container.InspectResponse{{ID: "other", Config: &container.Config{}, Mounts: []container.MountPoint{{Type: mount.TypeBind, Source: filepath.Join(path, "gone")}}}}, wantErr: true},
		{name: "container dangling source", containers: []container.InspectResponse{{ID: "other", Config: &container.Config{}, Mounts: []container.MountPoint{{Type: mount.TypeBind, Source: filepath.Join(dangling, "nested")}}}}, wantErr: true},
		{name: "own node", containers: []container.InspectResponse{{ID: "node", Config: &container.Config{Labels: map[string]string{"com.ploy.component": "node", "com.ploy.node_id": "node-id"}}, Mounts: []container.MountPoint{{Type: mount.TypeBind, Source: root}}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := checkCleanupPath(path, tc.mounts, tc.containers, "node-id")
			if (err != nil) != tc.wantErr {
				t.Fatalf("error=%v want=%v", err, tc.wantErr)
			}
		})
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if err := cleanupCanonicalDirectory(link); err == nil {
		t.Fatal("symlink accepted")
	}
}

// Host mountinfo preserves escaped mount targets and rejects incomplete inspection output.
func TestCleanupMountTargets(t *testing.T) {
	for _, tc := range []struct {
		name, data string
		want       []string
		wantErr    bool
	}{
		{"escaped", "1 0 0:1 / / rw - rootfs rootfs rw\n2 1 0:2 / /some\\040path rw shared:1 - ext4 /dev/sda rw\n", []string{"/", "/some path"}, false},
		{"empty", "", nil, true}, {"malformed", "broken\n", nil, true}, {"bad escape", "1 0 0:1 / /bad\\099path rw - rootfs rootfs rw\n", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "mountinfo")
			if err := os.WriteFile(path, []byte(tc.data), 0600); err != nil {
				t.Fatal(err)
			}
			got, err := readHostMountTargets(path)
			if (err != nil) != tc.wantErr || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got=%v error=%v", got, err)
			}
		})
	}
}

// Git clone age uses .git access time and find's whole-minute comparison.
func TestCleanupGitClonesUsesAccessTime(t *testing.T) {
	root := cleanupTestRoot(t)
	now := time.Now()
	for _, tc := range []struct {
		name string
		age  time.Duration
	}{{"expired", 25 * time.Hour}, {"boundary", 24*time.Hour + 30*time.Second}, {"recent", time.Hour}} {
		git := filepath.Join(root, "git-clones", tc.name, ".git")
		if err := os.MkdirAll(git, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(git, now.Add(-tc.age), now); err != nil {
			t.Fatal(err)
		}
	}
	if err := pruneCleanupGitClones(context.Background(), 24*time.Hour, now); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"expired", "boundary", "recent"} {
		_, err := os.Stat(filepath.Join(root, "git-clones", name))
		if os.IsNotExist(err) != (name == "expired") {
			t.Fatalf("%s error=%v", name, err)
		}
	}
}

// Cleanup age retains the host script's positive integer h/m format.
func TestCleanupAgeValidation(t *testing.T) {
	for _, value := range []string{"24h", "6h", "30m", "006h"} {
		if _, err := parseCleanupAge(value); err != nil {
			t.Fatal(err)
		}
	}
	for _, value := range []string{"", "0h", "-1h", "1.5h", "2s", "1h30m", strings.Repeat("9", 30) + "h"} {
		if _, err := parseCleanupAge(value); err == nil {
			t.Fatalf("accepted %q", value)
		}
	}
}
