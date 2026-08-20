package common

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/controlplane"
	"github.com/iw2rmb/ploy/internal/httpx"
)

// ResolveControlPlaneHTTP selects the base URL and HTTP client using
// PLOY_SERVER_URL and PLOY_AUTH_TOKEN.
// Returns a client configured for bearer token authentication.
func ResolveControlPlaneHTTP(_ context.Context) (*url.URL, *http.Client, error) {
	rawBase := strings.TrimSpace(os.Getenv("PLOY_SERVER_URL"))
	baseURL, err := controlplane.BaseURLFromServerURL(rawBase)
	if err != nil {
		return nil, nil, err
	}

	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, nil, fmt.Errorf("parse PLOY_SERVER_URL: %w", err)
	}

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

	finalTransport := transport
	if token := strings.TrimSpace(os.Getenv("PLOY_AUTH_TOKEN")); token != "" {
		finalTransport = httpx.NewHeaderTransport(transport, http.Header{
			"Authorization": []string{"Bearer " + token},
		})
	}

	client := &http.Client{
		Transport: finalTransport,
		Timeout:   10 * time.Second,
	}

	return u, client, nil
}

// controlPlaneHTTPError summarises a non-2xx control-plane response.
func ControlPlaneHTTPError(resp *http.Response) error {
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
func CloneForStream(c *http.Client) *http.Client {
	if c == nil {
		return &http.Client{Timeout: 0}
	}
	clone := *c
	clone.Timeout = 0
	return &clone
}
