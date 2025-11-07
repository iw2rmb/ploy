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
	"time"

	"github.com/iw2rmb/ploy/internal/cli/config"
)

// resolveControlPlaneHTTP selects the base URL and HTTP client using the
// default cluster descriptor at ~/.config/ploy/clusters/default. No
// environment variable overrides are supported.
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

	// Plain HTTP (no mTLS) if scheme is http.
	if strings.EqualFold(u.Scheme, "http") {
		return u, &http.Client{Timeout: 10 * time.Second}, nil
	}

	// HTTPS requires mTLS materials to be present in the descriptor.
	if strings.TrimSpace(desc.CAPath) == "" || strings.TrimSpace(desc.CertPath) == "" || strings.TrimSpace(desc.KeyPath) == "" {
		return nil, nil, fmt.Errorf("incomplete TLS config in cluster descriptor (ca, cert, key required)")
	}

	cert, err := tls.LoadX509KeyPair(desc.CertPath, desc.KeyPath)
	if err != nil {
		return nil, nil, fmt.Errorf("load client certificate: %w", err)
	}
	caData, err := os.ReadFile(desc.CAPath)
	if err != nil {
		return nil, nil, fmt.Errorf("load ca certificate: %w", err)
	}
	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caData) {
		return nil, nil, fmt.Errorf("failed to parse ca certificate")
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caCertPool,
		MinVersion:   tls.VersionTLS13,
	}
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: tlsConfig}, Timeout: 10 * time.Second}
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
