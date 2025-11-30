// claimer_buildgate.go separates buildgate job claiming and execution.
//
// This file contains buildgate-specific claim logic including job claiming,
// ACK (acknowledge) to transition job to running state, job execution, and
// completion reporting. Buildgate jobs are pre-flight validations that run
// before actual workflow execution. Isolating buildgate logic from general
// run claiming maintains separation between job types and simplifies testing.
package nodeagent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// claimAndExecuteBuildGateJob attempts to claim a buildgate job and execute it.
// Returns true if a job was claimed, false if no work is available (204 No Content).
//
// Flow:
//  1. POST /v1/nodes/{id}/buildgate/claim to attempt claiming a job.
//  2. If 204 returned, no buildgate jobs are available; return false.
//  3. If 200 returned, decode job ID and request payload.
//  4. Ack job start (transition job to "running" state).
//  5. Execute the buildgate job using BuildGateExecutor.
//  6. Upload result to server (complete endpoint).
//
// Note: This method returns true if a job was claimed (even if ack/execution fails),
// because the work has already been assigned to this node.
func (c *ClaimManager) claimAndExecuteBuildGateJob(ctx context.Context) (bool, error) {
	// Lazy initialization: create HTTP client if not yet initialized.
	// This allows bootstrap() to run first and create certificates.
	if c.client == nil {
		client, err := createHTTPClient(c.cfg)
		if err != nil {
			return false, fmt.Errorf("create http client: %w", err)
		}
		c.client = client
	}

	// POST /v1/nodes/{id}/buildgate/claim
	claimURL := fmt.Sprintf("%s/v1/nodes/%s/buildgate/claim", c.cfg.ServerURL, c.cfg.NodeID)

	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, claimURL, nil)
	if err != nil {
		return false, fmt.Errorf("create claim request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return false, fmt.Errorf("send claim request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Handle 204 No Content (no work available).
	if resp.StatusCode == http.StatusNoContent {
		return false, nil
	}

	// Handle non-200 responses.
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("claim failed: status %d: %s", resp.StatusCode, string(body))
	}

	// Decode claim response containing job ID and request payload.
	var claimResp struct {
		JobID   types.JobID `json:"job_id"`
		Request any         `json:"request"` // Arbitrary JSON; re-decode below.
		Status  string      `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&claimResp); err != nil {
		return false, fmt.Errorf("decode claim response: %w", err)
	}

	slog.Info("claimed buildgate job", "job_id", claimResp.JobID)

	// Re-encode request to proper type (BuildGateValidateRequest).
	reqBytes, err := json.Marshal(claimResp.Request)
	if err != nil {
		return true, fmt.Errorf("re-marshal request: %w", err)
	}

	var validateReq contracts.BuildGateValidateRequest
	if err := json.Unmarshal(reqBytes, &validateReq); err != nil {
		return true, fmt.Errorf("unmarshal validate request: %w", err)
	}

	// Ack job start → transition to "running" state.
	if err := c.ackBuildGateJobStart(ctx, claimResp.JobID); err != nil {
		return true, fmt.Errorf("ack buildgate job: %w", err)
	}

	// Execute the buildgate job.
	// Use background context to prevent cancellation from claim loop timeout.
	execCtx := context.Background()
	result, execErr := c.buildgateExec.Execute(execCtx, claimResp.JobID, validateReq)

	// Upload result to server (complete endpoint).
	if err := c.completeBuildGateJob(execCtx, claimResp.JobID, result, execErr); err != nil {
		return true, fmt.Errorf("complete buildgate job: %w", err)
	}

	return true, nil
}

// completeBuildGateJob sends the result of a buildgate job to the server.
// Posts to POST /v1/nodes/{id}/buildgate/{job_id}/complete with status, result, and error.
//
// Payload:
//   - status: "completed" if execErr is nil, "failed" otherwise.
//   - result: BuildGateStageMetadata if execution succeeded.
//   - error: error message string if execErr is non-nil.
func (c *ClaimManager) completeBuildGateJob(ctx context.Context, jobID types.JobID, result *contracts.BuildGateStageMetadata, execErr error) error {
	completeURL := fmt.Sprintf("%s/v1/nodes/%s/buildgate/%s/complete", c.cfg.ServerURL, c.cfg.NodeID, jobID.String())

	payload := struct {
		Status string                            `json:"status"`
		Result *contracts.BuildGateStageMetadata `json:"result,omitempty"`
		Error  *string                           `json:"error,omitempty"`
	}{
		Result: result,
	}

	// Determine status and populate error if needed.
	if execErr != nil {
		payload.Status = "failed"
		errMsg := execErr.Error()
		payload.Error = &errMsg
	} else {
		payload.Status = "completed"
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal completion payload: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, completeURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create complete request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("send complete request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("complete failed: status %d: %s", resp.StatusCode, string(respBody))
	}

	slog.Info("buildgate job result uploaded", "job_id", jobID, "status", payload.Status)
	return nil
}

// ackBuildGateJobStart sends POST /v1/nodes/{id}/buildgate/{job_id}/ack to mark job as running.
// This transitions the job from "assigned" to "running" state before execution begins.
func (c *ClaimManager) ackBuildGateJobStart(ctx context.Context, jobID types.JobID) error {
	ackURL := fmt.Sprintf("%s/v1/nodes/%s/buildgate/%s/ack", c.cfg.ServerURL, c.cfg.NodeID, jobID.String())

	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, ackURL, nil)
	if err != nil {
		return fmt.Errorf("create ack request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("send ack request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ack failed: status %d: %s", resp.StatusCode, string(body))
	}

	slog.Info("buildgate job acknowledged", "job_id", jobID)
	return nil
}
