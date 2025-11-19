package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/server/config"
	"github.com/iw2rmb/ploy/internal/server/events"
	httpapi "github.com/iw2rmb/ploy/internal/server/http"
)

// TestConfigGitLab_AdminOnly verifies GET/PUT require cli-admin role
// and are rejected for default control-plane callers.
func TestConfigGitLab_AdminOnly(t *testing.T) {
	// Server with default role: control-plane (not admin)
	aCP := auth.NewAuthorizer(auth.Options{AllowInsecure: true, DefaultRole: auth.RoleControlPlane})
	sCP, err := httpapi.New(httpapi.Options{Authorizer: aCP})
	if err != nil {
		t.Fatalf("http server: %v", err)
	}
	ev, err := events.New(events.Options{})
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	RegisterRoutes(sCP, &mockStore{}, ev, NewConfigHolder(config.GitLabConfig{}), "test-secret")

	// GET should be forbidden for control-plane.
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/config/gitlab", nil)
	sCP.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("GET status=%d, want 403", rr.Code)
	}

	// PUT should be forbidden for control-plane.
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/v1/config/gitlab", bytes.NewBufferString(`{"domain":"d","token":"t"}`))
	req.Header.Set("Content-Type", "application/json")
	sCP.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("PUT status=%d, want 403", rr.Code)
	}

	// Server with default role: cli-admin (allowed)
	aAdmin := auth.NewAuthorizer(auth.Options{AllowInsecure: true, DefaultRole: auth.RoleCLIAdmin})
	sAdmin, err := httpapi.New(httpapi.Options{Authorizer: aAdmin})
	if err != nil {
		t.Fatalf("http server: %v", err)
	}
	RegisterRoutes(sAdmin, &mockStore{}, ev, NewConfigHolder(config.GitLabConfig{}), "test-secret")

	// GET should succeed for cli-admin.
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/config/gitlab", nil)
	sAdmin.Handler().ServeHTTP(rr, req)
	if rr.Code == http.StatusForbidden || rr.Code == http.StatusNotFound {
		t.Fatalf("GET status=%d, want not 403/404", rr.Code)
	}
}
