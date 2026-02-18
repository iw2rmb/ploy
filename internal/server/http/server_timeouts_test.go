package httpserver

import (
	"context"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/server/config"
)

// Timeout tests cover default and custom HTTP timeout configuration.

// TestServer_Timeouts validates HTTP timeout configuration.
// It verifies default timeout application and custom timeout override behavior.
func TestServer_Timeouts(t *testing.T) {
	t.Run("default_timeouts", func(t *testing.T) {
		// Verify server applies safe default timeouts when not configured.
		// Timeouts are mandatory for production servers.
		authorizer := auth.NewAuthorizer(auth.Options{
			AllowInsecure: true,
			DefaultRole:   auth.RoleControlPlane,
		})
		opts := Options{
			Config: config.HTTPConfig{
				Listen: "127.0.0.1:0",
				// No timeouts set - defaults should be applied.
			},
			Authorizer: authorizer,
		}
		srv, err := New(opts)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		ctx := context.Background()
		if err := srv.Start(ctx); err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		defer func() { _ = srv.Stop(ctx) }()

		// Verify default timeouts were applied.
		srv.mu.Lock()
		httpSrv := srv.httpServer
		srv.mu.Unlock()

		// ReadHeaderTimeout default is 10s per server implementation.
		if httpSrv.ReadHeaderTimeout != 10*time.Second {
			t.Errorf("expected ReadHeaderTimeout 10s, got %v", httpSrv.ReadHeaderTimeout)
		}
		if httpSrv.ReadTimeout != 30*time.Second {
			t.Errorf("expected ReadTimeout 30s, got %v", httpSrv.ReadTimeout)
		}
		if httpSrv.WriteTimeout != 30*time.Second {
			t.Errorf("expected WriteTimeout 30s, got %v", httpSrv.WriteTimeout)
		}
		if httpSrv.IdleTimeout != 120*time.Second {
			t.Errorf("expected IdleTimeout 120s, got %v", httpSrv.IdleTimeout)
		}
	})

	t.Run("custom_timeouts", func(t *testing.T) {
		// Verify server respects custom timeout configuration.
		authorizer := auth.NewAuthorizer(auth.Options{
			AllowInsecure: true,
			DefaultRole:   auth.RoleControlPlane,
		})
		opts := Options{
			Config: config.HTTPConfig{
				Listen:       "127.0.0.1:0",
				ReadTimeout:  5 * time.Second,
				WriteTimeout: 10 * time.Second,
				IdleTimeout:  60 * time.Second,
			},
			Authorizer: authorizer,
		}
		srv, err := New(opts)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		ctx := context.Background()
		if err := srv.Start(ctx); err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		defer func() { _ = srv.Stop(ctx) }()

		// Verify custom timeouts were applied.
		srv.mu.Lock()
		httpSrv := srv.httpServer
		srv.mu.Unlock()

		if httpSrv.ReadTimeout != 5*time.Second {
			t.Errorf("expected ReadTimeout 5s, got %v", httpSrv.ReadTimeout)
		}
		if httpSrv.WriteTimeout != 10*time.Second {
			t.Errorf("expected WriteTimeout 10s, got %v", httpSrv.WriteTimeout)
		}
		if httpSrv.IdleTimeout != 60*time.Second {
			t.Errorf("expected IdleTimeout 60s, got %v", httpSrv.IdleTimeout)
		}
	})
}
