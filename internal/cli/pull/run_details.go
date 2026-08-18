package pull

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/httpx"
)

type runDetails struct {
	RepoID          domaintypes.RepoID    `json:"repo_id"`
	BaseRef         string                `json:"base_ref"`
	SourceCommitSHA string                `json:"source_commit_sha,omitempty"`
	Status          domaintypes.RunStatus `json:"status"`
}

func fetchRunDetails(ctx context.Context, httpClient *http.Client, baseURL *url.URL, runID domaintypes.RunID) (*runDetails, error) {
	if baseURL == nil {
		return nil, errors.New("base url required")
	}

	endpoint := baseURL.JoinPath("v1", "runs", runID.String())
	result, err := httpx.DoJSON[domaintypes.RunSummary](ctx, httpClient, http.MethodGet, endpoint.String(), nil, http.StatusOK, "fetch run details")
	if err != nil {
		return nil, err
	}
	return &runDetails{
		RepoID:          result.RepoID,
		BaseRef:         result.BaseRef,
		SourceCommitSHA: result.SourceCommitSHA,
		Status:          result.Status,
	}, nil
}
