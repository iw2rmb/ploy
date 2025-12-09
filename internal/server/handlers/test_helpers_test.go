package handlers

import (
	"log/slog"
	"net/http/httptest"
	"os"

	"github.com/iw2rmb/ploy/internal/server/events"
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
