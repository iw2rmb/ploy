package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthorizerAllowsInsecureDefaultRole(t *testing.T) {
	a := NewAuthorizer(Options{
		AllowInsecure: true,
		DefaultRole:   RoleControlPlane,
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/nodes", nil)
	rr := httptest.NewRecorder()
	called := false
	handler := a.Middleware(RoleControlPlane)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !called {
		t.Fatalf("expected wrapped handler invoked")
	}
}

func TestAuthorizerRejectsForbiddenRole(t *testing.T) {
	a := NewAuthorizer(Options{
		AllowInsecure: true,
		DefaultRole:   RoleWorker,
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/nodes", nil)
	rr := httptest.NewRecorder()
	handler := a.Middleware(RoleControlPlane)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}
