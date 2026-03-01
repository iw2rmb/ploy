package server_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/server"
	"github.com/iw2rmb/ploy/internal/server/config"
)

func TestServerStartStop(t *testing.T) {
	srv := server.NewMetricsServer("127.0.0.1:0")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	resp, err := http.Get("http://" + srv.Addr() + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	_ = resp.Body.Close()
	if err := srv.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestServerReload(t *testing.T) {
	srv := server.NewMetricsServer("127.0.0.1:0")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	oldAddr := srv.Addr()
	if err := srv.Reload(context.Background(), config.MetricsConfig{Listen: "127.0.0.1:0"}); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	if err := srv.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if srv.MetricsConfig().Listen == oldAddr {
		t.Fatalf("expected listen address updated")
	}
}

func TestServerStopTimeout(t *testing.T) {
	srv := server.NewMetricsServer("127.0.0.1:0")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	timeoutCtx, cancelTimeout := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancelTimeout()
	if err := srv.Stop(timeoutCtx); err != nil && err != context.DeadlineExceeded {
		t.Fatalf("Stop() unexpected error = %v", err)
	}
}
