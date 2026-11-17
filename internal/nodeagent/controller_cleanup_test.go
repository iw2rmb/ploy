package nodeagent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	containertypes "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"golang.org/x/sys/unix"
)

// Only a complete pass starts the cooldown; failures and contention can retry.
func TestCleanupCooldownTracksSuccessfulCompletion(t *testing.T) {
	for _, outcome := range []string{"success", "failure", "cancelled", "busy"} {
		t.Run(outcome, func(t *testing.T) {
			lock := isolatedMaintenanceLock(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			r := &runController{nodeCtx: ctx, cleanupPass: func(ctx context.Context) error {
				calls++
				assertMaintenanceBlocked(t, lock)
				if outcome == "failure" {
					return errors.New("inspection failed")
				}
				if outcome == "cancelled" {
					cancel()
				}
				return nil
			}}
			if outcome == "busy" {
				if err := unix.Flock(int(lock.Fd()), unix.LOCK_SH); err != nil {
					t.Fatal(err)
				}
			}
			r.tryCleanup()
			if r.cleanupRunning {
				t.Fatal("cleanupRunning remained set")
			}
			if outcome == "success" {
				if r.lastCleanupSuccess.IsZero() {
					t.Fatal("successful completion was not recorded")
				}
				r.tryCleanup()
				if calls != 1 {
					t.Fatal("successful pass did not start cooldown")
				}
				r.lastCleanupSuccess = time.Now().Add(-nodeCleanupInterval)
				r.tryCleanup()
				if calls != 2 {
					t.Fatal("elapsed interval did not permit cleanup")
				}
				return
			}
			if !r.lastCleanupSuccess.IsZero() {
				t.Fatal("incomplete pass started cooldown")
			}
			_ = unix.Flock(int(lock.Fd()), unix.LOCK_UN)
			r.nodeCtx = context.Background()
			r.cleanupPass = func(context.Context) error { calls++; return nil }
			r.tryCleanup()
			if r.lastCleanupSuccess.IsZero() {
				t.Fatal("later successful retry was suppressed")
			}
		})
	}
}

// Concurrent hooks start one pass, and another controller still respects flock.
func TestCleanupConcurrentHooksAndProcessesExcludeOverlap(t *testing.T) {
	lock := isolatedMaintenanceLock(t)
	started, release := make(chan struct{}), make(chan struct{})
	r := &runController{nodeCtx: context.Background(), cleanupPass: func(context.Context) error {
		close(started)
		<-release
		return nil
	}}
	done := make(chan struct{})
	go func() { defer close(done); r.tryCleanup() }()
	<-started
	r.tryCleanup()
	peer := &runController{nodeCtx: context.Background(), cleanupPass: func(context.Context) error {
		t.Error("another controller overlapped cleanup")
		return nil
	}}
	peer.tryCleanup()
	if !peer.lastCleanupSuccess.IsZero() {
		t.Error("busy peer started cooldown")
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestCleanupLockSubprocess$")
	cmd.Env = append(os.Environ(), "PLOY_TEST_CLEANUP_LOCK="+lock.Name())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("cross-process lock: %v: %s", err, out)
	}
	close(release)
	<-done
	peer.cleanupPass = func(context.Context) error { return nil }
	peer.tryCleanup()
	if peer.lastCleanupSuccess.IsZero() {
		t.Fatal("new process state did not permit cleanup after lock release")
	}
}

// The child must observe the parent's exclusive lock on the original inode.
func TestCleanupLockSubprocess(t *testing.T) {
	path := os.Getenv("PLOY_TEST_CLEANUP_LOCK")
	if path == "" {
		t.Skip("subprocess helper")
	}
	t.Setenv("PLOY_NODE_MAINTENANCE_LOCK_FILE", path)
	lock, err := tryMaintenanceCleanup(context.Background())
	if err != nil || lock != nil {
		t.Fatalf("parent lock not respected: lock=%v err=%v", lock, err)
	}
}

// Hooks retain the permit and reacquire shared protection even after a failed pass.
func TestCleanupHooksPreservePermitAndExecutionProtection(t *testing.T) {
	file := isolatedMaintenanceLock(t)
	r := &runController{cfg: Config{Concurrency: 1}, nodeCtx: context.Background()}
	var calls int
	r.cleanupPass = func(context.Context) error {
		calls++
		if len(r.jobSem) != 1 {
			t.Error("hook released its concurrency permit")
		}
		assertMaintenanceBlocked(t, file)
		return errors.New("retry later")
	}
	slot, err := r.AcquireSlot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := r.beforeJob(context.Background(), slot); err != nil {
		t.Fatal(err)
	}
	assertMaintenanceBlocked(t, file)
	r.FinishRunSlot(slot)
	if calls != 2 || len(r.jobSem) != 0 {
		t.Fatalf("calls=%d permits=%d", calls, len(r.jobSem))
	}
}

// Cancellation prevents execution when the before-job shared lock cannot be reacquired.
func TestCleanupBeforeJobReacquisitionFailureReportsCancellation(t *testing.T) {
	isolatedMaintenanceLock(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	completed := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if strings.HasSuffix(req.URL.Path, "/complete") {
			body, _ := io.ReadAll(req.Body)
			completed <- string(body)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	r := newTestController(t, newAgentConfig(srv.URL))
	var calls atomic.Int32
	r.cleanupPass = func(ctx context.Context) error {
		calls.Add(1)
		cancel()
		if ctx.Err() != nil {
			t.Error("job cancellation cancelled cleanup")
		}
		return errors.New("incomplete pass")
	}
	req := StartRunRequest{RunID: types.NewRunID(), JobID: types.NewJobID(), JobType: types.JobTypePreGate}
	slot := testRunSlot(t, r)
	if err := r.StartRun(ctx, req, slot); err != nil {
		t.Fatal(err)
	}
	select {
	case body := <-completed:
		if !strings.Contains(body, `"status":"Cancelled"`) {
			t.Fatalf("completion did not preserve cancellation: %s", body)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("missing cancellation completion")
	}
	waitCleanupIdle(t, r)
	if calls.Load() != 2 {
		t.Fatalf("before/after hook calls=%d", calls.Load())
	}
	if _, err := os.Stat(runDir(req.RunID)); !os.IsNotExist(err) {
		t.Fatalf("execution wrote run directories: %v", err)
	}
}

func waitCleanupIdle(t *testing.T, r *runController) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !r.nodeIsIdle() {
		if time.Now().After(deadline) {
			t.Fatal("controller did not release job and permit")
		}
		time.Sleep(time.Millisecond)
	}
}

// Recovered finalization holds shared protection and reaches the after-job hook.
func TestRecoveredFinalizationRunsCleanupAfterUpload(t *testing.T) {
	file := isolatedMaintenanceLock(t)
	uploading, release := make(chan struct{}), make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if strings.HasSuffix(req.URL.Path, "/complete") {
			assertMaintenanceBlocked(t, file)
			close(uploading)
			<-release
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	r := newTestController(t, newAgentConfig(srv.URL))
	var calls atomic.Int32
	r.cleanupPass = func(context.Context) error { calls.Add(1); return nil }
	c := setupClaimer(t, r.cfg, r)
	c.startupReconciler = &startupCrashReconciler{docker: &fakeDockerClient{
		inspectByID: map[string]client.ContainerInspectResult{
			"recovered": inspectWithState(false, containertypes.StateExited, time.Now().Format(time.RFC3339Nano)),
		},
	}}
	slot := testRunSlot(t, r)
	done := make(chan struct{})
	go func() {
		defer close(done)
		c.monitorRecoveredRunningContainer(context.Background(), recoveredContainer{ContainerID: "recovered", RunID: types.NewRunID(), JobID: types.NewJobID()}, slot)
	}()
	<-uploading
	if calls.Load() != 0 {
		t.Error("cleanup preceded finalization")
	}
	close(release)
	<-done
	if calls.Load() != 1 || len(r.jobSem) != 0 {
		t.Fatalf("after recovery calls=%d permits=%d", calls.Load(), len(r.jobSem))
	}
}

// Shutdown cancels an in-progress pass without recording a successful cleanup.
func TestCleanupStopsOnNodeShutdown(t *testing.T) {
	isolatedMaintenanceLock(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started, done := make(chan struct{}), make(chan struct{})
	r := &runController{nodeCtx: ctx, cleanupPass: func(ctx context.Context) error {
		if _, ok := ctx.Deadline(); !ok {
			t.Error("cleanup context has no bound")
		}
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}}
	go func() { defer close(done); r.tryCleanup() }()
	<-started
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cleanup ignored node shutdown")
	}
	if r.cleanupRunning || !r.lastCleanupSuccess.IsZero() {
		t.Fatal("shutdown left running state or started cooldown")
	}
}

// A direct start cannot take the only permit before recovery attaches monitors.
func TestDirectStartWaitsBeforeAcquiringRecoveryPermit(t *testing.T) {
	controller := &mockRunController{slotSem: make(chan struct{}, 1)}
	c := setupClaimer(t, newAgentConfig("http://127.0.0.1:8080"), controller)
	waiting := make(chan struct{})
	s := &Server{controller: controller, waitStartup: func(ctx context.Context) error {
		close(waiting)
		return c.waitStartup(ctx)
	}}
	start := newStartRunRequest()
	start.RunID, start.JobID, start.RepoID = types.NewRunID(), types.NewJobID(), types.NewRepoID()
	start.JobType = types.JobTypePreGate
	body, err := json.Marshal(start)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/runs/start", strings.NewReader(string(body)))
	result, done := httptest.NewRecorder(), make(chan struct{})
	go func() { defer close(done); s.handleRunStart(result, req) }()
	<-waiting
	if controller.AcquireCalls() != 0 {
		t.Fatal("direct start took a permit during reconciliation")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	recoverySlot, err := controller.AcquireSlot(ctx)
	if err != nil {
		t.Fatalf("recovery could not obtain its permit: %v", err)
	}
	controller.FinishRunSlot(recoverySlot)
	if err := c.runStartupReconcile(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("direct start did not resume after reconciliation")
	}
	if result.Code != http.StatusAccepted {
		t.Fatalf("direct start status=%d: %s", result.Code, result.Body)
	}
	controller.ReleaseSlot(&RunSlot{})
}

// Recovery above capacity must release completed permits while startup still holds its lock.
func TestRecoveredHooksDoNotWaitForStartupCompletion(t *testing.T) {
	isolatedMaintenanceLock(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	cfg := newAgentConfig(srv.URL)
	cfg.Concurrency = 1
	r := newTestController(t, cfg)
	var calls atomic.Int32
	r.cleanupPass = func(context.Context) error { calls.Add(1); return nil }
	c := setupClaimer(t, cfg, r)
	docker := &fakeDockerClient{inspectByID: make(map[string]client.ContainerInspectResult)}
	for _, name := range []string{"first", "second"} {
		docker.listResult.Items = append(docker.listResult.Items, containertypes.Summary{
			ID: name, Labels: map[string]string{types.LabelRunID: types.NewRunID().String(), types.LabelJobID: types.NewJobID().String()},
		})
		docker.inspectByID[name] = inspectWithState(true, containertypes.StateRunning, "")
	}
	c.startupReconciler = &startupCrashReconciler{docker: docker}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := c.runStartupReconcile(ctx); err != nil {
		t.Fatal(err)
	}
	waitCleanupIdle(t, r)
	if err := c.waitStartup(ctx); err != nil {
		t.Fatal(err)
	}
}

// Both admission paths run hooks around execution; empty claims perform no cleanup.
func TestJobAdmissionRunsCleanupHooks(t *testing.T) {
	for _, mode := range []string{"claim", "direct", "empty"} {
		t.Run(mode, func(t *testing.T) {
			isolatedMaintenanceLock(t)
			t.Setenv("PLOYD_CACHE_HOME", t.TempDir())
			t.Setenv("DOCKER_HOST", "invalid://cleanup-hook-test")
			claim := newClaimResponse()
			claim.JobType = types.JobTypePreGate
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				switch {
				case strings.HasSuffix(req.URL.Path, "/claim"):
					if mode == "empty" {
						w.WriteHeader(http.StatusNoContent)
						return
					}
					_ = json.NewEncoder(w).Encode(claim)
				case strings.HasSuffix(req.URL.Path, "/artifact"):
					w.WriteHeader(http.StatusCreated)
					_, _ = w.Write([]byte(`{"artifact_bundle_id":"test","cid":"test"}`))
				case strings.HasSuffix(req.URL.Path, "/status"):
					_, _ = w.Write([]byte(`{"status":"Running"}`))
				default:
					w.WriteHeader(http.StatusOK)
				}
			}))
			defer srv.Close()
			r := newTestController(t, newAgentConfig(srv.URL))
			var calls atomic.Int32
			r.cleanupPass = func(context.Context) error {
				count := calls.Add(1)
				path := runDir(claim.RunID)
				if count == 2 {
					path = jobDirectories(claim.RunID, claim.JobID).Tmp
				}
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Errorf("hook %d saw unexpected job writes at %s: %v", count, path, err)
				}
				return errors.New("retry at next boundary")
			}
			if mode == "direct" {
				s := &Server{cfg: r.cfg, controller: r}
				body, err := json.Marshal(StartRunRequest{RunID: claim.RunID, JobID: claim.JobID, RepoID: claim.RepoID, RepoURL: claim.RepoURL, JobType: claim.JobType})
				if err != nil {
					t.Fatal(err)
				}
				result := httptest.NewRecorder()
				s.handleRunStart(result, httptest.NewRequest(http.MethodPost, "/v1/runs/start", strings.NewReader(string(body))))
				if result.Code != http.StatusAccepted {
					t.Fatalf("start: %d %s", result.Code, result.Body)
				}
			} else {
				c := setupClaimer(t, r.cfg, r)
				claimed, err := c.claimAndExecute(context.Background())
				if err != nil || claimed != (mode == "claim") {
					t.Fatalf("claimed=%v err=%v", claimed, err)
				}
			}
			waitCleanupIdle(t, r)
			want := int32(2)
			if mode == "empty" {
				want = 0
			}
			if calls.Load() != want {
				t.Fatalf("cleanup calls=%d want=%d", calls.Load(), want)
			}
		})
	}
}
