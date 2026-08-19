package tui

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	sharedclient "github.com/iw2rmb/ploy/internal/client"
	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/httpx"
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
	result, err := httpx.DoJSON[domainapi.MigRepoListResponse](ctx, c.Client, http.MethodGet, endpoint.String(), nil, http.StatusOK, "count mig repos")
	if err != nil {
		return 0, err
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
		page, err := sharedclient.ListRunsCommand{
			Client:  c.Client,
			BaseURL: c.BaseURL,
			Limit:   pageSize,
			Offset:  offset,
		}.Run(ctx)
		if err != nil {
			return 0, fmt.Errorf("count mig runs: %w", err)
		}
		for _, run := range page {
			if run.MigID == c.MigID {
				total++
			}
		}
		if len(page) < int(pageSize) {
			break
		}
		offset += pageSize
	}

	return total, nil
}
