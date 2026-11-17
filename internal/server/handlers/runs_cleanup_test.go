package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/jackc/pgx/v5/pgtype"
)

// The run read exposes the same identity that authorized cleanup.
func TestGetRunCleanupStateResponse(t *testing.T) {
	finished := time.Now().UTC().Truncate(time.Second)
	id := types.NewRunID()
	st := &handlerStore{cleanupEligible: true}
	st.getRun.val = store.Run{ID: id, MigID: types.NewMigID(), SpecID: types.NewSpecID(), Status: types.RunStatusFail, Attempt: 3, Stats: []byte(`{"resume_count":2}`), FinishedAt: pgtype.Timestamptz{Time: finished, Valid: true}}
	req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+id.String(), nil)
	req.SetPathValue("run_id", id.String())
	rr := httptest.NewRecorder()
	getRunHandler(st).ServeHTTP(rr, req)
	assertStatus(t, rr, http.StatusOK)
	var got types.RunSummary
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ID != id || got.Attempt != 3 || got.ResumeCount != 2 || got.Status != types.RunStatusFail || got.FinishedAt == nil || !got.FinishedAt.Equal(finished) || !got.CleanupEligible {
		t.Fatalf("run cleanup response: %+v", got)
	}
}

// Workers can inspect eligibility through the existing run read route.
func TestGetRunCleanupStateAuthorization(t *testing.T) {
	for _, role := range []auth.Role{auth.RoleWorker, auth.RoleControlPlane, auth.RoleCLIAdmin} {
		t.Run(string(role), func(t *testing.T) {
			srv := newTestServerWithRole(t, role)
			rr := doRequest(t, srv.Handler(), http.MethodGet, "/v1/runs/"+types.NewRunID().String(), nil)
			if rr.Code == http.StatusForbidden {
				t.Fatalf("run read rejected role %s", role)
			}
		})
	}
}
