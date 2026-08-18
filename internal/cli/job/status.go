package job

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/iw2rmb/ploy/internal/cli/common"
	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/httpx"
)

type StatusOptions struct {
	JobID  string
	Output io.Writer
}

func RunStatus(ctx context.Context, opts StatusOptions) error {
	jobID := strings.TrimSpace(opts.JobID)
	if jobID == "" {
		return errors.New("job id required")
	}
	out := opts.Output
	if out == nil {
		out = io.Discard
	}

	base, httpClient, err := common.ResolveControlPlaneHTTP(ctx)
	if err != nil {
		return err
	}
	result, err := GetStatusCommand{
		Client:  httpClient,
		BaseURL: base,
		JobID:   domaintypes.JobID(jobID),
	}.Run(ctx)
	if err != nil {
		return err
	}

	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

type GetStatusCommand struct {
	Client  *http.Client
	BaseURL *url.URL
	JobID   domaintypes.JobID
}

func (c GetStatusCommand) Run(ctx context.Context) (domainapi.JobStatusResponse, error) {
	if err := httpx.RequireClientAndURL(c.Client, c.BaseURL); err != nil {
		return domainapi.JobStatusResponse{}, fmt.Errorf("job status: %w", err)
	}
	if c.JobID.IsZero() {
		return domainapi.JobStatusResponse{}, errors.New("job status: job id required")
	}

	endpoint := c.BaseURL.JoinPath("v1", "jobs", c.JobID.String(), "status")
	return httpx.DoJSON[domainapi.JobStatusResponse](ctx, c.Client, http.MethodGet, endpoint.String(), nil, http.StatusOK, "job status")
}
