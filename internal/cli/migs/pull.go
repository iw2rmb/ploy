// pull.go provides CLI client implementations for pull resolution APIs.
//
// These commands call the server endpoints:
//   - POST /v1/runs/{run_id}/pull (resolve repo for a run)
//   - POST /v1/migs/{mod_id}/pull (resolve repo for a mig)
//
// These endpoints help CLI clients resolve repo execution identifiers needed to
// pull diffs from the server.
package migs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/iw2rmb/ploy/internal/cli/httpx"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// =============================================================================
// Pull Resolution Response Types
// =============================================================================

// PullResolution is the response from pull resolution endpoints.
// It provides the identifiers needed to fetch diffs:
//   - RunID: the run containing the execution
//   - RepoID: the mod_repos.id for the matched repo
//   - RepoTargetRef: the target ref snapshot from run_repos
type PullResolution struct {
	RunID         domaintypes.RunID     `json:"run_id"`
	RepoID        domaintypes.MigRepoID `json:"repo_id"`
	RepoTargetRef domaintypes.GitRef    `json:"repo_target_ref"`
}

// =============================================================================
// Run Pull Resolution Command
// =============================================================================

// RunPullCommand resolves a repo_url to execution identifiers for a specific run.
// Endpoint: POST /v1/runs/{run_id}/pull
//
// Server matches the repo by joining run_repos to mod_repos by repo_id,
// filtering by run_id, and comparing normalized repo_url.
// Returns 404 if no repo matches, 409 if multiple repos match (ambiguous).
type RunPullCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	RunID   domaintypes.RunID
	RepoURL string // Repository URL to match
}

// Run executes POST /v1/runs/{run_id}/pull with the provided repo_url.
// Returns the PullResolution containing run_id, repo_id, and repo_target_ref.
func (c RunPullCommand) Run(ctx context.Context) (*PullResolution, error) {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return nil, fmt.Errorf("run pull: %w", err)
	}
	if c.RunID.IsZero() {
		return nil, fmt.Errorf("run pull: run id required")
	}
	repoURL := strings.TrimSpace(c.RepoURL)
	if repoURL == "" {
		return nil, fmt.Errorf("run pull: repo url required")
	}
	if err := domaintypes.RepoURL(repoURL).Validate(); err != nil {
		return nil, fmt.Errorf("run pull: repo url must be a valid repo url")
	}

	// Build endpoint: POST /v1/runs/{run_id}/pull
	endpoint := c.BaseURL.JoinPath("v1", "runs", c.RunID.String(), "pull")

	// Build request body: {"repo_url": "..."}
	reqBody := struct {
		RepoURL string `json:"repo_url"`
	}{
		RepoURL: repoURL,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("run pull: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("run pull: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("run pull: http request failed: %w", err)
	}
	defer httpx.DrainAndClose(resp)

	if resp.StatusCode != http.StatusOK {
		return nil, httpx.WrapError("run pull", resp.Status, resp.Body)
	}

	var result PullResolution
	if err := httpx.DecodeJSON(resp.Body, &result, httpx.MaxJSONBodyBytes); err != nil {
		return nil, fmt.Errorf("run pull: decode response: %w", err)
	}

	return &result, nil
}

// =============================================================================
// Mod Pull Resolution Command
// =============================================================================

// PullMode specifies which run to select for mig pull resolution.
type PullMode string

const (
	// PullModeLastSucceeded selects the newest terminal run with status=Success (default).
	PullModeLastSucceeded PullMode = "last-succeeded"
	// PullModeLastFailed selects the newest terminal run with status=Fail.
	PullModeLastFailed PullMode = "last-failed"
)

// ModPullCommand resolves a repo_url to execution identifiers for a mig.
// Endpoint: POST /v1/migs/{mod_id}/pull
//
// Server performs the lookup using mod_id + repo_url to find mod_repos.id,
// then selects the appropriate run_repos by created_at DESC, filtering by
// the requested terminal status (Success or Fail).
// Mode values:
//   - "last-succeeded" (default): newest run_repos with status=Success
//   - "last-failed": newest run_repos with status=Fail
//
// Returns 404 if no repo matches or no run with matching status found.
type ModPullCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	MigRef  domaintypes.MigRef
	RepoURL string   // Repository URL to match
	Mode    PullMode // Pull mode (last-succeeded or last-failed)
}

// Run executes POST /v1/migs/{mod_id}/pull with the provided repo_url and mode.
// Returns the PullResolution containing run_id, repo_id, and repo_target_ref.
func (c ModPullCommand) Run(ctx context.Context) (*PullResolution, error) {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return nil, fmt.Errorf("mig pull: %w", err)
	}
	if err := c.MigRef.Validate(); err != nil {
		return nil, fmt.Errorf("mig pull: mig id required")
	}
	repoURL := strings.TrimSpace(c.RepoURL)
	if repoURL == "" {
		return nil, fmt.Errorf("mig pull: repo url required")
	}
	if err := domaintypes.RepoURL(repoURL).Validate(); err != nil {
		return nil, fmt.Errorf("mig pull: repo url must be a valid repo url")
	}

	// Default mode is "last-succeeded".
	mode := c.Mode
	if mode == "" {
		mode = PullModeLastSucceeded
	}

	// Build endpoint: POST /v1/migs/{mod_id}/pull
	endpoint := c.BaseURL.JoinPath("v1", "migs", c.MigRef.String(), "pull")

	// Build request body: {"repo_url": "...", "mode": "..."}
	reqBody := struct {
		RepoURL string `json:"repo_url"`
		Mode    string `json:"mode,omitempty"`
	}{
		RepoURL: repoURL,
		Mode:    string(mode),
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("mig pull: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("mig pull: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("mig pull: http request failed: %w", err)
	}
	defer httpx.DrainAndClose(resp)

	if resp.StatusCode != http.StatusOK {
		return nil, httpx.WrapError("mig pull", resp.Status, resp.Body)
	}

	var result PullResolution
	if err := httpx.DecodeJSON(resp.Body, &result, httpx.MaxJSONBodyBytes); err != nil {
		return nil, fmt.Errorf("mig pull: decode response: %w", err)
	}

	return &result, nil
}
