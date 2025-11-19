package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/server/config"
	"github.com/iw2rmb/ploy/internal/server/events"
	httpapi "github.com/iw2rmb/ploy/internal/server/http"
)

// TestCancelTicket_Authorization verifies that the cancel endpoint is mounted
// with RoleControlPlane authorization (and implicitly allows cli-admin),
// while denying worker callers.
func TestCancelTicket_Authorization(t *testing.T) {
	// Common stubbed store: return not found so we can observe authorization
	// outcome via HTTP status (403 vs non-404). Route existence is covered
	// elsewhere; here we want role gating.
	st := &mockStore{getRunErr: pgx.ErrNoRows}
	ev, err := events.New(events.Options{})
	if err != nil {
		t.Fatalf("events: %v", err)
	}

	newServer := func(defaultRole string) *httpapi.Server {
		authz := auth.NewAuthorizer(auth.Options{AllowInsecure: true, DefaultRole: defaultRole})
		srv, err := httpapi.New(httpapi.Options{Authorizer: authz})
		if err != nil {
			t.Fatalf("http server: %v", err)
		}
		RegisterRoutes(srv, st, ev, NewConfigHolder(config.GitLabConfig{}), "test-secret")
		return srv
	}

	id := uuid.New().String()
	url := "/v1/mods/" + id + "/cancel"

	// Control-plane callers are allowed (expect 404 from handler/store).
	{
		srv := newServer(auth.RoleControlPlane)
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, url, nil)
		srv.Handler().ServeHTTP(rr, req)
		if rr.Code == http.StatusForbidden {
			t.Fatalf("control-plane should be allowed; got 403")
		}
	}

	// CLI admin is allowed as a superset for control-plane endpoints.
	{
		srv := newServer(auth.RoleCLIAdmin)
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, url, nil)
		srv.Handler().ServeHTTP(rr, req)
		if rr.Code == http.StatusForbidden {
			t.Fatalf("cli-admin should be allowed; got 403")
		}
	}

	// Worker callers are denied.
	{
		srv := newServer(auth.RoleWorker)
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, url, nil)
		srv.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Fatalf("worker should be forbidden; got %d", rr.Code)
		}
	}
}
