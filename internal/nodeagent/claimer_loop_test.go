package nodeagent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	containertypes "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

// Note: mockRunController is defined in handlers_test.go within the same package.

// TestClaimLoop_OnlyUnifiedEndpoint verifies that no separate Build Gate claim
// endpoint is called — all jobs come from the single unified queue.
func TestClaimLoop_OnlyUnifiedEndpoint(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var unifiedClaims int
	var unexpectedPaths []string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch r.URL.Path {
		case "/v1/nodes/" + testNodeID + "/claim":
			unifiedClaims++
			w.WriteHeader(http.StatusNoContent)
		default:
			unexpectedPaths = append(unexpectedPaths, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	claimer := setupClaimer(t, newTestConfig(ts.URL), &mockRunController{})
	runClaimerUntil(t, claimer, 200*time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if unifiedClaims == 0 {
		t.Error("unified claim endpoint not called")
	}
	if len(unexpectedPaths) > 0 {
		t.Errorf("unexpected paths called: %v", unexpectedPaths)
	}
}

func TestClaimAndExecute_PreClaimCleanupBlocksClaim(t *testing.T) {
	t.Parallel()

	var claimCount int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/nodes/"+testNodeID+"/claim" {
			claimCount++
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	controller := &mockRunController{}
	claimer := setupClaimer(t, newTestConfig(ts.URL), controller)
	claimer.preClaimCleanup = preClaimCleanupFunc(func(context.Context) (bool, error) {
		return false, nil
	})

	claimed, err := claimer.claimAndExecute(context.Background())
	if err != nil {
		t.Fatalf("claimAndExecute() error = %v, want nil", err)
	}
	if claimed {
		t.Fatalf("claimAndExecute() claimed = true, want false")
	}
	if claimCount != 0 {
		t.Fatalf("claim endpoint called %d times, want 0", claimCount)
	}
	if controller.acquireCalls != 0 {
		t.Fatalf("AcquireSlot calls = %d, want 0", controller.acquireCalls)
	}
	if controller.releaseCalls != 0 {
		t.Fatalf("ReleaseSlot calls = %d, want 0", controller.releaseCalls)
	}
}

func TestClaimAndExecute_PreClaimCleanupAllowsClaim(t *testing.T) {
	t.Parallel()

	var claimCount int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/nodes/"+testNodeID+"/claim" {
			claimCount++
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	controller := &mockRunController{}
	claimer := setupClaimer(t, newTestConfig(ts.URL), controller)
	claimer.preClaimCleanup = preClaimCleanupFunc(func(context.Context) (bool, error) {
		return true, nil
	})

	claimed, err := claimer.claimAndExecute(context.Background())
	if err != nil {
		t.Fatalf("claimAndExecute() error = %v, want nil", err)
	}
	if claimed {
		t.Fatalf("claimAndExecute() claimed = true, want false (no work)")
	}
	if claimCount != 1 {
		t.Fatalf("claim endpoint called %d times, want 1", claimCount)
	}
	if controller.acquireCalls != 1 {
		t.Fatalf("AcquireSlot calls = %d, want 1", controller.acquireCalls)
	}
	if controller.releaseCalls != 1 {
		t.Fatalf("ReleaseSlot calls = %d, want 1", controller.releaseCalls)
	}
}

func TestClaimLoop_StartupReconcileBeforeClaim_Contract(t *testing.T) {
	t.Parallel()

	jobID := types.NewJobID()
	var seq int32
	completeSeq := int32(0)
	claimSeq := int32(0)
	claimCount := int32(0)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/jobs/"+jobID.String()+"/complete":
			if atomic.CompareAndSwapInt32(&completeSeq, 0, atomic.AddInt32(&seq, 1)) {
				w.WriteHeader(http.StatusOK)
				return
			}
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/v1/nodes/"+testNodeID+"/claim":
			if atomic.CompareAndSwapInt32(&claimSeq, 0, atomic.AddInt32(&seq, 1)) {
				atomic.AddInt32(&claimCount, 1)
				w.WriteHeader(http.StatusNoContent)
				return
			}
			atomic.AddInt32(&claimCount, 1)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	claimer := setupClaimer(t, newTestConfig(ts.URL), &mockRunController{})
	claimer.startupReconciler = &startupCrashReconciler{
		docker: &fakeCrashReconcileDockerClient{
			listResult: client.ContainerListResult{Items: []containertypes.Summary{
				{
					ID:     "terminal-ctr",
					Labels: map[string]string{types.LabelRunID: types.NewRunID().String(), types.LabelJobID: jobID.String()},
				},
			}},
			inspectByID: map[string]client.ContainerInspectResult{
				"terminal-ctr": {
					Container: containertypes.InspectResponse{
						State: &containertypes.State{
							Running:    false,
							Status:     containertypes.ContainerState("exited"),
							ExitCode:   0,
							StartedAt:  "2026-02-26T12:00:00Z",
							FinishedAt: "2026-02-26T12:00:02Z",
						},
					},
				},
			},
			waitByID: map[string]containertypes.WaitResponse{
				"terminal-ctr": {StatusCode: 0},
			},
		},
		now:            func() time.Time { return time.Date(2026, 2, 26, 12, 0, 10, 0, time.UTC) },
		terminalWindow: 120 * time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 220*time.Millisecond)
	defer cancel()

	if err := claimer.Start(ctx); err == nil || err != context.DeadlineExceeded {
		t.Fatalf("Start() error = %v, want context deadline exceeded", err)
	}

	if atomic.LoadInt32(&completeSeq) == 0 {
		t.Fatal("startup reconciliation completion call was not made")
	}
	if atomic.LoadInt32(&claimCount) == 0 {
		t.Fatal("claim endpoint was not called after startup reconciliation")
	}
	if completeSeq > claimSeq {
		t.Fatalf("startup completion ran after claim: complete_seq=%d claim_seq=%d", completeSeq, claimSeq)
	}
}

func TestClaimLoop_StartupReconcileFailureStopsClaimLoop(t *testing.T) {
	t.Parallel()

	rec := &callRecorder{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.Record(r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	claimer := setupClaimer(t, newTestConfig(ts.URL), &mockRunController{})
	claimer.startupReconciler = &startupCrashReconciler{
		docker: &fakeCrashReconcileDockerClient{
			listErr: context.DeadlineExceeded,
		},
	}

	err := claimer.Start(context.Background())
	if err == nil {
		t.Fatal("Start() error = nil, want startup reconciliation error")
	}
	if paths := rec.All(); len(paths) != 0 {
		body, _ := json.Marshal(paths)
		t.Fatalf("claim loop should not run when startup reconciliation fails, got paths: %s", string(body))
	}
}

func TestClaimLoop_StartupReconcileRunsOncePerProcess(t *testing.T) {
	t.Parallel()

	claimer := setupClaimer(t, newTestConfig("http://127.0.0.1:8080"), &mockRunController{})

	fakeDocker := &fakeCrashReconcileDockerClient{
		listResult: client.ContainerListResult{},
	}
	claimer.startupReconciler = &startupCrashReconciler{docker: fakeDocker}

	if err := claimer.runStartupReconcile(context.Background()); err != nil {
		t.Fatalf("runStartupReconcile() first call error = %v", err)
	}
	if err := claimer.runStartupReconcile(context.Background()); err != nil {
		t.Fatalf("runStartupReconcile() second call error = %v", err)
	}
	if fakeDocker.listCalls != 1 {
		t.Fatalf("ContainerList calls = %d, want 1", fakeDocker.listCalls)
	}
}
