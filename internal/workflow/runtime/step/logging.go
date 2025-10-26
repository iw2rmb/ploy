package step

import (
	"context"

	"github.com/iw2rmb/ploy/internal/node/logstream"
)

// LogCollector retrieves container logs when a custom pathway exists.
type LogCollector interface {
	Collect(ctx context.Context, handle ContainerHandle) ([]byte, error)
}

// LogStreamPublisher publishes streaming events for live log consumers.
type LogStreamPublisher interface {
	PublishLog(ctx context.Context, streamID string, record logstream.LogRecord) error
	PublishRetention(ctx context.Context, streamID string, hint logstream.RetentionHint) error
	PublishStatus(ctx context.Context, streamID string, status logstream.Status) error
}
