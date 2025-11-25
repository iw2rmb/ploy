package nodeagent

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

// DiffUploader uploads diff and summary data to the control-plane server.
type DiffUploader struct {
	cfg    Config
	client *http.Client
}

// NewDiffUploader creates a new diff uploader.
func NewDiffUploader(cfg Config) (*DiffUploader, error) {
	client, err := createHTTPClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("create http client: %w", err)
	}

	return &DiffUploader{
		cfg:    cfg,
		client: client,
	}, nil
}

// UploadDiff compresses and uploads a diff to the server with optional step_index.
// The step_index parameter enables multi-node execution by tagging diffs with their
// logical step order, allowing rehydration to fetch diffs up to a specific step.
func (u *DiffUploader) UploadDiff(ctx context.Context, runID, stageID string, diffBytes []byte, summary types.DiffSummary, stepIndex *int32) error {
	// Gzip the diff content.
	var gzBuf bytes.Buffer
	gzWriter := gzip.NewWriter(&gzBuf)
	if _, err := gzWriter.Write(diffBytes); err != nil {
		return fmt.Errorf("gzip diff: %w", err)
	}
	if err := gzWriter.Close(); err != nil {
		return fmt.Errorf("finalize gzip: %w", err)
	}

	gzippedDiff := gzBuf.Bytes()

	// Check size cap (≤ 1 MiB gzipped).
	const maxDiffSize = 1 << 20 // 1 MiB
	if len(gzippedDiff) > maxDiffSize {
		return fmt.Errorf("gzipped diff exceeds size cap: %d > %d bytes", len(gzippedDiff), maxDiffSize)
	}

	// Build request payload.
	// Include step_index field for multi-step run ordering (nullable for backward compatibility).
	payload := map[string]interface{}{
		"run_id":  runID,
		"patch":   gzippedDiff,
		"summary": summary,
	}
	if stepIndex != nil {
		payload["step_index"] = *stepIndex
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	// Construct URL.
	url := fmt.Sprintf("%s/v1/nodes/%s/stage/%s/diff", u.cfg.ServerURL, u.cfg.NodeID, stageID)

	// Create HTTP request.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Send request.
	resp, err := u.client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Check response status.
	if resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed: status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

// createHTTPClient creates an HTTP client with bearer token authentication.
// Reads the bearer token from /etc/ploy/bearer-token and adds it to all requests.
func createHTTPClient(cfg Config) (*http.Client, error) {
	transport := &http.Transport{
		MaxIdleConns:        10,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}

	// Configure mTLS if enabled (for node's own server, not for control plane auth).
	if cfg.HTTP.TLS.Enabled {
		cert, err := tls.LoadX509KeyPair(cfg.HTTP.TLS.CertPath, cfg.HTTP.TLS.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("load client cert: %w", err)
		}

		caCert, err := os.ReadFile(cfg.HTTP.TLS.CAPath)
		if err != nil {
			return nil, fmt.Errorf("read ca cert: %w", err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("append ca cert failed")
		}

		transport.TLSClientConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
			RootCAs:      caCertPool,
			MinVersion:   tls.VersionTLS13,
		}
	}

	// Read bearer token for control plane authentication
	tokenFile := bearerTokenPath()
	bearerToken, err := os.ReadFile(tokenFile)
	if err != nil {
		return nil, fmt.Errorf("read bearer token: %w", err)
	}

	// Wrap transport with bearer token injector
	authenticatedTransport := &bearerTokenTransport{
		base:  transport,
		token: string(bearerToken),
	}

	return &http.Client{
		Transport: authenticatedTransport,
		Timeout:   30 * time.Second,
	}, nil
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
