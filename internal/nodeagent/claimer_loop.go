// claimer_loop.go contains the claim loop orchestration and backoff logic.
//
// This file owns the Start method that continuously polls for work (jobs)
// with exponential backoff. Backoff increases when no work is available or
// on errors, and resets when work is successfully claimed.
// Nodes claim from a single unified jobs queue (FIFO by step_index); there
// is no separate Build Gate queue or claim path. Isolating loop mechanics
// from claim/execution details simplifies backoff testing.
package nodeagent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

// Start begins the claim loop.
// Continuously polls for work (jobs) using a ticker with exponential backoff.
// Backoff increases when no work is available or on errors, resets when work is successfully claimed.
// Nodes claim from a single unified jobs queue (FIFO by step_index).
func (c *ClaimManager) Start(ctx context.Context) error {
	// Initialize ticker with the initial backoff interval from shared policy.
	ticker := time.NewTicker(c.backoff.GetDuration())
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			// Attempt to claim and execute a job from the unified queue.
			claimed, err := c.claimAndExecute(ctx)
			// Handle claim result: error, success, or no work available.
			switch {
			case err != nil:
				slog.Error("claim loop error", "err", err)
				// Apply backoff on error; advances to next exponential interval.
				c.backoff.Apply()
			case claimed:
				// Work was claimed and executed; reset backoff to initial interval.
				c.backoff.Reset()
			default:
				// No work available (204); apply backoff to increase polling interval.
				c.backoff.Apply()
			}

			// Update ticker interval based on current backoff duration from shared policy.
			ticker.Reset(c.backoff.GetDuration())
		}
	}
}

// claimAndExecute attempts to claim a job from the unified queue and execute it.
// Jobs are claimed FIFO by step_index from a single queue — there is no separate
// Build Gate queue or claim path. All job types (pre-gate, mod, heal, re-gate,
// post-gate) are consumed from the same queue.
// Returns true if a job was claimed, false if no work is available (204).
//
// Flow:
//  1. POST /v1/nodes/{id}/claim to attempt claiming a job.
//  2. If 204 returned, no jobs are available; return false.
//  3. If 200 returned, decode claim response and job metadata.
//  4. Parse spec into options and environment variables.
//  5. Invoke controller to start job execution.
//
// Note: Returns true if a job was claimed (even if ack/execution fails),
// because the work has already been assigned to this node.
func (c *ClaimManager) claimAndExecute(ctx context.Context) (bool, error) {
	// Lazy initialization: create HTTP client if not yet initialized.
	// This allows bootstrap() to run first and create certificates.
	if c.client == nil {
		client, err := createHTTPClient(c.cfg)
		if err != nil {
			return false, fmt.Errorf("create http client: %w", err)
		}
		c.client = client
	}

	// POST /v1/nodes/{id}/claim
	claimURL := fmt.Sprintf("%s/v1/nodes/%s/claim", c.cfg.ServerURL, c.cfg.NodeID)

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

	// Decode claim response.
	var claim ClaimResponse
	if err := json.NewDecoder(resp.Body).Decode(&claim); err != nil {
		return false, fmt.Errorf("decode claim response: %w", err)
	}

	slog.Info("claimed job", "run_id", claim.RunID, "job_id", claim.JobID, "job_name", claim.JobName, "repo_url", claim.RepoURL)

	// Map claim response to StartRunRequest.
	// Parse spec into options/env and typed RunOptions.
	optsFromSpec, envFromSpec, _ := parseSpec(claim.Spec)

	// Derive target ref: prefer server-provided value, otherwise default to
	// /mod/<run-id> so MR branch and PLOY_TARGET_REF have a deterministic name.
	targetRef := strings.TrimSpace(claim.TargetRef)
	if targetRef == "" {
		targetRef = fmt.Sprintf("/mod/%s", claim.RunID)
	}

	startReq := StartRunRequest{
		RunID:     claim.RunID, // Already types.RunID from ClaimResponse
		JobID:     claim.JobID, // Already types.JobID from ClaimResponse
		RepoURL:   types.RepoURL(claim.RepoURL),
		BaseRef:   types.GitRef(claim.BaseRef),
		TargetRef: types.GitRef(targetRef),
		CommitSHA: types.CommitSHA(stringValue(claim.CommitSha)),
		StepIndex: claim.StepIndex, // Job step_index from server
		ModType:   claim.ModType,
		ModImage:  claim.ModImage,
		JobName:   claim.JobName, // Job name for branch identification
		Options:   optsFromSpec,
		Env:       envFromSpec,
	}

	// Invoke controller.StartRun to execute the claimed job.
	if err := c.controller.StartRun(ctx, startReq); err != nil {
		// Even if StartRun fails, we've already claimed the work.
		// The controller's executeRun will upload terminal status.
		return true, fmt.Errorf("start run: %w", err)
	}

	return true, nil
}
