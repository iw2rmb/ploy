package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/jackc/pgx/v5"
)

type reportStore struct {
	*handlerStore
	reports []store.UpdateJobReportParams
}

func (s *reportStore) UpdateJobReport(_ context.Context, p store.UpdateJobReportParams) (int64, error) {
	s.reports = append(s.reports, p)
	return 1, nil
}

func TestJobReportAcceptsTextAndEnforcesOwnershipAndLimits(t *testing.T) {
	for _, tc := range []struct {
		name, body         string
		missing, wrongNode bool
		want               int
	}{
		{"multiline", "hello\n мир \n", false, false, 204},
		{"clear", "", false, false, 204},
		{"limit", strings.Repeat("x", DefaultMaxBodySize), false, false, 204},
		{"oversize", strings.Repeat("x", DefaultMaxBodySize+1), false, false, 413},
		{"invalid utf8", "\xff", false, false, 400},
		{"nul", "x\x00y", false, false, 400},
		{"missing", "text", true, false, 404},
		{"wrong node", "text", false, true, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			jobID, nodeID := types.NewJobID(), types.NodeID(types.NewNodeKey())
			st := &reportStore{handlerStore: &handlerStore{}}
			st.getJob.val = store.Job{ID: jobID, NodeID: &nodeID, Status: types.JobStatusSuccess}
			if tc.missing {
				st.getJob.err = pgx.ErrNoRows
			}
			caller := nodeID
			if tc.wrongNode {
				caller = types.NodeID(types.NewNodeKey())
			}
			req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/report", strings.NewReader(tc.body))
			req.SetPathValue("job_id", jobID.String())
			req.Header.Set(nodeUUIDHeader, caller.String())
			req = req.WithContext(auth.ContextWithIdentity(req.Context(), auth.Identity{Role: auth.RoleWorker, CommonName: caller.String()}))
			rr := httptest.NewRecorder()
			saveJobReportHandler(st).ServeHTTP(rr, req)
			assertStatus(t, rr, tc.want)
			if tc.want == 204 {
				if len(st.reports) != 1 || st.reports[0].Report != tc.body || st.reports[0].ID != jobID {
					t.Fatalf("report not preserved: %+v", st.reports)
				}
			} else if len(st.reports) != 0 {
				t.Fatal("invalid report persisted")
			}
		})
	}
}

func TestJobReportStatusProjectionsPreserveText(t *testing.T) {
	job := store.Job{Meta: []byte(`{"kind":"mig","report":"first\nsecond\n"}`)}
	if jobStatusFromStore(job).Report != "first\nsecond\n" || runJobFromStore(job).Report != "first\nsecond\n" {
		t.Fatal("report lost in status projection")
	}
}

func TestJobReportAuthorization(t *testing.T) {
	t.Run("unauthenticated", func(t *testing.T) {
		authorizer := auth.NewAuthorizer(auth.Options{})
		rr := httptest.NewRecorder()
		authorizer.Middleware(auth.RoleWorker)(saveJobReportHandler(nil)).ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/v1/jobs/"+types.NewJobID().String()+"/report", strings.NewReader("text")))
		assertStatus(t, rr, http.StatusUnauthorized)
	})
	for _, role := range []auth.Role{auth.RoleControlPlane, auth.RoleCLIAdmin, auth.RoleWorker} {
		t.Run(string(role), func(t *testing.T) {
			srv := newTestServerWithRole(t, role)
			rr := doRequest(t, srv.Handler(), http.MethodPost, "/v1/jobs/"+types.NewJobID().String()+"/report", "findings")
			if role != auth.RoleWorker {
				assertStatus(t, rr, http.StatusForbidden)
			} else {
				assertStatus(t, rr, http.StatusBadRequest)
			}
		})
	}
}
