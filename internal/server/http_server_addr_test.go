package server

import (
	"context"
	"testing"

	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/server/config"
)

// Address tests cover configured and resolved listen addresses.

// TestHTTPServer_Addr validates address resolution behavior.
// It verifies the server returns the configured address before start
// and the resolved address (with actual port) after start.
func TestHTTPServer_Addr(t *testing.T) {
	t.Run("before_start", func(t *testing.T) {
		// Before Start(), Addr() returns the configured listen address.
		authorizer := auth.NewAuthorizer(auth.Options{
			AllowInsecure: true,
			DefaultRole:   auth.RoleControlPlane,
		})
		opts := HTTPOptions{
			Config: config.HTTPConfig{
				Listen: ":8443",
			},
			Authorizer: authorizer,
		}
		srv, err := NewHTTPServer(opts)
		if err != nil {
			t.Fatalf("NewHTTPServer() error = %v", err)
		}

		addr := srv.Addr()
		if addr != ":8443" {
			t.Errorf("expected addr ':8443', got '%s'", addr)
		}
	})

	t.Run("after_start", func(t *testing.T) {
		// After Start(), Addr() returns the resolved address (port 0 becomes actual port).
		authorizer := auth.NewAuthorizer(auth.Options{
			AllowInsecure: true,
			DefaultRole:   auth.RoleControlPlane,
		})
		opts := HTTPOptions{
			Config: config.HTTPConfig{
				Listen: "127.0.0.1:0", // Port 0 requests OS-assigned port.
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

		addr := srv.Addr()
		if addr == "" || addr == "127.0.0.1:0" {
			t.Errorf("expected resolved addr, got '%s'", addr)
		}
	})
}
