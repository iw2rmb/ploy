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
	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/server/config"
	"github.com/iw2rmb/ploy/internal/server/events"
	httpapi "github.com/iw2rmb/ploy/internal/server/http"
	"github.com/iw2rmb/ploy/internal/store"
)

// createTestEventsService creates an events service for testing without a store.
// Use createTestEventsServiceWithStore for tests that need log/event persistence.
func createTestEventsService() (*events.Service, error) {
	return events.New(events.Options{
		BufferSize:  32,
		HistorySize: 256,
		Logger:      slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	})
}

// createTestEventsServiceWithStore creates an events service with a store for testing.
// This is required for log handlers that persist via eventsService.CreateAndPublishLog.
func createTestEventsServiceWithStore(st store.Store) (*events.Service, error) {
	return events.New(events.Options{
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
// modType defaults to "mod". stepIndex is stored in job meta for legacy surfaces.
func newJobFixture(modType string, stepIndex domaintypes.StepIndex) jobTestFixture {
	nodeIDStr := domaintypes.NewNodeKey()
	nodeID := domaintypes.NodeID(nodeIDStr)
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()
	if modType == "" {
		modType = "mod"
	}
	return jobTestFixture{
		NodeIDStr: nodeIDStr,
		NodeID:    nodeID,
		RunID:     runID,
		JobID:     jobID,
		Job: store.Job{
			ID:      jobID,
			RunID:   runID,
			NodeID:  &nodeID,
			Name:    modType + "-0",
			Status:  store.JobStatusRunning,
			JobType: modType,
			Meta:    withStepIndexMeta([]byte(`{}`), stepIndex),
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
func newTestServerWithRole(t *testing.T, role auth.Role) *httpapi.Server {
	t.Helper()
	authz := auth.NewAuthorizer(auth.Options{AllowInsecure: true, DefaultRole: role})
	srv, err := httpapi.New(httpapi.Options{Authorizer: authz})
	if err != nil {
		t.Fatalf("http server: %v", err)
	}
	ev, err := events.New(events.Options{})
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	st := &mockStore{}
	bs := bsmock.New()
	bp := blobpersist.New(st, bs)
	RegisterRoutes(srv, st, bs, bp, ev, NewConfigHolder(config.GitLabConfig{}, nil), "test-secret")
	return srv
}
