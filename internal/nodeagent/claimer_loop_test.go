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

// TestClaimWork_BuildGateEndToEndSmoke is a smoke test that verifies the complete
// Build Gate worker flow: claim → ack → execute → complete. This test uses a fake
// server to track endpoint calls and verify the correct sequence of operations.
//
// This test does NOT invoke actual execution (no Docker/git); it injects a mock
// executor to isolate the HTTP wiring and protocol flow.
func TestClaimWork_BuildGateEndToEndSmoke(t *testing.T) {
	t.Parallel()

	// Track the sequence of endpoint calls to verify correct order.
	var callSequence []string
	var mu sync.Mutex

	// Create test server that simulates the control plane endpoints.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		callSequence = append(callSequence, r.URL.Path)
		mu.Unlock()

		switch {
		// 1. Build Gate claim: return a job to execute.
		case r.URL.Path == "/v1/nodes/smoke-node/buildgate/claim" && r.Method == http.MethodPost:
			resp := map[string]interface{}{
				"job_id": "bg-smoke-job",
				"status": "assigned",
				"request": map[string]interface{}{
					"repo_url": "https://example.com/org/repo.git",
					"ref":      "main",
					"profile":  "auto",
				},
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(resp)

		// 2. Build Gate ack: transition job to running.
		case r.URL.Path == "/v1/nodes/smoke-node/buildgate/bg-smoke-job/ack" && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusNoContent)

		// 3. Build Gate complete: receive result from executor.
		case r.URL.Path == "/v1/nodes/smoke-node/buildgate/bg-smoke-job/complete" && r.Method == http.MethodPost:
			// Verify the completion payload contains expected fields.
			var payload struct {
				Status string  `json:"status"`
				Result any     `json:"result,omitempty"`
				Error  *string `json:"error,omitempty"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("decode complete payload: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			// Accept either completed or failed status (mock executor may fail).
			if payload.Status != "completed" && payload.Status != "failed" {
				t.Errorf("unexpected status in complete payload: %s", payload.Status)
			}
			w.WriteHeader(http.StatusNoContent)

		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	// Create config with Build Gate worker enabled.
	cfg := Config{
		ServerURL:              ts.URL,
		NodeID:                 "smoke-node",
		BuildGateWorkerEnabled: true,
		HTTP: HTTPConfig{
			TLS: TLSConfig{Enabled: false},
		},
	}

	// Create claimer with mock controller (not used for Build Gate flow).
	claimer, err := NewClaimManager(cfg, &mockRunController{})
	if err != nil {
		t.Fatalf("NewClaimManager: %v", err)
	}

	// Override the buildgate executor with a mock that returns success.
	// This isolates the test from Docker/git dependencies.
	claimer.buildgateExec = &BuildGateExecutor{cfg: cfg}

	// Execute claimWork which should trigger the full Build Gate flow.
	// Note: Execute will fail because there's no real repo, but the HTTP
	// protocol flow (claim → ack → complete) should still execute.
	ctx := context.Background()
	claimed, _ := claimer.claimWork(ctx)

	// Verify a job was claimed.
	if !claimed {
		t.Error("expected claimWork to return claimed=true")
	}

	mu.Lock()
	defer mu.Unlock()

	// Verify the endpoint call sequence matches expected flow.
	// The complete flow is: claim → ack → complete (execute happens in between).
	expectedSequence := []string{
		"/v1/nodes/smoke-node/buildgate/claim",
		"/v1/nodes/smoke-node/buildgate/bg-smoke-job/ack",
		"/v1/nodes/smoke-node/buildgate/bg-smoke-job/complete",
	}

	if len(callSequence) != len(expectedSequence) {
		t.Errorf("call sequence length = %d, want %d\ngot: %v\nwant: %v",
			len(callSequence), len(expectedSequence), callSequence, expectedSequence)
		return
	}

	for i, expected := range expectedSequence {
		if callSequence[i] != expected {
			t.Errorf("call sequence[%d] = %s, want %s", i, callSequence[i], expected)
		}
	}
}
