package orchestration

import (
	"bufio"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// LogStreamer streams Nomad allocation logs for a given job in near‑real‑time
// using the job-manager wrapper. It writes the full stream to a temp file and
// also keeps an in-memory ring buffer for quick inclusion in API payloads.
type LogStreamer struct {
	jobName  string
	mu       sync.Mutex
	buf      []byte
	maxBytes int
	filePath string
	started  bool
}

// NewLogStreamer creates a new streamer for the given job.
func NewLogStreamer(jobName string) *LogStreamer {
	return &LogStreamer{jobName: jobName, maxBytes: 1 * 1024 * 1024}
}

// append appends p to the ring buffer and file.
func (s *LogStreamer) append(w io.Writer, p []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// write to file first
	if w != nil && len(p) > 0 {
		_, _ = w.Write(p)
	}
	// ring buffer
	if len(p) == 0 {
		return
	}
	if len(s.buf)+len(p) <= s.maxBytes {
		s.buf = append(s.buf, p...)
		return
	}
	// drop from head to keep last maxBytes
	need := len(s.buf) + len(p) - s.maxBytes
	if need > len(s.buf) {
		s.buf = s.buf[:0]
	} else {
		s.buf = append([]byte{}, s.buf[need:]...)
	}
	s.buf = append(s.buf, p...)
}

// Results returns the current ring buffer contents and the full file path.
func (s *LogStreamer) Results() (string, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return string(s.buf), s.filePath
}

// jobMgrPath returns the job-manager wrapper path.
func jobMgrPath() string { return "/opt/hashicorp/bin/nomad-job-manager.sh" }

// findRunningAlloc polls for the running allocation ID for the job.
func (s *LogStreamer) findRunningAlloc(ctx context.Context) string {
	for {
		select {
		case <-ctx.Done():
			return ""
		default:
		}
		cmd := exec.CommandContext(ctx, jobMgrPath(), "running-alloc", "--job", s.jobName)
		b, _ := cmd.CombinedOutput()
		id := extractLastUUID(string(b))
		if strings.TrimSpace(id) != "" {
			return id
		}
		time.Sleep(300 * time.Millisecond)
	}
}

// extractLastUUID extracts the last UUID-like token from s.
func extractLastUUID(s string) string {
	// naive: UUIDs are 36 chars with dashes; scan tokens from the end
	fields := strings.Fields(s)
	for i := len(fields) - 1; i >= 0; i-- {
		f := strings.TrimSpace(fields[i])
		if len(f) == 36 && strings.Count(f, "-") == 4 {
			return f
		}
	}
	return ""
}

// candidateTasks provides a small list to try when a task flag is required.
func candidateTasks() []string {
	return []string{"kaniko", "compile", "builder", "osv-jvm", "build-wasm"}
}

// Run starts streaming until ctx is canceled. Safe to call once.
func (s *LogStreamer) Run(ctx context.Context) {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.mu.Unlock()

	// prepare temp file
	f, _ := os.CreateTemp("", "ploy-build-logs-*.log")
	var w io.Writer
	if f != nil {
		s.mu.Lock()
		s.filePath = f.Name()
		s.mu.Unlock()
		w = f
		defer func() { _ = f.Close() }()
	}

	// loop: attach to running alloc, stream, on EOF reattach
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		alloc := s.findRunningAlloc(ctx)
		if alloc == "" {
			// context canceled
			return
		}

		// try with known tasks first
		var cmd *exec.Cmd
		for _, t := range candidateTasks() {
			cmd = exec.CommandContext(ctx, jobMgrPath(), "logs", "--alloc-id", alloc, "--task", t, "--both", "--follow")
			if r, err := cmd.StdoutPipe(); err == nil {
				if e, err2 := cmd.StderrPipe(); err2 == nil {
					if err3 := cmd.Start(); err3 == nil {
						s.drainPipes(ctx, w, r, e)
						_ = cmd.Wait()
						break
					}
				}
			}
		}
		if cmd == nil {
			// try without task
			cmd = exec.CommandContext(ctx, jobMgrPath(), "logs", "--alloc-id", alloc, "--both", "--follow")
			if r, err := cmd.StdoutPipe(); err == nil {
				if e, err2 := cmd.StderrPipe(); err2 == nil {
					if err3 := cmd.Start(); err3 == nil {
						s.drainPipes(ctx, w, r, e)
						_ = cmd.Wait()
					}
				}
			}
		}
		// brief pause before reattach
		time.Sleep(200 * time.Millisecond)
	}
}

func (s *LogStreamer) drainPipes(ctx context.Context, w io.Writer, r1, r2 io.Reader) {
	var wg sync.WaitGroup
	drain := func(r io.Reader) {
		defer wg.Done()
		br := bufio.NewReader(r)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			line, err := br.ReadBytes('\n')
			if len(line) > 0 {
				s.append(w, line)
			}
			if err != nil {
				return
			}
		}
	}
	wg.Add(2)
	go drain(r1)
	go drain(r2)
	wg.Wait()
}

// CleanTemp removes the temp file if present.
func (s *LogStreamer) CleanTemp() {
	s.mu.Lock()
	p := s.filePath
	s.mu.Unlock()
	if p != "" {
		_ = os.Remove(p)
	}
}

// Dir returns the directory containing the temp file (for tests/diagnostics).
func (s *LogStreamer) Dir() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.filePath == "" {
		return ""
	}
	return filepath.Dir(s.filePath)
}
