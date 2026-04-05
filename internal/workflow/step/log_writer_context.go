package step

import (
	"context"
	"io"
)

type logWriterContextKey struct{}

// WithExecutionLogWriter stores an optional live log sink in context so
// lower-level executors (for example gate runtime) can stream container output
// through the standard node log uploader path.
func WithExecutionLogWriter(ctx context.Context, w io.Writer) context.Context {
	if w == nil {
		return ctx
	}
	return context.WithValue(ctx, logWriterContextKey{}, w)
}

func executionLogWriterFromContext(ctx context.Context) io.Writer {
	if ctx == nil {
		return nil
	}
	w, _ := ctx.Value(logWriterContextKey{}).(io.Writer)
	return w
}
