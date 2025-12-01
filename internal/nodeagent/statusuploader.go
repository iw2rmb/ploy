package nodeagent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/cenkalti/backoff/v5"
	types "github.com/iw2rmb/ploy/internal/domain/types"
	wfbackoff "github.com/iw2rmb/ploy/internal/workflow/backoff"
)

// StatusUploader uploads terminal status and stats to the control-plane server.
type StatusUploader struct {
	cfg    Config
	client *http.Client
}

// NewStatusUploader creates a new status uploader.
func NewStatusUploader(cfg Config) (*StatusUploader, error) {
	client, err := createHTTPClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("create http client: %w", err)
	}

	return &StatusUploader{
		cfg:    cfg,
		client: client,
	}, nil
}

// UploadStatus uploads terminal status and stats to the server with retry on transient 5xx errors.
// Includes job_id in the payload to identify the job being completed (avoids float equality issues).
// step_index is retained for logging/diagnostics but job_id is the authoritative lookup key.
// exitCode is the exit code from job execution (required for terminal status).
func (u *StatusUploader) UploadStatus(ctx context.Context, runID, status string, exitCode *int32, stats types.RunStats, stepIndex types.StepIndex, jobID types.JobID) error {
	// Build request payload with job_id as the authoritative job identifier.
	payload := map[string]interface{}{
		"run_id":     runID,
		"job_id":     jobID,
		"status":     status,
		"step_index": stepIndex,
	}

	if exitCode != nil {
		payload["exit_code"] = *exitCode
	}

	if stats != nil {
		payload["stats"] = stats
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	// Construct URL.
	url := fmt.Sprintf("%s/v1/nodes/%s/complete", u.cfg.ServerURL, u.cfg.NodeID)

	// Use shared backoff policy for status upload retries.
	// Retries on network errors and 5xx responses with exponential backoff.
	policy := wfbackoff.StatusUploaderPolicy()
	logger := slog.Default()

	// Track attempt count for logging (matches existing behavior).
	attempt := 0
	var lastErr error

	// Define the upload operation with retry logic.
	uploadOp := func() error {
		attempt++

		// Create HTTP request.
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			// Request creation errors are non-retryable.
			return backoff.Permanent(fmt.Errorf("create request: %w", err))
		}
		req.Header.Set("Content-Type", "application/json")

		// Send request.
		resp, err := u.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("send request: %w", err)
			// Network errors are retryable.
			logger.Warn("upload status request failed, retrying", "run_id", runID, "attempt", attempt, "error", err)
			return lastErr
		}

		// Read response body for error messages.
		bodyBytes, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		// Check response status.
		// Accept both 200 and 204 as success per ROADMAP requirement.
		if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent {
			return nil
		}

		// Retry on transient 5xx errors.
		if resp.StatusCode >= 500 && resp.StatusCode < 600 {
			lastErr = fmt.Errorf("upload failed: status %d: %s", resp.StatusCode, string(bodyBytes))
			logger.Warn("upload status received 5xx, retrying", "run_id", runID, "status_code", resp.StatusCode, "attempt", attempt)
			return lastErr
		}

		// Non-retryable error (4xx or other) - mark as permanent.
		return backoff.Permanent(fmt.Errorf("upload failed: status %d: %s", resp.StatusCode, string(bodyBytes)))
	}

	// Run the upload operation with shared backoff.
	return wfbackoff.RunWithBackoff(ctx, policy, logger, uploadOp)
}

// UploadJobStatus uploads terminal status and stats directly to the job-level endpoint.
// This simplified endpoint addresses jobs directly via /v1/jobs/{job_id}/complete,
// eliminating the need for run_id, step_index, and node_id in the request payload.
// Node identity is derived from the mTLS certificate.
//
// This is the preferred method for job completion as it simplifies the node → server contract.
func (u *StatusUploader) UploadJobStatus(ctx context.Context, jobID types.JobID, status string, exitCode *int32, stats types.RunStats) error {
	// Build simplified request payload (job_id is in URL, node identity from mTLS).
	payload := map[string]interface{}{
		"status": status,
	}

	if exitCode != nil {
		payload["exit_code"] = *exitCode
	}

	if stats != nil {
		payload["stats"] = stats
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	// Construct URL using job-level endpoint.
	url := fmt.Sprintf("%s/v1/jobs/%s/complete", u.cfg.ServerURL, jobID)

	// Use shared backoff policy for status upload retries.
	policy := wfbackoff.StatusUploaderPolicy()
	logger := slog.Default()

	attempt := 0
	var lastErr error

	// Define the upload operation with retry logic.
	uploadOp := func() error {
		attempt++

		// Create HTTP request.
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return backoff.Permanent(fmt.Errorf("create request: %w", err))
		}
		req.Header.Set("Content-Type", "application/json")

		// Send request (mTLS provides node identity via client certificate).
		resp, err := u.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("send request: %w", err)
			logger.Warn("upload job status request failed, retrying",
				"job_id", jobID,
				"attempt", attempt,
				"error", err,
			)
			return lastErr
		}

		// Read response body for error messages.
		bodyBytes, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		// Accept both 200 and 204 as success.
		if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent {
			return nil
		}

		// Retry on transient 5xx errors.
		if resp.StatusCode >= 500 && resp.StatusCode < 600 {
			lastErr = fmt.Errorf("upload failed: status %d: %s", resp.StatusCode, string(bodyBytes))
			logger.Warn("upload job status received 5xx, retrying",
				"job_id", jobID,
				"status_code", resp.StatusCode,
				"attempt", attempt,
			)
			return lastErr
		}

		// Non-retryable error (4xx or other).
		return backoff.Permanent(fmt.Errorf("upload failed: status %d: %s", resp.StatusCode, string(bodyBytes)))
	}

	return wfbackoff.RunWithBackoff(ctx, policy, logger, uploadOp)
}
