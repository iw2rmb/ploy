package common

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
)

// ResolvedRemoteRepo is the control-plane resolution of a repo selector such as
// namespace/repo:branch.
type ResolvedRemoteRepo struct {
	RepoURL   string
	Ref       string
	CommitSHA string
}

func ResolveRemoteRepoSelector(ctx context.Context, base *url.URL, httpClient *http.Client, selector string) (ResolvedRemoteRepo, error) {
	if base == nil {
		return ResolvedRemoteRepo{}, errors.New("repo resolve: base url required")
	}
	if httpClient == nil {
		return ResolvedRemoteRepo{}, errors.New("repo resolve: http client required")
	}

	namespaceRepo, ref := SplitRemoteRepoSelector(selector)
	endpoint := base.JoinPath("v1", "repos", "resolve")
	payload, err := json.Marshal(struct {
		Selector string `json:"selector"`
		Ref      string `json:"ref"`
	}{
		Selector: namespaceRepo,
		Ref:      ref,
	})
	if err != nil {
		return ResolvedRemoteRepo{}, fmt.Errorf("repo resolve: marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return ResolvedRemoteRepo{}, fmt.Errorf("repo resolve: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return ResolvedRemoteRepo{}, fmt.Errorf("repo resolve: http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		var apiErr struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(body, &apiErr); err == nil && strings.TrimSpace(apiErr.Error) != "" {
			return ResolvedRemoteRepo{}, fmt.Errorf("repo resolve: %s", strings.TrimSpace(apiErr.Error))
		}
		return ResolvedRemoteRepo{}, fmt.Errorf("repo resolve: %s", strings.TrimSpace(string(body)))
	}

	var resolved struct {
		RepoURL  string `json:"repo_url"`
		Ref      string `json:"ref"`
		RefIsSHA bool   `json:"ref_is_sha"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&resolved); err != nil {
		return ResolvedRemoteRepo{}, fmt.Errorf("repo resolve: decode response: %w", err)
	}
	repoURL := strings.TrimSpace(resolved.RepoURL)
	if repoURL == "" {
		return ResolvedRemoteRepo{}, errors.New("repo resolve: empty repo_url in response")
	}
	resolvedRef := strings.TrimSpace(resolved.Ref)
	if resolvedRef == "" {
		resolvedRef = ref
	}
	repo := ResolvedRemoteRepo{RepoURL: repoURL, Ref: resolvedRef}
	if resolved.RefIsSHA {
		repo.CommitSHA = resolvedRef
	}
	return repo, nil
}

func SplitRemoteRepoSelector(selector string) (string, string) {
	selector = strings.TrimSpace(selector)
	ref := "master"
	if slash := strings.Index(selector, "/"); slash >= 0 {
		if colon := strings.Index(selector[slash+1:], ":"); colon >= 0 {
			idx := slash + 1 + colon
			ref = strings.TrimSpace(selector[idx+1:])
			selector = selector[:idx]
		}
	}
	if ref == "" {
		ref = "master"
	}
	return selector, ref
}
