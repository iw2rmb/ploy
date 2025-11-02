package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/iw2rmb/ploy/internal/cli/config"
)

const controlPlaneURLEnv = "PLOY_CONTROL_PLANE_URL"

// resolveControlPlaneHTTP selects the base URL and HTTP client.
// Loads TLS configuration from the default cluster descriptor if available.
// For tests, honours PLOY_CONTROL_PLANE_URL and uses a plain client.
func resolveControlPlaneHTTP(_ context.Context) (*url.URL, *http.Client, error) {
	base := os.Getenv(controlPlaneURLEnv)
	if base == "" {
		base = "http://127.0.0.1:9094"
	}
	u, err := url.Parse(base)
	if err != nil {
		return nil, nil, err
	}

	// Load default cluster descriptor for TLS configuration.
	desc, err := config.LoadDefault()
	if err != nil || desc.CAPath == "" || desc.CertPath == "" || desc.KeyPath == "" {
		// No descriptor or incomplete TLS config: use plain client (for tests or legacy setups).
		return u, &http.Client{}, nil
	}

	// Load client certificate and key.
	cert, err := tls.LoadX509KeyPair(desc.CertPath, desc.KeyPath)
	if err != nil {
		return nil, nil, fmt.Errorf("load client certificate: %w", err)
	}

	// Load CA certificate for server verification.
	caData, err := os.ReadFile(desc.CAPath)
	if err != nil {
		return nil, nil, fmt.Errorf("load ca certificate: %w", err)
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caData) {
		return nil, nil, fmt.Errorf("failed to parse ca certificate")
	}

	// Create TLS config with mTLS and TLS 1.3 enforcement.
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caCertPool,
		MinVersion:   tls.VersionTLS13,
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}

	return u, client, nil
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
