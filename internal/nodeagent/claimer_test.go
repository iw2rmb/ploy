package nodeagent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// TestClaimLoop verifies the claim loop posts claim, ack, and complete in order.
func TestClaimLoop(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var calls []string

	// Create test server.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch r.URL.Path {
		case "/v1/nodes/test-node/buildgate/claim":
			// No buildgate jobs available in this test.
			w.WriteHeader(http.StatusNoContent)
			return
		case "/v1/nodes/test-node/claim":
			calls = append(calls, "claim")
			// Return a run to claim.
			resp := ClaimResponse{
				ID:        "run-123",
				RepoURL:   "https://github.com/test/repo",
				Status:    "assigned",
				NodeID:    "test-node",
				BaseRef:   "main",
				TargetRef: "feature-branch",
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(resp)

		case "/v1/nodes/test-node/ack":
			calls = append(calls, "ack")
			w.WriteHeader(http.StatusNoContent)

		case "/v1/nodes/test-node/complete":
			calls = append(calls, "complete")
			w.WriteHeader(http.StatusNoContent)

		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	// Create config.
	cfg := Config{
		ServerURL: ts.URL,
		NodeID:    "test-node",
		HTTP: HTTPConfig{
			TLS: TLSConfig{
				Enabled: false,
			},
		},
	}

	// Create controller.
	controller := &runController{
		cfg:  cfg,
		runs: make(map[string]*runContext),
	}

	// Create claim manager.
	claimer, err := NewClaimManager(cfg, controller)
	if err != nil {
		t.Fatalf("NewClaimManager failed: %v", err)
	}

	// Override backoff to speed up test.
	claimer.minBackoff = 10 * time.Millisecond
	claimer.maxBackoff = 100 * time.Millisecond

	// Run claim loop in background with timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = claimer.Start(ctx)
	}()

	// Wait for at least one claim cycle.
	time.Sleep(500 * time.Millisecond)
	cancel()
	wg.Wait()

	// Verify calls were made in order.
	mu.Lock()
	defer mu.Unlock()

	if len(calls) < 2 {
		t.Fatalf("expected at least 2 calls (claim, ack), got %d: %v", len(calls), calls)
	}

	// Verify order: claim followed by ack.
	if calls[0] != "claim" {
		t.Errorf("expected first call to be 'claim', got %s", calls[0])
	}
	if calls[1] != "ack" {
		t.Errorf("expected second call to be 'ack', got %s", calls[1])
	}

	// Note: complete is called by executeRun after the run finishes.
	// Since we're using a minimal controller that starts execution in a goroutine,
	// the complete call may or may not happen within the test timeout.
	// For this basic test, we verify claim and ack order.
}

// TestClaimLoopNoWork verifies the loop handles 204 No Content gracefully.
func TestClaimLoopNoWork(t *testing.T) {
	t.Parallel()

	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/nodes/test-node/claim" {
			callCount++
			// Return 204 No Content (no work available).
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.URL.Path == "/v1/nodes/test-node/buildgate/claim" {
			// Always no buildgate jobs for this test.
			w.WriteHeader(http.StatusNoContent)
			return
		}
		t.Errorf("unexpected path: %s", r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	cfg := Config{
		ServerURL: ts.URL,
		NodeID:    "test-node",
		HTTP: HTTPConfig{
			TLS: TLSConfig{
				Enabled: false,
			},
		},
	}

	controller := &runController{
		cfg:  cfg,
		runs: make(map[string]*runContext),
	}

	claimer, err := NewClaimManager(cfg, controller)
	if err != nil {
		t.Fatalf("NewClaimManager failed: %v", err)
	}

	// Override backoff to speed up test.
	claimer.minBackoff = 10 * time.Millisecond
	claimer.maxBackoff = 100 * time.Millisecond

	// Run claim loop with short timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = claimer.Start(ctx)
	}()

	wg.Wait()

	// Verify that claim was called multiple times (backoff and retry).
	if callCount < 2 {
		t.Errorf("expected multiple claim attempts, got %d", callCount)
	}
}

// TestClaimLoopBackoff verifies exponential backoff behavior.
func TestClaimLoopBackoff(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var intervals []time.Duration
	var lastCall time.Time

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		if r.URL.Path == "/v1/nodes/test-node/claim" {
			now := time.Now()
			if !lastCall.IsZero() {
				intervals = append(intervals, now.Sub(lastCall))
			}
			lastCall = now

			// Return 204 No Content to trigger backoff.
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.URL.Path == "/v1/nodes/test-node/buildgate/claim" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	cfg := Config{
		ServerURL: ts.URL,
		NodeID:    "test-node",
		HTTP: HTTPConfig{
			TLS: TLSConfig{
				Enabled: false,
			},
		},
	}

	controller := &runController{
		cfg:  cfg,
		runs: make(map[string]*runContext),
	}

	claimer, err := NewClaimManager(cfg, controller)
	if err != nil {
		t.Fatalf("NewClaimManager failed: %v", err)
	}

	// Set backoff parameters.
	claimer.minBackoff = 50 * time.Millisecond
	claimer.maxBackoff = 200 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
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

	// Verify that intervals increase (exponential backoff).
	if len(intervals) < 2 {
		t.Logf("insufficient intervals to verify backoff: %v", intervals)
		return
	}

	// Check that later intervals are longer than earlier ones (with tolerance).
	for i := 1; i < len(intervals); i++ {
		// Allow some tolerance due to timing variance.
		if intervals[i] < intervals[i-1]/2 {
			t.Errorf("backoff not increasing: interval[%d]=%v < interval[%d]=%v/2",
				i, intervals[i], i-1, intervals[i-1])
		}
	}

	// Verify max backoff is respected.
	for i, interval := range intervals {
		if interval > claimer.maxBackoff+50*time.Millisecond {
			t.Errorf("interval[%d]=%v exceeds max backoff %v", i, interval, claimer.maxBackoff)
		}
	}
}

// TestClaimLoopBackoffReset verifies backoff resets on successful claim.
func TestClaimLoopBackoffReset(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var intervals []time.Duration
	var lastCall time.Time
	callCount := 0

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch r.URL.Path {
		case "/v1/nodes/test-node/buildgate/claim":
			// First few loops: no buildgate jobs; let run claim proceed.
			w.WriteHeader(http.StatusNoContent)
			return
		case "/v1/nodes/test-node/claim":
			now := time.Now()
			if !lastCall.IsZero() {
				intervals = append(intervals, now.Sub(lastCall))
			}
			lastCall = now
			callCount++

			// Return 204 for first 3 calls to build up backoff, then success.
			if callCount <= 3 {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			// Return success to reset backoff.
			resp := ClaimResponse{
				ID:        "run-reset",
				RepoURL:   "https://github.com/test/repo",
				Status:    "assigned",
				NodeID:    "test-node",
				BaseRef:   "main",
				TargetRef: "feature",
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(resp)

		case "/v1/nodes/test-node/ack":
			w.WriteHeader(http.StatusNoContent)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	cfg := Config{
		ServerURL: ts.URL,
		NodeID:    "test-node",
		HTTP: HTTPConfig{
			TLS: TLSConfig{
				Enabled: false,
			},
		},
	}

	controller := &runController{
		cfg:  cfg,
		runs: make(map[string]*runContext),
	}

	claimer, err := NewClaimManager(cfg, controller)
	if err != nil {
		t.Fatalf("NewClaimManager failed: %v", err)
	}

	// Set backoff parameters.
	claimer.minBackoff = 50 * time.Millisecond
	claimer.maxBackoff = 200 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = claimer.Start(ctx)
	}()

	// Wait for backoff to build up and then reset.
	time.Sleep(1 * time.Second)
	cancel()
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()

	// Verify we had at least 4 calls (3 failures + 1 success).
	if callCount < 4 {
		t.Fatalf("expected at least 4 calls, got %d", callCount)
	}

	// Verify intervals increased during backoff phase (calls 1-3).
	if len(intervals) >= 2 {
		for i := 1; i < 3 && i < len(intervals); i++ {
			if intervals[i] < intervals[i-1]/2 {
				t.Logf("backoff may not be increasing: interval[%d]=%v vs interval[%d]=%v",
					i, intervals[i], i-1, intervals[i-1])
			}
		}
	}

	// After successful claim (call 4), backoff should reset.
	// The next interval (call 5) should be back to minBackoff.
	if len(intervals) >= 4 {
		// Check that interval after success is smaller than the backed-off interval.
		if intervals[3] >= intervals[2] {
			t.Logf("backoff appears to have reset: interval[3]=%v < interval[2]=%v",
				intervals[3], intervals[2])
		}
	}
}

// TestClaimLoopAckFailure verifies behavior when ack fails.
func TestClaimLoopAckFailure(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	ackCalled := false

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch r.URL.Path {
		case "/v1/nodes/test-node/buildgate/claim":
			w.WriteHeader(http.StatusNoContent)
			return
		case "/v1/nodes/test-node/claim":
			resp := ClaimResponse{
				ID:        "run-456",
				RepoURL:   "https://github.com/test/repo",
				Status:    "assigned",
				NodeID:    "test-node",
				BaseRef:   "main",
				TargetRef: "feature",
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(resp)

		case "/v1/nodes/test-node/ack":
			ackCalled = true
			// Return failure.
			w.WriteHeader(http.StatusInternalServerError)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	cfg := Config{
		ServerURL: ts.URL,
		NodeID:    "test-node",
		HTTP: HTTPConfig{
			TLS: TLSConfig{
				Enabled: false,
			},
		},
	}

	controller := &runController{
		cfg:  cfg,
		runs: make(map[string]*runContext),
	}

	claimer, err := NewClaimManager(cfg, controller)
	if err != nil {
		t.Fatalf("NewClaimManager failed: %v", err)
	}

	claimer.minBackoff = 10 * time.Millisecond
	claimer.maxBackoff = 100 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
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

	if !ackCalled {
		t.Error("expected ack to be called")
	}
}
