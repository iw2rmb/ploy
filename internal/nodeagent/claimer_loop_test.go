package nodeagent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/backoff"
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
