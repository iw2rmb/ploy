package buildgate_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/buildgate"
)

type fakeClock struct {
	mu    sync.Mutex
	times []time.Time
}

func (f *fakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.times) == 0 {
		return time.Now()
	}
	current := f.times[0]
	f.times = f.times[1:]
	return current
}

type stubSandboxExecutor struct {
	mu         sync.Mutex
	result     buildgate.SandboxBuildResult
	err        error
	called     bool
	lastSpec   buildgate.SandboxSpec
	deadlineOK bool
	waitForCtx bool
}

func (s *stubSandboxExecutor) Execute(ctx context.Context, spec buildgate.SandboxSpec) (buildgate.SandboxBuildResult, error) {
	s.mu.Lock()
	s.called = true
	s.lastSpec = spec
	s.mu.Unlock()

	if _, ok := ctx.Deadline(); ok {
		s.mu.Lock()
		s.deadlineOK = true
		s.mu.Unlock()
	}

	if s.waitForCtx {
		<-ctx.Done()
		return buildgate.SandboxBuildResult{}, ctx.Err()
	}

	return s.result, s.err
}

func TestSandboxRunnerSuccessRecordsDurationAndCacheHit(t *testing.T) {
	start := time.Date(2025, 9, 27, 10, 0, 0, 0, time.UTC)
	end := start.Add(42 * time.Second)
	clock := &fakeClock{times: []time.Time{start, end}}
	executor := &stubSandboxExecutor{result: buildgate.SandboxBuildResult{
		Success:   true,
		CacheHit:  true,
		LogDigest: "  sha256:abc123  ",
		Metadata: buildgate.Metadata{
			LogFindings: []buildgate.LogFinding{{
				Code:     "shift.summary",
				Severity: "info",
				Message:  "lane lane.docker.jvm via docker",
			}},
		},
		Report: []byte(`{"status":"success"}`),
	}}

	runner := buildgate.NewSandboxRunner(executor, buildgate.SandboxRunnerOptions{
		Clock:          clock,
		DefaultTimeout: time.Minute,
	})

	outcome, err := runner.Run(context.Background(), buildgate.SandboxSpec{CacheKey: "repo@sha", Command: []string{"go", "build"}})
	if err != nil {
		t.Fatalf("unexpected run error: %v", err)
	}
	if !executor.called {
		t.Fatalf("expected executor to be invoked")
	}
	if !executor.deadlineOK {
		t.Fatalf("expected sandbox runner to set a timeout on the context")
	}
	if executor.lastSpec.CacheKey != "repo@sha" {
		t.Fatalf("expected cache key propagated, got %q", executor.lastSpec.CacheKey)
	}
	if len(executor.lastSpec.Command) != 2 {
		t.Fatalf("expected command propagated, got %#v", executor.lastSpec.Command)
	}
	if outcome.Status != buildgate.SandboxStatusSucceeded {
		t.Fatalf("expected success status, got %s", outcome.Status)
	}
	if !outcome.CacheHit {
		t.Fatalf("expected cache hit recorded")
	}
	if got := outcome.Duration; got != end.Sub(start) {
		t.Fatalf("expected duration %v, got %v", end.Sub(start), got)
	}
	if outcome.LogDigest != "sha256:abc123" {
		t.Fatalf("expected log digest trimmed, got %q", outcome.LogDigest)
	}
	if outcome.FailureReason != "" || outcome.FailureDetail != "" {
		t.Fatalf("expected no failure metadata, got %#v", outcome)
	}
	if len(outcome.Metadata.LogFindings) != 1 {
		t.Fatalf("expected metadata propagated, got %#v", outcome.Metadata)
	}
	if string(outcome.Report) != `{"status":"success"}` {
		t.Fatalf("expected report propagated, got %q", outcome.Report)
	}
}

func TestSandboxRunnerFailureCapturesReasonAndDetails(t *testing.T) {
	start := time.Date(2025, 9, 27, 11, 0, 0, 0, time.UTC)
	end := start.Add(15 * time.Second)
	clock := &fakeClock{times: []time.Time{start, end}}
	executor := &stubSandboxExecutor{result: buildgate.SandboxBuildResult{
		Success:       false,
		CacheHit:      false,
		FailureReason: "  compile-error  ",
		FailureDetail: "  go build failed  ",
	}}

	runner := buildgate.NewSandboxRunner(executor, buildgate.SandboxRunnerOptions{Clock: clock})

	outcome, err := runner.Run(context.Background(), buildgate.SandboxSpec{CacheKey: "repo@sha", Timeout: 30 * time.Second})
	if err != nil {
		t.Fatalf("unexpected run error: %v", err)
	}
	if outcome.Status != buildgate.SandboxStatusFailed {
		t.Fatalf("expected failed status, got %s", outcome.Status)
	}
	if outcome.CacheHit {
		t.Fatalf("expected cache miss on failure")
	}
	if outcome.Duration != end.Sub(start) {
		t.Fatalf("expected duration %v, got %v", end.Sub(start), outcome.Duration)
	}
	if outcome.FailureReason != "compile-error" {
		t.Fatalf("expected trimmed failure reason, got %q", outcome.FailureReason)
	}
	if outcome.FailureDetail != "go build failed" {
		t.Fatalf("expected trimmed failure detail, got %q", outcome.FailureDetail)
	}
}

func TestSandboxRunnerTimeout(t *testing.T) {
	timeout := 10 * time.Millisecond
	start := time.Now()
	end := start.Add(timeout)
	clock := &fakeClock{times: []time.Time{start, end}}
	executor := &stubSandboxExecutor{waitForCtx: true}

	runner := buildgate.NewSandboxRunner(executor, buildgate.SandboxRunnerOptions{Clock: clock})

	outcome, err := runner.Run(context.Background(), buildgate.SandboxSpec{CacheKey: "repo@sha", Timeout: timeout})
	if err != nil {
		t.Fatalf("unexpected run error: %v", err)
	}
	if outcome.Status != buildgate.SandboxStatusTimedOut {
		t.Fatalf("expected timeout status, got %s", outcome.Status)
	}
	if !strings.Contains(outcome.FailureDetail, context.DeadlineExceeded.Error()) {
		t.Fatalf("expected failure detail to record deadline exceeded, got %q", outcome.FailureDetail)
	}
	if outcome.FailureReason != buildgate.SandboxFailureReasonTimeout {
		t.Fatalf("expected timeout failure reason, got %q", outcome.FailureReason)
	}
	if outcome.CacheHit {
		t.Fatalf("expected cache miss when timeout occurs")
	}
	if outcome.Duration != timeout {
		t.Fatalf("expected duration %v, got %v", timeout, outcome.Duration)
	}
}
