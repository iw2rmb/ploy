package orchestration

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLogStreamerAppendTrimsBuffer(t *testing.T) {
	streamer := NewLogStreamer("job-test")
	streamer.maxBytes = 6

	var file bytes.Buffer

	streamer.append(&file, []byte("abc"))
	streamer.append(&file, []byte("def"))
	streamer.append(&file, []byte("gh"))

	got, _ := streamer.Results()
	if got != "cdefgh" {
		t.Fatalf("expected ring buffer to truncate to last 6 bytes, got %q", got)
	}

	if file.String() != "abcdefgh" {
		t.Fatalf("expected file writer to receive all bytes, got %q", file.String())
	}
}

func TestExtractLastUUID(t *testing.T) {
	src := "noise 123e4567-e89b-12d3-a456-426614174000 trailing"
	got := extractLastUUID(src)
	want := "123e4567-e89b-12d3-a456-426614174000"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestLogStreamerDrainPipesCopiesData(t *testing.T) {
	streamer := NewLogStreamer("job-pipes")
	streamer.maxBytes = 64

	r1, w1 := io.Pipe()
	r2, w2 := io.Pipe()
	defer func() { _ = r1.Close() }()
	defer func() { _ = r2.Close() }()

	var file bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		streamer.drainPipes(ctx, &file, r1, r2)
		close(done)
	}()

	_, _ = w1.Write([]byte("out-line\n"))
	_, _ = w2.Write([]byte("err-line\n"))
	_ = w1.Close()
	_ = w2.Close()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("drainPipes did not finish in time")
	}
	cancel()

	got, _ := streamer.Results()
	if !strings.Contains(got, "out-line") || !strings.Contains(got, "err-line") {
		t.Fatalf("expected buffered output to contain both streams, got %q", got)
	}
	if !strings.Contains(file.String(), "out-line") || !strings.Contains(file.String(), "err-line") {
		t.Fatalf("expected file writer to receive both streams, got %q", file.String())
	}
}

func TestLogStreamerRunStreamsLogs(t *testing.T) {
	original := execCommandContext
	defer func() { execCommandContext = original }()

	stateDir := t.TempDir()
	if err := os.Setenv("LOG_STREAMER_TEST_STATE", stateDir); err != nil {
		t.Fatalf("failed to set helper env: %v", err)
	}
	defer func() { _ = os.Unsetenv("LOG_STREAMER_TEST_STATE") }()

	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		cs := []string{"-test.run=TestLogStreamerHelperProcess", "--", name}
		cs = append(cs, args...)
		cmd := exec.CommandContext(ctx, os.Args[0], cs...)
		cmd.Env = append(os.Environ(), "LOG_STREAMER_TEST_HELPER=1")
		return cmd
	}

	streamer := NewLogStreamer("job-helper")
	streamer.maxBytes = 128

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go streamer.Run(ctx)

	deadline := time.Now().Add(2 * time.Second)
	var filePath string
	for {
		buf, path := streamer.Results()
		if strings.Contains(buf, "stdout-line") && strings.Contains(buf, "stderr-line") {
			if path == "" {
				t.Fatal("expected temp file path after streaming")
			}
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("expected temp file to exist, got error: %v", err)
			}
			filePath = path
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for log stream to populate")
		}
		time.Sleep(20 * time.Millisecond)
	}

	cancel()
	// allow run loop to exit
	time.Sleep(50 * time.Millisecond)

	path := streamer.Dir()
	if path == "" {
		t.Fatalf("expected Dir to return a directory path, got empty string")
	}
	if filepath.Dir(filePath) != path {
		t.Fatalf("expected Dir to return directory of temp file, got %q want %q", path, filepath.Dir(filePath))
	}

	streamer.CleanTemp()
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Fatalf("expected temp file to be removed, got err=%v", err)
	}
}

func TestLogStreamerDirWhenNoFile(t *testing.T) {
	streamer := NewLogStreamer("job-dir")
	if dir := streamer.Dir(); dir != "" {
		t.Fatalf("expected empty dir when no temp file, got %q", dir)
	}
}

func TestLogStreamerCleanTempWithoutFile(t *testing.T) {
	streamer := NewLogStreamer("job-clean")
	streamer.CleanTemp() // should not panic when filePath is empty
}

func TestLogStreamerHelperProcess(t *testing.T) {
	if os.Getenv("LOG_STREAMER_TEST_HELPER") != "1" {
		return
	}

	args := os.Args
	idx := -1
	for i, arg := range args {
		if arg == "--" {
			idx = i
			break
		}
	}
	if idx == -1 {
		os.Exit(2)
	}

	cmd := args[idx+1]
	rest := args[idx+2:]

	switch cmd {
	case jobMgrPath():
		if len(rest) == 0 {
			os.Exit(0)
		}
		switch rest[0] {
		case "running-alloc":
			// Always emit a UUID so findRunningAlloc succeeds.
			_, _ = io.WriteString(os.Stdout, "prefix 123e4567-e89b-12d3-a456-426614174000\n")
			os.Exit(0)
		case "logs":
			_, _ = io.WriteString(os.Stdout, "stdout-line\n")
			_, _ = io.WriteString(os.Stderr, "stderr-line\n")
			// keep helper process around briefly to exercise context polling
			time.Sleep(20 * time.Millisecond)
			os.Exit(0)
		default:
			_, _ = io.WriteString(os.Stderr, "unexpected subcommand")
			os.Exit(1)
		}
	default:
		os.Exit(1)
	}
}
