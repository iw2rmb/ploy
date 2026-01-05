// repos.go provides CLI client implementations for repository-centric API operations.
//
// This file implements the client for fetching run history by repository URL, which
// is used by `ploy mod run pull` to resolve <run-id> to a specific run.
//
// v1: repo_id is the mod_repos.id (NanoID string), not a URL-encoded repo_url.
// The CLI resolves repo_url → repo_id via GET /v1/repos?contains=... first.
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

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/vcs"
)

// RepoRunSummary represents a run for a specific repository from the control-plane.
// It mirrors the server's handlers.RepoRunSummary type for CLI consumption.
// Used by `ploy mod run pull` to resolve run identifiers for a given repository.
type RepoRunSummary struct {
	// RunID is the parent batch run ID (KSUID-backed string).
	RunID domaintypes.RunID `json:"run_id"`

	// ModID is the mod (project) identifier for the run.
	ModID string `json:"mod_id"`

	// RunStatus is the parent run status ("Started", "Finished", "Cancelled").
	RunStatus string `json:"run_status"`

	// RepoStatus is the per-repo status within the run ("Queued", "Running", "Success", "Fail", "Cancelled").
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
}

// ListRunsForRepoCommand fetches runs for a specific repository URL.
// Used by `ploy mod run pull` to locate runs associated with the current git remote.
type ListRunsForRepoCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	RepoURL string // Repository URL (used to resolve repo_id)
	Limit   int32  // Max results to return (default 100)
	Offset  int32  // Number of results to skip
}

// Run executes GET /v1/repos/{repo_id}/runs to list runs for the repository.
// v1: repo_id is resolved via GET /v1/repos?contains=... (repo_url substring match).
func (c ListRunsForRepoCommand) Run(ctx context.Context) ([]RepoRunSummary, error) {
	if c.Client == nil {
		return nil, fmt.Errorf("list runs for repo: http client required")
	}
	if c.BaseURL == nil {
		return nil, fmt.Errorf("list runs for repo: base url required")
	}
	repoURL := strings.TrimSpace(c.RepoURL)
	if repoURL == "" {
		return nil, fmt.Errorf("list runs for repo: repo url required")
	}
	if err := domaintypes.RepoURL(repoURL).Validate(); err != nil {
		return nil, fmt.Errorf("list runs for repo: repo_url: %w", err)
	}

	normalized := vcs.NormalizeRepoURL(repoURL)

	repoID, err := resolveRepoID(ctx, c.Client, c.BaseURL, normalized)
	if err != nil {
		return nil, err
	}

	// Build endpoint: /v1/repos/{repo_id}/runs
	endpoint := c.BaseURL.JoinPath("/v1/repos", url.PathEscape(repoID), "runs")

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

type repoSummary struct {
	RepoID  string `json:"repo_id"`
	RepoURL string `json:"repo_url"`
}

func resolveRepoID(ctx context.Context, client *http.Client, baseURL *url.URL, normalizedRepoURL string) (string, error) {
	endpoint := baseURL.JoinPath("/v1/repos")
	q := endpoint.Query()
	q.Set("contains", normalizedRepoURL)
	endpoint.RawQuery = q.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return "", fmt.Errorf("list runs for repo: build repos request: %w", err)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("list runs for repo: http repos request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", decodeHTTPError(resp, "list runs for repo")
	}

	var result struct {
		Repos []repoSummary `json:"repos"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("list runs for repo: decode repos response: %w", err)
	}

	for _, repo := range result.Repos {
		if vcs.NormalizeRepoURL(repo.RepoURL) == normalizedRepoURL {
			return repo.RepoID, nil
		}
	}

	return "", fmt.Errorf("list runs for repo: repo not found")
}

// ResolveRunForRepo finds a run by <run-id> within the list of runs for a repo.
// v1: runs are resolved by RunID only (runs no longer have a name).
//
// Parameters:
//   - runs: list of RepoRunSummary from ListRunsForRepoCommand
//   - runNameOrID: the <run-id> argument from CLI
//
// Returns the matched run summary, or nil if no match found.
func ResolveRunForRepo(runs []RepoRunSummary, runNameOrID string) *RepoRunSummary {
	if len(runs) == 0 {
		return nil
	}
	needle := strings.TrimSpace(runNameOrID)
	if needle == "" {
		return nil
	}

	for i := range runs {
		if runs[i].RunID.String() == needle {
			return &runs[i]
		}
	}

	return nil
}
