package prep

import (
	"context"
	"fmt"

	"github.com/iw2rmb/ploy/internal/store"
)

const (
	FailureCodeToolNotDetected        = "tool_not_detected"
	FailureCodeRuntimeVersionMismatch = "runtime_version_mismatch"
	FailureCodeDockerAPIMismatch      = "docker_api_mismatch"
	FailureCodeRegistryAuthFailed     = "registry_auth_failed"
	FailureCodeRegistryCATrustFailed  = "registry_ca_trust_failed"
	FailureCodeServiceUnreachable     = "external_service_unreachable"
	FailureCodeCommandNotFound        = "command_not_found"
	FailureCodeTimeout                = "timeout"
	FailureCodeUnknown                = "unknown"
)

// Runner executes one non-interactive prep attempt.
type Runner interface {
	Run(ctx context.Context, req RunRequest) (RunResult, error)
}

// RunRequest provides repository identity and attempt metadata for one prep run.
type RunRequest struct {
	Repo    store.MigRepo
	Attempt int32
}

// RunResult contains outputs produced by a successful prep execution.
type RunResult struct {
	ProfileJSON []byte
	ResultJSON  []byte
	LogsRef     *string
}

// RunError carries structured failure data produced by the runner.
type RunError struct {
	Cause       error
	Message     string
	FailureCode string
	ResultJSON  []byte
	LogsRef     *string
}

func (e *RunError) Error() string {
	if e == nil {
		return "prep runner failed"
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Cause != nil {
		return e.Cause.Error()
	}
	return "prep runner failed"
}

func (e *RunError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func normalizeFailureCode(code string) string {
	switch code {
	case FailureCodeToolNotDetected,
		FailureCodeRuntimeVersionMismatch,
		FailureCodeDockerAPIMismatch,
		FailureCodeRegistryAuthFailed,
		FailureCodeRegistryCATrustFailed,
		FailureCodeServiceUnreachable,
		FailureCodeCommandNotFound,
		FailureCodeTimeout,
		FailureCodeUnknown:
		return code
	default:
		return FailureCodeUnknown
	}
}

func newRunError(cause error, failureCode string, resultJSON []byte, logsRef *string) *RunError {
	return &RunError{
		Cause:       cause,
		Message:     fmt.Sprintf("prep runner failed: %v", cause),
		FailureCode: normalizeFailureCode(failureCode),
		ResultJSON:  resultJSON,
		LogsRef:     logsRef,
	}
}
