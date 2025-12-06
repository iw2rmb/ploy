package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/iw2rmb/ploy/internal/server/auth"
	httpapi "github.com/iw2rmb/ploy/internal/server/http"
)

// Sanity check that GET /v1/buildgate/jobs/{id} pattern mounts and is routable.
func TestRoute_Mounts_BuildGateJobsGet(t *testing.T) {
	authz := auth.NewAuthorizer(auth.Options{AllowInsecure: true, DefaultRole: auth.RoleControlPlane})
	srv, err := httpapi.New(httpapi.Options{Authorizer: authz})
	if err != nil {
		t.Fatalf("server: %v", err)
	}
	// Register only the target route
	srv.HandleFunc("GET /v1/buildgate/jobs/{id}", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }, auth.RoleControlPlane)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/buildgate/jobs/123e4567-e89b-12d3-a456-426614174000", nil)
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code == http.StatusNotFound {
		t.Fatalf("route returned 404 with direct registration")
	}
}
