package nodeagent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/backoff"
)

// Note: mockRunController is defined in handlers_test.go within the same package.

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
		case "/v1/nodes/test-node/claim":
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
		NodeID:    "test-node",
		HTTP: HTTPConfig{
			TLS: TLSConfig{Enabled: false},
		},
	}

	controller := &mockRunController{}
	claimer, err := NewClaimManager(cfg, controller)
	if err != nil {
		t.Fatalf("NewClaimManager: %v", err)
	}

	// Override backoff policy to speed up test.
	testPolicy := backoff.Policy{
		InitialInterval: 10 * time.Millisecond,
		MaxInterval:     50 * time.Millisecond,
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
		case "/v1/nodes/test-node/claim":
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
		NodeID:    "test-node",
		HTTP: HTTPConfig{
			TLS: TLSConfig{Enabled: false},
		},
	}

	controller := &mockRunController{}
	claimer, err := NewClaimManager(cfg, controller)
	if err != nil {
		t.Fatalf("NewClaimManager: %v", err)
	}

	// Override backoff policy to speed up test.
	testPolicy := backoff.Policy{
		InitialInterval: 10 * time.Millisecond,
		MaxInterval:     50 * time.Millisecond,
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
