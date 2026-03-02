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
// jobType defaults to "mig".
func newJobFixture(jobType string, _ float64) jobTestFixture {
	nodeIDStr := domaintypes.NewNodeKey()
	nodeID := domaintypes.NodeID(nodeIDStr)
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()
	if jobType == "" {
		jobType = "mig"
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
			Name:      jobType + "-0",
			Status:    store.JobStatusRunning,
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
