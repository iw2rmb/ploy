package runs

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/iw2rmb/ploy/internal/cli/httpx"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	migsapi "github.com/iw2rmb/ploy/internal/migs/api"
	"github.com/iw2rmb/ploy/internal/workflow/jobchain"
)

type RepoJobEntry = migsapi.RunRepoJob

// ListRepoJobsResult contains the response from listing repo jobs.
type ListRepoJobsResult = migsapi.ListRunRepoJobsResponse

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
	if err := httpx.DecodeResponseJSON(resp.Body, &result, httpx.MaxJSONBodyBytes); err != nil {
		return ListRepoJobsResult{}, fmt.Errorf("list repo jobs: decode response: %w", err)
	}

	result.Jobs = orderRepoJobsByChain(result.Jobs)

	return result, nil
}

// orderRepoJobsByChain reconstructs execution order from linked next_id pointers.
// Head jobs are derived as jobs that have no predecessor in the same payload.
func orderRepoJobsByChain(jobs []RepoJobEntry) []RepoJobEntry {
	return jobchain.Order(
		jobs,
		func(job RepoJobEntry) domaintypes.JobID { return job.JobID },
		func(job RepoJobEntry) *domaintypes.JobID { return job.NextID },
	)
}
