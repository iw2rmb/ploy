package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/config"
)

// resolveControlPlaneHTTP selects the base URL and HTTP client using the
// default cluster descriptor marker at ~/.config/ploy/default.
// Returns a client configured for bearer token authentication.
func resolveControlPlaneHTTP(_ context.Context) (*url.URL, *http.Client, error) {
	// Load default cluster descriptor (required).
	desc, err := config.LoadDefault()
	if err != nil {
		return nil, nil, fmt.Errorf("load default cluster descriptor: %w", err)
	}
	if strings.TrimSpace(desc.Address) == "" {
		return nil, nil, fmt.Errorf("default cluster descriptor missing address")
	}

	u, err := url.Parse(desc.Address)
	if err != nil {
		return nil, nil, fmt.Errorf("parse cluster address: %w", err)
	}

	// Build transport: TLS only when using https.
	var transport http.RoundTripper
	if u.Scheme == "https" {
		transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS13,
			},
		}
	}
	if transport == nil {
		transport = http.DefaultTransport
	}

	// Wrap transport with bearer token injector if token is available
	finalTransport := transport
	if strings.TrimSpace(desc.Token) != "" {
		finalTransport = &bearerTokenTransport{
			base:  transport,
			token: strings.TrimSpace(desc.Token),
		}
	}

	client := &http.Client{
		Transport: finalTransport,
		Timeout:   10 * time.Second,
	}

	return u, client, nil
}

// resolveControlPlaneToken returns the configured default cluster bearer token.
// It is used for rendering browser-friendly artifact links with auth_token query parameters.
func resolveControlPlaneToken() (string, error) {
	desc, err := config.LoadDefault()
	if err != nil {
		return "", fmt.Errorf("load default cluster descriptor: %w", err)
	}
	return strings.TrimSpace(desc.Token), nil
}

// bearerTokenTransport wraps an http.RoundTripper and adds Authorization header to all requests.
type bearerTokenTransport struct {
	base  http.RoundTripper
	token string
}

func (t *bearerTokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clone request to avoid modifying the original
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(req)
}

// makeAuthenticatedRequest creates an HTTP request with bearer token authorization.
// This helper should be used for all API calls that require authentication.
func makeAuthenticatedRequest(ctx context.Context, method, endpoint string, body io.Reader) (*http.Request, error) {
	desc, err := config.LoadDefault()
	if err != nil {
		return nil, fmt.Errorf("load descriptor: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, err
	}

	// Add bearer token if available
	if strings.TrimSpace(desc.Token) != "" {
		req.Header.Set("Authorization", "Bearer "+desc.Token)
	}

	return req, nil
}

// controlPlaneHTTPError summarises a non-2xx control-plane response.
func controlPlaneHTTPError(resp *http.Response) error {
	if resp == nil {
		return fmt.Errorf("control-plane: nil response")
	}
	defer func() { _ = resp.Body.Close() }()
	var body string
	if data, err := io.ReadAll(io.LimitReader(resp.Body, 2048)); err == nil {
		body = strings.TrimSpace(string(data))
	}
	if body != "" {
		return fmt.Errorf("control-plane: %s %s -> %d: %s", resp.Request.Method, resp.Request.URL.Path, resp.StatusCode, body)
	}
	return fmt.Errorf("control-plane: %s %s -> %d", resp.Request.Method, resp.Request.URL.Path, resp.StatusCode)
}

// cloneForStream returns a shallow copy of the provided HTTP client with
// Timeout disabled (0). Used for SSE calls which should not have a global
// client timeout.
func cloneForStream(c *http.Client) *http.Client {
	if c == nil {
		return &http.Client{Timeout: 0}
	}
	clone := *c
	clone.Timeout = 0
	return &clone
}
