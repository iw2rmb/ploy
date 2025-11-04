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
)

// ClaimManager periodically polls the server for work and executes claimed runs.
type ClaimManager struct {
	cfg             Config
	client          *http.Client
	controller      RunController
	backoffDuration time.Duration
	minBackoff      time.Duration
	maxBackoff      time.Duration
}

// ClaimResponse represents the response from POST /v1/nodes/{id}/claim.
type ClaimResponse struct {
	ID        string          `json:"id"`
	RepoURL   string          `json:"repo_url"`
	Status    string          `json:"status"`
	NodeID    string          `json:"node_id"`
	BaseRef   string          `json:"base_ref"`
	TargetRef string          `json:"target_ref"`
	CommitSha *string         `json:"commit_sha,omitempty"`
	StartedAt string          `json:"started_at"`
	CreatedAt string          `json:"created_at"`
	Spec      json.RawMessage `json:"spec,omitempty"`
}

// NewClaimManager constructs a claim manager.
func NewClaimManager(cfg Config, controller RunController) (*ClaimManager, error) {
	client, err := createHTTPClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("create http client: %w", err)
	}

	return &ClaimManager{
		cfg:             cfg,
		client:          client,
		controller:      controller,
		backoffDuration: 0,
		minBackoff:      250 * time.Millisecond,
		maxBackoff:      5 * time.Second,
	}, nil
}

// Start begins the claim loop.
func (c *ClaimManager) Start(ctx context.Context) error {
	ticker := time.NewTicker(c.minBackoff)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			// Attempt to claim and execute a run.
			claimed, err := c.claimAndExecute(ctx)
			if err != nil {
				slog.Error("claim loop error", "err", err)
				c.applyBackoff()
			} else if claimed {
				// Work was claimed and executed; reset backoff.
				c.resetBackoff()
			} else {
				// No work available (204); apply backoff.
				c.applyBackoff()
			}

			// Update ticker interval based on current backoff.
			ticker.Reset(c.getBackoffDuration())
		}
	}
}

// claimAndExecute attempts to claim a run and execute it.
// Returns true if a run was claimed, false if no work is available (204).
func (c *ClaimManager) claimAndExecute(ctx context.Context) (bool, error) {
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
	// Parse spec into options/env
	optsFromSpec, envFromSpec := parseSpec(claim.Spec)

	startReq := StartRunRequest{
		RunID:     claim.ID,
		RepoURL:   claim.RepoURL,
		BaseRef:   claim.BaseRef,
		TargetRef: claim.TargetRef,
		CommitSHA: stringValue(claim.CommitSha),
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

// applyBackoff increases backoff duration exponentially.
func (c *ClaimManager) applyBackoff() {
	if c.backoffDuration == 0 {
		c.backoffDuration = c.minBackoff
	} else {
		c.backoffDuration = c.backoffDuration * 2
		if c.backoffDuration > c.maxBackoff {
			c.backoffDuration = c.maxBackoff
		}
	}
}

// resetBackoff clears backoff duration when work is successfully claimed.
func (c *ClaimManager) resetBackoff() {
	c.backoffDuration = c.minBackoff
}

// getBackoffDuration returns the current backoff duration.
func (c *ClaimManager) getBackoffDuration() time.Duration {
	if c.backoffDuration == 0 {
		return c.minBackoff
	}
	return c.backoffDuration
}

// stringValue safely dereferences a string pointer, returning empty string if nil.
func stringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// decodeSpecToOptions tries to decode a JSON spec payload into a flat options map.
// If spec is empty or invalid, returns an empty map.
// parseSpec splits a simplified spec into options and env maps.
func parseSpec(spec json.RawMessage) (map[string]any, map[string]string) {
	opts := map[string]any{}
	env := map[string]string{}
	if len(spec) == 0 {
		return opts, env
	}
	var root any
	if err := json.Unmarshal(spec, &root); err != nil {
		return opts, env
	}
	m, ok := root.(map[string]any)
	if !ok {
		return map[string]any{"spec": root}, env
	}
	// Extract known fields
	if v, ok := m["image"].(string); ok && v != "" {
		opts["image"] = v
	}
	if v, ok := m["command"].(string); ok && v != "" {
		opts["command"] = v
	}
	if arr, ok := m["command"].([]any); ok && len(arr) > 0 {
		out := make([]string, 0, len(arr))
		for _, e := range arr {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		if len(out) > 0 {
			opts["command"] = out
		}
	}
	if e, ok := m["env"].(map[string]any); ok {
		for k, v := range e {
			if s, ok := v.(string); ok {
				env[k] = s
			}
		}
	}
	// Pass through stage_id if present (server injects it on claim)
	if sid, ok := m["stage_id"].(string); ok && sid != "" {
		opts["stage_id"] = sid
	}
	return opts, env
}
