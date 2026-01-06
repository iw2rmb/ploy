// pull.go provides CLI client implementations for pull resolution APIs.
//
// These commands call the server endpoints:
//   - POST /v1/runs/{run_id}/pull (resolve repo for a run)
//   - POST /v1/mods/{mod_id}/pull (resolve repo for a mod)
//
// Per roadmap/v1/api.md:57-82 and roadmap/v1/api.md:227-251.
// These endpoints help CLI clients resolve repo execution identifiers needed to
// pull diffs from the server.
package mods

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
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
	RunID         string `json:"run_id"`
	RepoID        string `json:"repo_id"`
	RepoTargetRef string `json:"repo_target_ref"`
}

// =============================================================================
// Run Pull Resolution Command
// =============================================================================

// RunPullCommand resolves a repo_url to execution identifiers for a specific run.
// Endpoint: POST /v1/runs/{run_id}/pull
//
// Per roadmap/v1/api.md:227-251:
//   - Server matches the repo by joining run_repos to mod_repos by repo_id,
//     filtering by run_id, and comparing normalized repo_url.
//   - If no repo matches: 404 error.
//   - If multiple repos match: 409 error (ambiguous).
type RunPullCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	RunID   string // Run ID (KSUID-backed string)
	RepoURL string // Repository URL to match
}

// Run executes POST /v1/runs/{run_id}/pull with the provided repo_url.
// Returns the PullResolution containing run_id, repo_id, and repo_target_ref.
func (c RunPullCommand) Run(ctx context.Context) (*PullResolution, error) {
	if c.Client == nil {
		return nil, fmt.Errorf("run pull: http client required")
	}
	if c.BaseURL == nil {
		return nil, fmt.Errorf("run pull: base url required")
	}
	runID := strings.TrimSpace(c.RunID)
	if runID == "" {
		return nil, fmt.Errorf("run pull: run id required")
	}
	repoURL := strings.TrimSpace(c.RepoURL)
	if repoURL == "" {
		return nil, fmt.Errorf("run pull: repo url required")
	}

	// Build endpoint: POST /v1/runs/{run_id}/pull
	endpoint := c.BaseURL.JoinPath("/v1/runs", url.PathEscape(runID), "pull")

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
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, decodeHTTPError(resp, "run pull")
	}

	var result PullResolution
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("run pull: decode response: %w", err)
	}

	return &result, nil
}

// =============================================================================
// Mod Pull Resolution Command
// =============================================================================

// PullMode specifies which run to select for mod pull resolution.
// See roadmap/v1/api.md:67-69.
type PullMode string

const (
	// PullModeLastSucceeded selects the newest terminal run with status=Success (default).
	PullModeLastSucceeded PullMode = "last-succeeded"
	// PullModeLastFailed selects the newest terminal run with status=Fail.
	PullModeLastFailed PullMode = "last-failed"
)

// ModPullCommand resolves a repo_url to execution identifiers for a mod.
// Endpoint: POST /v1/mods/{mod_id}/pull
//
// Per roadmap/v1/api.md:57-82:
//   - Server performs the lookup using mod_id + repo_url → mod_repos.id.
//   - Then selects the appropriate run_repos by created_at DESC, filtering by
//     the requested terminal status (Success or Fail).
//   - Mode values:
//   - "last-succeeded" (default): newest run_repos with status=Success
//   - "last-failed": newest run_repos with status=Fail
//   - If no repo matches: 404 error.
//   - If no run with matching status found: 404 error.
type ModPullCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	ModID   string   // Mod ID or name
	RepoURL string   // Repository URL to match
	Mode    PullMode // Pull mode (last-succeeded or last-failed)
}

// Run executes POST /v1/mods/{mod_id}/pull with the provided repo_url and mode.
// Returns the PullResolution containing run_id, repo_id, and repo_target_ref.
func (c ModPullCommand) Run(ctx context.Context) (*PullResolution, error) {
	if c.Client == nil {
		return nil, fmt.Errorf("mod pull: http client required")
	}
	if c.BaseURL == nil {
		return nil, fmt.Errorf("mod pull: base url required")
	}
	modID := strings.TrimSpace(c.ModID)
	if modID == "" {
		return nil, fmt.Errorf("mod pull: mod id required")
	}
	repoURL := strings.TrimSpace(c.RepoURL)
	if repoURL == "" {
		return nil, fmt.Errorf("mod pull: repo url required")
	}

	// Default mode is "last-succeeded" per roadmap/v1/api.md:68.
	mode := c.Mode
	if mode == "" {
		mode = PullModeLastSucceeded
	}

	// Build endpoint: POST /v1/mods/{mod_id}/pull
	endpoint := c.BaseURL.JoinPath("/v1/mods", url.PathEscape(modID), "pull")

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
		return nil, fmt.Errorf("mod pull: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("mod pull: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("mod pull: http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, decodeHTTPError(resp, "mod pull")
	}

	var result PullResolution
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("mod pull: decode response: %w", err)
	}

	return &result, nil
}
