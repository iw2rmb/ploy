package server

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/iw2rmb/ploy/internal/server/auth"
)

func TestHTTPServer_RegisterRouteFunc(t *testing.T) {
	t.Run("without_middleware", func(t *testing.T) {
		srv := newTestServer(t)
		srv.RegisterRouteFunc("/test", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		})

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
	})

	t.Run("with_role_middleware", func(t *testing.T) {
		srv := newTestServer(t)
		srv.RegisterRouteFunc("/admin", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("admin"))
		}, auth.RoleCLIAdmin)

		ctx := context.Background()
		if err := srv.Start(ctx); err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		defer func() { _ = srv.Stop(ctx) }()

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

func TestHTTPServer_RegisterRoute(t *testing.T) {
	srv := newTestServer(t)
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
	srv := newTestServer(t)
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
	srv := newTestServer(t)
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
