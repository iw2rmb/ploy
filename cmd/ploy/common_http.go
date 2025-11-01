package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

const controlPlaneURLEnv = "PLOY_CONTROL_PLANE_URL"

// resolveControlPlaneHTTP selects the base URL and HTTP client.
// For unit tests we honour PLOY_CONTROL_PLANE_URL and use a default client.
func resolveControlPlaneHTTP(_ context.Context) (*url.URL, *http.Client, error) {
	base := os.Getenv(controlPlaneURLEnv)
	if base == "" {
		base = "http://127.0.0.1:9094"
	}
	u, err := url.Parse(base)
	if err != nil {
		return nil, nil, err
	}
	return u, &http.Client{}, nil
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
