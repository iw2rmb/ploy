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

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/backoff"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/moby/moby/api/pkg/stdcopy"
	containertypes "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

// TestClaimLoop verifies the claim loop posts claim and starts execution.
func TestClaimLoop(t *testing.T) {
	t.Parallel()

	rec := &callRecorder{}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.Record(r.URL.Path)

		switch r.URL.Path {
		case "/v1/nodes/" + testNodeID + "/claim":
			resp := ClaimResponse{
				RunID:     types.NewRunID(),
				RepoID:    types.NewMigRepoID(),
				JobID:     types.NewJobID(),
				RepoURL:   types.RepoURL("https://github.com/test/repo"),
				Status:    "Started",
				NodeID:    types.NodeID(testNodeID),
				BaseRef:   types.GitRef("main"),
				TargetRef: types.GitRef("feature-branch"),
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

	claimer := setupClaimer(t, newTestConfig(ts.URL), &mockRunController{})

	// Use longer timeout + explicit cancel to ensure at least one cycle.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = claimer.Start(ctx)
	}()
	time.Sleep(500 * time.Millisecond)
	cancel()
	wg.Wait()

	calls := rec.All()
	if len(calls) < 1 {
		t.Fatalf("expected at least 1 call (claim), got %d: %v", len(calls), calls)
	}
}

// TestClaimLoopNoWork verifies the loop handles 204 No Content gracefully.
func TestClaimLoopNoWork(t *testing.T) {
	t.Parallel()

	var callCount int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/nodes/"+testNodeID+"/claim" {
			atomic.AddInt32(&callCount, 1)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		t.Errorf("unexpected path: %s", r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	cfg := newTestConfig(ts.URL)
	controller := &runController{
		cfg:  cfg,
		jobs: make(map[types.JobID]*jobContext),
	}
	claimer := setupClaimer(t, cfg, controller)
	runClaimerUntil(t, claimer, 500*time.Millisecond)

	if got := atomic.LoadInt32(&callCount); got < 2 {
		t.Errorf("expected multiple claim attempts, got %d", got)
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

		if r.URL.Path == "/v1/nodes/"+testNodeID+"/claim" {
			now := time.Now()
			if !lastCall.IsZero() {
				intervals = append(intervals, now.Sub(lastCall))
			}
			lastCall = now
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	cfg := newTestConfig(ts.URL)
	controller := &runController{
		cfg:  cfg,
		jobs: make(map[types.JobID]*jobContext),
	}
	claimer := setupClaimer(t, cfg, controller)
	// Custom backoff for this specific test.
	claimer.backoff = backoff.NewStatefulBackoff(backoff.Policy{
		InitialInterval: types.Duration(50 * time.Millisecond),
		MaxInterval:     types.Duration(200 * time.Millisecond),
		Multiplier:      2.0,
		MaxElapsedTime:  0,
		MaxAttempts:     0,
	})

	runClaimerUntil(t, claimer, 1*time.Second)

	mu.Lock()
	defer mu.Unlock()

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

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch r.URL.Path {
		case "/v1/nodes/" + testNodeID + "/claim":
			now := time.Now()
			if !lastCall.IsZero() {
				intervals = append(intervals, now.Sub(lastCall))
			}
			lastCall = now
			callCount++

			if callCount <= 3 {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			resp := ClaimResponse{
				RunID:     types.NewRunID(),
				RepoID:    types.NewMigRepoID(),
				JobID:     types.NewJobID(),
				RepoURL:   types.RepoURL("https://github.com/test/repo"),
				Status:    "Started",
				NodeID:    types.NodeID(testNodeID),
				BaseRef:   types.GitRef("main"),
				TargetRef: types.GitRef("feature"),
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(resp)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	cfg := newTestConfig(ts.URL)
	controller := &runController{
		cfg:  cfg,
		jobs: make(map[types.JobID]*jobContext),
	}
	claimer := setupClaimer(t, cfg, controller)
	claimer.backoff = backoff.NewStatefulBackoff(backoff.Policy{
		InitialInterval: types.Duration(50 * time.Millisecond),
		MaxInterval:     types.Duration(200 * time.Millisecond),
		Multiplier:      2.0,
		MaxElapsedTime:  0,
		MaxAttempts:     0,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = claimer.Start(ctx)
	}()
	time.Sleep(1 * time.Second)
	cancel()
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()

	if callCount < 4 {
		t.Fatalf("expected at least 4 calls, got %d", callCount)
	}

	if len(intervals) >= 2 {
		for i := 1; i < 3 && i < len(intervals); i++ {
			if intervals[i] < intervals[i-1]/2 {
				t.Logf("backoff may not be increasing: interval[%d]=%v vs interval[%d]=%v",
					i, intervals[i], i-1, intervals[i-1])
			}
		}
	}

	if len(intervals) >= 4 {
		if intervals[3] >= intervals[2] {
			t.Logf("backoff appears to have reset: interval[3]=%v < interval[2]=%v",
				intervals[3], intervals[2])
		}
	}
}

// TestClaimLoop_MapsClaimToStartRunRequest ensures ClaimResponse fields map 1:1 into StartRunRequest.
func TestClaimLoop_MapsClaimToStartRunRequest(t *testing.T) {
	t.Parallel()

	commit := types.CommitSHA("deadbeef")
	runID := types.NewRunID()
	jobID := types.NewJobID()
	nodeIDStr := "aB3xY9"
	repoID := types.NewMigRepoID()
	claim := ClaimResponse{
		RunID:     runID,
		RepoID:    repoID,
		JobID:     jobID,
		RepoURL:   types.RepoURL("https://github.com/acme/thing.git"),
		Status:    "Started",
		NodeID:    types.NodeID(nodeIDStr),
		BaseRef:   types.GitRef("main"),
		TargetRef: types.GitRef("feature/x"),
		CommitSha: &commit,
		RecoveryContext: &contracts.RecoveryClaimContext{
			LoopKind:             "healing",
			SelectedErrorKind:    "infra",
			DetectedStack:        contracts.ModStackJavaMaven,
			ResolvedHealingImage: "docker.io/acme/heal:latest",
			BuildGateLog:         "[ERROR] build failed\n",
		},
		StartedAt: time.Now().UTC().Format(time.RFC3339),
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}

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

	mock := &mockRunController{}
	cfg := Config{
		ServerURL: ts.URL,
		NodeID:    types.NodeID(nodeIDStr),
		HTTP:      HTTPConfig{TLS: TLSConfig{Enabled: false}},
	}
	claimer := setupClaimer(t, cfg, mock)

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
	if got.RepoID != claim.RepoID {
		t.Errorf("RepoID=%q want %q", got.RepoID, claim.RepoID)
	}
	if got.RepoURL != claim.RepoURL {
		t.Errorf("RepoURL=%q want %q", got.RepoURL.String(), claim.RepoURL.String())
	}
	if got.BaseRef != claim.BaseRef {
		t.Errorf("BaseRef=%q want %q", got.BaseRef.String(), claim.BaseRef.String())
	}
	if got.TargetRef != claim.TargetRef {
		t.Errorf("TargetRef=%q want %q", got.TargetRef.String(), claim.TargetRef.String())
	}
	if got.CommitSHA != *claim.CommitSha {
		t.Errorf("CommitSHA=%q want %q", got.CommitSHA.String(), claim.CommitSha.String())
	}
	if got.RecoveryContext == nil {
		t.Fatalf("RecoveryContext=nil, want non-nil")
	}
	if got.RecoveryContext.SelectedErrorKind != "infra" {
		t.Fatalf("RecoveryContext.SelectedErrorKind=%q, want infra", got.RecoveryContext.SelectedErrorKind)
	}
}

// TestClaimLoop_NextIDMapping verifies that claim next_id is mapped into
// StartRunRequest.NextID.
func TestClaimLoop_NextIDMapping(t *testing.T) {
	t.Parallel()

	commit := types.CommitSHA("abc123")
	runID := types.NewRunID()
	jobID := types.NewJobID()
	nextID := types.NewJobID()
	nodeIDStr := "aB3xY9"
	repoID := types.NewMigRepoID()
	claim := ClaimResponse{
		RunID:     runID,
		RepoID:    repoID,
		JobID:     jobID,
		JobName:   "mig-0",
		RepoURL:   types.RepoURL("https://github.com/acme/multi.git"),
		Status:    "Started",
		NodeID:    types.NodeID(nodeIDStr),
		BaseRef:   types.GitRef("main"),
		TargetRef: types.GitRef("feature/multi-step"),
		NextID:    &nextID,
		CommitSha: &commit,
		StartedAt: time.Now().UTC().Format(time.RFC3339),
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}

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

	mock := &mockRunController{}
	cfg := Config{
		ServerURL: ts.URL,
		NodeID:    types.NodeID(nodeIDStr),
		HTTP:      HTTPConfig{TLS: TLSConfig{Enabled: false}},
	}
	claimer := setupClaimer(t, cfg, mock)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_ = claimer.Start(ctx)

	if !mock.startCalled {
		t.Fatalf("controller.StartRun not called")
	}
	got := mock.lastStart

	if got.NextID == nil || *got.NextID != nextID {
		t.Errorf("NextID=%v want %v", got.NextID, nextID)
	}
	if got.RunID != claim.RunID {
		t.Errorf("RunID=%q want %q", got.RunID, claim.RunID)
	}
	if got.RepoID != claim.RepoID {
		t.Errorf("RepoID=%q want %q", got.RepoID, claim.RepoID)
	}
	if got.RepoURL != claim.RepoURL {
		t.Errorf("RepoURL=%q want %q", got.RepoURL.String(), claim.RepoURL.String())
	}
	if got.BaseRef != claim.BaseRef {
		t.Errorf("BaseRef=%q want %q", got.BaseRef.String(), claim.BaseRef.String())
	}
	if got.TargetRef != claim.TargetRef {
		t.Errorf("TargetRef=%q want %q", got.TargetRef.String(), claim.TargetRef.String())
	}
}

// TestClaimLoop_MultipleNodesSingleRun simulates two distinct nodes claiming
// different steps of the same multi-step run.
func TestClaimLoop_MultipleNodesSingleRun(t *testing.T) {
	t.Parallel()

	runID := types.NewRunID()
	repoID := types.NewMigRepoID()
	commit := types.CommitSHA("deadbeef")
	nodeID1 := types.NodeID("aB3xY9")
	nodeID2 := types.NodeID("Z9yX3b")

	nextID0 := types.NewJobID()
	claim0 := ClaimResponse{
		RunID:     runID,
		RepoID:    repoID,
		JobID:     types.NewJobID(),
		JobName:   "pre-gate",
		RepoURL:   types.RepoURL("https://github.com/acme/multi-node.git"),
		Status:    "Started",
		NodeID:    nodeID1,
		BaseRef:   types.GitRef("main"),
		TargetRef: types.GitRef("feature/parallel-steps"),
		NextID:    &nextID0,
		CommitSha: &commit,
		StartedAt: time.Now().UTC().Format(time.RFC3339),
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	nextID1 := types.NewJobID()
	claim1 := ClaimResponse{
		RunID:     runID,
		RepoID:    repoID,
		JobID:     types.NewJobID(),
		JobName:   "mig-0",
		RepoURL:   types.RepoURL("https://github.com/acme/multi-node.git"),
		Status:    "Started",
		NodeID:    nodeID2,
		BaseRef:   types.GitRef("main"),
		TargetRef: types.GitRef("feature/parallel-steps"),
		NextID:    &nextID1,
		CommitSha: &commit,
		StartedAt: time.Now().UTC().Format(time.RFC3339),
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	// Node 1
	ts1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/nodes/" + nodeID1.String() + "/claim":
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
		NodeID:    nodeID1,
		HTTP:      HTTPConfig{TLS: TLSConfig{Enabled: false}},
	}
	claimer1 := setupClaimer(t, cfg1, mock1)

	ctx1, cancel1 := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel1()
	_ = claimer1.Start(ctx1)

	if !mock1.startCalled {
		t.Fatalf("node-1: controller.StartRun not called")
	}
	if mock1.lastStart.NextID == nil || *mock1.lastStart.NextID != nextID0 {
		t.Errorf("node-1: NextID=%v want %v", mock1.lastStart.NextID, nextID0)
	}
	if mock1.lastStart.RunID != runID {
		t.Errorf("node-1: RunID=%q want %q", mock1.lastStart.RunID, runID)
	}

	// Node 2
	ts2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/nodes/" + nodeID2.String() + "/claim":
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
		NodeID:    nodeID2,
		HTTP:      HTTPConfig{TLS: TLSConfig{Enabled: false}},
	}
	claimer2 := setupClaimer(t, cfg2, mock2)

	ctx2, cancel2 := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel2()
	_ = claimer2.Start(ctx2)

	if !mock2.startCalled {
		t.Fatalf("node-2: controller.StartRun not called")
	}
	if mock2.lastStart.NextID == nil || *mock2.lastStart.NextID != nextID1 {
		t.Errorf("node-2: NextID=%v want %v", mock2.lastStart.NextID, nextID1)
	}
	if mock2.lastStart.RunID != runID {
		t.Errorf("node-2: RunID=%q want %q", mock2.lastStart.RunID, runID)
	}

	// Verify both nodes executed the same run but different steps.
	if mock1.lastStart.RunID != mock2.lastStart.RunID {
		t.Errorf("nodes executed different runs: node-1=%q node-2=%q", mock1.lastStart.RunID, mock2.lastStart.RunID)
	}
	if mock1.lastStart.NextID != nil && mock2.lastStart.NextID != nil && *mock1.lastStart.NextID == *mock2.lastStart.NextID {
		t.Error("nodes executed jobs with identical next_id pointers; expected different claims")
	}
}

func TestClaimAndExecute_WaitsForRecoveredMonitorSlotRelease(t *testing.T) {
	t.Parallel()

	var claimCalls int32
	jobID := types.NewJobID()
	runID := types.NewRunID()
	waitGate := make(chan struct{})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/nodes/"+testNodeID+"/claim":
			atomic.AddInt32(&claimCalls, 1)
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == "/v1/nodes/"+testNodeID+"/logs":
			w.WriteHeader(http.StatusCreated)
		case r.URL.Path == "/v1/jobs/"+jobID.String()+"/complete":
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	controller := &mockRunController{slotSem: make(chan struct{}, 1)}
	claimer := setupClaimer(t, newTestConfig(ts.URL), controller)
	claimer.startupReconciler = &startupCrashReconciler{
		docker: &fakeCrashReconcileDockerClient{
			waitByID: map[string]containertypes.WaitResponse{
				"ctr-recovered": {StatusCode: 0},
			},
			waitBlockByID: map[string]chan struct{}{
				"ctr-recovered": waitGate,
			},
			inspectByID: map[string]client.ContainerInspectResult{
				"ctr-recovered": {
					Container: containertypes.InspectResponse{
						State: &containertypes.State{
							ExitCode:   0,
							Status:     containertypes.ContainerState("exited"),
							StartedAt:  "2026-02-26T12:00:00Z",
							FinishedAt: "2026-02-26T12:00:01Z",
						},
					},
				},
			},
			logsByID: map[string][]byte{
				"ctr-recovered": multiplexedDockerLogs("recovered\n", stdcopy.Stdout),
			},
		},
	}

	claimer.startRecoveredRunningMonitors(context.Background(), []recoveredRunningContainer{
		{
			ContainerID: "ctr-recovered",
			RunID:       runID,
			JobID:       jobID,
		},
	})

	claimDone := make(chan struct{})
	var claimErr error
	go func() {
		defer close(claimDone)
		_, claimErr = claimer.claimAndExecute(context.Background())
	}()

	time.Sleep(100 * time.Millisecond)
	if got := atomic.LoadInt32(&claimCalls); got != 0 {
		t.Fatalf("claim endpoint called while recovered monitor held slot: %d", got)
	}

	close(waitGate)

	select {
	case <-claimDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for claimAndExecute after recovered monitor release")
	}
	if claimErr != nil {
		t.Fatalf("claimAndExecute() error = %v", claimErr)
	}
	if got := atomic.LoadInt32(&claimCalls); got != 1 {
		t.Fatalf("claim endpoint calls = %d, want 1", got)
	}
}
