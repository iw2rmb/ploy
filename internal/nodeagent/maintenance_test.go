package nodeagent

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"golang.org/x/sys/unix"
)

func isolatedMaintenanceLock(t *testing.T) *os.File {
	t.Helper()
	path := filepath.Join(t.TempDir(), "maintenance.lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { file.Close() })
	t.Setenv("PLOY_NODE_MAINTENANCE_LOCK_FILE", path)
	return file
}

func assertMaintenanceBlocked(t *testing.T, file *os.File) {
	t.Helper()
	err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if !errors.Is(err, unix.EWOULDBLOCK) {
		t.Fatalf("exclusive maintenance must be blocked: %v", err)
	}
}

// Independent job locks allow concurrency and exclude maintenance until the last release.
func TestMaintenanceWaitsForEverySlot(t *testing.T) {
	file := isolatedMaintenanceLock(t)
	r := &runController{cfg: Config{Concurrency: 2}}
	first, err := r.AcquireSlot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	second, err := r.AcquireSlot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	assertMaintenanceBlocked(t, file)
	r.ReleaseSlot(first)
	assertMaintenanceBlocked(t, file)
	r.ReleaseSlot(second)
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := r.AcquireSlot(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("maintenance must block new work: %v", err)
	}
	if len(r.jobSem) != 0 {
		t.Fatal("cancelled acquisition leaked a slot")
	}
}

// Claim requests are protected even before Docker has created any container.
func TestMaintenanceProtectsClaimAndReleasesNoWork(t *testing.T) {
	file := isolatedMaintenanceLock(t)
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		called = true
		assertMaintenanceBlocked(t, file)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	cfg := newAgentConfig(server.URL)
	r := newTestController(t, cfg)
	c := setupClaimer(t, cfg, r)
	claimed, err := c.claimAndExecute(context.Background())
	if err != nil || claimed || !called {
		t.Fatalf("claim = %v, %v; called=%v", claimed, err, called)
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		t.Fatal(err)
	}
}

// A terminal status must not release maintenance while deferred artifacts are uploading.
func TestMaintenanceWaitsForDeferredGateArtifacts(t *testing.T) {
	file := isolatedMaintenanceLock(t)
	t.Setenv("PLOYD_CACHE_HOME", t.TempDir())
	// Force runtime initialization to fail, taking the gate's deferred artifact path.
	t.Setenv("DOCKER_HOST", "invalid://maintenance-test")
	uploaded := make(chan struct{})
	unblock := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if strings.HasSuffix(req.URL.Path, "/artifact") {
			close(uploaded)
			<-unblock
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"artifact_bundle_id":"test","cid":"test"}`))
			return
		}
		if strings.HasSuffix(req.URL.Path, "/status") {
			_, _ = w.Write([]byte(`{"status":"Running"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	// Unblock the upload before closing the server, including failure paths.
	defer func() {
		select {
		case <-unblock:
		default:
			close(unblock)
		}
		server.Close()
	}()
	cfg := newAgentConfig(server.URL)
	r := newTestController(t, cfg)
	req := StartRunRequest{RunID: types.NewRunID(), JobID: types.NewJobID(), RepoID: types.NewRepoID(), JobType: types.JobTypePreGate}
	slot, err := r.AcquireSlot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := r.StartRun(context.Background(), req, slot); err != nil {
		r.ReleaseSlot(slot)
		t.Fatal(err)
	}
	select {
	case <-uploaded:
	case <-time.After(5 * time.Second):
		t.Fatal("deferred upload did not start")
	}
	assertMaintenanceBlocked(t, file)
	close(unblock)
	deadline := time.Now().Add(5 * time.Second)
	for {
		if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("execution did not release maintenance")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := os.Stat(jobDirectories(req.RunID, req.JobID).Tmp); !os.IsNotExist(err) {
		t.Fatalf("runtime cleanup did not finish: %v", err)
	}
}

// A missing host mount must fail closed instead of creating a private lock inode.
func TestMaintenanceMissingLockRejectsActivity(t *testing.T) {
	t.Setenv("PLOY_NODE_MAINTENANCE_LOCK_FILE", filepath.Join(t.TempDir(), "missing"))
	r := &runController{}
	if _, err := r.AcquireSlot(context.Background()); err == nil {
		t.Fatal("missing lock accepted")
	}
	if len(r.jobSem) != 0 {
		t.Fatal("missing lock leaked a slot")
	}
}
