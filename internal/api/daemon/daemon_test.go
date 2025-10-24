package daemon_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/node/logstream"
	"github.com/iw2rmb/ploy/internal/api/config"
	"github.com/iw2rmb/ploy/internal/api/daemon"
	"github.com/iw2rmb/ploy/internal/workflow/runtime"
)

func TestRunWorkerStartsComponents(t *testing.T) {
	t.Helper()
	cfg := loadConfig(t, `
mode: worker
control_plane:
  endpoint: https://control.example.com
  ca: /etc/ploy/pki/ca.pem
  certificate: /etc/ploy/pki/node.pem
  key: /etc/ploy/pki/node-key.pem
runtime:
  plugins:
    - name: local
      module: internal
`)

	http := newStubComponent()
	metrics := newStubComponent()
	control := newStubComponent()
	pki := newStubComponent()
	scheduler := newStubComponent()
	runtimeRegistry := runtime.NewRegistry()
	streams := logstream.NewHub(logstream.Options{})

	svc, err := daemon.New(daemon.Options{
		Config:          cfg,
		RuntimeRegistry: runtimeRegistry,
		LogStreams:      streams,
		HTTP:            http,
		Metrics:         metrics,
		ControlPlane:    control,
		PKI:             pki,
		Scheduler:       scheduler,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		if err := svc.Run(ctx); err != nil && err != context.Canceled {
			t.Errorf("Run() error = %v", err)
		}
		close(done)
	}()

	waitStarted(t, http, metrics, control, pki, scheduler)
	cancel()
	waitDone(t, done)

	if http.starts() != 1 || http.stops() != 1 {
		t.Fatalf("http starts=%d stops=%d", http.starts(), http.stops())
	}
	if metrics.starts() != 1 || metrics.stops() != 1 {
		t.Fatalf("metrics starts=%d stops=%d", metrics.starts(), metrics.stops())
	}
	if control.starts() != 1 || control.stops() != 1 {
		t.Fatalf("control starts=%d stops=%d", control.starts(), control.stops())
	}
	if pki.starts() != 1 || pki.stops() != 1 {
		t.Fatalf("pki starts=%d stops=%d", pki.starts(), pki.stops())
	}
	if scheduler.starts() != 1 || scheduler.stops() != 1 {
		t.Fatalf("scheduler starts=%d stops=%d", scheduler.starts(), scheduler.stops())
	}
}

func TestRunBootstrapTransitionsToWorker(t *testing.T) {
	t.Helper()
	cfg := loadConfig(t, `
mode: bootstrap
control_plane:
  endpoint: https://control.example.com
  ca: /etc/ploy/pki/ca.pem
  certificate: /etc/ploy/pki/node.pem
  key: /etc/ploy/pki/node-key.pem
runtime:
  plugins:
    - name: local
      module: internal
`)

	http := newStubComponent()
	metrics := newStubComponent()
	control := newStubComponent()
	pki := newStubComponent()
	scheduler := newStubComponent()
	bootstrap := &stubBootstrap{}

	svc, err := daemon.New(daemon.Options{
		Config:          cfg,
		RuntimeRegistry: runtime.NewRegistry(),
		LogStreams:      logstream.NewHub(logstream.Options{}),
		HTTP:            http,
		Metrics:         metrics,
		ControlPlane:    control,
		PKI:             pki,
		Scheduler:       scheduler,
		Bootstrap:       bootstrap,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		if err := svc.Run(ctx); err != nil && err != context.Canceled {
			t.Errorf("Run() error = %v", err)
		}
		close(done)
	}()

	waitStarted(t, http, metrics, control, pki, scheduler)
	cancel()
	waitDone(t, done)

	if bootstrap.calls() != 1 {
		t.Fatalf("bootstrap calls=%d, want 1", bootstrap.calls())
	}
	if http.starts() != 1 {
		t.Fatalf("http starts=%d, want 1", http.starts())
	}
	if cfg.Mode != config.ModeBootstrap {
		t.Fatalf("original config should remain bootstrap, got %q", cfg.Mode)
	}
}

func TestReloadPropagates(t *testing.T) {
	t.Helper()
	cfg := loadConfig(t, `
mode: worker
control_plane:
  endpoint: https://control.example.com
  ca: /etc/ploy/pki/ca.pem
  certificate: /etc/ploy/pki/node.pem
  key: /etc/ploy/pki/node-key.pem
runtime:
  plugins:
    - name: local
      module: internal
`)

	http := newStubComponent()
	metrics := newStubComponent()
	control := newStubComponent()
	pki := newStubComponent()
	scheduler := newStubComponent()

	svc, err := daemon.New(daemon.Options{
		Config:          cfg,
		RuntimeRegistry: runtime.NewRegistry(),
		LogStreams:      logstream.NewHub(logstream.Options{}),
		HTTP:            http,
		Metrics:         metrics,
		ControlPlane:    control,
		PKI:             pki,
		Scheduler:       scheduler,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		if err := svc.Run(ctx); err != nil && err != context.Canceled {
			t.Errorf("Run() error = %v", err)
		}
		close(done)
	}()
	waitStarted(t, http, metrics, control, pki, scheduler)

	updated := cfg
	updated.HTTP.Listen = "127.0.0.1:28443"
	if err := svc.Reload(context.Background(), updated); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	if http.reloads() != 1 {
		t.Fatalf("http reloads=%d want 1", http.reloads())
	}
	if metrics.reloads() != 1 {
		t.Fatalf("metrics reloads=%d want 1", metrics.reloads())
	}
	if control.reloads() != 1 {
		t.Fatalf("control reloads=%d want 1", control.reloads())
	}
	if pki.reloads() != 1 {
		t.Fatalf("pki reloads=%d want 1", pki.reloads())
	}
	if scheduler.reloads() != 1 {
		t.Fatalf("scheduler reloads=%d want 1", scheduler.reloads())
	}

	cancel()
	waitDone(t, done)
}

// Helpers

func loadConfig(t *testing.T, raw string) config.Config {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "ployd.yaml")
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	return cfg
}

type stubComponent struct {
	mu          sync.Mutex
	startCount  int
	stopCount   int
	reloadCount int
	startCh     chan struct{}
	started     bool
}

func newStubComponent() *stubComponent {
	return &stubComponent{
		startCh: make(chan struct{}),
	}
}

func (s *stubComponent) Start(context.Context) error {
	s.mu.Lock()
	s.startCount++
	if !s.started {
		close(s.startCh)
		s.started = true
	}
	s.mu.Unlock()
	return nil
}

func (s *stubComponent) Stop(context.Context) error {
	s.mu.Lock()
	s.stopCount++
	s.mu.Unlock()
	return nil
}

func (s *stubComponent) Reload(context.Context, config.Config) error {
	s.mu.Lock()
	s.reloadCount++
	s.mu.Unlock()
	return nil
}

func (s *stubComponent) starts() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.startCount
}

func (s *stubComponent) stops() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopCount
}

func (s *stubComponent) reloads() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reloadCount
}

type stubBootstrap struct {
	mu    sync.Mutex
	count int
}

func (s *stubBootstrap) Run(context.Context, config.Config) error {
	s.mu.Lock()
	s.count++
	s.mu.Unlock()
	return nil
}

func (s *stubBootstrap) calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.count
}

func waitStarted(t *testing.T, comps ...*stubComponent) {
	t.Helper()
	for _, comp := range comps {
		select {
		case <-comp.startCh:
		case <-time.After(2 * time.Second):
			t.Fatalf("component did not start within timeout")
		}
	}
}

func waitDone(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("daemon did not stop within timeout")
	}
}
