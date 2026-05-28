package pull

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

type runRepoDetails struct {
	RepoID          domaintypes.MigRepoID `json:"repo_id"`
	BaseRef         string                `json:"base_ref"`
	SourceCommitSHA string                `json:"source_commit_sha,omitempty"`
	Status          string                `json:"status"`
}

func fetchRunRepoDetails(ctx context.Context, httpClient *http.Client, baseURL *url.URL, runID domaintypes.RunID, repoID domaintypes.MigRepoID) (*runRepoDetails, error) {
	if baseURL == nil {
		return nil, errors.New("base url required")
	}

	endpoint := baseURL.JoinPath("v1", "runs", runID.String(), "repos")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var result struct {
		Repos []runRepoDetails `json:"repos"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	for _, repo := range result.Repos {
		if repo.RepoID == repoID {
			return &repo, nil
		}
	}
	return nil, fmt.Errorf("repo %s not found in run %s", repoID.String(), runID.String())
}
