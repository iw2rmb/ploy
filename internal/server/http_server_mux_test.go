package server

import (
	"context"
	"net/http"
	"testing"

	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/server/config"
)

// Multiplexer tests cover handler registration via HandleFunc and Handle.

// TestHTTPServer_HandleFunc verifies the multiplexer API for handler registration.
// It validates both direct registration and role-based middleware enforcement.
func TestHTTPServer_HandleFunc(t *testing.T) {
	t.Run("without_middleware", func(t *testing.T) {
		// Verify basic handler registration without middleware.
		authorizer := auth.NewAuthorizer(auth.Options{
			AllowInsecure: true,
			DefaultRole:   auth.RoleControlPlane,
		})
		opts := HTTPOptions{
			Config: config.HTTPConfig{
				Listen: "127.0.0.1:0",
			},
			Authorizer: authorizer,
		}
		srv, err := NewHTTPServer(opts)
		if err != nil {
			t.Fatalf("NewHTTPServer() error = %v", err)
		}

		// Register a test handler without role restrictions.
		srv.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		})

		ctx := context.Background()
		if err := srv.Start(ctx); err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		defer func() { _ = srv.Stop(ctx) }()

		// Make a request to verify handler is registered.
		resp, err := http.Get("http://" + srv.Addr() + "/test")
		if err != nil {
			t.Fatalf("GET /test error = %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}
	})

	t.Run("with_role_middleware", func(t *testing.T) {
		// Verify role-based access control via optional middleware.
		authorizer := auth.NewAuthorizer(auth.Options{
			AllowInsecure: true,
			DefaultRole:   auth.RoleControlPlane, // Insecure requests get ControlPlane role.
		})
		opts := HTTPOptions{
			Config: config.HTTPConfig{
				Listen: "127.0.0.1:0",
			},
			Authorizer: authorizer,
		}
		srv, err := NewHTTPServer(opts)
		if err != nil {
			t.Fatalf("NewHTTPServer() error = %v", err)
		}

		// Register handler requiring CLIAdmin role (higher than default).
		srv.HandleFunc("/admin", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("admin"))
		}, auth.RoleCLIAdmin)

		ctx := context.Background()
		if err := srv.Start(ctx); err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		defer func() { _ = srv.Stop(ctx) }()

		// Request should be forbidden (ControlPlane < CLIAdmin).
		resp, err := http.Get("http://" + srv.Addr() + "/admin")
		if err != nil {
			t.Fatalf("GET /admin error = %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected status 403, got %d", resp.StatusCode)
		}
	})
}

// TestHTTPServer_Handle verifies the Handle method for registering http.Handler.
// This complements HandleFunc by supporting the full http.Handler interface.
func TestHTTPServer_Handle(t *testing.T) {
	authorizer := auth.NewAuthorizer(auth.Options{
		AllowInsecure: true,
		DefaultRole:   auth.RoleControlPlane,
	})
	opts := HTTPOptions{
		Config: config.HTTPConfig{
			Listen: "127.0.0.1:0",
		},
		Authorizer: authorizer,
	}
	srv, err := NewHTTPServer(opts)
	if err != nil {
		t.Fatalf("NewHTTPServer() error = %v", err)
	}

	// Register a handler using Handle (http.Handler interface).
	srv.Handle("/test", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("test"))
	}))

	ctx := context.Background()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() { _ = srv.Stop(ctx) }()

	resp, err := http.Get("http://" + srv.Addr() + "/test")
	if err != nil {
		t.Fatalf("GET /test error = %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}
