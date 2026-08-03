package config

import (
	"fmt"
	"net/url"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func parseSpecRepos(raw string) (SpecRepos, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}

	parts := strings.Split(raw, ",")
	repos := make(SpecRepos, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, fmt.Errorf("config: PLOY_SPECS_REPOS entry %d is empty", i+1)
		}

		repoURL, identity, err := validateSpecRepoURL(part)
		if err != nil {
			return nil, fmt.Errorf("config: PLOY_SPECS_REPOS entry %d: %w", i+1, err)
		}
		if _, ok := seen[identity]; ok {
			return nil, fmt.Errorf("config: PLOY_SPECS_REPOS contains duplicate repository %q", identity)
		}
		seen[identity] = struct{}{}
		repos = append(repos, repoURL)
	}
	return repos, nil
}

func validateSpecRepoURL(raw string) (domaintypes.RepoURL, string, error) {
	var repoURL domaintypes.RepoURL
	if err := repoURL.UnmarshalText([]byte(raw)); err != nil {
		return "", "", fmt.Errorf("invalid repository URL")
	}

	parsed, err := url.Parse(raw)
	if err != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", "", fmt.Errorf("invalid repository URL")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "https", "ssh":
		if parsed.Host == "" || strings.Trim(parsed.Path, "/") == "" {
			return "", "", fmt.Errorf("invalid repository URL")
		}
	case "file":
		if parsed.Path == "" || !strings.HasPrefix(parsed.Path, "/") {
			return "", "", fmt.Errorf("invalid repository URL")
		}
	default:
		return "", "", fmt.Errorf("unsupported repository URL scheme")
	}

	identity := domaintypes.NormalizeRepoURL(raw)
	if identity == "" {
		return "", "", fmt.Errorf("invalid repository URL")
	}
	return repoURL, identity, nil
}
