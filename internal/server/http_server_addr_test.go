package server

import (
	"context"
	"testing"

	"github.com/iw2rmb/ploy/internal/server/config"
)

func TestHTTPServer_Addr(t *testing.T) {
	t.Run("before_start", func(t *testing.T) {
		srv := newTestServer(t, config.HTTPConfig{Listen: ":8443"})
		if addr := srv.Addr(); addr != ":8443" {
			t.Errorf("expected addr ':8443', got %q", addr)
		}
	})

	t.Run("after_start", func(t *testing.T) {
		srv := newTestServer(t)
		ctx := context.Background()
		if err := srv.Start(ctx); err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		defer func() { _ = srv.Stop(ctx) }()

		if addr := srv.Addr(); addr == "" || addr == "127.0.0.1:0" {
			t.Errorf("expected resolved addr, got %q", addr)
		}
	})
}
