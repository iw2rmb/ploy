package handlers

import (
	"net/http"
	"testing"

	"github.com/iw2rmb/ploy/internal/server/auth"
)

// TestConfigEndpoints_AdminOnly verifies that all config endpoints require
// cli-admin role and are rejected for control-plane and worker callers.
func TestConfigEndpoints_AdminOnly(t *testing.T) {
	endpoints := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "GET /v1/config/gitlab", method: http.MethodGet, path: "/v1/config/gitlab"},
		{name: "PUT /v1/config/gitlab", method: http.MethodPut, path: "/v1/config/gitlab", body: `{"domain":"https://gitlab.example.com","token":"test-token"}`},
		{name: "GET /v1/config/env", method: http.MethodGet, path: "/v1/config/env"},
		{name: "GET /v1/config/env/{key}", method: http.MethodGet, path: "/v1/config/env/TEST_KEY"},
		{name: "PUT /v1/config/env/{key}", method: http.MethodPut, path: "/v1/config/env/TEST_KEY", body: `{"value":"test","target":"all"}`},
		{name: "DELETE /v1/config/env/{key}", method: http.MethodDelete, path: "/v1/config/env/TEST_KEY"},
	}

	assertAdminOnly(t, endpoints)
}

// assertAdminOnly checks that each endpoint is forbidden for control-plane and
// worker roles, and allowed (not 403) for cli-admin.
func assertAdminOnly(t *testing.T, endpoints []struct {
	name   string
	method string
	path   string
	body   string
}) {
	t.Helper()

	roles := []struct {
		role       auth.Role
		name       string
		wantBlock  bool
	}{
		{auth.RoleControlPlane, "control-plane", true},
		{auth.RoleWorker, "worker", true},
		{auth.RoleCLIAdmin, "cli-admin", false},
	}

	for _, ep := range endpoints {
		t.Run(ep.name, func(t *testing.T) {
			for _, rc := range roles {
				t.Run(rc.name, func(t *testing.T) {
					srv := newTestServerWithRole(t, rc.role)

					var body any
					if ep.body != "" {
						body = ep.body
					}
					rr := doRequest(t, srv.Handler(), ep.method, ep.path, body)

					if rc.wantBlock {
						assertStatus(t, rr, http.StatusForbidden)
					} else if rr.Code == http.StatusForbidden {
						t.Errorf("%s: got 403, want not 403", rc.name)
					}
				})
			}
		})
	}
}
