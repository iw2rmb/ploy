package tui

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	"github.com/iw2rmb/ploy/internal/cli/httpx"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// CountMigReposCommand counts the repos in a migration's repo set.
type CountMigReposCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	MigID   domaintypes.MigID
}

// Run executes GET /v1/migs/{mig_id}/repos and returns the total repo count.
func (c CountMigReposCommand) Run(ctx context.Context) (int, error) {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return 0, fmt.Errorf("count mig repos: %w", err)
	}
	if c.MigID.IsZero() {
		return 0, fmt.Errorf("count mig repos: mig id required")
	}

	endpoint := c.BaseURL.JoinPath("v1", "migs", c.MigID.String(), "repos")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return 0, fmt.Errorf("count mig repos: build request: %w", err)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("count mig repos: http request failed: %w", err)
	}
	defer httpx.DrainAndClose(resp)

	if resp.StatusCode != http.StatusOK {
		return 0, httpx.WrapError("count mig repos", resp.Status, resp.Body)
	}

	var result domainapi.MigRepoListResponse
	if err := httpx.DecodeResponseJSON(resp.Body, &result, httpx.MaxJSONBodyBytes); err != nil {
		return 0, fmt.Errorf("count mig repos: decode response: %w", err)
	}

	return len(result.Repos), nil
}

// CountMigRunsCommand counts runs belonging to a specific migration by scanning the runs list.
type CountMigRunsCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	MigID   domaintypes.MigID
}

// Run fetches runs pages and counts those whose MigID matches the configured migration.
func (c CountMigRunsCommand) Run(ctx context.Context) (int, error) {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return 0, fmt.Errorf("count mig runs: %w", err)
	}
	if c.MigID.IsZero() {
		return 0, fmt.Errorf("count mig runs: mig id required")
	}

	const pageSize = int32(100)
	var offset int32
	total := 0

	for {
		page, err := ListRunsCommand{
			Client:  c.Client,
			BaseURL: c.BaseURL,
			Limit:   pageSize,
			Offset:  offset,
		}.Run(ctx)
		if err != nil {
			return 0, fmt.Errorf("count mig runs: %w", err)
		}
		for _, run := range page.Runs {
			if run.MigID == c.MigID {
				total++
			}
		}
		if len(page.Runs) < int(pageSize) {
			break
		}
		offset += pageSize
	}

	return total, nil
}
