package tui

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	"github.com/iw2rmb/ploy/internal/cli/httpx"
)

// ListMigsCommand fetches a paginated list of migrations.
type ListMigsCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	Limit   int32
	Offset  int32
}

// Run executes GET /v1/migs.
func (c ListMigsCommand) Run(ctx context.Context) (domainapi.MigListResponse, error) {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return domainapi.MigListResponse{}, fmt.Errorf("list migs: %w", err)
	}

	endpoint := c.BaseURL.JoinPath("v1", "migs")
	q := endpoint.Query()
	if c.Limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", c.Limit))
	}
	if c.Offset > 0 {
		q.Set("offset", fmt.Sprintf("%d", c.Offset))
	}
	if len(q) > 0 {
		endpoint.RawQuery = q.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return domainapi.MigListResponse{}, fmt.Errorf("list migs: build request: %w", err)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return domainapi.MigListResponse{}, fmt.Errorf("list migs: http request failed: %w", err)
	}
	defer httpx.DrainAndClose(resp)

	if resp.StatusCode != http.StatusOK {
		return domainapi.MigListResponse{}, httpx.WrapError("list migs", resp.Status, resp.Body)
	}

	var result domainapi.MigListResponse
	if err := httpx.DecodeResponseJSON(resp.Body, &result, httpx.MaxJSONBodyBytes); err != nil {
		return domainapi.MigListResponse{}, fmt.Errorf("list migs: decode response: %w", err)
	}

	return result, nil
}
