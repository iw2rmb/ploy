package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/store"
)

type grypeTokenQuerier struct {
	store.Querier
	activeJob bool
}

func (q *grypeTokenQuerier) IsJobTokenActive(context.Context, string) (bool, error) {
	return q.activeJob, nil
}

func (*grypeTokenQuerier) CheckAPITokenRevoked(context.Context, string) (pgtype.Timestamptz, error) {
	return pgtype.Timestamptz{}, pgx.ErrNoRows
}

func (*grypeTokenQuerier) UpdateAPITokenLastUsed(context.Context, string) error { return nil }

func TestGrypeDatabaseRouteRequiresAuthorizedBearer(t *testing.T) {
	const secret = "grype-route-test"
	for _, tt := range []struct {
		name      string
		role      auth.Role
		activeJob bool
		want      int
	}{
		{"missing token", "", false, http.StatusUnauthorized},
		{"active job", auth.RoleJob, true, http.StatusBadRequest},
		{"inactive job", auth.RoleJob, false, http.StatusUnauthorized},
		{"worker", auth.RoleWorker, false, http.StatusBadRequest},
		{"control-plane", auth.RoleControlPlane, false, http.StatusBadRequest},
		{"cli-admin", auth.RoleCLIAdmin, false, http.StatusBadRequest},
	} {
		t.Run(tt.name, func(t *testing.T) {
			authorizer := auth.NewAuthorizer(auth.Options{TokenSecret: secret, Querier: &grypeTokenQuerier{activeJob: tt.activeJob}})
			t.Cleanup(authorizer.Wait)
			srv := newTestServerWithAuthorizer(t, authorizer)
			// The proxy rejects this path before contacting Anchore, after authorization succeeds.
			req := httptest.NewRequest(http.MethodGet, "/v1/grype/databases/v6/%2e%2e/latest.json", nil)
			if tt.role != "" {
				token, err := auth.GenerateAPIToken(secret, string(tt.role), time.Now().Add(time.Hour))
				if err != nil {
					t.Fatal(err)
				}
				req.Header.Set("Authorization", "Bearer "+token)
			}
			rr := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rr, req)
			assertStatus(t, rr, tt.want)
		})
	}
}
