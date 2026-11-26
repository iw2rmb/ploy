package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/gitauth"
)

func parseGitLabOrigin(domain string) (*url.URL, error) {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return nil, fmt.Errorf("gitlab domain is not configured")
	}
	if !strings.Contains(domain, "://") {
		domain = "https://" + domain
	}
	parsed, err := url.Parse(domain)
	if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return nil, fmt.Errorf("gitlab domain is invalid")
	}
	return &url.URL{Scheme: parsed.Scheme, Host: parsed.Host}, nil
}

func resolveGitLabShortSHA(ctx context.Context, repoURL, prefix string, auth gitauth.Options) (string, error) {
	origin, err := parseGitLabOrigin(auth.GitLabDomain)
	if err != nil {
		return "", err
	}
	repo, err := url.Parse(repoURL)
	if err != nil || repo.Scheme != origin.Scheme || !strings.EqualFold(repo.Host, origin.Host) {
		return "", fmt.Errorf("short SHA lookup requires a repository on the configured GitLab origin")
	}
	project := strings.TrimSuffix(strings.Trim(repo.Path, "/"), ".git")
	prefix = strings.ToLower(prefix)
	endpoint := origin.JoinPath("api/v4/projects", url.PathEscape(project), "repository/commits", prefix)
	endpoint.RawQuery = "stats=false"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return "", err
	}
	if token := strings.TrimSpace(auth.GitLabPAT); token != "" {
		req.Header.Set("PRIVATE-TOKEN", token)
	}
	// Go forwards PRIVATE-TOKEN across redirects because it is a custom header.
	client := http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("GitLab commit lookup: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("GitLab could not resolve short SHA %q to a unique commit (HTTP 404)", prefix)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitLab commit lookup returned HTTP %d", resp.StatusCode)
	}
	var commit struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&commit); err != nil {
		return "", fmt.Errorf("invalid GitLab commit response: %w", err)
	}
	sha, ok := domaintypes.NormalizeFullCommitSHA(commit.ID)
	if !ok || !strings.HasPrefix(sha.String(), prefix) {
		return "", fmt.Errorf("GitLab returned a commit ID that does not match short SHA %q", prefix)
	}
	return sha.String(), nil
}
