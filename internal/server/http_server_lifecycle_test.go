package server

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/server/config"
)

// Server lifecycle tests cover construction, start/stop behavior, and graceful shutdown.

// TestNewHTTPServer verifies server construction with valid and invalid options.
// It ensures the authorizer is required and properly assigned.
func TestNewHTTPServer(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// Create authorizer for testing (insecure mode allows requests without certs).
		authorizer := auth.NewAuthorizer(auth.Options{
			AllowInsecure: true,
			DefaultRole:   auth.RoleControlPlane,
		})
		opts := HTTPOptions{
			Config: config.HTTPConfig{
				Listen: ":0",
			},
			Authorizer: authorizer,
		}
		srv, err := NewHTTPServer(opts)
		if err != nil {
			t.Fatalf("NewHTTPServer() error = %v", err)
		}
		if srv == nil {
			t.Fatal("NewHTTPServer() returned nil server")
		}
		if srv.authorizer != authorizer {
			t.Error("authorizer not set correctly")
		}
	})

	t.Run("error_missing_authorizer", func(t *testing.T) {
		// NewHTTPServer() requires an authorizer; omitting it should fail fast.
		opts := HTTPOptions{
			Config: config.HTTPConfig{
				Listen: ":0",
			},
		}
		srv, err := NewHTTPServer(opts)
		if err == nil {
			t.Fatal("NewHTTPServer() expected error for missing authorizer")
		}
		if srv != nil {
			t.Error("NewHTTPServer() should return nil server on error")
		}
	})
}

// TestHTTPServer_StartStop validates server lifecycle management.
// It covers normal start/stop, double-start prevention, and idempotent stop.
func TestHTTPServer_StartStop(t *testing.T) {
	t.Run("plain_http", func(t *testing.T) {
		// Verify basic HTTP server startup and shutdown without TLS.
		authorizer := auth.NewAuthorizer(auth.Options{
			AllowInsecure: true,
			DefaultRole:   auth.RoleControlPlane,
		})
		opts := HTTPOptions{
			Config: config.HTTPConfig{
				Listen: "127.0.0.1:0", // OS-assigned port for parallel tests.
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

		// Verify server is running and address is resolved.
		addr := srv.Addr()
		if addr == "" {
			t.Fatal("Addr() returned empty string")
		}

		// Stop the server gracefully.
		if err := srv.Stop(ctx); err != nil {
			t.Fatalf("Stop() error = %v", err)
		}

		// Verify server state is updated after stop.
		if srv.running {
			t.Error("server still marked as running after Stop()")
		}
	})

	t.Run("already_running", func(t *testing.T) {
		// Verify Start() fails when called on a running server.
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

		ctx := context.Background()
		if err := srv.Start(ctx); err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		defer func() { _ = srv.Stop(ctx) }()

		// Attempt to start again should fail.
		if err := srv.Start(ctx); err == nil {
			t.Fatal("Start() expected error when already running")
		}
	})

	t.Run("stop_when_not_running", func(t *testing.T) {
		// Verify Stop() is idempotent and safe to call when not running.
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

		ctx := context.Background()
		// Stop without starting should not error (idempotent behavior).
		if err := srv.Stop(ctx); err != nil {
			t.Fatalf("Stop() error = %v", err)
		}
	})
}

// TestHTTPServer_GracefulShutdown verifies graceful shutdown behavior.
// It ensures in-flight requests complete before the server stops.
func TestHTTPServer_GracefulShutdown(t *testing.T) {
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

	// Register a slow handler to simulate in-flight request.
	srv.RegisterRouteFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})

	ctx := context.Background()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Start a request in the background before shutdown.
	errChan := make(chan error, 1)
	go func() {
		resp, err := http.Get("http://" + srv.Addr() + "/slow")
		if err != nil {
			errChan <- err
			return
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
		errChan <- nil
	}()

	// Give the request time to start processing.
	time.Sleep(10 * time.Millisecond)

	// Stop the server (should wait for in-flight request).
	if err := srv.Stop(ctx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	// Verify in-flight request completed or got expected error.
	select {
	case err := <-errChan:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("request error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("request did not complete in time")
	}
}
