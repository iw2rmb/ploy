package runs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/iw2rmb/ploy/internal/cli/httpx"
	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// RestartCommand requests a new attempt for a terminal run.
type RestartCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	RunID   domaintypes.RunID

	GitLabToken string
}

func (c RestartCommand) Run(ctx context.Context) (domaintypes.RunSummary, error) {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return domaintypes.RunSummary{}, fmt.Errorf("runs restart: %w", err)
	}
	if c.RunID.IsZero() {
		return domaintypes.RunSummary{}, errors.New("runs restart: run id required")
	}
	var body io.Reader
	if strings.TrimSpace(c.GitLabToken) != "" {
		reqBody := domainapi.RunRestartRequest{GitLabToken: &c.GitLabToken}
		payload, err := json.Marshal(reqBody)
		if err != nil {
			return domaintypes.RunSummary{}, fmt.Errorf("runs restart: marshal request: %w", err)
		}
		body = bytes.NewReader(payload)
	}
	endpoint := c.BaseURL.JoinPath("v1", "runs", c.RunID.String(), "restart")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), body)
	if err != nil {
		return domaintypes.RunSummary{}, fmt.Errorf("runs restart: build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.Client.Do(req)
	if err != nil {
		return domaintypes.RunSummary{}, fmt.Errorf("runs restart: http request failed: %w", err)
	}
	defer httpx.DrainAndClose(resp)
	if resp.StatusCode != http.StatusOK {
		return domaintypes.RunSummary{}, httpx.WrapError("runs restart", resp.Status, resp.Body)
	}
	var out domaintypes.RunSummary
	if err := httpx.DecodeResponseJSON(resp.Body, &out, httpx.MaxJSONBodyBytes); err != nil {
		return domaintypes.RunSummary{}, fmt.Errorf("runs restart: decode response: %w", err)
	}
	return out, nil
}
