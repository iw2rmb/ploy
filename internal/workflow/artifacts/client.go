package artifacts

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ClusterClientOptions configures the IPFS Cluster client.
type ClusterClientOptions struct {
	BaseURL string

	AuthToken            string
	BasicAuthUsername    string
	BasicAuthPassword    string
	ReplicationFactorMin int
	ReplicationFactorMax int

	HTTPClient *http.Client
}

// ClusterClient provides helpers for interacting with an IPFS Cluster REST API.
type ClusterClient struct {
	base           *url.URL
	http           *http.Client
	authHeader     string
	defaultReplMin int
	defaultReplMax int
}

// NewClusterClient constructs an IPFS Cluster client with sane defaults.
func NewClusterClient(opts ClusterClientOptions) (*ClusterClient, error) {
	trimmed := strings.TrimSpace(opts.BaseURL)
	if trimmed == "" {
		return nil, errors.New("artifacts: cluster base url required")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("artifacts: parse cluster base url: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, errors.New("artifacts: cluster base url must include scheme and host")
	}
	sanitized := *parsed
	if sanitized.Path == "" {
		sanitized.Path = ""
	}
	sanitized.RawQuery = ""
	sanitized.Fragment = ""

	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	authHeader := ""
	if token := strings.TrimSpace(opts.AuthToken); token != "" {
		authHeader = "Bearer " + token
	} else if strings.TrimSpace(opts.BasicAuthUsername) != "" || strings.TrimSpace(opts.BasicAuthPassword) != "" {
		creds := opts.BasicAuthUsername + ":" + opts.BasicAuthPassword
		authHeader = "Basic " + base64.StdEncoding.EncodeToString([]byte(creds))
	}

	return &ClusterClient{
		base:           &sanitized,
		http:           httpClient,
		authHeader:     authHeader,
		defaultReplMin: opts.ReplicationFactorMin,
		defaultReplMax: opts.ReplicationFactorMax,
	}, nil
}

// resolve constructs an absolute URL for the provided path relative to the base.
func (c *ClusterClient) resolve(path string) *url.URL {
	relative := &url.URL{Path: path}
	return c.base.ResolveReference(relative)
}

// applyAuth injects the Authorization header when the client has credentials.
func (c *ClusterClient) applyAuth(req *http.Request) {
	if c.authHeader != "" {
		req.Header.Set("Authorization", c.authHeader)
	}
}
