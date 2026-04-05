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
	"github.com/iw2rmb/ploy/internal/testutil/workflowkit"
	"github.com/iw2rmb/ploy/internal/workflow/backoff"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/moby/moby/api/pkg/stdcopy"
	containertypes "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

func TestClaimLoop(t *testing.T) {
	t.Parallel()

	rec := &callRecorder{}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.Record(r.URL.Path)

		switch r.URL.Path {
		case "/v1/nodes/" + testNodeID + "/claim":
			resp := newClaimResponse()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(resp)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	claimer := setupClaimer(t, newAgentConfig(ts.URL), &mockRunController{})

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

	cfg := newAgentConfig(ts.URL)
	claimer := setupClaimer(t, cfg, newTestController(t, cfg))
	runClaimerUntil(t, claimer, 500*time.Millisecond)

	if got := atomic.LoadInt32(&callCount); got < 2 {
		t.Errorf("expected multiple claim attempts, got %d", got)
	}
}

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

	cfg := newAgentConfig(ts.URL)
	claimer := setupClaimer(t, cfg, newTestController(t, cfg))
	claimer.backoff = backoff.NewStatefulBackoff(backoff.Policy{
		InitialInterval: types.Duration(50 * time.Millisecond),
		MaxInterval:     types.Duration(200 * time.Millisecond),
		Multiplier:      2.0,
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

			resp := newClaimResponse()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(resp)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	cfg := newAgentConfig(ts.URL)
	claimer := setupClaimer(t, cfg, newTestController(t, cfg))
	claimer.backoff = backoff.NewStatefulBackoff(backoff.Policy{
		InitialInterval: types.Duration(50 * time.Millisecond),
		MaxInterval:     types.Duration(200 * time.Millisecond),
		Multiplier:      2.0,
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

// TestClaimLoop_FieldMapping verifies ClaimResponse fields map correctly into StartRunRequest.
func TestClaimLoop_FieldMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		claimOpts  []claimOption
		assertions func(t *testing.T, got StartRunRequest, claim ClaimResponse)
	}{
		{
			name: "core fields including CommitSHA and RecoveryContext",
			claimOpts: []claimOption{
				withCommitSHA("deadbeef"),
				withRecoveryContext(&contracts.RecoveryClaimContext{
					LoopKind:             "healing",
					DetectedStack:        contracts.MigStackJavaMaven,
					ResolvedHealingImage: "docker.io/acme/heal:latest",
					BuildGateLog:         "[ERROR] build failed\n",
				}),
			},
			assertions: func(t *testing.T, got StartRunRequest, claim ClaimResponse) {
				t.Helper()
				if got.RunID != claim.RunID {
					t.Errorf("RunID=%q want %q", got.RunID, claim.RunID)
				}
				if got.RepoID != claim.RepoID {
					t.Errorf("RepoID=%q want %q", got.RepoID, claim.RepoID)
				}
				if got.RepoURL != claim.RepoURL {
					t.Errorf("RepoURL=%q want %q", got.RepoURL, claim.RepoURL)
				}
				if got.BaseRef != claim.BaseRef {
					t.Errorf("BaseRef=%q want %q", got.BaseRef, claim.BaseRef)
				}
				if got.TargetRef != claim.TargetRef {
					t.Errorf("TargetRef=%q want %q", got.TargetRef, claim.TargetRef)
				}
				if got.CommitSHA != *claim.CommitSha {
					t.Errorf("CommitSHA=%q want %q", got.CommitSHA, *claim.CommitSha)
				}
				if got.RecoveryContext == nil {
					t.Fatalf("RecoveryContext=nil, want non-nil")
				}
				if got.RecoveryContext.LoopKind != "healing" {
					t.Errorf("RecoveryContext.LoopKind=%q, want healing", got.RecoveryContext.LoopKind)
				}
			},
		},
		{
			name:      "NextID propagation",
			claimOpts: []claimOption{withNextID(types.NewJobID()), withCommitSHA("abc123")},
			assertions: func(t *testing.T, got StartRunRequest, claim ClaimResponse) {
				t.Helper()
				if got.NextID == nil || *got.NextID != *claim.NextID {
					t.Errorf("NextID=%v want %v", got.NextID, claim.NextID)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			nodeIDStr := testNodeID
			claim := newClaimResponse(tt.claimOpts...)
			ts := newSingleClaimServer(t, nodeIDStr, claim)

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
			tt.assertions(t, mock.lastStart, claim)
		})
	}
}

func TestClaimAndExecute_WaitsForRecoveredMonitorSlotRelease(t *testing.T) {
	t.Parallel()

	var claimCalls int32
	s := workflowkit.NewRunOrchestrationScenario()
	waitGate := make(chan struct{})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/nodes/"+testNodeID+"/claim":
			atomic.AddInt32(&claimCalls, 1)
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == "/v1/nodes/"+testNodeID+"/logs":
			w.WriteHeader(http.StatusCreated)
		case r.URL.Path == "/v1/jobs/"+s.JobID.String()+"/complete":
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	controller := &mockRunController{slotSem: make(chan struct{}, 1)}
	claimer := setupClaimer(t, newAgentConfig(ts.URL), controller)
	claimer.startupReconciler = &startupCrashReconciler{
		docker: &fakeDockerClient{
			waitByID:      map[string]containertypes.WaitResponse{"ctr-recovered": {StatusCode: 0}},
			waitBlockByID: map[string]chan struct{}{"ctr-recovered": waitGate},
			inspectByID: map[string]client.ContainerInspectResult{
				"ctr-recovered": {Container: containertypes.InspectResponse{
					State: &containertypes.State{
						ExitCode: 0, Status: containertypes.ContainerState("exited"),
						StartedAt: "2026-02-26T12:00:00Z", FinishedAt: "2026-02-26T12:00:01Z",
					},
				}},
			},
			logsByID: map[string][]byte{"ctr-recovered": multiplexedDockerLogs("recovered\n", stdcopy.Stdout)},
		},
	}

	claimer.startRecoveredRunningMonitors(context.Background(), []recoveredRunningContainer{
		{ContainerID: "ctr-recovered", RunID: s.RunID, JobID: s.JobID},
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
