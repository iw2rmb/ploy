// Package client owns shared control-plane API commands.
package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/httpx"
)

// ListMigsCommand lists migration projects with optional filters.
type ListMigsCommand struct {
	Client        *http.Client
	BaseURL       *url.URL
	Limit         int32
	Offset        int32
	NameSubstring *string
	Archived      *bool
	RepoURL       *string
}

// Run executes GET /v1/migs.
func (c ListMigsCommand) Run(ctx context.Context) ([]domainapi.MigSummary, error) {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return nil, fmt.Errorf("mig list: %w", err)
	}

	endpoint := c.BaseURL.JoinPath("v1", "migs")
	q := endpoint.Query()
	if c.Limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", c.Limit))
	}
	if c.Offset > 0 {
		q.Set("offset", fmt.Sprintf("%d", c.Offset))
	}
	if c.NameSubstring != nil && *c.NameSubstring != "" {
		q.Set("name_substring", *c.NameSubstring)
	}
	if c.Archived != nil {
		q.Set("archived", fmt.Sprintf("%t", *c.Archived))
	}
	if c.RepoURL != nil && *c.RepoURL != "" {
		q.Set("repo_url", *c.RepoURL)
	}
	endpoint.RawQuery = q.Encode()

	result, err := httpx.DoJSON[domainapi.MigListResponse](ctx, c.Client, http.MethodGet, endpoint.String(), nil, http.StatusOK, "mig list")
	if err != nil {
		return nil, err
	}
	return result.Migs, nil
}

// ListRunsCommand lists runs with optional filters.
type ListRunsCommand struct {
	Client    *http.Client
	BaseURL   *url.URL
	Limit     int32
	Offset    int32
	RepoURL   string
	CreatedBy string
	All       bool
}

// Run executes GET /v1/runs.
func (c ListRunsCommand) Run(ctx context.Context) ([]domaintypes.RunSummary, error) {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return nil, fmt.Errorf("run list: %w", err)
	}

	endpoint := c.BaseURL.JoinPath("v1", "runs")
	q := endpoint.Query()
	if c.Limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", c.Limit))
	}
	if c.Offset > 0 {
		q.Set("offset", fmt.Sprintf("%d", c.Offset))
	}
	if repoURL := strings.TrimSpace(c.RepoURL); repoURL != "" {
		q.Set("repo_url", repoURL)
	}
	if createdBy := strings.TrimSpace(c.CreatedBy); createdBy != "" {
		q.Set("created_by", createdBy)
	}
	if c.All {
		q.Set("all", "true")
	}
	endpoint.RawQuery = q.Encode()

	result, err := httpx.DoJSON[struct {
		Runs []domaintypes.RunSummary `json:"runs"`
	}](ctx, c.Client, http.MethodGet, endpoint.String(), nil, http.StatusOK, "run list")
	if err != nil {
		return nil, err
	}
	return result.Runs, nil
}
