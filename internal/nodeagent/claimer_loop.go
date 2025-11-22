// claimer_loop.go contains the claim loop orchestration and backoff logic.
//
// This file owns the Start method that continuously polls for work (runs or
// buildgate jobs) with exponential backoff. Backoff increases when no work
// is available or on errors, and resets when work is successfully claimed.
// Work claiming prioritizes buildgate jobs before regular runs. Isolating
// loop mechanics from claim/execution details simplifies backoff testing.
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

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

// Start begins the claim loop.
// Continuously polls for work (runs or buildgate jobs) using a ticker with exponential backoff.
// Backoff increases when no work is available or on errors, resets when work is successfully claimed.
func (c *ClaimManager) Start(ctx context.Context) error {
	// Initialize ticker with the initial backoff interval from shared policy.
	ticker := time.NewTicker(c.backoff.GetDuration())
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			// Attempt to claim and execute work (runs or buildgate jobs).
			claimed, err := c.claimWork(ctx)
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

// claimWork attempts to claim either a run or a buildgate job.
// Tries buildgate jobs first (lighter weight), then falls back to runs.
// Returns true if work was claimed, false if no work is available.
func (c *ClaimManager) claimWork(ctx context.Context) (bool, error) {
	// Try claiming a buildgate job first.
	claimedBuildGate, err := c.claimAndExecuteBuildGateJob(ctx)
	if err != nil {
		return false, fmt.Errorf("claim buildgate job: %w", err)
	}
	if claimedBuildGate {
		return true, nil
	}

	// If no buildgate job available, try claiming a run.
	claimedRun, err := c.claimAndExecute(ctx)
	if err != nil {
		return false, fmt.Errorf("claim run: %w", err)
	}

	return claimedRun, nil
}

// claimAndExecute attempts to claim a run and execute it.
// Returns true if a run was claimed, false if no work is available (204).
//
// Flow:
//  1. POST /v1/nodes/{id}/claim to attempt claiming a run.
//  2. If 204 returned, no runs are available; return false.
//  3. If 200 returned, decode claim response and run metadata.
//  4. Ack run start (transition run to "running" state).
//  5. Parse spec into options and environment variables.
//  6. Invoke controller to start run execution.
//
// Note: Returns true if a run was claimed (even if ack/execution fails),
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

	slog.Info("claimed run", "run_id", claim.ID, "repo_url", claim.RepoURL)

	// Send ack before execution.
	if err := c.ackRun(ctx, claim.ID); err != nil {
		return true, fmt.Errorf("ack run: %w", err)
	}

	// Map claim response to StartRunRequest.
	// Parse spec into options/env and typed RunOptions.
	optsFromSpec, envFromSpec, _ := parseSpec(claim.Spec)

	startReq := StartRunRequest{
		RunID:     types.RunID(claim.ID),
		RepoURL:   types.RepoURL(claim.RepoURL),
		BaseRef:   types.GitRef(claim.BaseRef),
		TargetRef: types.GitRef(claim.TargetRef),
		CommitSHA: types.CommitSHA(stringValue(claim.CommitSha)),
		Options:   optsFromSpec,
		Env:       envFromSpec,
	}

	// Invoke controller.StartRun to execute the run.
	if err := c.controller.StartRun(ctx, startReq); err != nil {
		// Even if StartRun fails, we've already claimed the work.
		// The controller's executeRun will upload terminal status.
		return true, fmt.Errorf("start run: %w", err)
	}

	return true, nil
}

// ackRun sends POST /v1/nodes/{id}/ack to acknowledge run start.
// This transitions the run from "assigned" to "running" state before execution begins.
func (c *ClaimManager) ackRun(ctx context.Context, runID string) error {
	ackURL := fmt.Sprintf("%s/v1/nodes/%s/ack", c.cfg.ServerURL, c.cfg.NodeID)

	payload := map[string]string{"run_id": runID}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal ack payload: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, ackURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create ack request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("send ack request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ack failed: status %d: %s", resp.StatusCode, string(respBody))
	}

	slog.Info("run acknowledged", "run_id", runID)
	return nil
}
