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
	"github.com/iw2rmb/ploy/internal/workflow/backoff"
	containertypes "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

// Note: mockRunController is defined in handlers_test.go within the same package.

type preClaimCleanupFunc func(context.Context) (bool, error)

func (f preClaimCleanupFunc) EnsureCapacity(ctx context.Context) (bool, error) {
	return f(ctx)
}

// TestClaimLoop_UnifiedQueue verifies that the claim loop polls the single unified
// jobs queue (POST /v1/nodes/{id}/claim) and properly handles 204 No Content.
// There is no separate Build Gate queue — all job types are claimed from the same endpoint.
func TestClaimLoop_UnifiedQueue(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var claimCount int

	// Create test server that tracks claim attempts on the unified queue.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch r.URL.Path {
		case "/v1/nodes/" + testNodeID + "/claim":
			// Track unified claim attempts; return 204 (no work available).
			claimCount++
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	cfg := Config{
		ServerURL: ts.URL,
		NodeID:    testNodeID,
		HTTP: HTTPConfig{
			TLS: TLSConfig{Enabled: false},
		},
	}

	controller := &mockRunController{}
	claimer, err := NewClaimManager(cfg, controller)
	if err != nil {
		t.Fatalf("NewClaimManager: %v", err)
	}
	claimer.preClaimCleanup = noopPreClaimCleanup{}
	installNoopStartupReconciler(claimer)

	// Override backoff policy to speed up test.
	testPolicy := backoff.Policy{
		InitialInterval: types.Duration(10 * time.Millisecond),
		MaxInterval:     types.Duration(50 * time.Millisecond),
		Multiplier:      2.0,
		MaxElapsedTime:  0,
		MaxAttempts:     0,
	}
	claimer.backoff = backoff.NewStatefulBackoff(testPolicy)

	// Run the claim loop briefly.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = claimer.Start(ctx)
	}()
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()

	// Verify claim was called at least once on the unified queue.
	if claimCount == 0 {
		t.Error("claim not called, expected at least 1 attempt on unified queue")
	}
}

// TestClaimLoop_OnlyUnifiedEndpoint verifies that no separate Build Gate claim
// endpoint is called — all jobs come from the single unified queue.
func TestClaimLoop_OnlyUnifiedEndpoint(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var unifiedClaims int
	var unexpectedPaths []string

	// Create test server that tracks all endpoint calls.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch r.URL.Path {
		case "/v1/nodes/" + testNodeID + "/claim":
			// This is the only claim endpoint that should be called.
			unifiedClaims++
			w.WriteHeader(http.StatusNoContent)
		default:
			// Track any unexpected paths (e.g., buildgate/claim).
			unexpectedPaths = append(unexpectedPaths, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	cfg := Config{
		ServerURL: ts.URL,
		NodeID:    testNodeID,
		HTTP: HTTPConfig{
			TLS: TLSConfig{Enabled: false},
		},
	}

	controller := &mockRunController{}
	claimer, err := NewClaimManager(cfg, controller)
	if err != nil {
		t.Fatalf("NewClaimManager: %v", err)
	}
	claimer.preClaimCleanup = noopPreClaimCleanup{}
	installNoopStartupReconciler(claimer)

	// Override backoff policy to speed up test.
	testPolicy := backoff.Policy{
		InitialInterval: types.Duration(10 * time.Millisecond),
		MaxInterval:     types.Duration(50 * time.Millisecond),
		Multiplier:      2.0,
		MaxElapsedTime:  0,
		MaxAttempts:     0,
	}
	claimer.backoff = backoff.NewStatefulBackoff(testPolicy)

	// Run the claim loop briefly.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = claimer.Start(ctx)
	}()
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()

	// Verify unified claim endpoint was called.
	if unifiedClaims == 0 {
		t.Error("unified claim endpoint not called")
	}

	// Verify no unexpected paths were called (e.g., buildgate/claim).
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

	cfg := Config{
		ServerURL: ts.URL,
		NodeID:    testNodeID,
		HTTP: HTTPConfig{
			TLS: TLSConfig{Enabled: false},
		},
	}
	controller := &mockRunController{}
	claimer, err := NewClaimManager(cfg, controller)
	if err != nil {
		t.Fatalf("NewClaimManager: %v", err)
	}
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

	cfg := Config{
		ServerURL: ts.URL,
		NodeID:    testNodeID,
		HTTP: HTTPConfig{
			TLS: TLSConfig{Enabled: false},
		},
	}
	controller := &mockRunController{}
	claimer, err := NewClaimManager(cfg, controller)
	if err != nil {
		t.Fatalf("NewClaimManager: %v", err)
	}
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

	cfg := Config{
		ServerURL: ts.URL,
		NodeID:    testNodeID,
		HTTP:      HTTPConfig{TLS: TLSConfig{Enabled: false}},
	}
	controller := &mockRunController{}
	claimer, err := NewClaimManager(cfg, controller)
	if err != nil {
		t.Fatalf("NewClaimManager: %v", err)
	}
	claimer.preClaimCleanup = noopPreClaimCleanup{}
	claimer.backoff = backoff.NewStatefulBackoff(backoff.Policy{
		InitialInterval: types.Duration(10 * time.Millisecond),
		MaxInterval:     types.Duration(50 * time.Millisecond),
		Multiplier:      2.0,
		MaxElapsedTime:  0,
		MaxAttempts:     0,
	})
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

	var calledPaths []string
	var mu sync.Mutex
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calledPaths = append(calledPaths, r.URL.Path)
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	cfg := Config{
		ServerURL: ts.URL,
		NodeID:    testNodeID,
		HTTP:      HTTPConfig{TLS: TLSConfig{Enabled: false}},
	}
	controller := &mockRunController{}
	claimer, err := NewClaimManager(cfg, controller)
	if err != nil {
		t.Fatalf("NewClaimManager: %v", err)
	}
	claimer.preClaimCleanup = noopPreClaimCleanup{}
	claimer.startupReconciler = &startupCrashReconciler{
		docker: &fakeCrashReconcileDockerClient{
			listErr: context.DeadlineExceeded,
		},
	}

	err = claimer.Start(context.Background())
	if err == nil {
		t.Fatal("Start() error = nil, want startup reconciliation error")
	}
	if len(calledPaths) != 0 {
		body, _ := json.Marshal(calledPaths)
		t.Fatalf("claim loop should not run when startup reconciliation fails, got paths: %s", string(body))
	}
}

func TestClaimLoop_StartupReconcileRunsOncePerProcess(t *testing.T) {
	t.Parallel()

	cfg := Config{
		ServerURL: "http://127.0.0.1:8080",
		NodeID:    testNodeID,
		HTTP:      HTTPConfig{TLS: TLSConfig{Enabled: false}},
	}
	controller := &mockRunController{}
	claimer, err := NewClaimManager(cfg, controller)
	if err != nil {
		t.Fatalf("NewClaimManager: %v", err)
	}
	claimer.preClaimCleanup = noopPreClaimCleanup{}

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
