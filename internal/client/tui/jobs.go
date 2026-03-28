package tui

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/iw2rmb/ploy/internal/cli/httpx"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// JobItem represents a single job entry from the list API.
type JobItem struct {
	JobID      domaintypes.JobID     `json:"job_id"`
	Name       string                `json:"name"`
	Status     domaintypes.JobStatus `json:"status"`
	DurationMs int64                 `json:"duration_ms"`
	JobImage   string                `json:"job_image"`
	NodeID     *domaintypes.NodeID   `json:"node_id"`
	MigName    string                `json:"mig_name"`
	RunID      domaintypes.RunID     `json:"run_id"`
	RepoID     domaintypes.RepoID    `json:"repo_id"`
}

// ListJobsResult is the response from GET /v1/jobs.
type ListJobsResult struct {
	Jobs  []JobItem `json:"jobs"`
	Total int64     `json:"total"`
}

// ListJobsCommand fetches a paginated list of jobs with an optional run_id filter.
type ListJobsCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	Limit   int32
	Offset  int32
	RunID   *domaintypes.RunID // Optional: filter jobs to a specific run.
}

// Run executes GET /v1/jobs.
func (c ListJobsCommand) Run(ctx context.Context) (ListJobsResult, error) {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return ListJobsResult{}, fmt.Errorf("list jobs: %w", err)
	}

	endpoint := c.BaseURL.JoinPath("v1", "jobs")
	q := endpoint.Query()
	if c.Limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", c.Limit))
	}
	if c.Offset > 0 {
		q.Set("offset", fmt.Sprintf("%d", c.Offset))
	}
	if c.RunID != nil && !c.RunID.IsZero() {
		q.Set("run_id", c.RunID.String())
	}
	if len(q) > 0 {
		endpoint.RawQuery = q.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return ListJobsResult{}, fmt.Errorf("list jobs: build request: %w", err)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return ListJobsResult{}, fmt.Errorf("list jobs: http request failed: %w", err)
	}
	defer httpx.DrainAndClose(resp)

	if resp.StatusCode != http.StatusOK {
		return ListJobsResult{}, httpx.WrapError("list jobs", resp.Status, resp.Body)
	}

	var result ListJobsResult
	if err := httpx.DecodeResponseJSON(resp.Body, &result, httpx.MaxJSONBodyBytes); err != nil {
		return ListJobsResult{}, fmt.Errorf("list jobs: decode response: %w", err)
	}

	return result, nil
}
