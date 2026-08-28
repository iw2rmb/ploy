package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/httpx"
)

type ListNodesCommand struct {
	Client  *http.Client
	BaseURL *url.URL
}

func (c ListNodesCommand) Run(ctx context.Context) ([]domainapi.Node, error) {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}
	endpoint := c.BaseURL.JoinPath("v1", "nodes")
	return httpx.DoJSON[[]domainapi.Node](ctx, c.Client, http.MethodGet, endpoint.String(), nil, http.StatusOK, "list nodes")
}

type ListNodeDiagnosticsCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	NodeID  domaintypes.NodeID
}

func (c ListNodeDiagnosticsCommand) Run(ctx context.Context) ([]domainapi.NodeDiagnostic, error) {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return nil, fmt.Errorf("list node diagnostics: %w", err)
	}
	if c.NodeID.IsZero() {
		return nil, fmt.Errorf("list node diagnostics: node id required")
	}
	endpoint := c.BaseURL.JoinPath("v1", "nodes", c.NodeID.String(), "diagnostics")
	return httpx.DoJSON[[]domainapi.NodeDiagnostic](ctx, c.Client, http.MethodGet, endpoint.String(), nil, http.StatusOK, "list node diagnostics")
}
