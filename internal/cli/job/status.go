package job

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/common"
	"github.com/iw2rmb/ploy/internal/cli/httpx"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

type StatusOptions struct {
	JobID  string
	Output io.Writer
}

type StatusResult struct {
	JobID       domaintypes.JobID     `json:"job_id"`
	RunID       domaintypes.RunID     `json:"run_id"`
	RepoID      domaintypes.RepoID    `json:"repo_id"`
	Attempt     int32                 `json:"attempt"`
	Name        string                `json:"name"`
	JobType     domaintypes.JobType   `json:"job_type"`
	Status      domaintypes.JobStatus `json:"status"`
	JobImage    string                `json:"job_image"`
	NodeID      *domaintypes.NodeID   `json:"node_id"`
	ExitCode    *int32                `json:"exit_code"`
	StartedAt   *time.Time            `json:"started_at"`
	FinishedAt  *time.Time            `json:"finished_at"`
	DurationMs  int64                 `json:"duration_ms"`
	RepoShaIn   string                `json:"repo_sha_in"`
	RepoShaOut  string                `json:"repo_sha_out"`
	RepoShaIn8  string                `json:"repo_sha_in8"`
	RepoShaOut8 string                `json:"repo_sha_out8"`
}

func RunStatus(ctx context.Context, opts StatusOptions) error {
	jobID := strings.TrimSpace(opts.JobID)
	if jobID == "" {
		return errors.New("job id required")
	}
	out := opts.Output
	if out == nil {
		out = io.Discard
	}

	base, httpClient, err := common.ResolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}
	result, err := GetStatusCommand{
		Client:  httpClient,
		BaseURL: base,
		JobID:   domaintypes.JobID(jobID),
	}.Run(ctx)
	if err != nil {
		return err
	}

	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

type GetStatusCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	JobID   domaintypes.JobID
}

func (c GetStatusCommand) Run(ctx context.Context) (StatusResult, error) {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return StatusResult{}, fmt.Errorf("job status: %w", err)
	}
	if c.JobID.IsZero() {
		return StatusResult{}, errors.New("job status: job id required")
	}

	endpoint := c.BaseURL.JoinPath("v1", "jobs", c.JobID.String(), "status")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return StatusResult{}, fmt.Errorf("job status: build request: %w", err)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return StatusResult{}, fmt.Errorf("job status: http request failed: %w", err)
	}
	defer httpx.DrainAndClose(resp)

	if resp.StatusCode != http.StatusOK {
		return StatusResult{}, httpx.WrapError("job status", resp.Status, resp.Body)
	}

	var result StatusResult
	if err := httpx.DecodeResponseJSON(resp.Body, &result, httpx.MaxJSONBodyBytes); err != nil {
		return StatusResult{}, fmt.Errorf("job status: decode response: %w", err)
	}

	return result, nil
}
