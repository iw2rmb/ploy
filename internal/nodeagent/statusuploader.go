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
	*baseUploader
}

// NewStatusUploader creates a new status uploader.
func NewStatusUploader(cfg Config) (*StatusUploader, error) {
	base, err := newBaseUploader(cfg)
	if err != nil {
		return nil, err
	}
	return &StatusUploader{baseUploader: base}, nil
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
	apiPath := fmt.Sprintf("/v1/jobs/%s/complete", jobID)
	url := MustBuildURL(u.cfg.ServerURL, apiPath)

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
