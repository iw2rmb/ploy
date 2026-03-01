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

func TestAdminAllowedForControlPlaneEndpoints(t *testing.T) {
	// In insecure mode with default role set to cli-admin, requests to a
	// handler that allows control-plane should be permitted.
	a := NewAuthorizer(Options{
		AllowInsecure: true,
		DefaultRole:   RoleCLIAdmin,
	})
	rr := httptest.NewRecorder()
	h := a.Middleware(RoleControlPlane)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/v1/runs", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rr.Code)
	}
}

func TestAdminNotAllowedForWorkerEndpoints(t *testing.T) {
	// Even as cli-admin, endpoints restricted to worker role should reject.
	a := NewAuthorizer(Options{
		AllowInsecure: true,
		DefaultRole:   RoleCLIAdmin,
	})
	rr := httptest.NewRecorder()
	h := a.Middleware(RoleWorker)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/x/heartbeat", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden, got %d", rr.Code)
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

// TestAuthorizerRoleGates verifies role-based access control with multiple allowed roles.
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
		hasTLS        bool
		wantCode      int
		wantCalled    bool
		wantErrorMsg  string
	}{
		{
			name:          "secure mode rejects request without bearer token or mTLS",
			allowInsecure: false,
			hasTLS:        false,
			wantCode:      http.StatusForbidden,
			wantCalled:    false,
			wantErrorMsg:  "authentication failed",
		},
		{
			name:          "insecure mode allows request without authentication",
			allowInsecure: true,
			hasTLS:        false,
			wantCode:      http.StatusOK,
			wantCalled:    true,
			wantErrorMsg:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := NewAuthorizer(Options{
				AllowInsecure: tt.allowInsecure,
				DefaultRole:   RoleControlPlane,
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			// Note: We don't set req.TLS to simulate missing mTLS.
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
				body := rr.Body.String()
				if !contains(body, tt.wantErrorMsg) {
					t.Errorf("expected error message to contain %q, got %q", tt.wantErrorMsg, body)
				}
			}
		})
	}
}

// contains checks if a string contains a substring (case-sensitive).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || indexOf(s, substr) >= 0)
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
