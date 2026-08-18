package runs

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"

	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/httpx"
)

// ListRunDiffsCommand fetches run-scoped diffs in server-provided order.
type ListRunDiffsCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	RunID   domaintypes.RunID
}

func (c ListRunDiffsCommand) Run(ctx context.Context) ([]domainapi.DiffListItem, error) {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return nil, fmt.Errorf("list run diffs: %w", err)
	}
	if c.RunID.IsZero() {
		return nil, fmt.Errorf("list run diffs: run id required")
	}

	endpoint := c.BaseURL.JoinPath("v1", "runs", c.RunID.String(), "diffs")
	result, err := httpx.DoJSON[domainapi.DiffListResponse](ctx, c.Client, http.MethodGet, endpoint.String(), nil, http.StatusOK, "list run diffs")
	if err != nil {
		return nil, err
	}
	if result.Diffs == nil {
		result.Diffs = []domainapi.DiffListItem{}
	}
	return result.Diffs, nil
}

// DownloadDiffCommand downloads and decompresses one run diff.
type DownloadDiffCommand struct {
	Client      *http.Client
	BaseURL     *url.URL
	RunID       domaintypes.RunID
	DiffID      domaintypes.DiffID
	Accumulated bool
}

func (c DownloadDiffCommand) Run(ctx context.Context) ([]byte, error) {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return nil, fmt.Errorf("download diff: %w", err)
	}
	if c.RunID.IsZero() {
		return nil, fmt.Errorf("download diff: run id required")
	}
	if c.DiffID.IsZero() {
		return nil, fmt.Errorf("download diff: diff id required")
	}

	resp, err := c.download(ctx, "download diff")
	if err != nil {
		return nil, err
	}
	defer httpx.DrainAndClose(resp)

	patch, err := httpx.GunzipToBytes(io.LimitReader(resp.Body, httpx.MaxDownloadBodyBytes), httpx.MaxGunzipOutputBytes)
	if err != nil {
		return nil, fmt.Errorf("download diff: gunzip: %w", err)
	}
	return patch, nil
}

func (c DownloadDiffCommand) download(ctx context.Context, operation string) (*http.Response, error) {
	endpoint := c.BaseURL.JoinPath("v1", "runs", c.RunID.String(), "diffs")
	query := endpoint.Query()
	query.Set("download", "true")
	query.Set("diff_id", c.DiffID.String())
	if c.Accumulated {
		query.Set("accumulated", "true")
	}
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("%s: build request: %w", operation, err)
	}
	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: http request failed: %w", operation, err)
	}
	if resp.StatusCode != http.StatusOK {
		defer httpx.DrainAndClose(resp)
		return nil, httpx.WrapError(operation, resp.Status, resp.Body)
	}
	return resp, nil
}
