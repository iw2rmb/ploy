package worker_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/api/config"
	"github.com/iw2rmb/ploy/internal/api/controlplane"
)

// TestClientClaimsJobAndCompletes ensures the worker claims a job and reports completion.
func TestClientClaimsJobAndCompletes(t *testing.T) {
	t.Helper()
	var claimCalls atomic.Int32
	var heartbeatCalls atomic.Int32
	completeCh := make(chan completePayload, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/jobs/claim":
			if claimCalls.Add(1) == 1 {
				writeJSON(t, w, map[string]any{
					"status":  "claimed",
					"node_id": "node-test",
					"job": map[string]any{
						"id":       "job-123",
						"ticket":   "ticket-abc",
						"step_id":  "step-main",
						"priority": "default",
						"metadata": map[string]string{"runtime": "docker"},
					},
				})
				return
			}
			writeJSON(t, w, map[string]any{"status": "empty"})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/jobs/job-123/heartbeat":
			heartbeatCalls.Add(1)
			writeJSON(t, w, map[string]any{"status": "ok"})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/jobs/job-123/complete":
			var payload completePayload
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode complete payload: %v", err)
			}
			select {
			case completeCh <- payload:
			default:
			}
			writeJSON(t, w, map[string]any{"status": "ok"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	results := make(chan error, 1)
	results <- nil
	exec := newStubExecutor(results)

	cfg := config.ControlPlaneConfig{
		Endpoint:               srv.URL,
		NodeID:                 "node-test",
		JobClaimEndpoint:       "/v1/jobs/claim",
		JobHeartbeatEndpoint:   "/v1/jobs",
		JobCompleteEndpoint:    "/v1/jobs",
		HeartbeatInterval:      10 * time.Millisecond,
		AssignmentPollInterval: 5 * time.Millisecond,
		InitialBackoff:         5 * time.Millisecond,
		MaxBackoff:             20 * time.Millisecond,
	}

	client, err := controlplane.New(controlplane.Options{
		Config:     cfg,
		Executor:   exec,
		Status:     stubStatusProvider{},
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := client.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	select {
	case <-exec.assignments:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("assignment not delivered to executor")
	}

	var completion completePayload
	select {
	case completion = <-completeCh:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("completion not posted")
	}

	if completion.State != "succeeded" {
		t.Fatalf("expected completion state succeeded, got %s", completion.State)
	}
	if heartbeatCalls.Load() == 0 {
		t.Fatal("expected at least one heartbeat call")
	}

	if err := client.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

// TestClientHeartbeatsUntilCompletion verifies heartbeats continue until the executor finishes.
func TestClientHeartbeatsUntilCompletion(t *testing.T) {
	t.Helper()
	var claimOnce atomic.Bool
	var heartbeatCount atomic.Int32
	done := make(chan struct{})
	results := make(chan error)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/jobs/claim":
			if claimOnce.CompareAndSwap(false, true) {
				writeJSON(t, w, map[string]any{
					"status":  "claimed",
					"node_id": "node-heartbeat",
					"job": map[string]any{
						"id":       "job-hb",
						"ticket":   "ticket-hb",
						"metadata": map[string]string{},
					},
				})
				return
			}
			writeJSON(t, w, map[string]any{"status": "empty"})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/jobs/job-hb/heartbeat":
			heartbeatCount.Add(1)
			writeJSON(t, w, map[string]any{"status": "ok"})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/jobs/job-hb/complete":
			var payload completePayload
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode completion: %v", err)
			}
			if payload.State != "succeeded" {
				t.Fatalf("unexpected completion state: %s", payload.State)
			}
			close(done)
			writeJSON(t, w, map[string]any{"status": "ok"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	exec := newStubExecutor(results)

	cfg := config.ControlPlaneConfig{
		Endpoint:               srv.URL,
		NodeID:                 "node-heartbeat",
		JobClaimEndpoint:       "/v1/jobs/claim",
		JobHeartbeatEndpoint:   "/v1/jobs",
		JobCompleteEndpoint:    "/v1/jobs",
		HeartbeatInterval:      25 * time.Millisecond,
		AssignmentPollInterval: 10 * time.Millisecond,
		InitialBackoff:         5 * time.Millisecond,
		MaxBackoff:             50 * time.Millisecond,
	}

	client, err := controlplane.New(controlplane.Options{
		Config:     cfg,
		Executor:   exec,
		Status:     stubStatusProvider{},
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := client.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	select {
	case <-exec.assignments:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("no assignment received")
	}

	time.Sleep(120 * time.Millisecond)
	if heartbeatCount.Load() < 2 {
		t.Fatalf("expected multiple heartbeats, got %d", heartbeatCount.Load())
	}

	select {
	case results <- nil:
	default:
	}

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("completion did not fire")
	}

	if err := client.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

// TestClientRetriesCompletionOnTransientError asserts complete retries after transient failures.
func TestClientRetriesCompletionOnTransientError(t *testing.T) {
	t.Helper()
	var claimDone atomic.Bool
	var completeCalls atomic.Int32
	completeCh := make(chan completePayload, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/jobs/claim":
			if claimDone.CompareAndSwap(false, true) {
				writeJSON(t, w, map[string]any{
					"status":  "claimed",
					"node_id": "node-retry",
					"job": map[string]any{
						"id":     "job-retry",
						"ticket": "ticket-retry",
					},
				})
				return
			}
			writeJSON(t, w, map[string]any{"status": "empty"})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/jobs/job-retry/heartbeat":
			writeJSON(t, w, map[string]any{"status": "ok"})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/jobs/job-retry/complete":
			seq := completeCalls.Add(1)
			if seq == 1 {
				http.Error(w, "transient", http.StatusInternalServerError)
				return
			}
			var payload completePayload
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode completion payload: %v", err)
			}
			completeCh <- payload
			writeJSON(t, w, map[string]any{"status": "ok"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	results := make(chan error, 1)
	results <- nil
	exec := newStubExecutor(results)

	cfg := config.ControlPlaneConfig{
		Endpoint:               srv.URL,
		NodeID:                 "node-retry",
		JobClaimEndpoint:       "/v1/jobs/claim",
		JobHeartbeatEndpoint:   "/v1/jobs",
		JobCompleteEndpoint:    "/v1/jobs",
		HeartbeatInterval:      15 * time.Millisecond,
		AssignmentPollInterval: 5 * time.Millisecond,
		InitialBackoff:         5 * time.Millisecond,
		MaxBackoff:             30 * time.Millisecond,
	}

	client, err := controlplane.New(controlplane.Options{
		Config:     cfg,
		Executor:   exec,
		Status:     stubStatusProvider{},
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := client.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	select {
	case <-completeCh:
	case <-time.After(750 * time.Millisecond):
		t.Fatal("expected completion after retry")
	}

	if completeCalls.Load() < 2 {
		t.Fatalf("expected completion retries, got %d calls", completeCalls.Load())
	}

	if err := client.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

// writeJSON serialises the payload and writes it to the response.
func writeJSON(t *testing.T, w http.ResponseWriter, payload any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatalf("write json: %v", err)
	}
}

// completePayload captures the completion schema expectations.
type completePayload struct {
	Ticket string `json:"ticket"`
	NodeID string `json:"node_id"`
	State  string `json:"state"`
	Error  any    `json:"error,omitempty"`
}

// stubStatusProvider implements a static status snapshot for tests.
type stubStatusProvider struct{}

// Snapshot reports a fixed status payload for the node.
func (stubStatusProvider) Snapshot(context.Context) (map[string]any, error) {
	return map[string]any{"state": "ok"}, nil
}

// stubExecutor captures assignments and returns results provided by callers.
type stubExecutor struct {
	assignments chan controlplane.Assignment
	results     <-chan error
}

// newStubExecutor constructs a stub executor around the provided result channel.
func newStubExecutor(results <-chan error) *stubExecutor {
	return &stubExecutor{
		assignments: make(chan controlplane.Assignment, 4),
		results:     results,
	}
}

// Execute records the assignment and waits for the result channel.
func (s *stubExecutor) Execute(ctx context.Context, assignment controlplane.Assignment) error {
	select {
	case s.assignments <- assignment:
	default:
	}
	select {
	case err := <-s.results:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
