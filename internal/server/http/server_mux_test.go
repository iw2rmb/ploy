package httpserver

import (
	"context"
	"net/http"
	"testing"

	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/server/config"
)

// Multiplexer tests cover handler registration via HandleFunc and Handle.

// TestServer_HandleFunc verifies the multiplexer API for handler registration.
// It validates both direct registration and role-based middleware enforcement.
func TestServer_HandleFunc(t *testing.T) {
	t.Run("without_middleware", func(t *testing.T) {
		// Verify basic handler registration without middleware.
		authorizer := auth.NewAuthorizer(auth.Options{
			AllowInsecure: true,
			DefaultRole:   auth.RoleControlPlane,
		})
		opts := Options{
			Config: config.HTTPConfig{
				Listen: "127.0.0.1:0",
				TLS: config.TLSConfig{
					Enabled: false,
				},
			},
			Authorizer: authorizer,
		}
		srv, err := New(opts)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		// Register a test handler without role restrictions.
		srv.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ok"))
		})

		ctx := context.Background()
		if err := srv.Start(ctx); err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		defer srv.Stop(ctx)

		// Make a request to verify handler is registered.
		resp, err := http.Get("http://" + srv.Addr() + "/test")
		if err != nil {
			t.Fatalf("GET /test error = %v", err)
		}
		defer resp.Body.Close()

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
		opts := Options{
			Config: config.HTTPConfig{
				Listen: "127.0.0.1:0",
				TLS: config.TLSConfig{
					Enabled: false,
				},
			},
			Authorizer: authorizer,
		}
		srv, err := New(opts)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		// Register handler requiring CLIAdmin role (higher than default).
		srv.HandleFunc("/admin", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("admin"))
		}, auth.RoleCLIAdmin)

		ctx := context.Background()
		if err := srv.Start(ctx); err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		defer srv.Stop(ctx)

		// Request should be forbidden (ControlPlane < CLIAdmin).
		resp, err := http.Get("http://" + srv.Addr() + "/admin")
		if err != nil {
			t.Fatalf("GET /admin error = %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected status 403, got %d", resp.StatusCode)
		}
	})
}

// TestServer_Handle verifies the Handle method for registering http.Handler.
// This complements HandleFunc by supporting the full http.Handler interface.
func TestServer_Handle(t *testing.T) {
	authorizer := auth.NewAuthorizer(auth.Options{
		AllowInsecure: true,
		DefaultRole:   auth.RoleControlPlane,
	})
	opts := Options{
		Config: config.HTTPConfig{
			Listen: "127.0.0.1:0",
			TLS: config.TLSConfig{
				Enabled: false,
			},
		},
		Authorizer: authorizer,
	}
	srv, err := New(opts)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Register a handler using Handle (http.Handler interface).
	srv.Handle("/test", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test"))
	}))

	ctx := context.Background()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer srv.Stop(ctx)

	resp, err := http.Get("http://" + srv.Addr() + "/test")
	if err != nil {
		t.Fatalf("GET /test error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}
