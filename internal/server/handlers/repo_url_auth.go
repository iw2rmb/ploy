package handlers

import (
	"net"
	"net/url"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func repoURLContainsCredentials(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return false
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return false
	}
	return parsed.User != nil
}

func repoURLWithGitLabPATFromSpec(repoURL string, specJSON []byte) string {
	spec, err := contracts.ParseMigSpecJSON(specJSON)
	if err != nil || spec == nil {
		return repoURL
	}
	return repoURLWithGitLabPAT(repoURL, spec.GitLabDomain, spec.GitLabPAT)
}

func repoURLWithGitLabPAT(repoURL, gitlabDomain, gitlabPAT string) string {
	pat := strings.TrimSpace(gitlabPAT)
	if pat == "" {
		return repoURL
	}

	trimmedURL := strings.TrimSpace(repoURL)
	if trimmedURL == "" {
		return repoURL
	}

	parsed, err := url.Parse(trimmedURL)
	if err != nil {
		return repoURL
	}
	if parsed.User != nil {
		return repoURL
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "https" && scheme != "http" {
		return repoURL
	}

	domainHost := normalizeGitLabDomainHost(gitlabDomain)
	if domainHost != "" && !strings.EqualFold(parsed.Hostname(), domainHost) {
		return repoURL
	}

	parsed.User = url.UserPassword("oauth2", pat)
	return parsed.String()
}

func normalizeGitLabDomainHost(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	trimmed = strings.TrimSuffix(trimmed, "/")

	if strings.Contains(trimmed, "://") {
		parsed, err := url.Parse(trimmed)
		if err == nil {
			return strings.ToLower(strings.TrimSpace(parsed.Hostname()))
		}
	}

	if slash := strings.IndexByte(trimmed, '/'); slash >= 0 {
		trimmed = trimmed[:slash]
	}
	if host, _, err := net.SplitHostPort(trimmed); err == nil {
		trimmed = host
	}

	return strings.ToLower(strings.TrimSpace(trimmed))
}
