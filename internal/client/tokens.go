package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	"github.com/iw2rmb/ploy/internal/httpx"
)

// ErrTokenListForbidden lets callers distinguish unavailable admin controls from request failures.
var ErrTokenListForbidden = errors.New("list tokens: forbidden")

type ListTokensCommand struct {
	Client  *http.Client
	BaseURL *url.URL
}

func (c ListTokensCommand) Run(ctx context.Context) (domainapi.APITokenListResponse, error) {
	var result domainapi.APITokenListResponse
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return result, fmt.Errorf("list tokens: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL.JoinPath("v1", "tokens").String(), nil)
	if err != nil {
		return result, fmt.Errorf("list tokens: %w", err)
	}
	resp, err := c.Client.Do(req)
	if err != nil {
		return result, fmt.Errorf("list tokens: %w", err)
	}
	defer httpx.DrainAndClose(resp)
	if resp.StatusCode == http.StatusForbidden {
		return result, ErrTokenListForbidden
	}
	if err := httpx.CheckStatus(resp, http.StatusOK, "list tokens"); err != nil {
		return result, err
	}
	if err := httpx.DecodeResponseJSON(resp.Body, &result, httpx.MaxJSONBodyBytes); err != nil {
		return result, fmt.Errorf("list tokens: %w", err)
	}
	return result, nil
}
