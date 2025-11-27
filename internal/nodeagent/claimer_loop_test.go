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

	"github.com/iw2rmb/ploy/internal/workflow/backoff"
)

// Note: mockRunController is defined in handlers_test.go within the same package.

// TestClaimWork_BuildGateWorkerDisabled verifies that when BuildGateWorkerEnabled
// is false, claimWork skips the Build Gate claim path and only attempts run claims.
// This test injects a fake ClaimManager with BuildGateWorkerEnabled=false and asserts
// that only the run-claim endpoint is called.
func TestClaimWork_BuildGateWorkerDisabled(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var buildGateClaims, runClaims int

	// Create test server that tracks which endpoints are called.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch r.URL.Path {
		case "/v1/nodes/test-node/buildgate/claim":
			// Track Build Gate claim attempts.
			buildGateClaims++
			w.WriteHeader(http.StatusNoContent)
		case "/v1/nodes/test-node/claim":
			// Track run claim attempts; return 204 (no work available).
			runClaims++
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	// Config with BuildGateWorkerEnabled=false.
	cfg := Config{
		ServerURL:              ts.URL,
		NodeID:                 "test-node",
		BuildGateWorkerEnabled: false, // Build Gate worker disabled.
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

	// Verify Build Gate claim was NOT called (worker disabled).
	if buildGateClaims > 0 {
		t.Errorf("buildgate/claim called %d times, expected 0 (worker disabled)", buildGateClaims)
	}

	// Verify run claim WAS called (at least once).
	if runClaims == 0 {
		t.Error("run claim not called, expected at least 1 attempt")
	}
}

// TestClaimWork_BuildGateWorkerEnabled verifies that when BuildGateWorkerEnabled
// is true, claimWork attempts both Build Gate and run claims.
func TestClaimWork_BuildGateWorkerEnabled(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var buildGateClaims, runClaims int

	// Create test server that tracks which endpoints are called.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch r.URL.Path {
		case "/v1/nodes/test-node/buildgate/claim":
			// Track Build Gate claim attempts; return 204 (no jobs available).
			buildGateClaims++
			w.WriteHeader(http.StatusNoContent)
		case "/v1/nodes/test-node/claim":
			// Track run claim attempts; return 204 (no work available).
			runClaims++
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	// Config with BuildGateWorkerEnabled=true.
	cfg := Config{
		ServerURL:              ts.URL,
		NodeID:                 "test-node",
		BuildGateWorkerEnabled: true, // Build Gate worker enabled.
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

	// Verify Build Gate claim WAS called (worker enabled).
	if buildGateClaims == 0 {
		t.Error("buildgate/claim not called, expected at least 1 attempt (worker enabled)")
	}

	// Verify run claim WAS called (fallback after no Build Gate jobs).
	if runClaims == 0 {
		t.Error("run claim not called, expected at least 1 attempt")
	}
}

// TestClaimWork_BuildGateClaimedSkipsRunClaim verifies that when a Build Gate job
// is claimed successfully (via direct claimWork call), the run claim path is NOT
// executed in that same call.
func TestClaimWork_BuildGateClaimedSkipsRunClaim(t *testing.T) {
	t.Parallel()

	var buildGateClaims, runClaims int32

	// Create test server that returns a Build Gate job on first claim.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/nodes/test-node/buildgate/claim":
			atomic.AddInt32(&buildGateClaims, 1)
			// Return a Build Gate job.
			resp := map[string]interface{}{
				"job_id": "bg-job-123",
				"request": map[string]interface{}{
					"repo_url": "https://example.com/test/repo.git",
					"ref":      "main",
					"profile":  "auto",
				},
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(resp)
		case "/v1/nodes/test-node/buildgate/bg-job-123/ack":
			// Ack the Build Gate job.
			w.WriteHeader(http.StatusNoContent)
		case "/v1/nodes/test-node/buildgate/bg-job-123/complete":
			// Complete the Build Gate job.
			w.WriteHeader(http.StatusNoContent)
		case "/v1/nodes/test-node/claim":
			// Track run claim attempts.
			atomic.AddInt32(&runClaims, 1)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	// Config with BuildGateWorkerEnabled=true.
	cfg := Config{
		ServerURL:              ts.URL,
		NodeID:                 "test-node",
		BuildGateWorkerEnabled: true, // Build Gate worker enabled.
		HTTP: HTTPConfig{
			TLS: TLSConfig{Enabled: false},
		},
	}

	claimer, err := NewClaimManager(cfg, &mockRunController{})
	if err != nil {
		t.Fatalf("NewClaimManager: %v", err)
	}

	// Call claimWork directly to verify single-invocation behavior.
	ctx := context.Background()
	claimed, err := claimer.claimWork(ctx)

	// Verify Build Gate job was claimed (execution will fail, but claim happens).
	if !claimed {
		t.Error("claimWork should have claimed Build Gate job")
	}

	finalBuildGateClaims := atomic.LoadInt32(&buildGateClaims)
	finalRunClaims := atomic.LoadInt32(&runClaims)

	// Verify Build Gate claim was called.
	if finalBuildGateClaims != 1 {
		t.Errorf("buildgate/claim called %d times, expected 1", finalBuildGateClaims)
	}

	// Verify run claim was NOT called when Build Gate job was claimed.
	// When Build Gate returns a job, we short-circuit and don't try run claim.
	if finalRunClaims != 0 {
		t.Errorf("run/claim called %d times, expected 0 (Build Gate job was claimed)", finalRunClaims)
	}

	// Error is expected because Execute will fail (no actual repo to clone),
	// but the key assertion is that run claim was skipped.
	if err == nil {
		t.Log("note: claimWork returned no error (unexpected but acceptable)")
	}
}

// TestClaimWork_DirectInvocation tests claimWork directly (not via Start loop)
// to verify the guard logic in isolation.
func TestClaimWork_DirectInvocation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		workerEnabled   bool
		wantBuildGate   bool
		wantRunClaim    bool
		buildGateReturn int
		runClaimReturn  int
	}{
		{
			name:            "worker disabled skips buildgate",
			workerEnabled:   false,
			wantBuildGate:   false,
			wantRunClaim:    true,
			buildGateReturn: http.StatusNoContent,
			runClaimReturn:  http.StatusNoContent,
		},
		{
			name:            "worker enabled calls buildgate then run",
			workerEnabled:   true,
			wantBuildGate:   true,
			wantRunClaim:    true,
			buildGateReturn: http.StatusNoContent, // No Build Gate job, falls through to run.
			runClaimReturn:  http.StatusNoContent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buildGateCalled, runClaimCalled bool

			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/nodes/test-node/buildgate/claim":
					buildGateCalled = true
					w.WriteHeader(tt.buildGateReturn)
				case "/v1/nodes/test-node/claim":
					runClaimCalled = true
					w.WriteHeader(tt.runClaimReturn)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer ts.Close()

			cfg := Config{
				ServerURL:              ts.URL,
				NodeID:                 "test-node",
				BuildGateWorkerEnabled: tt.workerEnabled,
				HTTP:                   HTTPConfig{TLS: TLSConfig{Enabled: false}},
			}

			claimer, err := NewClaimManager(cfg, &mockRunController{})
			if err != nil {
				t.Fatalf("NewClaimManager: %v", err)
			}

			// Call claimWork directly.
			ctx := context.Background()
			_, _ = claimer.claimWork(ctx)

			if buildGateCalled != tt.wantBuildGate {
				t.Errorf("buildgate/claim called=%v, want=%v", buildGateCalled, tt.wantBuildGate)
			}
			if runClaimCalled != tt.wantRunClaim {
				t.Errorf("run/claim called=%v, want=%v", runClaimCalled, tt.wantRunClaim)
			}
		})
	}
}
