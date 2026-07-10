package handlers

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/iw2rmb/ploy/internal/gitauth"
	"github.com/iw2rmb/ploy/internal/gitlabtoken"
	"github.com/iw2rmb/ploy/internal/server/gitlabtokens"
)

func optionalGitLabTokenRegistry(registries []*gitlabtokens.Registry) *gitlabtokens.Registry {
	if len(registries) == 0 {
		return nil
	}
	return registries[0]
}

func validateGitLabTokenRequest(token *string) (string, string, error) {
	if token == nil {
		return "", "", nil
	}
	return gitlabtoken.ValidateRequest(*token)
}

func validateGitLabTokenRequestForRepos(token *string, configuredDomain string, repoURLs []string) (string, string, error) {
	hash, trimmedToken, err := validateGitLabTokenRequest(token)
	if err != nil || hash == "" {
		return hash, trimmedToken, err
	}
	configuredHost := gitauth.NormalizeGitLabDomainHost(configuredDomain)
	if configuredHost == "" {
		return "", "", fmt.Errorf("ephemeral GitLab token requires a configured GitLab domain")
	}
	for _, repoURL := range repoURLs {
		repoScheme, repoHost := schemeAndHostForRepoURL(repoURL)
		if repoScheme != "https" {
			return "", "", fmt.Errorf("ephemeral GitLab token is only allowed for https repos on configured GitLab domain %s, got %s", configuredHost, schemeHostForError(repoScheme, repoHost))
		}
		if !strings.EqualFold(repoHost, configuredHost) {
			return "", "", fmt.Errorf("ephemeral GitLab token is only allowed for repos on configured GitLab domain %s, got %s", configuredHost, repoHost)
		}
	}
	return hash, trimmedToken, nil
}

func gitAuthWithEphemeralToken(base gitauth.Options, token string) gitauth.Options {
	token = strings.TrimSpace(token)
	if token == "" {
		return base
	}
	auth := base
	auth.GitLabPAT = token
	return auth
}

func schemeAndHostForRepoURL(raw string) (string, string) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", ""
	}
	return strings.ToLower(strings.TrimSpace(parsed.Scheme)), strings.ToLower(strings.TrimSpace(parsed.Hostname()))
}

func schemeHostForError(scheme, host string) string {
	if scheme == "" {
		return host
	}
	return scheme + "://" + host
}
