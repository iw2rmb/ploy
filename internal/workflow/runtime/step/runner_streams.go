package step

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/node/logstream"
)

// publishLogStream sends buffered container logs to the configured log stream.
func (r Runner) publishLogStream(ctx context.Context, streamID string, data []byte) {
	if r.Streams == nil || strings.TrimSpace(streamID) == "" || len(data) == 0 {
		return
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r\n")
		if err := r.Streams.PublishLog(ctx, streamID, logstream.LogRecord{
			Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
			Stream:    "stdout",
			Line:      line,
		}); err != nil && !errors.Is(err, logstream.ErrStreamClosed) {
			break
		}
	}
}

// publishRetentionHint emits retention metadata for the run to the log stream.
func (r Runner) publishRetentionHint(ctx context.Context, streamID string, result Result) {
	if r.Streams == nil || strings.TrimSpace(streamID) == "" {
		return
	}
	hint := logstream.RetentionHint{
		Retained: result.Retained,
		TTL:      strings.TrimSpace(result.RetentionTTL),
		Bundle:   strings.TrimSpace(result.LogArtifact.CID),
		Expires:  "",
	}
	if err := r.Streams.PublishRetention(ctx, streamID, hint); err != nil && !errors.Is(err, logstream.ErrStreamClosed) {
		return
	}
}
