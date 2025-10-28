package controlplane_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/api/config"
	"github.com/iw2rmb/ploy/internal/api/controlplane"
)

// TestClientClaimsJobAndExecutes verifies the client claims a job and runs the executor.
func TestClientClaimsJobAndExecutes(t *testing.T) {
	t.Helper()
	completions := make(chan map[string]any, 1)
	var claimCount int
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/jobs/claim":
			mu.Lock()
			claimCount++
			call := claimCount
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			if call == 1 {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"status":  "claimed",
					"node_id": "node-1",
					"job": map[string]any{
						"id":       "job-1",
						"ticket":   "ticket-1",
						"step_id":  "step-a",
						"priority": "default",
						"metadata": map[string]string{"runtime": "docker"},
					},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "empty"})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/jobs/job-1/heartbeat":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/jobs/job-1/complete":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode completion: %v", err)
			}
			select {
			case completions <- payload:
			default:
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
		case r.Method == http.MethodPatch && r.URL.Path == "/nodes/node-1":
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	exec := &stubExecutor{done: make(chan struct{}, 1)}
	cfg := config.ControlPlaneConfig{
		Endpoint:               srv.URL,
		NodeID:                 "node-1",
		NodeStatusEndpoint:     "/nodes",
		JobClaimEndpoint:       "/v1/jobs/claim",
		JobHeartbeatEndpoint:   "/v1/jobs",
		JobCompleteEndpoint:    "/v1/jobs",
		HeartbeatInterval:      25 * time.Millisecond,
		AssignmentPollInterval: 10 * time.Millisecond,
		InitialBackoff:         5 * time.Millisecond,
		MaxBackoff:             50 * time.Millisecond,
		CAPath:                 filepath.Join(t.TempDir(), "ca.pem"),
		Certificate:            filepath.Join(t.TempDir(), "cert.pem"),
		Key:                    filepath.Join(t.TempDir(), "key.pem"),
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
	case <-exec.done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("executor not invoked")
	}

	var completion map[string]any
	select {
	case completion = <-completions:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("completion not observed")
	}
	if completion["state"] != "succeeded" {
		t.Fatalf("expected succeeded completion, got %v", completion["state"])
	}

	if err := client.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if exec.count() == 0 {
		t.Fatal("expected executor to process at least one job")
	}
}

// TestClientReloadUpdatesConfig ensures reloading swaps the claim endpoint.
func TestClientReloadUpdatesConfig(t *testing.T) {
	t.Helper()
	var mu sync.Mutex
	claimPath := "/v1/jobs/claim"
	callCh := make(chan string, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost:
			mu.Lock()
			current := claimPath
			mu.Unlock()
			if r.URL.Path == current {
				select {
				case callCh <- current:
				default:
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"status": "empty"})
				return
			}
			http.NotFound(w, r)
		case r.Method == http.MethodPatch && r.URL.Path == "/nodes/node-2":
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg := config.ControlPlaneConfig{
		Endpoint:               srv.URL,
		NodeID:                 "node-2",
		NodeStatusEndpoint:     "/nodes",
		JobClaimEndpoint:       "/v1/jobs/claim",
		JobHeartbeatEndpoint:   "/v1/jobs",
		JobCompleteEndpoint:    "/v1/jobs",
		AssignmentPollInterval: 10 * time.Millisecond,
		InitialBackoff:         5 * time.Millisecond,
		MaxBackoff:             20 * time.Millisecond,
		HeartbeatInterval:      250 * time.Millisecond,
		CAPath:                 filepath.Join(t.TempDir(), "ca.pem"),
		Certificate:            filepath.Join(t.TempDir(), "cert.pem"),
		Key:                    filepath.Join(t.TempDir(), "key.pem"),
	}

	client, err := controlplane.New(controlplane.Options{
		Config:     cfg,
		Executor:   &stubExecutor{},
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
	case path := <-callCh:
		if path != "/v1/jobs/claim" {
			t.Fatalf("expected initial claim path, got %s", path)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("claim not observed before reload")
	}

	mu.Lock()
	claimPath = "/v1/jobs/claim-alt"
	mu.Unlock()
	updated := cfg
	updated.JobClaimEndpoint = "/v1/jobs/claim-alt"
	if err := client.Reload(context.Background(), updated); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}

	select {
	case path := <-callCh:
		if path != "/v1/jobs/claim-alt" {
			t.Fatalf("expected updated claim path, got %s", path)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("updated claim path not observed")
	}

	if err := client.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

type stubExecutor struct {
	mu   sync.Mutex
	jobs []controlplane.Assignment
	done chan struct{}
}

// Execute records the assignment and optionally signals completion.
func (s *stubExecutor) Execute(ctx context.Context, assignment controlplane.Assignment) error {
	_ = ctx
	s.mu.Lock()
	s.jobs = append(s.jobs, assignment)
	s.mu.Unlock()
	if s.done != nil {
		select {
		case s.done <- struct{}{}:
		default:
		}
	}
	return nil
}

// count returns the number of assignments executed.
func (s *stubExecutor) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.jobs)
}

type stubStatusProvider struct{}

// Snapshot provides a static node status payload.
func (stubStatusProvider) Snapshot(context.Context) (map[string]any, error) {
	return map[string]any{"state": "ok"}, nil
}
