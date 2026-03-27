package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/iw2rmb/ploy/internal/server/auth"
)

// TestConfigEnv_AdminOnly verifies that /v1/config/env endpoints require cli-admin role
// and are rejected for control-plane and worker callers.
func TestConfigEnv_AdminOnly(t *testing.T) {
	// Test cases for each endpoint.
	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{
			name:   "GET /v1/config/env",
			method: http.MethodGet,
			path:   "/v1/config/env",
		},
		{
			name:   "GET /v1/config/env/{key}",
			method: http.MethodGet,
			path:   "/v1/config/env/TEST_KEY",
		},
		{
			name:   "PUT /v1/config/env/{key}",
			method: http.MethodPut,
			path:   "/v1/config/env/TEST_KEY",
			body:   `{"value":"test","scope":"all"}`,
		},
		{
			name:   "DELETE /v1/config/env/{key}",
			method: http.MethodDelete,
			path:   "/v1/config/env/TEST_KEY",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Control-plane callers should be forbidden.
			t.Run("control-plane forbidden", func(t *testing.T) {
				srv := newTestServerWithRole(t, auth.RoleControlPlane)
				var body *bytes.Buffer
				if tt.body != "" {
					body = bytes.NewBufferString(tt.body)
				}
				var req *http.Request
				if body != nil {
					req = httptest.NewRequest(tt.method, tt.path, body)
					req.Header.Set("Content-Type", "application/json")
				} else {
					req = httptest.NewRequest(tt.method, tt.path, nil)
				}
				rr := httptest.NewRecorder()
				srv.Handler().ServeHTTP(rr, req)

				assertStatus(t, rr, http.StatusForbidden)
			})

			// Worker callers should be forbidden.
			t.Run("worker forbidden", func(t *testing.T) {
				srv := newTestServerWithRole(t, auth.RoleWorker)
				var body *bytes.Buffer
				if tt.body != "" {
					body = bytes.NewBufferString(tt.body)
				}
				var req *http.Request
				if body != nil {
					req = httptest.NewRequest(tt.method, tt.path, body)
					req.Header.Set("Content-Type", "application/json")
				} else {
					req = httptest.NewRequest(tt.method, tt.path, nil)
				}
				rr := httptest.NewRecorder()
				srv.Handler().ServeHTTP(rr, req)

				assertStatus(t, rr, http.StatusForbidden)
			})

			// CLI admin callers should be allowed (not forbidden).
			t.Run("cli-admin allowed", func(t *testing.T) {
				srv := newTestServerWithRole(t, auth.RoleCLIAdmin)
				var body *bytes.Buffer
				if tt.body != "" {
					body = bytes.NewBufferString(tt.body)
				}
				var req *http.Request
				if body != nil {
					req = httptest.NewRequest(tt.method, tt.path, body)
					req.Header.Set("Content-Type", "application/json")
				} else {
					req = httptest.NewRequest(tt.method, tt.path, nil)
				}
				rr := httptest.NewRecorder()
				srv.Handler().ServeHTTP(rr, req)

				// Should not be forbidden (may return other errors like 404, but not 403).
				if rr.Code == http.StatusForbidden {
					t.Errorf("cli-admin: got %d, want not 403", rr.Code)
				}
			})
		})
	}
}
