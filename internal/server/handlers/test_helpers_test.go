package handlers

import (
	"log/slog"
	"net/http/httptest"
	"os"

	"github.com/iw2rmb/ploy/internal/server/events"
)

// createTestEventsService creates an events service for testing.
func createTestEventsService() (*events.Service, error) {
	return events.New(events.Options{
		BufferSize:  32,
		HistorySize: 256,
		Logger:      slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	})
}

// flushRecorder adapts httptest.ResponseRecorder to also implement http.Flusher.
type flushRecorder struct{ *httptest.ResponseRecorder }

func (f *flushRecorder) Flush() {}
