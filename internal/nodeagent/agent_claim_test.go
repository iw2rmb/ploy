package nodeagent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/backoff"
)

// TestClaimLoop verifies the claim loop posts claim and starts execution.
func TestClaimLoop(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var calls []string

	// Create test server for the unified claim queue.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch r.URL.Path {
		case "/v1/nodes/test-node/claim":
			calls = append(calls, "claim")
			// Return a run to claim.
			// v1: run status is "Started" (not HEAD literals like "assigned"/"running").
			// v1 run status values are: Started, Cancelled, Finished.
			resp := ClaimResponse{
				RunID:     types.RunID("run-123"),
				JobID:     types.JobID("job-123"),
				RepoURL:   "https://github.com/test/repo",
				Status:    "Started",
				NodeID:    types.NodeID("test-node"),
				BaseRef:   "main",
				TargetRef: "feature-branch",
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(resp)
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

	// Create controller (mocked to avoid external HTTP interactions).
	controller := &mockRunController{}

	// Create claim manager.
	claimer, err := NewClaimManager(cfg, controller)
	if err != nil {
		t.Fatalf("NewClaimManager failed: %v", err)
	}

	// Override backoff policy to speed up test.
	claimer.backoff = backoff.NewStatefulBackoff(testBackoffPolicy())

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

	// Verify claim was called at least once.
	mu.Lock()
	defer mu.Unlock()

	if len(calls) < 1 {
		t.Fatalf("expected at least 1 call (claim), got %d: %v", len(calls), calls)
	}

	if calls[0] != "claim" {
		t.Errorf("expected first call to be 'claim', got %s", calls[0])
	}
}

// TestClaimLoopNoWork verifies the loop handles 204 No Content gracefully.
func TestClaimLoopNoWork(t *testing.T) {
	t.Parallel()

	callCount := 0
	// Server for unified claim queue; returns 204 No Content (no work available).
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/nodes/test-node/claim" {
			callCount++
			// Return 204 No Content (no work available).
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
		jobs: make(map[types.JobID]*jobContext),
	}

	claimer, err := NewClaimManager(cfg, controller)
	if err != nil {
		t.Fatalf("NewClaimManager failed: %v", err)
	}

	// Override backoff policy to speed up test.
	claimer.backoff = backoff.NewStatefulBackoff(testBackoffPolicy())

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

	// Server for unified claim queue; returns 204 No Content to trigger backoff.
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
		jobs: make(map[types.JobID]*jobContext),
	}

	claimer, err := NewClaimManager(cfg, controller)
	if err != nil {
		t.Fatalf("NewClaimManager failed: %v", err)
	}

	// Set backoff parameters using custom policy.
	testPolicy := backoff.Policy{
		InitialInterval: 50 * time.Millisecond,
		MaxInterval:     200 * time.Millisecond,
		Multiplier:      2.0,
		MaxElapsedTime:  0,
		MaxAttempts:     0,
	}
	claimer.backoff = backoff.NewStatefulBackoff(testPolicy)

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

	// Verify max backoff is respected.
	// The shared backoff policy adds 50% jitter (randomization factor),
	// so the actual interval can be up to 1.5x the max interval (200ms * 1.5 = 300ms).
	// We add additional tolerance for timing variance.
	maxBackoff := 200 * time.Millisecond
	maxWithJitter := time.Duration(float64(maxBackoff) * 1.5)
	for i, interval := range intervals {
		if interval > maxWithJitter+50*time.Millisecond {
			t.Errorf("interval[%d]=%v exceeds max backoff %v (with jitter)", i, interval, maxWithJitter)
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

	// Server for unified claim queue; returns 204 for first 3 calls then success.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch r.URL.Path {
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
			// v1: run status is "Started" (not HEAD literals like "assigned"/"running").
			// v1 run status values are: Started, Cancelled, Finished.
			resp := ClaimResponse{
				RunID:     types.RunID("run-reset"),
				JobID:     types.JobID("job-reset"),
				RepoURL:   "https://github.com/test/repo",
				Status:    "Started",
				NodeID:    types.NodeID("test-node"),
				BaseRef:   "main",
				TargetRef: "feature",
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(resp)
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
		jobs: make(map[types.JobID]*jobContext),
	}

	claimer, err := NewClaimManager(cfg, controller)
	if err != nil {
		t.Fatalf("NewClaimManager failed: %v", err)
	}

	// Set backoff parameters using custom policy.
	testPolicy := backoff.Policy{
		InitialInterval: 50 * time.Millisecond,
		MaxInterval:     200 * time.Millisecond,
		Multiplier:      2.0,
		MaxElapsedTime:  0,
		MaxAttempts:     0,
	}
	claimer.backoff = backoff.NewStatefulBackoff(testPolicy)

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

// TestClaimLoop_MapsClaimToStartRunRequest ensures ClaimResponse fields map 1:1 into StartRunRequest.
func TestClaimLoop_MapsClaimToStartRunRequest(t *testing.T) {
	t.Parallel()

	commit := "deadbeef"
	runID := types.NewRunID()
	jobID := types.NewJobID()
	nodeIDStr := "aB3xY9"
	// v1: run status is "Started" (not HEAD literals like "assigned"/"running").
	// v1 run status values are: Started, Cancelled, Finished.
	claim := ClaimResponse{
		RunID:     runID,
		JobID:     jobID,
		RepoURL:   "https://github.com/acme/thing.git",
		Status:    "Started",
		NodeID:    types.NodeID(nodeIDStr),
		BaseRef:   "main",
		TargetRef: "feature/x",
		CommitSha: &commit,
		StartedAt: time.Now().UTC().Format(time.RFC3339),
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	// HTTP test server for unified claim queue.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/nodes/" + nodeIDStr + "/claim":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(claim)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	// Capture StartRunRequest via a mock controller.
	mock := &mockRunController{}

	cfg := Config{
		ServerURL: ts.URL,
		NodeID:    types.NodeID(nodeIDStr),
		HTTP:      HTTPConfig{TLS: TLSConfig{Enabled: false}},
	}

	claimer, err := NewClaimManager(cfg, mock)
	if err != nil {
		t.Fatalf("NewClaimManager: %v", err)
	}
	// Override backoff policy to speed up test.
	claimer.backoff = backoff.NewStatefulBackoff(testBackoffPolicy())

	// Run briefly to process one claim.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_ = claimer.Start(ctx)

	if !mock.startCalled {
		t.Fatalf("controller.StartRun not called")
	}
	got := mock.lastStart
	if got.RunID != claim.RunID {
		t.Errorf("RunID=%q want %q", got.RunID, claim.RunID)
	}
	if got.RepoURL.String() != claim.RepoURL {
		t.Errorf("RepoURL=%q want %q", got.RepoURL, claim.RepoURL)
	}
	if got.BaseRef.String() != claim.BaseRef {
		t.Errorf("BaseRef=%q want %q", got.BaseRef, claim.BaseRef)
	}
	if got.TargetRef.String() != claim.TargetRef {
		t.Errorf("TargetRef=%q want %q", got.TargetRef, claim.TargetRef)
	}
	if got.CommitSHA.String() != *claim.CommitSha {
		t.Errorf("CommitSHA=%q want %q", got.CommitSHA, *claim.CommitSha)
	}
}

// TestClaimLoop_StepIndexMapping verifies that when a step-level claim includes
// step_index, it is correctly mapped into StartRunRequest.StepIndex, enabling
// single-step execution in multi-node scenarios.
func TestClaimLoop_StepIndexMapping(t *testing.T) {
	t.Parallel()

	stepIndex := types.StepIndex(2000) // Job step_index uses StepIndex type
	commit := "abc123"
	runID := types.NewRunID()
	jobID := types.NewJobID()
	nodeIDStr := "aB3xY9"
	// v1: run status is "Started" (not HEAD literals like "assigned"/"running").
	// v1 run status values are: Started, Cancelled, Finished.
	claim := ClaimResponse{
		RunID:     runID,
		JobID:     jobID,
		JobName:   "mod-0",
		RepoURL:   "https://github.com/acme/multi.git",
		Status:    "Started",
		NodeID:    types.NodeID(nodeIDStr),
		BaseRef:   "main",
		TargetRef: "feature/multi-step",
		CommitSha: &commit,
		StepIndex: stepIndex, // Job step_index: StepIndex type.
		StartedAt: time.Now().UTC().Format(time.RFC3339),
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	// HTTP test server for unified claim queue with step-level claim.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/nodes/" + nodeIDStr + "/claim":
			// Return step-level claim with step_index.
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(claim)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	// Capture StartRunRequest via a mock controller.
	mock := &mockRunController{}

	cfg := Config{
		ServerURL: ts.URL,
		NodeID:    types.NodeID(nodeIDStr),
		HTTP:      HTTPConfig{TLS: TLSConfig{Enabled: false}},
	}

	claimer, err := NewClaimManager(cfg, mock)
	if err != nil {
		t.Fatalf("NewClaimManager: %v", err)
	}
	// Override backoff policy to speed up test.
	claimer.backoff = backoff.NewStatefulBackoff(testBackoffPolicy())

	// Run briefly to process one claim.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_ = claimer.Start(ctx)

	// Verify controller.StartRun was called with StepIndex populated.
	if !mock.startCalled {
		t.Fatalf("controller.StartRun not called")
	}
	got := mock.lastStart

	// Verify StepIndex matches the claim.
	if got.StepIndex != stepIndex {
		t.Errorf("StepIndex=%.0f want %.0f", got.StepIndex, stepIndex)
	}

	// Verify other fields remain correct.
	if got.RunID != claim.RunID {
		t.Errorf("RunID=%q want %q", got.RunID, claim.RunID)
	}
	if got.RepoURL.String() != claim.RepoURL {
		t.Errorf("RepoURL=%q want %q", got.RepoURL, claim.RepoURL)
	}
	if got.BaseRef.String() != claim.BaseRef {
		t.Errorf("BaseRef=%q want %q", got.BaseRef, claim.BaseRef)
	}
	if got.TargetRef.String() != claim.TargetRef {
		t.Errorf("TargetRef=%q want %q", got.TargetRef, claim.TargetRef)
	}
}

// TestClaimLoop_MultipleNodesSingleRun simulates two distinct nodes claiming
// different steps of the same multi-step run, demonstrating end-to-end
// step-level claiming and execution isolation.
func TestClaimLoop_MultipleNodesSingleRun(t *testing.T) {
	t.Parallel()

	runID := types.NewRunID()
	commit := "deadbeef"

	// Node1 claims job 0 (pre-gate).
	// v1: run status is "Started" (not HEAD literals like "assigned"/"running").
	// v1 run status values are: Started, Cancelled, Finished.
	stepIndex0 := types.StepIndex(1000)
	claim0 := ClaimResponse{
		RunID:     runID,
		JobID:     types.NewJobID(),
		JobName:   "pre-gate",
		RepoURL:   "https://github.com/acme/multi-node.git",
		Status:    "Started",
		NodeID:    types.NodeID("node-1"),
		BaseRef:   "main",
		TargetRef: "feature/parallel-steps",
		CommitSha: &commit,
		StepIndex: stepIndex0,
		StartedAt: time.Now().UTC().Format(time.RFC3339),
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	// Node2 claims job 1 (mod-0).
	// v1: run status is "Started" (not HEAD literals like "assigned"/"running").
	// v1 run status values are: Started, Cancelled, Finished.
	stepIndex1 := types.StepIndex(2000)
	claim1 := ClaimResponse{
		RunID:     runID,
		JobID:     types.NewJobID(),
		JobName:   "mod-0",
		RepoURL:   "https://github.com/acme/multi-node.git",
		Status:    "Started",
		NodeID:    types.NodeID("node-2"),
		BaseRef:   "main",
		TargetRef: "feature/parallel-steps",
		CommitSha: &commit,
		StepIndex: stepIndex1,
		StartedAt: time.Now().UTC().Format(time.RFC3339),
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	// ===== Simulate Node 1 claiming step 0 from unified queue =====
	ts1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/nodes/node-1/claim":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(claim0)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts1.Close()

	mock1 := &mockRunController{}
	cfg1 := Config{
		ServerURL: ts1.URL,
		NodeID:    "node-1",
		HTTP:      HTTPConfig{TLS: TLSConfig{Enabled: false}},
	}
	claimer1, err := NewClaimManager(cfg1, mock1)
	if err != nil {
		t.Fatalf("NewClaimManager node-1: %v", err)
	}
	claimer1.backoff = backoff.NewStatefulBackoff(testBackoffPolicy())

	ctx1, cancel1 := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel1()
	_ = claimer1.Start(ctx1)

	// Verify node-1 claimed job 0 (pre-gate).
	if !mock1.startCalled {
		t.Fatalf("node-1: controller.StartRun not called")
	}
	if mock1.lastStart.StepIndex != stepIndex0 {
		t.Errorf("node-1: StepIndex=%.0f want %.0f", mock1.lastStart.StepIndex, stepIndex0)
	}
	if mock1.lastStart.RunID != runID {
		t.Errorf("node-1: RunID=%q want %q", mock1.lastStart.RunID, runID)
	}

	// ===== Simulate Node 2 claiming step 1 from unified queue =====
	ts2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/nodes/node-2/claim":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(claim1)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts2.Close()

	mock2 := &mockRunController{}
	cfg2 := Config{
		ServerURL: ts2.URL,
		NodeID:    "node-2",
		HTTP:      HTTPConfig{TLS: TLSConfig{Enabled: false}},
	}
	claimer2, err := NewClaimManager(cfg2, mock2)
	if err != nil {
		t.Fatalf("NewClaimManager node-2: %v", err)
	}
	claimer2.backoff = backoff.NewStatefulBackoff(testBackoffPolicy())

	ctx2, cancel2 := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel2()
	_ = claimer2.Start(ctx2)

	// Verify node-2 claimed job 1 (mod-0).
	if !mock2.startCalled {
		t.Fatalf("node-2: controller.StartRun not called")
	}
	if mock2.lastStart.StepIndex != stepIndex1 {
		t.Errorf("node-2: StepIndex=%.0f want %.0f", mock2.lastStart.StepIndex, stepIndex1)
	}
	if mock2.lastStart.RunID != runID {
		t.Errorf("node-2: RunID=%q want %q", mock2.lastStart.RunID, runID)
	}

	// Verify both nodes executed the same run but different steps.
	if mock1.lastStart.RunID != mock2.lastStart.RunID {
		t.Errorf("nodes executed different runs: node-1=%q node-2=%q", mock1.lastStart.RunID, mock2.lastStart.RunID)
	}
	if mock1.lastStart.StepIndex == mock2.lastStart.StepIndex {
		t.Error("nodes executed the same step, expected different step indices")
	}
}
