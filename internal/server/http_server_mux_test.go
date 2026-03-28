package server

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/server/config"
)

// Multiplexer tests cover handler registration via RegisterRouteFunc and RegisterRoute.

// TestHTTPServer_RegisterRouteFunc verifies the multiplexer API for handler registration.
// It validates both direct registration and role-based middleware enforcement.
func TestHTTPServer_RegisterRouteFunc(t *testing.T) {
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
		srv.RegisterRouteFunc("/test", func(w http.ResponseWriter, r *http.Request) {
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
		srv.RegisterRouteFunc("/admin", func(w http.ResponseWriter, r *http.Request) {
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

// TestHTTPServer_RegisterRoute verifies the RegisterRoute method for registering http.Handler.
// This complements RegisterRouteFunc by supporting the full http.Handler interface.
func TestHTTPServer_RegisterRoute(t *testing.T) {
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
	srv.RegisterRoute("/test", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func TestHTTPServer_RecoversHandlerPanic(t *testing.T) {
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

	srv.RegisterRouteFunc("/panic", func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	})
	srv.RegisterRouteFunc("/alive", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	ctx := context.Background()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() { _ = srv.Stop(ctx) }()

	resp, err := http.Get("http://" + srv.Addr() + "/panic")
	if err != nil {
		t.Fatalf("GET /panic error = %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("GET /panic status = %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}

	resp, err = http.Get("http://" + srv.Addr() + "/alive")
	if err != nil {
		t.Fatalf("GET /alive error = %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /alive status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

type panicErr struct{}

func (panicErr) Error() string {
	panic("panic while formatting error")
}

func TestHTTPServer_RecoversPanicWithBrokenError(t *testing.T) {
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

	srv.RegisterRouteFunc("/panic-error", func(w http.ResponseWriter, r *http.Request) {
		panic(panicErr{})
	})

	ctx := context.Background()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() { _ = srv.Stop(ctx) }()

	resp, err := http.Get("http://" + srv.Addr() + "/panic-error")
	if err != nil {
		t.Fatalf("GET /panic-error error = %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("GET /panic-error status = %d, want %d", resp.StatusCode, http.StatusInternalServerError)
	}
}
