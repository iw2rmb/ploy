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
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// ClaimManager periodically polls the server for work and executes claimed runs.
type ClaimManager struct {
	cfg             Config
	client          *http.Client
	controller      RunController
	buildgateExec   *BuildGateExecutor
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

	buildgateExec := NewBuildGateExecutor(cfg)

	return &ClaimManager{
		cfg:             cfg,
		client:          client,
		controller:      controller,
		buildgateExec:   buildgateExec,
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
			// Attempt to claim and execute work (runs or buildgate jobs).
			claimed, err := c.claimWork(ctx)
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

// claimAndExecuteBuildGateJob attempts to claim a buildgate job and execute it.
// Returns true if a job was claimed, false if no work is available.
func (c *ClaimManager) claimAndExecuteBuildGateJob(ctx context.Context) (bool, error) {
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

	// Decode claim response.
	var claimResp struct {
		JobID   string `json:"job_id"`
		Request any    `json:"request"`
		Status  string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&claimResp); err != nil {
		return false, fmt.Errorf("decode claim response: %w", err)
	}

	slog.Info("claimed buildgate job", "job_id", claimResp.JobID)

	// Re-encode request to proper type.
	reqBytes, err := json.Marshal(claimResp.Request)
	if err != nil {
		return true, fmt.Errorf("re-marshal request: %w", err)
	}

	var validateReq contracts.BuildGateValidateRequest
	if err := json.Unmarshal(reqBytes, &validateReq); err != nil {
		return true, fmt.Errorf("unmarshal validate request: %w", err)
	}

	// Execute the buildgate job.
	execCtx := context.Background() // Don't cancel execution on claim loop timeout.
	result, execErr := c.buildgateExec.Execute(execCtx, claimResp.JobID, validateReq)

	// Upload result to server.
	if err := c.completeBuildGateJob(execCtx, claimResp.JobID, result, execErr); err != nil {
		return true, fmt.Errorf("complete buildgate job: %w", err)
	}

	return true, nil
}

// completeBuildGateJob sends the result of a buildgate job to the server.
func (c *ClaimManager) completeBuildGateJob(ctx context.Context, jobID string, result *contracts.BuildGateStageMetadata, execErr error) error {
	completeURL := fmt.Sprintf("%s/v1/nodes/%s/buildgate/%s/complete", c.cfg.ServerURL, c.cfg.NodeID, jobID)

	payload := struct {
		Status string                            `json:"status"`
		Result *contracts.BuildGateStageMetadata `json:"result,omitempty"`
		Error  *string                           `json:"error,omitempty"`
	}{
		Result: result,
	}

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
	// Optional retain flag for container lifecycle
	if b, ok := m["retain_container"].(bool); ok {
		opts["retain_container"] = b
	}
	// Pass through stage_id if present (server injects it on claim)
	if sid, ok := m["stage_id"].(string); ok && sid != "" {
		opts["stage_id"] = sid
	}
	// Pass through GitLab config (PAT and domain) if present (server injects defaults on claim)
	if pat, ok := m["gitlab_pat"].(string); ok && pat != "" {
		opts["gitlab_pat"] = pat
	}
	if domain, ok := m["gitlab_domain"].(string); ok && domain != "" {
		opts["gitlab_domain"] = domain
	}
	// Pass through MR creation flags if present
	if mrSuccess, ok := m["mr_on_success"].(bool); ok {
		opts["mr_on_success"] = mrSuccess
	}
	if mrFail, ok := m["mr_on_fail"].(bool); ok {
		opts["mr_on_fail"] = mrFail
	}

	// Pass through build_gate_healing block if present so the agent can
	// execute the heal → re‑gate loop when the initial gate fails.
	// Accept either a map (decoded object) or arbitrary JSON; store as-is.
	if healing, ok := m["build_gate_healing"]; ok {
		// Only include non-empty objects/arrays to avoid confusing downstream logic.
		switch h := healing.(type) {
		case map[string]any:
			if len(h) > 0 {
				opts["build_gate_healing"] = h
			}
		case []any:
			// Unlikely, but preserve if provided (defensive).
			if len(h) > 0 {
				opts["build_gate_healing"] = h
			}
		default:
			// Preserve scalar values only when not empty
			if healing != nil {
				opts["build_gate_healing"] = healing
			}
		}
	}

	// Flatten nested mod.* into options/env (top-level values take precedence).
	if mod, ok := m["mod"].(map[string]any); ok {
		// image
		if _, present := opts["image"]; !present {
			if v, ok := mod["image"].(string); ok && v != "" {
				opts["image"] = v
			}
		}
		// command (string or array)
		if _, present := opts["command"]; !present {
			switch v := mod["command"].(type) {
			case []any:
				out := make([]string, 0, len(v))
				for _, e := range v {
					if s, ok := e.(string); ok {
						out = append(out, s)
					}
				}
				if len(out) > 0 {
					opts["command"] = out
				}
			case string:
				if s := v; s != "" {
					opts["command"] = s
				}
			}
		}
		// retain_container
		if _, present := opts["retain_container"]; !present {
			if b, ok := mod["retain_container"].(bool); ok {
				opts["retain_container"] = b
			}
		}
		// env merge (top-level env wins on conflict)
		if em, ok := mod["env"].(map[string]any); ok {
			for k, v := range em {
				if s, ok := v.(string); ok {
					if _, exists := env[k]; !exists {
						env[k] = s
					}
				}
			}
		}
	}

	// Flatten build_gate.enabled/profile for manifest builder to honor.
	if bg, ok := m["build_gate"].(map[string]any); ok {
		if b, ok := bg["enabled"].(bool); ok {
			opts["build_gate_enabled"] = b
		}
		if p, ok := bg["profile"].(string); ok && p != "" {
			opts["build_gate_profile"] = p
		}
	}
	return opts, env
}
