package handlers

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	bsmock "github.com/iw2rmb/ploy/internal/blobstore/mock"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server"
	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/server/config"
	"github.com/iw2rmb/ploy/internal/store"
)

// createTestEventsService creates an events service for testing without a store.
// Use createTestEventsServiceWithStore for tests that need log/event persistence.
func createTestEventsService() (*server.EventsService, error) {
	return server.NewEventsService(server.EventsOptions{
		BufferSize:  32,
		HistorySize: 256,
		Logger:      slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	})
}

// createTestEventsServiceWithStore creates an events service with a store for testing.
// This is required for log handlers that persist via eventsService.CreateAndPublishLog.
func createTestEventsServiceWithStore(st store.Store) (*server.EventsService, error) {
	return server.NewEventsService(server.EventsOptions{
		BufferSize:  32,
		HistorySize: 256,
		Logger:      slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		Store:       st,
	})
}

// flushRecorder adapts httptest.ResponseRecorder to also implement http.Flusher.
type flushRecorder struct{ *httptest.ResponseRecorder }

func (f *flushRecorder) Flush() {}

// strPtr returns a pointer to s.
func strPtr(s string) *string { return &s }

// mockError is a simple error type for testing store error paths.
type mockError struct{ msg string }

func (e *mockError) Error() string { return e.msg }

var errMockDatabase = &mockError{msg: "mock database error"}

// jobTestFixture holds the common identifiers and job used across jobs_complete tests.
type jobTestFixture struct {
	NodeIDStr string
	NodeID    domaintypes.NodeID
	RunID     domaintypes.RunID
	JobID     domaintypes.JobID
	Job       store.Job
}

// newJobFixture creates a running job fixture with default values.
// jobType defaults to domaintypes.JobTypeMod.
func newJobFixture(jobType domaintypes.JobType, _ float64) jobTestFixture {
	nodeIDStr := domaintypes.NewNodeKey()
	nodeID := domaintypes.NodeID(nodeIDStr)
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()
	if jobType == "" {
		jobType = domaintypes.JobTypeMod
	}
	return jobTestFixture{
		NodeIDStr: nodeIDStr,
		NodeID:    nodeID,
		RunID:     runID,
		JobID:     jobID,
		Job: store.Job{
			ID:        jobID,
			RunID:     runID,
			NodeID:    &nodeID,
			Name:      jobType.String() + "-0",
			Status:    domaintypes.JobStatusRunning,
			JobType:   jobType,
			RepoShaIn: "0123456789abcdef0123456789abcdef01234567",
			Meta:      []byte(`{}`),
		},
	}
}

// completeJobReq builds an HTTP request for POST /v1/jobs/{job_id}/complete
// with the given body map and worker auth context.
func (f jobTestFixture) completeJobReq(bodyMap map[string]any) *http.Request {
	body, _ := json.Marshal(bodyMap)
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+f.JobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", f.JobID.String())
	req.Header.Set(nodeUUIDHeader, f.NodeIDStr)
	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: f.NodeIDStr,
	})
	return req.WithContext(ctx)
}

// newMockStoreForJob returns a mockStore pre-configured for a standard running job fixture.
// The store has getRunResult (Started), getJobResult, and listJobsByRunResult set.
func newMockStoreForJob(f jobTestFixture) *mockStore {
	return &mockStore{
		getRunResult:        store.Run{ID: f.RunID, Status: domaintypes.RunStatusStarted},
		getJobResult:        f.Job,
		listJobsByRunResult: []store.Job{f.Job},
	}
}

// doJSON sends a JSON request to handler and returns the recorder.
// body is marshaled to JSON; pass nil for no body.
func doJSON(t *testing.T, handler http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	return doRequest(t, handler, method, path, body)
}

// doRequest sends a request to handler and returns the recorder.
// body: nil → no body; string → raw body; otherwise → JSON-marshaled.
// pathParams are key-value pairs: "mig_ref", "mod123", "run_id", "run1".
func doRequest(t *testing.T, handler http.Handler, method, path string, body any, pathParams ...string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	switch v := body.(type) {
	case nil:
		r = httptest.NewRequest(method, path, nil)
	case string:
		r = httptest.NewRequest(method, path, bytes.NewBufferString(v))
		r.Header.Set("Content-Type", "application/json")
	case []byte:
		r = httptest.NewRequest(method, path, bytes.NewReader(v))
		r.Header.Set("Content-Type", "application/json")
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		r = httptest.NewRequest(method, path, bytes.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(pathParams); i += 2 {
		r.SetPathValue(pathParams[i], pathParams[i+1])
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, r)
	return rr
}

// assertStatus fails the test if rr.Code != want.
func assertStatus(t *testing.T, rr *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rr.Code != want {
		t.Fatalf("expected status %d, got %d: %s", want, rr.Code, rr.Body.String())
	}
}

// decodeBody decodes rr.Body as JSON into T.
func decodeBody[T any](t *testing.T, rr *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(rr.Body).Decode(&v); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	return v
}

// newTestServerWithRole creates an HTTP server with routes registered and
// the given auth role as the default for all requests.
func newTestServerWithRole(t *testing.T, role auth.Role) *server.HTTPServer {
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
	st := &mockStore{}
	bs := bsmock.New()
	bp := blobpersist.New(st, bs)
	RegisterRoutes(srv, st, bs, bp, ev, NewConfigHolder(config.GitLabConfig{}, nil), "test-secret")
	return srv
}
