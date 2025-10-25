package httpserver_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/api/admin"
	"github.com/iw2rmb/ploy/internal/api/config"
	"github.com/iw2rmb/ploy/internal/api/httpserver"
	"github.com/iw2rmb/ploy/internal/node/logstream"
)

func TestLogStreamEndpoint(t *testing.T) {
	t.Helper()
	cfg := loadConfig(t, `
http:
  listen: 127.0.0.1:0
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
	hub := logstream.NewHub(logstream.Options{})
	status := &stubStatus{}
	server, err := httpserver.New(httpserver.Options{
		Config:  cfg,
		Streams: hub,
		Status:  status,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := server.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() {
		_ = server.Stop(context.Background())
	}()

	jobID := "abc123"
	_ = hub.PublishLog(ctx, jobID, logstream.LogRecord{Timestamp: time.Now().UTC().Format(time.RFC3339Nano), Stream: "stdout", Line: "hello"})
	go func() {
		time.Sleep(25 * time.Millisecond)
		_ = hub.PublishStatus(ctx, jobID, logstream.Status{Status: "done"})
	}()

	addr := server.Address()
	if addr == "" {
		t.Fatal("server address empty")
	}
	resp, err := http.Get("http://" + addr + "/v1/node/jobs/" + jobID + "/logs/stream")
	if err != nil {
		t.Fatalf("GET logs: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("unexpected status %d body=%s", resp.StatusCode, string(body))
	}
	if !strings.Contains(string(body), `"line":"hello"`) {
		t.Fatalf("expected log frame in response: %s", string(body))
	}
}

func TestStatusEndpoint(t *testing.T) {
	t.Helper()
	cfg := loadConfig(t, `
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
	hub := logstream.NewHub(logstream.Options{})
	status := &stubStatus{
		payload: map[string]any{
			"state": "ok",
			"services": map[string]any{
				"docker": "healthy",
			},
		},
	}
	server, err := httpserver.New(httpserver.Options{
		Config:  cfg,
		Streams: hub,
		Status:  status,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	app := server.App()
	req := httptest.NewRequest("GET", "/v1/node/status", nil)
	resp, err := app.Test(req, 1000)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status %d body=%s", resp.StatusCode, string(body))
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["state"] != "ok" {
		t.Fatalf("state=%v want ok", out["state"])
	}
}

func TestReloadReconfigures(t *testing.T) {
	t.Helper()
	cfg := loadConfig(t, `
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
	hub := logstream.NewHub(logstream.Options{})
	status := &stubStatus{}
	server, err := httpserver.New(httpserver.Options{
		Config:  cfg,
		Streams: hub,
		Status:  status,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	updated := cfg
	updated.HTTP.Listen = "127.0.0.1:28443"
	if err := server.Reload(context.Background(), updated); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	if server.Config().HTTP.Listen != "127.0.0.1:28443" {
		t.Fatalf("config not updated, got %s", server.Config().HTTP.Listen)
	}
}

func TestAdminNodeCreate(t *testing.T) {
	t.Helper()
	cfg := loadConfig(t, `
http:
  listen: 127.0.0.1:0
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
	admin := &stubAdmin{}
	server, err := httpserver.New(httpserver.Options{
		Config:  cfg,
		Streams: logstream.NewHub(logstream.Options{}),
		Status:  &stubStatus{},
		Admin:   admin,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	app := server.App()
	body := `{"cluster_id":"cluster","address":"10.0.0.5"}`
	req := httptest.NewRequest("POST", "/v1/admin/nodes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, 1000)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("unexpected status %d", resp.StatusCode)
	}
	if admin.calls != 1 {
		t.Fatalf("expected admin register invoked once, got %d", admin.calls)
	}
}

func TestControlPlaneRoutesMounted(t *testing.T) {
	t.Helper()
	cfg := loadConfig(t, `
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
	hub := logstream.NewHub(logstream.Options{})
	status := &stubStatus{}

	var (
		mu    sync.Mutex
		paths []string
	)
	control := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		w.WriteHeader(http.StatusTeapot)
	})

	server, err := httpserver.New(httpserver.Options{
		Config:       cfg,
		Streams:      hub,
		Status:       status,
		ControlPlane: control,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	app := server.App()
	req := httptest.NewRequest("GET", "/v1/health", nil)
	resp, err := app.Test(req, 1000)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusTeapot {
		t.Fatalf("unexpected status %d", resp.StatusCode)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(paths) != 1 || paths[0] != "/v1/health" {
		t.Fatalf("control-plane handler paths = %v", paths)
	}
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

type stubStatus struct {
	mu      sync.Mutex
	payload map[string]any
}

func (s *stubStatus) Snapshot(context.Context) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.payload == nil {
		return map[string]any{"state": "ok"}, nil
	}
	dup := make(map[string]any, len(s.payload))
	for k, v := range s.payload {
		dup[k] = v
	}
	return dup, nil
}

type stubAdmin struct {
	calls int
}

func (s *stubAdmin) RegisterNode(context.Context, admin.NodeRegistrationRequest) (admin.NodeRegistrationResponse, error) {
	s.calls++
	return admin.NodeRegistrationResponse{WorkerID: "worker"}, nil
}
