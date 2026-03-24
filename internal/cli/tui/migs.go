package tui

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/iw2rmb/ploy/internal/cli/httpx"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// MigItem represents a single migration entry from the list API.
type MigItem struct {
	ID        domaintypes.MigID   `json:"id"`
	Name      string              `json:"name"`
	SpecID    *domaintypes.SpecID `json:"spec_id,omitempty"`
	CreatedBy *string             `json:"created_by,omitempty"`
	Archived  bool                `json:"archived"`
	CreatedAt string              `json:"created_at"`
}

// ListMigsResult is the response from GET /v1/migs.
type ListMigsResult struct {
	Migs []MigItem `json:"migs"`
}

// ListMigsCommand fetches a paginated list of migrations.
type ListMigsCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	Limit   int32
	Offset  int32
}

// Run executes GET /v1/migs.
func (c ListMigsCommand) Run(ctx context.Context) (ListMigsResult, error) {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return ListMigsResult{}, fmt.Errorf("list migs: %w", err)
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
		return ListMigsResult{}, fmt.Errorf("list migs: build request: %w", err)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return ListMigsResult{}, fmt.Errorf("list migs: http request failed: %w", err)
	}
	defer httpx.DrainAndClose(resp)

	if resp.StatusCode != http.StatusOK {
		return ListMigsResult{}, httpx.WrapError("list migs", resp.Status, resp.Body)
	}

	var result ListMigsResult
	if err := httpx.DecodeJSON(resp.Body, &result, httpx.MaxJSONBodyBytes); err != nil {
		return ListMigsResult{}, fmt.Errorf("list migs: decode response: %w", err)
	}

	return result, nil
}
