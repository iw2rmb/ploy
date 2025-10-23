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

	"github.com/iw2rmb/ploy/internal/ployd/config"
	"github.com/iw2rmb/ploy/internal/ployd/controlplane"
)

func TestClientProcessesAssignments(t *testing.T) {
	t.Helper()
	assignmentsServed := make(chan struct{}, 1)
	var mu sync.Mutex
	var assignmentCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/assignments":
			mu.Lock()
			assignmentCalls++
			call := assignmentCalls
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			if call == 1 {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"assignments": []map[string]any{
						{"id": "task-1", "runtime": "local", "payload": map[string]any{"step": "build"}},
					},
				})
			} else {
				_ = json.NewEncoder(w).Encode(map[string]any{"assignments": []any{}})
			}
		case "/nodes/node-1":
			if r.Method == http.MethodPatch {
				w.WriteHeader(http.StatusOK)
				return
			}
			w.WriteHeader(http.StatusMethodNotAllowed)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	exec := &stubExecutor{done: assignmentsServed}
	status := &stubStatusProvider{}

	cfg := config.ControlPlaneConfig{
		Endpoint:               srv.URL,
		AssignmentsEndpoint:    "/assignments",
		NodeStatusEndpoint:     "/nodes",
		NodeID:                 "node-1",
		AssignmentPollInterval: 10 * time.Millisecond,
		StatusPublishInterval:  250 * time.Millisecond,
		HeartbeatInterval:      250 * time.Millisecond,
		CAPath:                 filepath.Join(t.TempDir(), "ca.pem"),
		Certificate:            filepath.Join(t.TempDir(), "cert.pem"),
		Key:                    filepath.Join(t.TempDir(), "key.pem"),
	}

	client, err := controlplane.New(controlplane.Options{
		Config:     cfg,
		Executor:   exec,
		Status:     status,
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
	case <-assignmentsServed:
	case <-time.After(500 * time.Millisecond):
		cancel()
		t.Fatal("assignment not processed in time")
	}
	if err := client.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	if exec.count() == 0 {
		t.Fatal("assignment executor not invoked")
	}
}

func TestClientReloadUpdatesConfig(t *testing.T) {
	t.Helper()
	var mu sync.Mutex
	var endpoint string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		path := endpoint
		mu.Unlock()
		if r.URL.Path != path {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"assignments": []any{}})
	}))
	defer srv.Close()

	endpoint = "/assignments"
	exec := &stubExecutor{done: make(chan struct{}, 1)}
	cfg := config.ControlPlaneConfig{
		Endpoint:               srv.URL,
		AssignmentsEndpoint:    endpoint,
		NodeStatusEndpoint:     "/nodes",
		NodeID:                 "node-2",
		AssignmentPollInterval: 20 * time.Millisecond,
		StatusPublishInterval:  200 * time.Millisecond,
		HeartbeatInterval:      200 * time.Millisecond,
		CAPath:                 filepath.Join(t.TempDir(), "ca.pem"),
		Certificate:            filepath.Join(t.TempDir(), "cert.pem"),
		Key:                    filepath.Join(t.TempDir(), "key.pem"),
	}

	client, err := controlplane.New(controlplane.Options{
		Config:     cfg,
		Executor:   exec,
		Status:     &stubStatusProvider{},
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

	mu.Lock()
	endpoint = "/new-assignments"
	mu.Unlock()

	updated := cfg
	updated.AssignmentsEndpoint = "/new-assignments"
	if err := client.Reload(context.Background(), updated); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}

	if client.Config().AssignmentsEndpoint != "/new-assignments" {
		t.Fatalf("expected assignments endpoint updated, got %s", client.Config().AssignmentsEndpoint)
	}
	if err := client.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

type stubExecutor struct {
	mu   sync.Mutex
	jobs []controlplane.Assignment
	done chan<- struct{}
}

func (s *stubExecutor) Execute(ctx context.Context, a controlplane.Assignment) error {
	_ = ctx
	s.mu.Lock()
	s.jobs = append(s.jobs, a)
	s.mu.Unlock()
	if s.done != nil {
		select {
		case s.done <- struct{}{}:
		default:
		}
	}
	return nil
}

func (s *stubExecutor) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.jobs)
}

type stubStatusProvider struct{}

func (stubStatusProvider) Snapshot(context.Context) (map[string]any, error) {
	return map[string]any{"state": "ok"}, nil
}
