package step

import (
	"bytes"
	"encoding/binary"
	"testing"

	// Docker Engine v29 SDK (moby) — stdcopy provides Docker log stream demux.
	// See container_docker.go for migration notes.
	"github.com/moby/moby/api/pkg/stdcopy"
)

// writeMultiplexedFrame writes a single frame in Docker's multiplexed stream
// format. The format is: [8]byte{STREAM_TYPE, 0, 0, 0, SIZE1, SIZE2, SIZE3, SIZE4}
// followed by the payload. STREAM_TYPE is 1 for stdout, 2 for stderr.
// SIZE is big-endian uint32 length of payload.
func writeMultiplexedFrame(buf *bytes.Buffer, streamType stdcopy.StdType, payload []byte) {
	header := make([]byte, 8)
	header[0] = byte(streamType)
	// header[1:4] are zero (padding)
	binary.BigEndian.PutUint32(header[4:8], uint32(len(payload)))
	buf.Write(header)
	buf.Write(payload)
}

// TestDockerLogDemux verifies that multiplexed Docker logs are demultiplexed
// into a single plain-text byte slice containing both stdout and stderr.
// The moby Engine v29 SDK (github.com/moby/moby/api/pkg/stdcopy) provides only
// StdCopy for reading multiplexed streams; NewStdWriter was removed. We manually
// construct the multiplexed format for testing.
func TestDockerLogDemux(t *testing.T) {
	// Build a synthetic multiplexed stream using Docker's wire format:
	// [8-byte header][payload] for each frame.
	var mux bytes.Buffer
	writeMultiplexedFrame(&mux, stdcopy.Stdout, []byte("hello stdout\n"))
	writeMultiplexedFrame(&mux, stdcopy.Stderr, []byte("oops stderr\n"))

	// Use the same demux logic as the runtime (stdcopy.StdCopy).
	var stdoutBuf, stderrBuf bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdoutBuf, &stderrBuf, bytes.NewReader(mux.Bytes())); err != nil {
		t.Fatalf("StdCopy error: %v", err)
	}
	combined := append(stdoutBuf.Bytes(), stderrBuf.Bytes()...)

	got := string(combined)
	if !containsAll(got, []string{"hello stdout", "oops stderr"}) {
		t.Fatalf("demuxed logs missing content: %q", got)
	}
}

// TestDockerLogDemuxMultipleFrames verifies demultiplexing with multiple
// interleaved stdout and stderr frames.
func TestDockerLogDemuxMultipleFrames(t *testing.T) {
	var mux bytes.Buffer
	writeMultiplexedFrame(&mux, stdcopy.Stdout, []byte("line1\n"))
	writeMultiplexedFrame(&mux, stdcopy.Stderr, []byte("err1\n"))
	writeMultiplexedFrame(&mux, stdcopy.Stdout, []byte("line2\n"))
	writeMultiplexedFrame(&mux, stdcopy.Stderr, []byte("err2\n"))

	var stdoutBuf, stderrBuf bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdoutBuf, &stderrBuf, bytes.NewReader(mux.Bytes())); err != nil {
		t.Fatalf("StdCopy error: %v", err)
	}

	if got, want := stdoutBuf.String(), "line1\nline2\n"; got != want {
		t.Errorf("stdout: got %q, want %q", got, want)
	}
	if got, want := stderrBuf.String(), "err1\nerr2\n"; got != want {
		t.Errorf("stderr: got %q, want %q", got, want)
	}
}

func containsAll(s string, parts []string) bool {
	for _, p := range parts {
		if !bytes.Contains([]byte(s), []byte(p)) {
			return false
		}
	}
	return true
}
