package common

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/iw2rmb/ploy/internal/httpx"
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
	request := struct {
		Selector string `json:"selector"`
		Ref      string `json:"ref"`
	}{
		Selector: namespaceRepo,
		Ref:      ref,
	}

	type resolveResponse struct {
		RepoURL  string `json:"repo_url"`
		Ref      string `json:"ref"`
		RefIsSHA bool   `json:"ref_is_sha"`
	}
	resolved, err := httpx.DoJSON[resolveResponse](ctx, httpClient, http.MethodPost, endpoint.String(), request, http.StatusOK, "repo resolve")
	if err != nil {
		return ResolvedRemoteRepo{}, err
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
