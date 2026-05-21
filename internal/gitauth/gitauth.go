package gitauth

import (
	"encoding/base64"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// Options configures transient Git HTTP authentication.
type Options struct {
	GitLabPAT    string
	GitLabDomain string
}

// PreparedURL contains the clean URL to pass to Git and per-process env auth.
type PreparedURL struct {
	URL string
	Env []string
}

// PrepareURL strips HTTP(S) credentials from rawURL and returns Git config env
// that authenticates only the current Git process.
func PrepareURL(rawURL string, opts Options) PreparedURL {
	trimmed := strings.TrimSpace(rawURL)
	prepared := PreparedURL{URL: trimmed}
	if trimmed == "" {
		return prepared
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return prepared
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "https" && scheme != "http" {
		return prepared
	}

	clean := *parsed
	clean.User = nil
	prepared.URL = clean.String()

	username, password, ok := explicitCredentials(parsed)
	if !ok {
		username, password, ok = gitLabCredentials(clean, opts)
	}
	if !ok {
		return prepared
	}

	prepared.Env = extraHeaderEnv(clean, username, password)
	return prepared
}

// PrepareBasicURL strips URL credentials and attaches the provided Basic auth pair.
func PrepareBasicURL(rawURL, username, password string) (PreparedURL, error) {
	trimmed := strings.TrimSpace(rawURL)
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return PreparedURL{}, fmt.Errorf("parse remote url: %w", err)
	}
	clean := *parsed
	clean.User = nil
	return PreparedURL{
		URL: clean.String(),
		Env: extraHeaderEnv(clean, username, password),
	}, nil
}

// OptionsFromManifest extracts Git auth options from manifest options.
func OptionsFromManifest(manifest contracts.StepManifest) Options {
	opts := Options{}
	if pat, ok := manifest.OptionString("gitlab_pat"); ok {
		opts.GitLabPAT = pat
	}
	if domain, ok := manifest.OptionString("gitlab_domain"); ok {
		opts.GitLabDomain = domain
	}
	return opts
}

func extraHeaderEnv(repoURL url.URL, username, password string) []string {
	scope := fmt.Sprintf("%s://%s/", strings.ToLower(repoURL.Scheme), repoURL.Host)
	payload := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
	return []string{
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=http." + scope + ".extraHeader",
		"GIT_CONFIG_VALUE_0=Authorization: Basic " + payload,
	}
}

func explicitCredentials(parsed *url.URL) (string, string, bool) {
	if parsed.User == nil {
		return "", "", false
	}
	username := parsed.User.Username()
	password, _ := parsed.User.Password()
	if username == "" && password == "" {
		return "", "", false
	}
	return username, password, true
}

func gitLabCredentials(repoURL url.URL, opts Options) (string, string, bool) {
	pat := strings.TrimSpace(opts.GitLabPAT)
	if pat == "" {
		return "", "", false
	}
	domainHost := normalizeGitLabDomainHost(opts.GitLabDomain)
	if domainHost != "" && !strings.EqualFold(repoURL.Hostname(), domainHost) {
		return "", "", false
	}
	return "oauth2", pat, true
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
