// Package migs provides CLI client implementations for Migs operations.
// This file implements the run lifecycle client for create/list/stop/status
// operations against the control-plane /v1/runs endpoints.
//
// RunClient encapsulates HTTP calls to the control-plane and maps responses
// to domain types for CLI consumption. It follows the same Command pattern
// used by SubmitCommand and InspectCommand.
package migs

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/iw2rmb/ploy/internal/cli/httpx"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// ListRunsCommand lists runs from the control plane.
type ListRunsCommand struct {
	Client    *http.Client
	BaseURL   *url.URL
	Limit     int32 // Max results to return (default 50, max 100).
	Offset    int32 // Number of results to skip.
	RepoURL   string
	CreatedBy string
	All       bool
}

// Run executes GET /v1/runs to list runs with pagination.
func (c ListRunsCommand) Run(ctx context.Context) ([]domaintypes.RunSummary, error) {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return nil, fmt.Errorf("run list: %w", err)
	}

	// Build endpoint with query params.
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

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("run list: build request: %w", err)
	}

	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("run list: http request failed: %w", err)
	}
	defer httpx.DrainAndClose(resp)

	if resp.StatusCode != http.StatusOK {
		return nil, httpx.WrapError("run list", resp.Status, resp.Body)
	}

	// Response structure: {"runs": [...]}
	var result struct {
		Runs []domaintypes.RunSummary `json:"runs"`
	}
	if err := httpx.DecodeResponseJSON(resp.Body, &result, httpx.MaxJSONBodyBytes); err != nil {
		return nil, fmt.Errorf("run list: decode response: %w", err)
	}

	return result.Runs, nil
}
