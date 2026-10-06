package runs

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/httpx"
)

// RestartCommand requests a new attempt for a terminal run.
type RestartCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	RunID   domaintypes.RunID

	GitLabToken string
	FromFailed  bool
}

func (c RestartCommand) Run(ctx context.Context) (domaintypes.RunSummary, error) {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return domaintypes.RunSummary{}, fmt.Errorf("runs restart: %w", err)
	}
	if c.RunID.IsZero() {
		return domaintypes.RunSummary{}, errors.New("runs restart: run id required")
	}
	var body any
	req := domainapi.RunRestartRequest{FromFailed: c.FromFailed}
	if strings.TrimSpace(c.GitLabToken) != "" {
		req.GitLabToken = &c.GitLabToken
	}
	if req.FromFailed || req.GitLabToken != nil {
		body = req
	}
	endpoint := c.BaseURL.JoinPath("v1", "runs", c.RunID.String(), "restart")
	return httpx.DoJSON[domaintypes.RunSummary](ctx, c.Client, http.MethodPost, endpoint.String(), body, http.StatusOK, "runs restart")
}
