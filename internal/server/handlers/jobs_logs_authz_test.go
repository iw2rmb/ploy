package handlers

import (
	"net/http"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/auth"
)

// TestGetJobLogs_Authorization verifies role enforcement on GET /v1/jobs/{job_id}/logs.
// The endpoint is registered with RoleControlPlane, so workers must be rejected.
func TestGetJobLogs_Authorization(t *testing.T) {
	t.Parallel()
	jobID := domaintypes.NewJobID()
	path := "/v1/jobs/" + jobID.String() + "/logs"

	tests := []struct {
		name      string
		role      auth.Role
		wantBlock bool
	}{
		{"worker", auth.RoleWorker, true},
		{"control-plane", auth.RoleControlPlane, false},
		{"cli-admin", auth.RoleCLIAdmin, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newTestServerWithRole(t, tt.role)
			rr := doRequest(t, srv.Handler(), http.MethodGet, path, nil)
			if tt.wantBlock {
				assertStatus(t, rr, http.StatusForbidden)
			} else if rr.Code == http.StatusForbidden {
				t.Errorf("got 403, want not 403 for role %s", tt.role)
			}
		})
	}
}

// TestCreateJobLogs_Authorization verifies role enforcement on POST /v1/jobs/{job_id}/logs.
// The endpoint is registered with RoleWorker, so control-plane and cli-admin must be rejected.
func TestCreateJobLogs_Authorization(t *testing.T) {
	t.Parallel()
	jobID := domaintypes.NewJobID()
	path := "/v1/jobs/" + jobID.String() + "/logs"
	body := `{"chunk_no":0,"data":"aGVsbG8="}`

	tests := []struct {
		name      string
		role      auth.Role
		wantBlock bool
	}{
		{"control-plane", auth.RoleControlPlane, true},
		{"cli-admin", auth.RoleCLIAdmin, true},
		{"worker", auth.RoleWorker, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newTestServerWithRole(t, tt.role)
			rr := doRequest(t, srv.Handler(), http.MethodPost, path, body)
			if tt.wantBlock {
				assertStatus(t, rr, http.StatusForbidden)
			} else if rr.Code == http.StatusForbidden {
				t.Errorf("got 403, want not 403 for role %s", tt.role)
			}
		})
	}
}
