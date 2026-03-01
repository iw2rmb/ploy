package server

import (
	"context"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/server/config"
)

// Timeout tests cover default and custom HTTP timeout configuration.

// TestHTTPServer_Timeouts validates HTTP timeout configuration.
// It verifies default timeout application and custom timeout override behavior.
func TestHTTPServer_Timeouts(t *testing.T) {
	t.Run("default_timeouts", func(t *testing.T) {
		// Verify server uses config defaults (applied by config.Load / applyDefaults).
		// HTTPServer itself does not apply fallbacks — config loading is the
		// single source of truth for default timeouts.
		authorizer := auth.NewAuthorizer(auth.Options{
			AllowInsecure: true,
			DefaultRole:   auth.RoleControlPlane,
		})
		opts := HTTPOptions{
			Config: config.HTTPConfig{
				Listen:       "127.0.0.1:0",
				ReadTimeout:  15 * time.Second,
				WriteTimeout: 15 * time.Second,
				IdleTimeout:  60 * time.Second,
			},
			Authorizer: authorizer,
		}
		srv, err := NewHTTPServer(opts)
		if err != nil {
			t.Fatalf("NewHTTPServer() error = %v", err)
		}

		ctx := context.Background()
		if err := srv.Start(ctx); err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		defer func() { _ = srv.Stop(ctx) }()

		srv.mu.Lock()
		httpSrv := srv.httpServer
		srv.mu.Unlock()

		if httpSrv.ReadHeaderTimeout != httpReadHeaderTimeout {
			t.Errorf("expected ReadHeaderTimeout %v, got %v", httpReadHeaderTimeout, httpSrv.ReadHeaderTimeout)
		}
		if httpSrv.ReadTimeout != 15*time.Second {
			t.Errorf("expected ReadTimeout 15s, got %v", httpSrv.ReadTimeout)
		}
		if httpSrv.WriteTimeout != 15*time.Second {
			t.Errorf("expected WriteTimeout 15s, got %v", httpSrv.WriteTimeout)
		}
		if httpSrv.IdleTimeout != 60*time.Second {
			t.Errorf("expected IdleTimeout 60s, got %v", httpSrv.IdleTimeout)
		}
	})

	t.Run("custom_timeouts", func(t *testing.T) {
		// Verify server respects custom timeout configuration.
		authorizer := auth.NewAuthorizer(auth.Options{
			AllowInsecure: true,
			DefaultRole:   auth.RoleControlPlane,
		})
		opts := HTTPOptions{
			Config: config.HTTPConfig{
				Listen:       "127.0.0.1:0",
				ReadTimeout:  5 * time.Second,
				WriteTimeout: 10 * time.Second,
				IdleTimeout:  60 * time.Second,
			},
			Authorizer: authorizer,
		}
		srv, err := NewHTTPServer(opts)
		if err != nil {
			t.Fatalf("NewHTTPServer() error = %v", err)
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
