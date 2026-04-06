package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestAuthorizerRoleGates verifies role-based access control with multiple allowed roles
// in insecure mode, covering all role/allowlist combinations.
func TestAuthorizerRoleGates(t *testing.T) {
	tests := []struct {
		name         string
		allowedRoles []Role
		defaultRole  Role
		wantCode     int
		wantCalled   bool
	}{
		{
			name:         "single allowed role matches",
			allowedRoles: []Role{RoleControlPlane},
			defaultRole:  RoleControlPlane,
			wantCode:     http.StatusOK,
			wantCalled:   true,
		},
		{
			name:         "multiple allowed roles with match",
			allowedRoles: []Role{RoleControlPlane, RoleWorker, RoleCLIAdmin},
			defaultRole:  RoleWorker,
			wantCode:     http.StatusOK,
			wantCalled:   true,
		},
		{
			name:         "multiple allowed roles without match",
			allowedRoles: []Role{RoleControlPlane, RoleCLIAdmin},
			defaultRole:  RoleWorker,
			wantCode:     http.StatusForbidden,
			wantCalled:   false,
		},
		{
			name:         "empty allowlist permits any role",
			allowedRoles: []Role{},
			defaultRole:  RoleWorker,
			wantCode:     http.StatusOK,
			wantCalled:   true,
		},
		{
			name:         "nil allowlist permits any role",
			allowedRoles: nil,
			defaultRole:  RoleControlPlane,
			wantCode:     http.StatusOK,
			wantCalled:   true,
		},
		{
			name:         "cli-admin can access control-plane endpoints",
			allowedRoles: []Role{RoleControlPlane},
			defaultRole:  RoleCLIAdmin,
			wantCode:     http.StatusOK,
			wantCalled:   true,
		},
		{
			name:         "cli-admin cannot access worker endpoints",
			allowedRoles: []Role{RoleWorker},
			defaultRole:  RoleCLIAdmin,
			wantCode:     http.StatusForbidden,
			wantCalled:   false,
		},
		{
			name:         "worker cannot access control-plane endpoints",
			allowedRoles: []Role{RoleControlPlane},
			defaultRole:  RoleWorker,
			wantCode:     http.StatusForbidden,
			wantCalled:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := NewAuthorizer(Options{
				AllowInsecure: true,
				DefaultRole:   tt.defaultRole,
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			rr := httptest.NewRecorder()
			called := false

			handler := a.Middleware(tt.allowedRoles...)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusOK)
			}))

			handler.ServeHTTP(rr, req)

			if rr.Code != tt.wantCode {
				t.Errorf("got status %d, want %d", rr.Code, tt.wantCode)
			}
			if called != tt.wantCalled {
				t.Errorf("handler called=%v, want %v", called, tt.wantCalled)
			}
		})
	}
}

// TestAuthorizerInsecureDefaultOff verifies that AllowInsecure=false rejects unauthenticated requests.
func TestAuthorizerInsecureDefaultOff(t *testing.T) {
	tests := []struct {
		name          string
		allowInsecure bool
		wantCode      int
		wantCalled    bool
		wantErrorMsg  string
	}{
		{
			name:          "secure mode rejects request without bearer token or mTLS",
			allowInsecure: false,
			wantCode:      http.StatusUnauthorized,
			wantCalled:    false,
			wantErrorMsg:  "authentication failed",
		},
		{
			name:          "insecure mode allows request without authentication",
			allowInsecure: true,
			wantCode:      http.StatusOK,
			wantCalled:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := NewAuthorizer(Options{
				AllowInsecure: tt.allowInsecure,
				DefaultRole:   RoleControlPlane,
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			rr := httptest.NewRecorder()
			called := false

			handler := a.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusOK)
			}))

			handler.ServeHTTP(rr, req)

			if rr.Code != tt.wantCode {
				t.Errorf("got status %d, want %d", rr.Code, tt.wantCode)
			}
			if called != tt.wantCalled {
				t.Errorf("handler called=%v, want %v", called, tt.wantCalled)
			}
			if tt.wantErrorMsg != "" {
				if !strings.Contains(rr.Body.String(), tt.wantErrorMsg) {
					t.Errorf("expected error message to contain %q, got %q", tt.wantErrorMsg, rr.Body.String())
				}
			}
		})
	}
}

func TestMiddlewareNilNextReturns404(t *testing.T) {
	a := NewAuthorizer(Options{AllowInsecure: true, DefaultRole: RoleControlPlane})
	h := a.Middleware()(nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestIdentityFromContextNone(t *testing.T) {
	if _, ok := IdentityFromContext(context.TODO()); ok {
		t.Fatalf("expected no identity for nil context")
	}
}
