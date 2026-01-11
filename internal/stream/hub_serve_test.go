package logstream

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestServeWritesSSEFrames(t *testing.T) {
	hub := NewHub(Options{BufferSize: 4, HistorySize: 8})
	ctx := context.Background()
	runID := domaintypes.NewRunID()

	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = hub.PublishLog(ctx, runID, LogRecord{Timestamp: "2025-10-22T12:10:00Z", Stream: "stdout", Line: "hello"})
		_ = hub.PublishStatus(ctx, runID, Status{Status: "completed"})
	}()

	req := httptest.NewRequest("GET", "/", nil)
	recorder := &flushRecorder{ResponseRecorder: httptest.NewRecorder()}

	if err := Serve(recorder, req, hub, runID, 0); err != nil {
		t.Fatalf("serve: %v", err)
	}

	body := recorder.Body.String()
	if !strings.Contains(body, "event: log") || !strings.Contains(body, "event: done") {
		t.Fatalf("unexpected SSE payload: %s", body)
	}
}

type flushRecorder struct {
	*httptest.ResponseRecorder
}

func (f *flushRecorder) Flush() {
	// ResponseRecorder buffers writes; nothing else required.
}
