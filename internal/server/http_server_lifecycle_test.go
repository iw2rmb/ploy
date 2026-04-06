package server

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestNewHTTPServer(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := newTestServer(t)
		if srv == nil {
			t.Fatal("NewHTTPServer() returned nil server")
		}
	})

	t.Run("error_missing_authorizer", func(t *testing.T) {
		srv, err := NewHTTPServer(HTTPOptions{})
		if err == nil {
			t.Fatal("NewHTTPServer() expected error for missing authorizer")
		}
		if srv != nil {
			t.Error("NewHTTPServer() should return nil server on error")
		}
	})
}

func TestHTTPServer_StartStop(t *testing.T) {
	t.Run("plain_http", func(t *testing.T) {
		srv := newTestServer(t)
		ctx := context.Background()
		if err := srv.Start(ctx); err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		if addr := srv.Addr(); addr == "" {
			t.Fatal("Addr() returned empty string")
		}
		if err := srv.Stop(ctx); err != nil {
			t.Fatalf("Stop() error = %v", err)
		}
		if srv.running {
			t.Error("server still marked as running after Stop()")
		}
	})

	t.Run("already_running", func(t *testing.T) {
		srv := newTestServer(t)
		ctx := context.Background()
		if err := srv.Start(ctx); err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		defer func() { _ = srv.Stop(ctx) }()

		if err := srv.Start(ctx); err == nil {
			t.Fatal("Start() expected error when already running")
		}
	})

	t.Run("stop_when_not_running", func(t *testing.T) {
		srv := newTestServer(t)
		if err := srv.Stop(context.Background()); err != nil {
			t.Fatalf("Stop() error = %v", err)
		}
	})
}

func TestHTTPServer_GracefulShutdown(t *testing.T) {
	srv := newTestServer(t)

	srv.RegisterRouteFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})

	ctx := context.Background()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

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

	time.Sleep(10 * time.Millisecond)

	if err := srv.Stop(ctx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	select {
	case err := <-errChan:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("request error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("request did not complete in time")
	}
}
