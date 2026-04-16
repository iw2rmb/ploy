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

	f := newRepoScopedFixture(domaintypes.JobTypeMig)
	linkedChild := store.Job{
		ID:      domaintypes.NewJobID(),
		RunID:   f.RunID,
		RepoID:  f.Job.RepoID,
		Attempt: f.Job.Attempt,
		JobType: domaintypes.JobTypeReGate,
		Status:  domaintypes.JobStatusRunning,
		Meta: []byte(`{
			"kind":"mig",
			"trigger":{"kind":"child_gate_request","parent_job_id":"` + f.JobID.String() + `"}
		}`),
	}

	tests := []struct {
		name               string
		method             string
		path               string
		pathValues         map[string]string
		body               string
		nodeIDHeader       string
		store              *jobStore
		wantStatus         int
		wantGetJobCalled   bool
		wantCreateJobCalls int
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
			name:               "post creates child for running parent",
			method:             http.MethodPost,
			path:               "/v1/jobs/" + f.JobID.String() + "/builds",
			pathValues:         map[string]string{"parent_job_id": f.JobID.String()},
			body:               `{"build_kind":"re_gate","reason":"child_build_validation"}`,
			nodeIDHeader:       f.NodeIDStr,
			store:              &jobStore{getJobResult: f.Job},
			wantStatus:         http.StatusCreated,
			wantGetJobCalled:   true,
			wantCreateJobCalls: 1,
		},
		{
			name:             "post rejects non running parent",
			method:           http.MethodPost,
			path:             "/v1/jobs/" + f.JobID.String() + "/builds",
			pathValues:       map[string]string{"parent_job_id": f.JobID.String()},
			body:             `{"build_kind":"re_gate","reason":"child_build_validation"}`,
			nodeIDHeader:     f.NodeIDStr,
			store:            &jobStore{getJobResult: store.Job{ID: f.JobID, NodeID: &f.NodeID, JobType: domaintypes.JobTypeMig, Status: domaintypes.JobStatusQueued}},
			wantStatus:       http.StatusConflict,
			wantGetJobCalled: true,
		},
		{
			name:             "get rejects empty parent_job_id before store calls",
			method:           http.MethodGet,
			path:             "/v1/jobs//builds/" + domaintypes.NewJobID().String(),
			pathValues:       map[string]string{"parent_job_id": "", "child_job_id": domaintypes.NewJobID().String()},
			nodeIDHeader:     f.NodeIDStr,
			store:            &jobStore{},
			wantStatus:       http.StatusBadRequest,
			wantGetJobCalled: false,
		},
		{
			name:             "get rejects empty child_job_id before store calls",
			method:           http.MethodGet,
			path:             "/v1/jobs/" + f.JobID.String() + "/builds/",
			pathValues:       map[string]string{"parent_job_id": f.JobID.String(), "child_job_id": ""},
			nodeIDHeader:     f.NodeIDStr,
			store:            &jobStore{},
			wantStatus:       http.StatusBadRequest,
			wantGetJobCalled: false,
		},
		{
			name:         "get returns child status projection",
			method:       http.MethodGet,
			path:         "/v1/jobs/" + f.JobID.String() + "/builds/" + linkedChild.ID.String(),
			pathValues:   map[string]string{"parent_job_id": f.JobID.String(), "child_job_id": linkedChild.ID.String()},
			nodeIDHeader: f.NodeIDStr,
			store: &jobStore{
				getJobResults: map[domaintypes.JobID]store.Job{
					f.JobID:        f.Job,
					linkedChild.ID: linkedChild,
				},
			},
			wantStatus:       http.StatusOK,
			wantGetJobCalled: true,
		},
		{
			name:         "get rejects unrelated child id",
			method:       http.MethodGet,
			path:         "/v1/jobs/" + f.JobID.String() + "/builds/" + linkedChild.ID.String(),
			pathValues:   map[string]string{"parent_job_id": f.JobID.String(), "child_job_id": linkedChild.ID.String()},
			nodeIDHeader: f.NodeIDStr,
			store: &jobStore{
				getJobResults: map[domaintypes.JobID]store.Job{
					f.JobID: f.Job,
					linkedChild.ID: {
						ID:      linkedChild.ID,
						RunID:   linkedChild.RunID,
						RepoID:  linkedChild.RepoID,
						Attempt: linkedChild.Attempt,
						JobType: domaintypes.JobTypeReGate,
						Status:  domaintypes.JobStatusRunning,
						Meta:    []byte(`{"kind":"mig","trigger":{"kind":"child_gate_request","parent_job_id":"` + domaintypes.NewJobID().String() + `"}}`),
					},
				},
			},
			wantStatus:       http.StatusNotFound,
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
			store:            &jobStore{getJobResult: store.Job{ID: f.JobID, NodeID: &f.NodeID, JobType: domaintypes.JobTypePreGate, Status: domaintypes.JobStatusRunning}},
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
			if got := len(tt.store.createJob.calls); got != tt.wantCreateJobCalls {
				t.Fatalf("CreateJob calls = %d, want %d", got, tt.wantCreateJobCalls)
			}
		})
	}
}

func TestChildBuildRoutes_SuccessResponses(t *testing.T) {
	t.Parallel()

	f := newRepoScopedFixture(domaintypes.JobTypeMig)

	t.Run("post response contains child id and status url", func(t *testing.T) {
		t.Parallel()

		st := &jobStore{getJobResult: f.Job}
		req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+f.JobID.String()+"/builds", bytes.NewBufferString(`{"build_kind":"re_gate","reason":"child_build_validation"}`))
		req.SetPathValue("parent_job_id", f.JobID.String())
		req.Header.Set(nodeUUIDHeader, f.NodeIDStr)
		req.Header.Set("Content-Type", "application/json")

		rr := httptest.NewRecorder()
		createJobBuildHandler(st).ServeHTTP(rr, req)

		assertStatus(t, rr, http.StatusCreated)
		resp := decodeBody[createJobBuildResponse](t, rr)
		if resp.ChildJobID.IsZero() {
			t.Fatal("child_job_id must be set")
		}
		if resp.Status != string(domaintypes.JobStatusQueued) {
			t.Fatalf("status = %q, want %q", resp.Status, domaintypes.JobStatusQueued)
		}
		wantURL := "http://example.com/v1/jobs/" + f.JobID.String() + "/builds/" + resp.ChildJobID.String()
		if resp.StatusURL != wantURL {
			t.Fatalf("status_url = %q, want %q", resp.StatusURL, wantURL)
		}
	})

	t.Run("get response projects terminal and success", func(t *testing.T) {
		t.Parallel()

		childID := domaintypes.NewJobID()
		child := store.Job{
			ID:      childID,
			RunID:   f.RunID,
			RepoID:  f.Job.RepoID,
			Attempt: f.Job.Attempt,
			JobType: domaintypes.JobTypeReGate,
			Status:  domaintypes.JobStatusSuccess,
			Meta:    []byte(`{"kind":"mig","trigger":{"kind":"child_gate_request","parent_job_id":"` + f.JobID.String() + `"}}`),
		}
		st := &jobStore{
			getJobResults: map[domaintypes.JobID]store.Job{
				f.JobID: f.Job,
				childID: child,
			},
		}
		req := httptest.NewRequest(http.MethodGet, "/v1/jobs/"+f.JobID.String()+"/builds/"+childID.String(), nil)
		req.SetPathValue("parent_job_id", f.JobID.String())
		req.SetPathValue("child_job_id", childID.String())
		req.Header.Set(nodeUUIDHeader, f.NodeIDStr)

		rr := httptest.NewRecorder()
		getJobBuildStatusHandler(st).ServeHTTP(rr, req)

		assertStatus(t, rr, http.StatusOK)
		resp := decodeBody[getJobBuildResponse](t, rr)
		if resp.JobID != childID {
			t.Fatalf("job_id = %s, want %s", resp.JobID, childID)
		}
		if !resp.Terminal {
			t.Fatal("terminal = false, want true")
		}
		if !resp.Success {
			t.Fatal("success = false, want true")
		}
	})
}

func TestChildBuildRoutes_RoutedWorkerAuthAndMalformedPathValues(t *testing.T) {
	t.Parallel()

	f := newRepoScopedFixture(domaintypes.JobTypeMig)
	validChildID := domaintypes.NewJobID().String()
	validBody := `{"build_kind":"re_gate","reason":"child_build_validation"}`

	tests := []struct {
		name             string
		role             auth.Role
		method           string
		path             string
		body             string
		nodeIDHeader     string
		store            *jobStore
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
			store:            &jobStore{getJobResult: f.Job},
			wantStatus:       http.StatusCreated,
			wantGetJobCalled: true,
		},
		{
			name:         "get route reachable under worker auth",
			role:         auth.RoleWorker,
			method:       http.MethodGet,
			path:         "/v1/jobs/" + f.JobID.String() + "/builds/" + validChildID,
			nodeIDHeader: f.NodeIDStr,
			store: &jobStore{
				getJobResults: map[domaintypes.JobID]store.Job{
					f.JobID: f.Job,
					domaintypes.JobID(validChildID): {
						ID:      domaintypes.JobID(validChildID),
						RunID:   f.RunID,
						RepoID:  f.Job.RepoID,
						Attempt: f.Job.Attempt,
						JobType: domaintypes.JobTypeReGate,
						Status:  domaintypes.JobStatusRunning,
						Meta:    []byte(`{"kind":"mig","trigger":{"kind":"child_gate_request","parent_job_id":"` + f.JobID.String() + `"}}`),
					},
				},
			},
			wantStatus:       http.StatusOK,
			wantGetJobCalled: true,
		},
		{
			name:             "post rejects malformed parent_job_id at routed endpoint",
			role:             auth.RoleWorker,
			method:           http.MethodPost,
			path:             "/v1/jobs/not-a-ksuid/builds",
			body:             validBody,
			nodeIDHeader:     f.NodeIDStr,
			store:            &jobStore{},
			wantStatus:       http.StatusBadRequest,
			wantGetJobCalled: false,
		},
		{
			name:             "get rejects malformed parent_job_id at routed endpoint",
			role:             auth.RoleWorker,
			method:           http.MethodGet,
			path:             "/v1/jobs/not-a-ksuid/builds/" + validChildID,
			nodeIDHeader:     f.NodeIDStr,
			store:            &jobStore{},
			wantStatus:       http.StatusBadRequest,
			wantGetJobCalled: false,
		},
		{
			name:             "get rejects malformed child_job_id at routed endpoint",
			role:             auth.RoleWorker,
			method:           http.MethodGet,
			path:             "/v1/jobs/" + f.JobID.String() + "/builds/not-a-ksuid",
			nodeIDHeader:     f.NodeIDStr,
			store:            &jobStore{},
			wantStatus:       http.StatusBadRequest,
			wantGetJobCalled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler := newTestRouterWithRoleForStore(t, tt.store, tt.role)

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
			if tt.store.getJobCalled != tt.wantGetJobCalled {
				t.Fatalf("GetJob called = %v, want %v", tt.store.getJobCalled, tt.wantGetJobCalled)
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
