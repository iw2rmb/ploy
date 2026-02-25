package gitlab

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	gitlabapi "gitlab.com/gitlab-org/api/client-go"
)

// ClientConfig holds configuration for creating a GitLab API client.
type ClientConfig struct {
	// Domain is the GitLab domain (e.g., "gitlab.com" or "gitlab.example.com").
	Domain string
	// PAT is the Personal Access Token for authentication.
	PAT string
	// HTTPClient is the underlying HTTP client to use.
	// If nil, a default client with 30s timeout will be created.
	HTTPClient *http.Client
}

// NewClient creates a configured GitLab API client.
// It constructs the base URL using the domain, automatically selecting http://
// for localhost/127.0.0.1 and https:// for all other domains.
// Authentication headers include both Authorization (Bearer token) and PRIVATE-TOKEN
// to preserve compatibility across GitLab setups.
func NewClient(cfg ClientConfig) (*gitlabapi.Client, error) {
	if cfg.Domain == "" {
		return nil, fmt.Errorf("domain is required")
	}
	if cfg.PAT == "" {
		return nil, fmt.Errorf("pat is required")
	}

	// Construct base URL with appropriate scheme.
	// Use http:// for localhost/127.0.0.1 (testing), https:// otherwise.
	scheme := "https"
	if strings.HasPrefix(cfg.Domain, "localhost") || strings.HasPrefix(cfg.Domain, "127.0.0.1") {
		scheme = "http"
	}
	baseURL := fmt.Sprintf("%s://%s", scheme, cfg.Domain)

	// Use provided HTTP client or create default.
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 30 * time.Second,
		}
	}

	// Create client with PAT authentication.
	// The client-go library sets Authorization header with Bearer token.
	// We disable built-in retries since we manage them via the shared backoff helper.
	client, err := gitlabapi.NewClient(
		cfg.PAT,
		gitlabapi.WithBaseURL(baseURL),
		gitlabapi.WithHTTPClient(httpClient),
		gitlabapi.WithoutRetries(),
	)
	if err != nil {
		return nil, fmt.Errorf("create gitlab client: %w", err)
	}

	// Wrap the HTTP client to inject PRIVATE-TOKEN header on every request.
	// This preserves the dual-auth behavior (Authorization + PRIVATE-TOKEN)
	// that the legacy HTTP client used for compatibility.
	client.HTTPClient().Transport = &tokenInjector{
		base:  httpClient.Transport,
		token: cfg.PAT,
	}

	return client, nil
}

// tokenInjector is an http.RoundTripper that injects the Authorization header
// on every request, preserving the dual-auth behavior from the legacy client.
// The client-go library already sets PRIVATE-TOKEN, so we only need to add Authorization.
type tokenInjector struct {
	base  http.RoundTripper
	token string
}

// RoundTrip implements http.RoundTripper by injecting the Authorization Bearer header.
func (t *tokenInjector) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}

	// Copy request metadata and clone headers without Request.Clone().
	reqClone := new(http.Request)
	*reqClone = *req
	if req.Header != nil {
		reqClone.Header = req.Header.Clone()
	} else {
		reqClone.Header = make(http.Header, 1)
	}

	// Inject Authorization Bearer header to complement the PRIVATE-TOKEN header
	// that the client-go library already sets. This dual-auth approach ensures
	// compatibility across different GitLab setups.
	reqClone.Header.Set("Authorization", "Bearer "+t.token)

	// Use the base transport (or http.DefaultTransport if not set).
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}

	return base.RoundTrip(reqClone)
}
