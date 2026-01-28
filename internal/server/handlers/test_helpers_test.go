package handlers

import (
	"log/slog"
	"net/http/httptest"
	"os"
	"testing"

	bsmock "github.com/iw2rmb/ploy/internal/blobstore/mock"
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
