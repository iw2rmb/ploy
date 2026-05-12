package step

import (
	"bytes"
	"context"
	"io"
	"time"
)

// logStreamingRuntime is implemented by ContainerRuntime variants that can
// follow logs while a container is still running.
type logStreamingRuntime interface {
	StreamLogs(ctx context.Context, handle ContainerHandle, stdout, stderr io.Writer) error
}

// splitLogWriter lets a single io.Writer expose distinct stdout and stderr
// sinks (used by the live log uploader to keep streams separate).
type splitLogWriter interface {
	StdoutWriter() io.Writer
	StderrWriter() io.Writer
}

// streamContainerLogs begins an async stream of container logs from rt. The
// returned channel emits the StreamLogs result and is closed after one send.
// Returns nil when rt does not implement logStreamingRuntime.
//   - capture (optional): receives combined demuxed bytes.
//   - live (optional):    mirrors bytes; split-aware via splitLogWriter.
func streamContainerLogs(
	ctx context.Context,
	rt ContainerRuntime,
	handle ContainerHandle,
	capture *bytes.Buffer,
	live io.Writer,
) <-chan error {
	streamer, ok := rt.(logStreamingRuntime)
	if !ok {
		return nil
	}
	stdoutW, stderrW := teeStreamWriters(capture, live)
	done := make(chan error, 1)
	go func() { done <- streamer.StreamLogs(ctx, handle, stdoutW, stderrW) }()
	return done
}

// awaitStreamWithin returns true when done emits a nil error within timeout.
// Returns false on error, timeout, or a nil channel.
func awaitStreamWithin(done <-chan error, timeout time.Duration) bool {
	if done == nil {
		return false
	}
	select {
	case err := <-done:
		return err == nil
	case <-time.After(timeout):
		return false
	}
}

func teeStreamWriters(capture *bytes.Buffer, live io.Writer) (io.Writer, io.Writer) {
	var liveOut, liveErr io.Writer
	if live != nil {
		if split, ok := live.(splitLogWriter); ok {
			liveOut = split.StdoutWriter()
			liveErr = split.StderrWriter()
		} else {
			liveOut = live
			liveErr = live
		}
	}
	return teeWriter(capture, liveOut), teeWriter(capture, liveErr)
}

func teeWriter(capture *bytes.Buffer, live io.Writer) io.Writer {
	switch {
	case capture != nil && live != nil:
		return io.MultiWriter(capture, live)
	case capture != nil:
		return capture
	case live != nil:
		return live
	default:
		return io.Discard
	}
}
