package runs

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/httpx"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// RepoJobEntry represents a job within a repo execution.
type RepoJobEntry struct {
	JobID       domaintypes.JobID   `json:"job_id"`
	Name        string              `json:"name"`
	JobType     string              `json:"job_type"`
	JobImage    string              `json:"job_image"`
	NextID      *domaintypes.JobID  `json:"next_id"`
	NodeID      *domaintypes.NodeID `json:"node_id"`
	Status      store.JobStatus     `json:"status"`
	StartedAt   *time.Time          `json:"started_at,omitempty"`
	FinishedAt  *time.Time          `json:"finished_at,omitempty"`
	DurationMs  int64               `json:"duration_ms"`
	DisplayName string              `json:"display_name,omitempty"`
}

// ListRepoJobsResult contains the response from listing repo jobs.
type ListRepoJobsResult struct {
	RunID   domaintypes.RunID     `json:"run_id"`
	RepoID  domaintypes.MigRepoID `json:"repo_id"`
	Attempt int32                 `json:"attempt"`
	Jobs    []RepoJobEntry        `json:"jobs"`
}

// ListRepoJobsCommand fetches jobs for a repo execution.
type ListRepoJobsCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	RunID   domaintypes.RunID
	RepoID  domaintypes.MigRepoID
	Attempt *int32 // Optional: specific attempt
}

// Run executes GET /v1/runs/{run_id}/repos/{repo_id}/jobs.
func (c ListRepoJobsCommand) Run(ctx context.Context) (ListRepoJobsResult, error) {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return ListRepoJobsResult{}, fmt.Errorf("list repo jobs: %w", err)
	}
	if c.RunID.IsZero() {
		return ListRepoJobsResult{}, fmt.Errorf("list repo jobs: run id required")
	}
	if c.RepoID.IsZero() {
		return ListRepoJobsResult{}, fmt.Errorf("list repo jobs: repo id required")
	}

	endpoint := c.BaseURL.JoinPath("v1", "runs", c.RunID.String(), "repos", c.RepoID.String(), "jobs")
	if c.Attempt != nil {
		q := endpoint.Query()
		q.Set("attempt", fmt.Sprintf("%d", *c.Attempt))
		endpoint.RawQuery = q.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return ListRepoJobsResult{}, fmt.Errorf("list repo jobs: build request: %w", err)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return ListRepoJobsResult{}, fmt.Errorf("list repo jobs: http request failed: %w", err)
	}
	defer httpx.DrainAndClose(resp)

	if resp.StatusCode != http.StatusOK {
		return ListRepoJobsResult{}, httpx.WrapError("list repo jobs", resp.Status, resp.Body)
	}

	var result ListRepoJobsResult
	if err := httpx.DecodeJSON(resp.Body, &result, httpx.MaxJSONBodyBytes); err != nil {
		return ListRepoJobsResult{}, fmt.Errorf("list repo jobs: decode response: %w", err)
	}

	return result, nil
}
