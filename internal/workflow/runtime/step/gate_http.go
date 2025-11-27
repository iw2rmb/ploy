// gate_http.go implements the HTTP-based GateExecutor for remote build validation.
//
// This executor delegates build validation to the Build Gate API via HTTP, enabling
// gate jobs to run on any eligible Build Gate worker node rather than on the local
// node agent. This decouples gate execution from the Mods execution node, supporting
// multi-VPS architectures where dedicated Build Gate workers handle validation.
//
// ## Usage
//
// HTTPGateExecutor requires a BuildGateHTTPClient (see gate_http_client.go) for
// communication with the Build Gate API. Create one via NewHTTPGateExecutor:
//
//	client, _ := NewBuildGateHTTPClient(cfg)
//	executor := NewHTTPGateExecutor(client)
//	result, err := executor.Execute(ctx, spec, workspace)
//
// ## Sync vs Async Execution
//
// This implementation supports both synchronous and asynchronous job execution:
//   - If Validate returns an immediate result (Status=Completed), it is returned directly.
//   - If Validate returns Status=Pending/Claimed/Running, the executor polls GetJob
//     with exponential backoff until the job completes, fails, or the context times out.
//
// ## Timeout Configuration
//
// Async polling uses a timeout derived from (in priority order):
//  1. Context deadline (if already set by caller)
//  2. PLOY_BUILDGATE_TIMEOUT environment variable (e.g., "10m")
//  3. Default of 10 minutes
//
// ## Relationship to DockerGateExecutor
//
// HTTPGateExecutor and dockerGateExecutor both implement GateExecutor, allowing
// configuration-driven selection (Phase B4). The HTTP executor sends validation
// requests to remote workers, while the Docker executor runs builds locally.
// Both produce equivalent BuildGateStageMetadata for consistent downstream handling.
package step

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/backoff"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

const (
	// defaultBuildGateTimeout is the default timeout for async job polling when no
	// context deadline is set and PLOY_BUILDGATE_TIMEOUT is not configured.
	defaultBuildGateTimeout = 10 * time.Minute

	// buildGateTimeoutEnv is the environment variable for configuring the timeout.
	buildGateTimeoutEnv = "PLOY_BUILDGATE_TIMEOUT"
)

// httpGateExecutor implements GateExecutor by delegating to the Build Gate HTTP API.
// This enables gate validation to run on remote Build Gate workers rather than locally.
type httpGateExecutor struct {
	// client handles HTTP communication with the Build Gate API.
	client BuildGateHTTPClient

	// logger for structured logging during polling. Falls back to slog.Default() if nil.
	logger *slog.Logger
}

// NewHTTPGateExecutor constructs a GateExecutor that uses the Build Gate HTTP API
// for remote validation. The provided client must implement BuildGateHTTPClient
// (typically created via NewBuildGateHTTPClient).
//
// Returns nil if client is nil, allowing callers to gracefully degrade when
// HTTP-based gate execution is not configured.
func NewHTTPGateExecutor(client BuildGateHTTPClient) GateExecutor {
	if client == nil {
		return nil
	}
	return &httpGateExecutor{client: client, logger: slog.Default()}
}

// NewHTTPGateExecutorWithLogger constructs a GateExecutor with a custom logger
// for structured logging during async job polling.
func NewHTTPGateExecutorWithLogger(client BuildGateHTTPClient, logger *slog.Logger) GateExecutor {
	if client == nil {
		return nil
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &httpGateExecutor{client: client, logger: logger}
}

// Execute submits a build validation request to the Build Gate HTTP API.
//
// Behavior mirrors dockerGateExecutor for consistency:
//   - Returns (nil, nil) when spec is nil or spec.Enabled is false.
//   - Returns (nil, ctx.Err()) if the context is already cancelled.
//
// Request construction:
//   - Profile and Timeout are taken from spec if provided.
//   - RepoURL and Ref are temporary placeholders; Phase C will wire repo+diff
//     metadata from the step manifest.
//
// Response handling:
//   - If the API returns an immediate result (Status=Completed), Result is returned.
//   - If the API returns Status=Failed, an error is returned with the job ID.
//   - If the API returns Status=Pending/Claimed/Running (async job), the executor
//     polls GetJob with exponential backoff until completion, failure, or timeout.
//
// The workspace parameter is currently unused for HTTP mode since remote workers
// receive repo+diff payloads rather than direct workspace access.
func (e *httpGateExecutor) Execute(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
	// Honor context cancellation early (consistent with dockerGateExecutor).
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Early return for disabled or nil spec (mirrors dockerGateExecutor behavior).
	if spec == nil || !spec.Enabled {
		return nil, nil
	}

	// Build a minimal request. Phase C (C1) will wire repo metadata (RepoURL, Ref)
	// from the step manifest. For now, these are temporary placeholders to satisfy
	// the BuildGateValidateRequest.Validate() requirements.
	//
	// NOTE: Callers must ensure RepoURL and Ref are populated before using this
	// executor in production. This skeleton uses placeholder values that will fail
	// validation in the HTTP client unless the spec carries real repo metadata.
	req := contracts.BuildGateValidateRequest{
		// Temporary: placeholder values. Phase C will populate from step manifest.
		// The HTTP client will return a validation error if these remain empty,
		// which is the expected behavior until repo+diff wiring is complete.
		RepoURL: "", // TODO(B2): Wire from step manifest in Phase C.
		Ref:     "", // TODO(B2): Wire from step manifest in Phase C.

		// Profile from spec if provided; empty string triggers auto-detection on server.
		Profile: spec.Profile,
		// Timeout: StepGateSpec doesn't have timeout; use server default. Phase C may
		// add timeout from manifest or env (PLOY_BUILDGATE_TIMEOUT).
	}

	// Submit validation request to the Build Gate API.
	resp, err := e.client.Validate(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("validate via http: %w", err)
	}

	// Handle response based on job status.
	return e.handleValidateResponse(ctx, resp)
}

// handleValidateResponse processes the validate response and polls if needed.
// For sync completion (Completed/Failed), returns immediately.
// For async statuses (Pending/Claimed/Running), polls GetJob until terminal state.
func (e *httpGateExecutor) handleValidateResponse(ctx context.Context, resp *contracts.BuildGateValidateResponse) (*contracts.BuildGateStageMetadata, error) {
	switch resp.Status {
	case contracts.BuildGateJobStatusCompleted:
		// Sync completion: result is available immediately.
		if resp.Result != nil {
			return resp.Result, nil
		}
		// Completed status but no result is unexpected; treat as empty metadata.
		return &contracts.BuildGateStageMetadata{}, nil

	case contracts.BuildGateJobStatusFailed:
		// Job failed on the server. Return error with job ID for debugging.
		return nil, fmt.Errorf("build gate job failed: job_id=%s", resp.JobID)

	case contracts.BuildGateJobStatusPending, contracts.BuildGateJobStatusClaimed, contracts.BuildGateJobStatusRunning:
		// Async job: poll GetJob until completion or timeout.
		return e.pollJobUntilDone(ctx, resp.JobID)

	default:
		// Unknown status: defensive error handling.
		return nil, fmt.Errorf("unexpected job status: %s", resp.Status)
	}
}

// pollJobUntilDone polls the Build Gate API for job status until the job reaches
// a terminal state (Completed or Failed) or the context times out.
//
// Timeout resolution (priority order):
//  1. Existing context deadline (if set by caller)
//  2. PLOY_BUILDGATE_TIMEOUT environment variable
//  3. defaultBuildGateTimeout (10 minutes)
//
// Uses exponential backoff via internal/workflow/backoff to avoid hammering the API.
func (e *httpGateExecutor) pollJobUntilDone(ctx context.Context, jobID string) (*contracts.BuildGateStageMetadata, error) {
	// Create a polling context with appropriate timeout.
	pollCtx, cancel := e.createPollContext(ctx)
	defer cancel()

	// Track result and error across poll iterations. The backoff.PollWithBackoff
	// returns when condition returns (true, nil) or an error. We capture the final
	// result/error in these variables since the condition func can't return them.
	var (
		result   *contracts.BuildGateStageMetadata
		pollErr  error
		jobError string // Error message from failed job (from BuildGateJobStatusResponse.Error).
	)

	e.logger.Debug("starting async job polling",
		"job_id", jobID,
		"timeout", e.getPollTimeout(),
	)

	// Define the polling policy. Use DefaultPolicy as a base but adjust:
	// - Shorter initial interval (1s) for responsive polling
	// - Max interval of 10s to avoid long gaps
	// - No max attempts (controlled by context timeout instead)
	// - No max elapsed time (controlled by context timeout instead)
	policy := backoff.Policy{
		InitialInterval: 1 * time.Second,
		MaxInterval:     10 * time.Second,
		Multiplier:      1.5, // Gentler growth than default 2.0 for polling.
		MaxElapsedTime:  0,   // Rely on context timeout.
		MaxAttempts:     0,   // Rely on context timeout.
	}

	// Poll using backoff helper. The condition returns:
	// - (true, nil) when job is completed successfully
	// - (false, permanentErr) when job failed (stops polling)
	// - (false, nil) when job is still pending (continues polling)
	// - (false, err) when GetJob call failed (retries with backoff)
	err := backoff.PollWithBackoff(pollCtx, policy, e.logger, func() (bool, error) {
		resp, getErr := e.client.GetJob(pollCtx, jobID)
		if getErr != nil {
			// GetJob call failed (network error, etc). Retry with backoff.
			e.logger.Debug("getjob call failed, will retry",
				"job_id", jobID,
				"error", getErr,
			)
			return false, getErr
		}

		e.logger.Debug("poll status",
			"job_id", jobID,
			"status", resp.Status,
		)

		switch resp.Status {
		case contracts.BuildGateJobStatusCompleted:
			// Job completed successfully. Capture result and signal done.
			if resp.Result != nil {
				result = resp.Result
			} else {
				// Completed but no result is unexpected; use empty metadata.
				result = &contracts.BuildGateStageMetadata{}
			}
			return true, nil

		case contracts.BuildGateJobStatusFailed:
			// Job failed. Capture error and stop polling with permanent error.
			jobError = resp.Error
			pollErr = fmt.Errorf("build gate job failed: job_id=%s", jobID)
			if jobError != "" {
				pollErr = fmt.Errorf("build gate job failed: job_id=%s, error=%s", jobID, jobError)
			}
			// Return permanent error to stop retries.
			return false, backoff.Permanent(pollErr)

		case contracts.BuildGateJobStatusPending, contracts.BuildGateJobStatusClaimed, contracts.BuildGateJobStatusRunning:
			// Job still in progress. Continue polling.
			return false, nil

		default:
			// Unknown status. Treat as permanent error.
			pollErr = fmt.Errorf("unexpected job status during polling: %s", resp.Status)
			return false, backoff.Permanent(pollErr)
		}
	})

	// Handle polling outcome.
	if err != nil {
		// Check if context was cancelled/timed out.
		if pollCtx.Err() != nil {
			return nil, fmt.Errorf("build gate job polling timed out: job_id=%s: %w", jobID, pollCtx.Err())
		}
		// Check if we captured a specific poll error (e.g., job failed).
		if pollErr != nil {
			return nil, pollErr
		}
		// Otherwise return the backoff error (e.g., max retries on GetJob failures).
		return nil, fmt.Errorf("build gate job polling failed: job_id=%s: %w", jobID, err)
	}

	// Success: return the result captured during polling.
	e.logger.Debug("job polling completed successfully", "job_id", jobID)
	return result, nil
}

// createPollContext creates a context with appropriate timeout for job polling.
// If the parent context already has a deadline, it is used. Otherwise, the timeout
// is derived from PLOY_BUILDGATE_TIMEOUT env or defaultBuildGateTimeout.
func (e *httpGateExecutor) createPollContext(ctx context.Context) (context.Context, context.CancelFunc) {
	// If context already has a deadline, use it as-is.
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		return context.WithCancel(ctx)
	}

	// Otherwise apply configured or default timeout.
	timeout := e.getPollTimeout()
	return context.WithTimeout(ctx, timeout)
}

// getPollTimeout returns the polling timeout from environment or default.
func (e *httpGateExecutor) getPollTimeout() time.Duration {
	if envVal := os.Getenv(buildGateTimeoutEnv); envVal != "" {
		if d, err := time.ParseDuration(envVal); err == nil && d > 0 {
			return d
		}
		e.logger.Warn("invalid PLOY_BUILDGATE_TIMEOUT, using default",
			"value", envVal,
			"default", defaultBuildGateTimeout,
		)
	}
	return defaultBuildGateTimeout
}
