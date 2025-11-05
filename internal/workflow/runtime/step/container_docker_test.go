package step

import (
	"bytes"
	"testing"

	"github.com/docker/docker/pkg/stdcopy"
)

// TestDockerLogDemux verifies that multiplexed Docker logs are demultiplexed
// into a single plain-text byte slice containing both stdout and stderr.
func TestDockerLogDemux(t *testing.T) {
	// Build a synthetic multiplexed stream: write to stdout and stderr
	// using stdcopy so the format matches Docker's.
	var mux bytes.Buffer
	outW := stdcopy.NewStdWriter(&mux, stdcopy.Stdout)
	errW := stdcopy.NewStdWriter(&mux, stdcopy.Stderr)

	_, _ = outW.Write([]byte("hello stdout\n"))
	_, _ = errW.Write([]byte("oops stderr\n"))

	// Use the same demux logic as the runtime (unexported helper via inlined call).
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

func containsAll(s string, parts []string) bool {
	for _, p := range parts {
		if !bytes.Contains([]byte(s), []byte(p)) {
			return false
		}
	}
	return true
}
