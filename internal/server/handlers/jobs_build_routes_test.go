package handlers

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"

	bsmock "github.com/iw2rmb/ploy/internal/blobstore/mock"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server"
	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/server/config"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestChildBuildRoutes_PathAndRequestValidation(t *testing.T) {
	t.Parallel()

	f := newJobFixture("mig")

	tests := []struct {
		name             string
		method           string
		path             string
		pathValues       map[string]string
		body             string
		nodeIDHeader     string
		store            *jobStore
		wantStatus       int
		wantGetJobCalled bool
	}{
		{
			name:         "post rejects empty parent_job_id before store calls",
			method:       http.MethodPost,
			path:         "/v1/jobs//builds",
			pathValues:   map[string]string{"parent_job_id": ""},
			body:         `{"build_kind":"re_gate","reason":"child_build_validation"}`,
			nodeIDHeader: f.NodeIDStr,
			store:        &jobStore{},
			wantStatus:   http.StatusBadRequest,
		},
		{
			name:         "post rejects unknown parent_job_type field",
			method:       http.MethodPost,
			path:         "/v1/jobs/" + f.JobID.String() + "/builds",
			pathValues:   map[string]string{"parent_job_id": f.JobID.String()},
			body:         `{"build_kind":"re_gate","reason":"child_build_validation","parent_job_type":"mig"}`,
			nodeIDHeader: f.NodeIDStr,
			store:        &jobStore{},
			wantStatus:   http.StatusBadRequest,
		},
		{
			name:             "post route is worker-auth reachable after contract validation",
			method:           http.MethodPost,
			path:             "/v1/jobs/" + f.JobID.String() + "/builds",
			pathValues:       map[string]string{"parent_job_id": f.JobID.String()},
			body:             `{"build_kind":"re_gate","reason":"child_build_validation"}`,
			nodeIDHeader:     f.NodeIDStr,
			store:            &jobStore{getJobResult: f.Job},
			wantStatus:       http.StatusNotImplemented,
			wantGetJobCalled: true,
		},
		{
			name:         "get rejects empty parent_job_id before store calls",
			method:       http.MethodGet,
			path:         "/v1/jobs//builds/" + domaintypes.NewJobID().String(),
			pathValues:   map[string]string{"parent_job_id": "", "child_job_id": domaintypes.NewJobID().String()},
			nodeIDHeader: f.NodeIDStr,
			store:        &jobStore{},
			wantStatus:   http.StatusBadRequest,
		},
		{
			name:         "get rejects empty child_job_id before store calls",
			method:       http.MethodGet,
			path:         "/v1/jobs/" + f.JobID.String() + "/builds/",
			pathValues:   map[string]string{"parent_job_id": f.JobID.String(), "child_job_id": ""},
			nodeIDHeader: f.NodeIDStr,
			store:        &jobStore{},
			wantStatus:   http.StatusBadRequest,
		},
		{
			name:             "get route is worker-auth reachable after contract validation",
			method:           http.MethodGet,
			path:             "/v1/jobs/" + f.JobID.String() + "/builds/" + domaintypes.NewJobID().String(),
			pathValues:       map[string]string{"parent_job_id": f.JobID.String(), "child_job_id": domaintypes.NewJobID().String()},
			nodeIDHeader:     f.NodeIDStr,
			store:            &jobStore{getJobResult: f.Job},
			wantStatus:       http.StatusNotImplemented,
			wantGetJobCalled: true,
		},
		{
			name:             "parent not found",
			method:           http.MethodGet,
			path:             "/v1/jobs/" + f.JobID.String() + "/builds/" + domaintypes.NewJobID().String(),
			pathValues:       map[string]string{"parent_job_id": f.JobID.String(), "child_job_id": domaintypes.NewJobID().String()},
			nodeIDHeader:     f.NodeIDStr,
			store:            &jobStore{getJobErr: pgx.ErrNoRows},
			wantStatus:       http.StatusNotFound,
			wantGetJobCalled: true,
		},
		{
			name:             "store error",
			method:           http.MethodGet,
			path:             "/v1/jobs/" + f.JobID.String() + "/builds/" + domaintypes.NewJobID().String(),
			pathValues:       map[string]string{"parent_job_id": f.JobID.String(), "child_job_id": domaintypes.NewJobID().String()},
			nodeIDHeader:     f.NodeIDStr,
			store:            &jobStore{getJobErr: errors.New("db down")},
			wantStatus:       http.StatusInternalServerError,
			wantGetJobCalled: true,
		},
		{
			name:             "parent owner mismatch",
			method:           http.MethodGet,
			path:             "/v1/jobs/" + f.JobID.String() + "/builds/" + domaintypes.NewJobID().String(),
			pathValues:       map[string]string{"parent_job_id": f.JobID.String(), "child_job_id": domaintypes.NewJobID().String()},
			nodeIDHeader:     domaintypes.NewNodeKey(),
			store:            &jobStore{getJobResult: f.Job},
			wantStatus:       http.StatusForbidden,
			wantGetJobCalled: true,
		},
		{
			name:   "parent type derived from parent_job_id",
			method: http.MethodGet,
			path:   "/v1/jobs/" + f.JobID.String() + "/builds/" + domaintypes.NewJobID().String(),
			pathValues: map[string]string{
				"parent_job_id": f.JobID.String(),
				"child_job_id":  domaintypes.NewJobID().String(),
			},
			nodeIDHeader:     f.NodeIDStr,
			store:            &jobStore{getJobResult: store.Job{ID: f.JobID, NodeID: &f.NodeID, JobType: "pre_gate"}},
			wantStatus:       http.StatusConflict,
			wantGetJobCalled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var body *bytes.Reader
			if tt.body == "" {
				body = bytes.NewReader(nil)
			} else {
				body = bytes.NewReader([]byte(tt.body))
			}
			req := httptest.NewRequest(tt.method, tt.path, body)
			for key, value := range tt.pathValues {
				req.SetPathValue(key, value)
			}
			if tt.nodeIDHeader != "" {
				req.Header.Set(nodeUUIDHeader, tt.nodeIDHeader)
			}
			if tt.method == http.MethodPost {
				req.Header.Set("Content-Type", "application/json")
			}

			rr := httptest.NewRecorder()
			switch tt.method {
			case http.MethodPost:
				createJobBuildHandler(tt.store).ServeHTTP(rr, req)
			case http.MethodGet:
				getJobBuildStatusHandler(tt.store).ServeHTTP(rr, req)
			default:
				t.Fatalf("unsupported method %s", tt.method)
			}

			assertStatus(t, rr, tt.wantStatus)
			if tt.store.getJobCalled != tt.wantGetJobCalled {
				t.Fatalf("GetJob called = %v, want %v", tt.store.getJobCalled, tt.wantGetJobCalled)
			}
		})
	}
}

func TestChildBuildRoutes_RoutedWorkerAuthAndMalformedPathValues(t *testing.T) {
	t.Parallel()

	f := newJobFixture("mig")
	validChildID := domaintypes.NewJobID().String()
	validBody := `{"build_kind":"re_gate","reason":"child_build_validation"}`

	tests := []struct {
		name             string
		role             auth.Role
		method           string
		path             string
		body             string
		nodeIDHeader     string
		wantStatus       int
		wantGetJobCalled bool
	}{
		{
			name:             "post route reachable under worker auth",
			role:             auth.RoleWorker,
			method:           http.MethodPost,
			path:             "/v1/jobs/" + f.JobID.String() + "/builds",
			body:             validBody,
			nodeIDHeader:     f.NodeIDStr,
			wantStatus:       http.StatusNotImplemented,
			wantGetJobCalled: true,
		},
		{
			name:             "get route reachable under worker auth",
			role:             auth.RoleWorker,
			method:           http.MethodGet,
			path:             "/v1/jobs/" + f.JobID.String() + "/builds/" + validChildID,
			nodeIDHeader:     f.NodeIDStr,
			wantStatus:       http.StatusNotImplemented,
			wantGetJobCalled: true,
		},
		{
			name:             "post rejects malformed parent_job_id at routed endpoint",
			role:             auth.RoleWorker,
			method:           http.MethodPost,
			path:             "/v1/jobs/not-a-ksuid/builds",
			body:             validBody,
			nodeIDHeader:     f.NodeIDStr,
			wantStatus:       http.StatusBadRequest,
			wantGetJobCalled: false,
		},
		{
			name:             "get rejects malformed parent_job_id at routed endpoint",
			role:             auth.RoleWorker,
			method:           http.MethodGet,
			path:             "/v1/jobs/not-a-ksuid/builds/" + validChildID,
			nodeIDHeader:     f.NodeIDStr,
			wantStatus:       http.StatusBadRequest,
			wantGetJobCalled: false,
		},
		{
			name:             "get rejects malformed child_job_id at routed endpoint",
			role:             auth.RoleWorker,
			method:           http.MethodGet,
			path:             "/v1/jobs/" + f.JobID.String() + "/builds/not-a-ksuid",
			nodeIDHeader:     f.NodeIDStr,
			wantStatus:       http.StatusBadRequest,
			wantGetJobCalled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			st := &jobStore{getJobResult: f.Job}
			handler := newTestRouterWithRoleForStore(t, st, tt.role)

			var body *bytes.Reader
			if tt.body == "" {
				body = bytes.NewReader(nil)
			} else {
				body = bytes.NewReader([]byte(tt.body))
			}
			req := httptest.NewRequest(tt.method, tt.path, body)
			if tt.nodeIDHeader != "" {
				req.Header.Set(nodeUUIDHeader, tt.nodeIDHeader)
			}
			if tt.method == http.MethodPost {
				req.Header.Set("Content-Type", "application/json")
			}

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			assertStatus(t, rr, tt.wantStatus)
			if st.getJobCalled != tt.wantGetJobCalled {
				t.Fatalf("GetJob called = %v, want %v", st.getJobCalled, tt.wantGetJobCalled)
			}
		})
	}
}

func newTestRouterWithRoleForStore(t *testing.T, st store.Store, role auth.Role) http.Handler {
	t.Helper()

	authz := auth.NewAuthorizer(auth.Options{AllowInsecure: true, DefaultRole: role})
	srv, err := server.NewHTTPServer(server.HTTPOptions{Authorizer: authz})
	if err != nil {
		t.Fatalf("http server: %v", err)
	}
	ev, err := server.NewEventsService(server.EventsOptions{})
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	bs := bsmock.New()
	bp := blobpersist.New(st, bs)
	RegisterRoutes(srv, st, bs, bp, ev, NewConfigHolder(config.GitLabConfig{}, nil), "test-secret")
	return srv.Handler()
}
