// repos.go provides CLI client implementations for repository-centric API operations.
//
// This file implements the client for fetching run history by repository URL, which
// is used by `ploy mod run pull` to resolve <run-name|run-id> to a specific execution.
// The client calls GET /v1/repos/{repo_id}/runs where repo_id is the URL-encoded
// repository URL.
//
// The RepoRunSummary type mirrors the server's handlers.RepoRunSummary for CLI consumption.
package mods

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// RepoRunSummary represents a run for a specific repository from the control-plane.
// It mirrors the server's handlers.RepoRunSummary type for CLI consumption.
// Used by `ploy mod run pull` to resolve run identifiers for a given repository.
type RepoRunSummary struct {
	// RunID is the parent batch run ID (KSUID-backed string).
	RunID string `json:"run_id"`

	// Name is the optional human-readable batch name.
	Name *string `json:"name,omitempty"`

	// RunStatus is the parent run status (queued, running, succeeded, failed, canceled).
	RunStatus string `json:"run_status"`

	// RepoStatus is the per-repo status within the batch (pending, running, succeeded, etc.).
	RepoStatus string `json:"repo_status"`

	// BaseRef is the Git base ref for the run.
	BaseRef string `json:"base_ref"`

	// TargetRef is the Git target ref for the run.
	TargetRef string `json:"target_ref"`

	// Attempt is the retry attempt number for this repo in the batch.
	Attempt int32 `json:"attempt"`

	// StartedAt is the timestamp when execution started (nullable).
	StartedAt *time.Time `json:"started_at,omitempty"`

	// FinishedAt is the timestamp when execution completed (nullable).
	FinishedAt *time.Time `json:"finished_at,omitempty"`

	// ExecutionRunID is the child execution run ID (KSUID-backed string) for this repo
	// within the batch. It links to the Mods run created to process this specific repo.
	// Used to fetch diffs and status for the repo execution. Null when execution has
	// not started or no child run was created.
	ExecutionRunID *string `json:"execution_run_id,omitempty"`
}

// ListRunsForRepoCommand fetches runs for a specific repository URL.
// Used by `ploy mod run pull` to locate runs associated with the current git remote.
type ListRunsForRepoCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	RepoURL string // Repository URL (will be URL-encoded for the path segment)
	Limit   int32  // Max results to return (default 100)
	Offset  int32  // Number of results to skip
}

// Run executes GET /v1/repos/{repo_id}/runs to list runs for the repository.
// The repo_id path parameter is the URL-encoded repository URL.
func (c ListRunsForRepoCommand) Run(ctx context.Context) ([]RepoRunSummary, error) {
	if c.Client == nil {
		return nil, fmt.Errorf("list runs for repo: http client required")
	}
	if c.BaseURL == nil {
		return nil, fmt.Errorf("list runs for repo: base url required")
	}
	if strings.TrimSpace(c.RepoURL) == "" {
		return nil, fmt.Errorf("list runs for repo: repo url required")
	}

	// URL-encode the repo URL for the path segment per ROADMAP.md specification.
	// The server expects the raw repo URL as the repo_id, URL-encoded in the path.
	encodedRepoURL := url.PathEscape(c.RepoURL)

	// Build endpoint: /v1/repos/{repo_id}/runs
	endpoint := c.BaseURL.JoinPath("/v1/repos", encodedRepoURL, "runs")

	// Add query parameters for pagination.
	q := endpoint.Query()
	limit := c.Limit
	if limit <= 0 {
		limit = 100 // Default limit per ROADMAP.md specification.
	}
	q.Set("limit", fmt.Sprintf("%d", limit))
	if c.Offset > 0 {
		q.Set("offset", fmt.Sprintf("%d", c.Offset))
	}
	endpoint.RawQuery = q.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("list runs for repo: build request: %w", err)
	}

	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("list runs for repo: http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, decodeHTTPError(resp, "list runs for repo")
	}

	// Response structure: {"runs": [...]}
	var result struct {
		Runs []RepoRunSummary `json:"runs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("list runs for repo: decode response: %w", err)
	}

	return result.Runs, nil
}

// ResolveRunForRepo finds a run by <run-name|run-id> within the list of runs for a repo.
// Implements the resolution rules from ROADMAP.md:
//  1. First, try to match by RunID (exact string equality).
//  2. If no RunID match, match against Name (after trimming spaces).
//  3. Filter only runs whose RepoStatus indicates execution ran (succeeded, failed, skipped).
//  4. If multiple results match the same name, select the first entry (API returns DESC by created_at).
//  5. If no match found, return nil.
//
// Parameters:
//   - runs: list of RepoRunSummary from ListRunsForRepoCommand
//   - runNameOrID: the <run-name|run-id> argument from CLI
//
// Returns the matched run summary, or nil if no match found.
func ResolveRunForRepo(runs []RepoRunSummary, runNameOrID string) *RepoRunSummary {
	trimmed := strings.TrimSpace(runNameOrID)
	if trimmed == "" {
		return nil
	}

	// Terminal repo statuses that indicate execution actually ran (diffs may exist).
	// Per ROADMAP.md: filter only runs whose RepoStatus indicates execution ran.
	terminalStatuses := map[string]bool{
		"succeeded": true,
		"failed":    true,
		"skipped":   true,
	}

	// First pass: try to match by RunID (exact string equality).
	// RunID match takes precedence over Name match.
	for i := range runs {
		run := &runs[i]
		if run.RunID == trimmed && terminalStatuses[run.RepoStatus] {
			return run
		}
	}

	// Second pass: try to match by Name.
	// Select the first matching entry (API returns DESC by created_at).
	for i := range runs {
		run := &runs[i]
		if !terminalStatuses[run.RepoStatus] {
			continue
		}
		if run.Name != nil && strings.TrimSpace(*run.Name) == trimmed {
			return run
		}
	}

	return nil
}
