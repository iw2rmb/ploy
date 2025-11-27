// gate_http_client.go implements the HTTP client for the Build Gate API.
//
// This client provides typed access to POST /v1/buildgate/validate (submit validation)
// and GET /v1/buildgate/jobs/{id} (poll job status). It uses the centralized backoff
// package for retry logic on transient failures (5xx, network errors).
//
// Configuration is loaded from environment variables:
//   - PLOY_SERVER_URL: Base URL for the Build Gate API (required).
//   - PLOY_API_TOKEN: Bearer token for authentication (optional, uses mTLS otherwise).
//   - TLS configuration via PLOY_TLS_* envs for mTLS authentication.
//
// The client wraps permanent errors (4xx) with backoff.Permanent to prevent retries.
package step

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/backoff"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// BuildGateHTTPClientConfig holds configuration for the Build Gate HTTP client.
// Values are typically sourced from environment variables or node agent config.
type BuildGateHTTPClientConfig struct {
	// ServerURL is the base URL for the Build Gate API (e.g., "https://api.ploy.io").
	ServerURL string

	// APIToken is the bearer token for authentication. If empty, mTLS is used.
	APIToken string

	// TLS configuration for mTLS authentication.
	TLSEnabled bool
	TLSCert    string // Path to client certificate.
	TLSKey     string // Path to client private key.
	TLSCA      string // Path to CA certificate.

	// Timeout for individual HTTP requests.
	Timeout time.Duration

	// Logger for structured logging. If nil, slog.Default() is used.
	Logger *slog.Logger
}

// BuildGateHTTPClientConfigFromEnv loads client configuration from environment variables.
// Returns an error if required variables (PLOY_SERVER_URL) are missing.
func BuildGateHTTPClientConfigFromEnv() (BuildGateHTTPClientConfig, error) {
	serverURL := strings.TrimSpace(os.Getenv("PLOY_SERVER_URL"))
	if serverURL == "" {
		return BuildGateHTTPClientConfig{}, fmt.Errorf("PLOY_SERVER_URL environment variable is required")
	}

	cfg := BuildGateHTTPClientConfig{
		ServerURL:  serverURL,
		APIToken:   strings.TrimSpace(os.Getenv("PLOY_API_TOKEN")),
		TLSEnabled: strings.TrimSpace(os.Getenv("PLOY_TLS_ENABLED")) == "true",
		TLSCert:    strings.TrimSpace(os.Getenv("PLOY_TLS_CERT")),
		TLSKey:     strings.TrimSpace(os.Getenv("PLOY_TLS_KEY")),
		TLSCA:      strings.TrimSpace(os.Getenv("PLOY_TLS_CA")),
		Timeout:    30 * time.Second, // Default timeout for HTTP requests.
	}

	return cfg, nil
}

// buildGateHTTPClient implements BuildGateHTTPClient using net/http.
// It handles authentication (bearer token or mTLS), retries with backoff,
// and JSON encoding/decoding of requests and responses.
type buildGateHTTPClient struct {
	client    *http.Client
	serverURL string
	apiToken  string
	logger    *slog.Logger
}

// NewBuildGateHTTPClient creates a new BuildGateHTTPClient from configuration.
// The client is configured with appropriate TLS settings and timeouts.
func NewBuildGateHTTPClient(cfg BuildGateHTTPClientConfig) (BuildGateHTTPClient, error) {
	// Create HTTP transport with sensible defaults.
	transport := &http.Transport{
		MaxIdleConns:        10,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}

	// Configure mTLS if enabled.
	if cfg.TLSEnabled {
		tlsConfig, err := buildTLSConfig(cfg.TLSCert, cfg.TLSKey, cfg.TLSCA)
		if err != nil {
			return nil, fmt.Errorf("configure TLS: %w", err)
		}
		transport.TLSClientConfig = tlsConfig
	}

	// Default timeout if not specified.
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	// Default logger if not specified.
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &buildGateHTTPClient{
		client: &http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
		serverURL: strings.TrimSuffix(cfg.ServerURL, "/"),
		apiToken:  cfg.APIToken,
		logger:    logger,
	}, nil
}

// buildTLSConfig creates a TLS configuration for mTLS authentication.
// Loads client certificate/key pair and CA certificate for server verification.
func buildTLSConfig(certPath, keyPath, caPath string) (*tls.Config, error) {
	// Load client certificate and key.
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("load client certificate: %w", err)
	}

	// Load CA certificate for server verification.
	caCert, err := os.ReadFile(caPath)
	if err != nil {
		return nil, fmt.Errorf("read CA certificate: %w", err)
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("parse CA certificate failed")
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caCertPool,
		MinVersion:   tls.VersionTLS13,
	}, nil
}

// Validate submits a build validation request to POST /v1/buildgate/validate.
// Uses exponential backoff with retries for transient errors (5xx, network issues).
// Returns a permanent error (no retry) for 4xx client errors.
func (c *buildGateHTTPClient) Validate(ctx context.Context, req contracts.BuildGateValidateRequest) (*contracts.BuildGateValidateResponse, error) {
	// Validate the request before sending.
	if err := req.Validate(); err != nil {
		return nil, backoff.Permanent(fmt.Errorf("invalid request: %w", err))
	}

	// Encode request body as JSON.
	body, err := json.Marshal(req)
	if err != nil {
		return nil, backoff.Permanent(fmt.Errorf("encode request: %w", err))
	}

	// Build the validate endpoint URL.
	validateURL := fmt.Sprintf("%s/v1/buildgate/validate", c.serverURL)

	var resp *contracts.BuildGateValidateResponse

	// Execute with backoff for transient failures.
	policy := backoff.DefaultPolicy()
	err = backoff.RunWithBackoff(ctx, policy, c.logger, func() error {
		// Create HTTP request with context.
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, validateURL, bytes.NewReader(body))
		if err != nil {
			return backoff.Permanent(fmt.Errorf("create request: %w", err))
		}

		httpReq.Header.Set("Content-Type", "application/json")
		c.setAuthHeader(httpReq)

		// Send request.
		httpResp, err := c.client.Do(httpReq)
		if err != nil {
			// Network errors are retryable.
			return fmt.Errorf("send request: %w", err)
		}
		defer func() { _ = httpResp.Body.Close() }()

		// Handle HTTP status codes.
		if httpResp.StatusCode >= 500 {
			// Server errors are retryable.
			respBody, _ := io.ReadAll(httpResp.Body)
			return fmt.Errorf("server error %d: %s", httpResp.StatusCode, string(respBody))
		}
		if httpResp.StatusCode >= 400 {
			// Client errors are permanent (no retry).
			respBody, _ := io.ReadAll(httpResp.Body)
			return backoff.Permanent(fmt.Errorf("client error %d: %s", httpResp.StatusCode, string(respBody)))
		}

		// Decode successful response.
		var validateResp contracts.BuildGateValidateResponse
		if err := json.NewDecoder(httpResp.Body).Decode(&validateResp); err != nil {
			return backoff.Permanent(fmt.Errorf("decode response: %w", err))
		}

		resp = &validateResp
		return nil
	})

	if err != nil {
		return nil, err
	}

	return resp, nil
}

// GetJob retrieves the status of a build gate job via GET /v1/buildgate/jobs/{id}.
// Uses exponential backoff with retries for transient errors (5xx, network issues).
// Returns a permanent error (no retry) for 4xx client errors (e.g., job not found).
func (c *buildGateHTTPClient) GetJob(ctx context.Context, jobID string) (*contracts.BuildGateJobStatusResponse, error) {
	// Validate job ID.
	if strings.TrimSpace(jobID) == "" {
		return nil, backoff.Permanent(fmt.Errorf("job ID is required"))
	}

	// Build the job status endpoint URL (URL-escape the job ID for safety).
	jobURL := fmt.Sprintf("%s/v1/buildgate/jobs/%s", c.serverURL, url.PathEscape(jobID))

	var resp *contracts.BuildGateJobStatusResponse

	// Execute with backoff for transient failures.
	policy := backoff.DefaultPolicy()
	err := backoff.RunWithBackoff(ctx, policy, c.logger, func() error {
		// Create HTTP request with context.
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, jobURL, nil)
		if err != nil {
			return backoff.Permanent(fmt.Errorf("create request: %w", err))
		}

		c.setAuthHeader(httpReq)

		// Send request.
		httpResp, err := c.client.Do(httpReq)
		if err != nil {
			// Network errors are retryable.
			return fmt.Errorf("send request: %w", err)
		}
		defer func() { _ = httpResp.Body.Close() }()

		// Handle HTTP status codes.
		if httpResp.StatusCode >= 500 {
			// Server errors are retryable.
			respBody, _ := io.ReadAll(httpResp.Body)
			return fmt.Errorf("server error %d: %s", httpResp.StatusCode, string(respBody))
		}
		if httpResp.StatusCode >= 400 {
			// Client errors are permanent (no retry).
			respBody, _ := io.ReadAll(httpResp.Body)
			return backoff.Permanent(fmt.Errorf("client error %d: %s", httpResp.StatusCode, string(respBody)))
		}

		// Decode successful response.
		var jobResp contracts.BuildGateJobStatusResponse
		if err := json.NewDecoder(httpResp.Body).Decode(&jobResp); err != nil {
			return backoff.Permanent(fmt.Errorf("decode response: %w", err))
		}

		resp = &jobResp
		return nil
	})

	if err != nil {
		return nil, err
	}

	return resp, nil
}

// setAuthHeader adds the appropriate authentication header to the request.
// Uses bearer token if configured, otherwise relies on mTLS for authentication.
func (c *buildGateHTTPClient) setAuthHeader(req *http.Request) {
	if c.apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiToken)
	}
	// If no API token, rely on mTLS client certificate for authentication.
}
