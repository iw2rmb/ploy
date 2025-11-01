package httpserver_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/iw2rmb/ploy/internal/api/httpserver"
	"github.com/iw2rmb/ploy/internal/node/logstream"
)

type stubJobController struct {
	err   error
	last  string
	calls int
}

func (s *stubJobController) Cancel(id string) error { s.last = id; s.calls++; return s.err }

func TestNodeJobCancelEndpoint(t *testing.T) {
	t.Helper()
	cfg := loadConfig(t, `
control_plane:
  endpoint: https://control.example.com
  ca: /etc/ploy/pki/ca.pem
  certificate: /etc/ploy/pki/node.pem
  key: /etc/ploy/pki/node-key.pem
`)

	hub := logstream.NewHub(logstream.Options{})

	// 503 when controller missing
	srv, err := httpserver.New(httpserver.Options{Config: cfg, Streams: hub, Status: &stubStatus{}})
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	app := srv.App()
	req := httptest.NewRequest(http.MethodPost, "/v1/node/jobs/abc/cancel", nil)
	resp, err := app.Test(req, 1000)
	if err != nil {
		t.Fatalf("app.Test(): %v", err)
	}
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status=%d want 503", resp.StatusCode)
	}

	// 202 when canceled
	ctrl := &stubJobController{}
	srv2, err := httpserver.New(httpserver.Options{Config: cfg, Streams: hub, Status: &stubStatus{}, JobControl: ctrl})
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	app = srv2.App()
	req = httptest.NewRequest(http.MethodPost, "/v1/node/jobs/abc/cancel", nil)
	resp, err = app.Test(req, 1000)
	if err != nil {
		t.Fatalf("app.Test(): %v", err)
	}
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status=%d want 202", resp.StatusCode)
	}
	if ctrl.last != "abc" || ctrl.calls != 1 {
		t.Fatalf("controller not invoked: last=%s calls=%d", ctrl.last, ctrl.calls)
	}

	// 404 mapping
	ctrl404 := &stubJobController{err: httpserver.ErrJobNotFound}
	srv3, _ := httpserver.New(httpserver.Options{Config: cfg, Streams: hub, Status: &stubStatus{}, JobControl: ctrl404})
	app = srv3.App()
	req = httptest.NewRequest(http.MethodPost, "/v1/node/jobs/missing/cancel", nil)
	resp, err = app.Test(req, 1000)
	if err != nil {
		t.Fatalf("app.Test(): %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status=%d want 404", resp.StatusCode)
	}

	// 409 mapping
	ctrl409 := &stubJobController{err: httpserver.ErrJobNotRunning}
	srv4, _ := httpserver.New(httpserver.Options{Config: cfg, Streams: hub, Status: &stubStatus{}, JobControl: ctrl409})
	app = srv4.App()
	req = httptest.NewRequest(http.MethodPost, "/v1/node/jobs/finished/cancel", nil)
	resp, err = app.Test(req, 1000)
	if err != nil {
		t.Fatalf("app.Test(): %v", err)
	}
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status=%d want 409", resp.StatusCode)
	}
}
