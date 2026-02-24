// claimer_loop.go contains the claim loop orchestration and backoff logic.
//
// This file owns the Start method that continuously polls for work (jobs)
// with exponential backoff. Backoff increases when no work is available or
// on errors, and resets when work is successfully claimed.
// Nodes claim from a single unified jobs queue (FIFO by next_id); there
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
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

// Start begins the claim loop.
// Continuously polls for work (jobs) using a ticker with exponential backoff.
// Backoff increases when no work is available or on errors, resets when work is successfully claimed.
// Nodes claim from a single unified jobs queue (FIFO by next_id).
func (c *ClaimManager) Start(ctx context.Context) error {
	// Initialize ticker with the initial backoff interval from shared policy.
	ticker := time.NewTicker(time.Duration(c.backoff.GetDuration()))
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
			ticker.Reset(time.Duration(c.backoff.GetDuration()))
		}
	}
}

// claimAndExecute attempts to claim a job from the unified queue and execute it.
// Jobs are claimed FIFO by next_id from a single queue — there is no separate
// Build Gate queue or claim path. All job types (pre-gate, mod, heal, re-gate,
// post-gate) are consumed from the same queue.
// Returns true if a job was claimed, false if no work is available (204).
//
// Flow:
//  1. Acquire a concurrency slot before claiming (blocks until available).
//  2. POST /v1/nodes/{id}/claim to attempt claiming a job.
//  3. If 204 returned, release slot and return false (no jobs available).
//  4. If 200 returned, decode claim response and job metadata.
//  5. Parse spec into options and environment variables.
//  6. Invoke controller to start job execution (slot released when job completes).
//
// Note: Returns true if a job was claimed (even if ack/execution fails),
// because the work has already been assigned to this node.
func (c *ClaimManager) claimAndExecute(ctx context.Context) (bool, error) {
	// Acquire a concurrency slot before claiming. This ensures the node does not
	// claim more work than it can execute concurrently (Config.Concurrency).
	// The slot is released in executeRun's defer when the job completes.
	if err := c.controller.AcquireSlot(ctx); err != nil {
		// Context was canceled while waiting for a slot.
		return false, err
	}

	// slotHeld tracks whether we still own the concurrency slot.
	// Set to false after successful StartRun (controller takes ownership).
	slotHeld := true
	defer func() {
		// Release the slot if we still hold it (claim failed or no work available).
		if slotHeld {
			c.controller.ReleaseSlot()
		}
	}()

	// Lazy initialization: create HTTP client if not yet initialized.
	// This allows bootstrap() to run first and create certificates.
	// Uses sync.Once for thread-safe initialization.
	c.clientOnce.Do(func() {
		c.client, c.clientErr = createHTTPClient(c.cfg)
	})
	if c.clientErr != nil {
		return false, fmt.Errorf("create http client: %w", c.clientErr)
	}

	// POST /v1/nodes/{id}/claim
	apiPath := fmt.Sprintf("/v1/nodes/%s/claim", c.cfg.NodeID)
	claimURL := MustBuildURL(c.cfg.ServerURL, apiPath)

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
	defer drainAndClose(resp)

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
	// Parse spec into typed RunOptions and environment variables.
	// The typed RunOptions is the canonical source of truth; no raw map[string]any
	// is passed to StartRunRequest.
	envFromSpec, typedOpts, err := parseSpec(claim.Spec)
	if err != nil {
		return true, fmt.Errorf("parse spec: %w", err)
	}

	// Validate and derive Stack Gate chaining for multi-step runs.
	if err := validateAndDeriveStackGateChaining(typedOpts.Steps); err != nil {
		return true, fmt.Errorf("validate stack gate chaining: %w", err)
	}

	startReq := StartRunRequest{
		RunID:        claim.RunID, // Already types.RunID from ClaimResponse
		JobID:        claim.JobID, // Already types.JobID from ClaimResponse
		RepoID:       claim.RepoID,
		RepoURL:      claim.RepoURL,
		BaseRef:      claim.BaseRef,
		TargetRef:    claim.TargetRef,
		CommitSHA:    derefCommitSHA(claim.CommitSha),
		JobType:      claim.JobType,
		JobImage:     claim.JobImage,
		NextID:       claim.NextID,
		JobName:      claim.JobName, // Job name for branch identification
		TypedOptions: typedOpts,     // Strongly-typed run options (canonical source of truth)
		Env:          envFromSpec,
	}

	// Invoke controller.StartRun to execute the claimed job.
	if err := c.controller.StartRun(ctx, startReq); err != nil {
		// Even if StartRun fails, we've already claimed the work.
		// The node cannot execute this job, so the claim loop surfaces an error.
		return true, fmt.Errorf("start run: %w", err)
	}

	// Transfer slot ownership to the controller. The slot will be released
	// in executeRun's defer when the job completes.
	slotHeld = false

	return true, nil
}

func derefCommitSHA(v *types.CommitSHA) types.CommitSHA {
	if v == nil {
		return ""
	}
	return *v
}
