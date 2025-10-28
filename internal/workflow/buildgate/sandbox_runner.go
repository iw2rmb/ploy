package buildgate

import (
	"context"
	"errors"
	"strings"
	"time"
)

// SandboxStatus indicates the outcome of a sandbox build execution.
type SandboxStatus string

const (
	// SandboxStatusSucceeded indicates the sandbox build completed successfully.
	SandboxStatusSucceeded SandboxStatus = "succeeded"
	// SandboxStatusFailed indicates the sandbox build finished but reported a failure.
	SandboxStatusFailed SandboxStatus = "failed"
	// SandboxStatusTimedOut indicates the sandbox build exceeded the allotted timeout.
	SandboxStatusTimedOut SandboxStatus = "timed_out"
)

const (
	// SandboxFailureReasonTimeout is recorded when a sandbox execution times out.
	SandboxFailureReasonTimeout = "timeout"
	// sandboxFailureReasonExecution is used when the sandbox execution fails for other reasons.
	sandboxFailureReasonExecution = "execution"
)

// SandboxSpec describes a sandbox build invocation.
type SandboxSpec struct {
	CacheKey     string
	Command      []string
	Env          map[string]string
	Timeout      time.Duration
	ForceRebuild bool
	Workspace    string
}

// SandboxBuildResult captures the output of the sandbox executor.
type SandboxBuildResult struct {
	Success       bool
	CacheHit      bool
	LogDigest     string
	FailureReason string
	FailureDetail string
}

// SandboxOutcome summarises the sandbox execution for checkpoint metadata and callers.
type SandboxOutcome struct {
	Status        SandboxStatus
	Duration      time.Duration
	CacheHit      bool
	LogDigest     string
	FailureReason string
	FailureDetail string
}

// SandboxExecutor executes sandbox builds.
type SandboxExecutor interface {
	Execute(ctx context.Context, spec SandboxSpec) (SandboxBuildResult, error)
}

// Clock abstracts time for deterministic testing.
type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

// SandboxRunner executes sandbox builds and converts outputs into structured outcomes.
type SandboxRunner struct {
	executor       SandboxExecutor
	clock          Clock
	defaultTimeout time.Duration
}

// SandboxRunnerOptions configures optional behaviour for the sandbox runner.
type SandboxRunnerOptions struct {
	Clock          Clock
	DefaultTimeout time.Duration
}

// NewSandboxRunner constructs a sandbox runner using the provided executor.
func NewSandboxRunner(executor SandboxExecutor, opts SandboxRunnerOptions) *SandboxRunner {
	clock := opts.Clock
	if clock == nil {
		clock = systemClock{}
	}
	return &SandboxRunner{
		executor:       executor,
		clock:          clock,
		defaultTimeout: opts.DefaultTimeout,
	}
}

var errSandboxExecutorMissing = errors.New("buildgate: sandbox executor not configured")

// Run executes the sandbox build according to the provided spec.
func (r *SandboxRunner) Run(ctx context.Context, spec SandboxSpec) (SandboxOutcome, error) {
	if r == nil || r.executor == nil {
		return SandboxOutcome{}, errSandboxExecutorMissing
	}
	if ctx == nil {
		ctx = context.Background()
	}

	timeout := spec.Timeout
	if timeout <= 0 {
		timeout = r.defaultTimeout
	}
	if timeout > 0 {
		spec.Timeout = timeout
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	start := r.clock.Now()
	result, execErr := r.executor.Execute(ctx, spec)
	end := r.clock.Now()

	duration := end.Sub(start)
	if duration < 0 {
		duration = 0
	}

	outcome := SandboxOutcome{
		Duration:  duration,
		CacheHit:  result.CacheHit,
		LogDigest: strings.TrimSpace(result.LogDigest),
	}

	if result.Success && execErr == nil {
		outcome.Status = SandboxStatusSucceeded
		return outcome, nil
	}

	if errors.Is(execErr, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		outcome.Status = SandboxStatusTimedOut
		outcome.FailureReason = SandboxFailureReasonTimeout
		detail := strings.TrimSpace(result.FailureDetail)
		if detail == "" {
			detail = context.DeadlineExceeded.Error()
		}
		outcome.FailureDetail = detail
		return outcome, nil
	}

	outcome.Status = SandboxStatusFailed
	reason := strings.TrimSpace(result.FailureReason)
	if reason == "" {
		reason = sandboxFailureReasonExecution
	}
	outcome.FailureReason = reason

	detail := strings.TrimSpace(result.FailureDetail)
	if detail == "" && execErr != nil {
		detail = execErr.Error()
	}
	outcome.FailureDetail = detail
	return outcome, nil
}
