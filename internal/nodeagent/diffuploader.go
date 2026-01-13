package nodeagent

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

// DiffUploader uploads diff and summary data to the control-plane server.
type DiffUploader struct {
	*baseUploader
}

// NewDiffUploader creates a new diff uploader.
func NewDiffUploader(cfg Config) (*DiffUploader, error) {
	base, err := newBaseUploader(cfg)
	if err != nil {
		return nil, err
	}
	return &DiffUploader{baseUploader: base}, nil
}

// UploadDiff compresses and uploads a diff to the server.
// The diff is associated with a specific job via the job_id parameter.
// Step ordering is determined by the job's step_index in the database.
func (u *DiffUploader) UploadDiff(ctx context.Context, runID types.RunID, jobID types.JobID, diffBytes []byte, summary types.DiffSummary) error {
	// Gzip and validate size using shared compression helper.
	gzippedDiff, err := gzipCompress(diffBytes, "gzipped diff")
	if err != nil {
		return err
	}

	// Build request payload.
	// run_id and job_id are in the URL path, not the body.
	payload := map[string]interface{}{
		"patch":   gzippedDiff,
		"summary": summary,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	// Construct URL using job-scoped endpoint.
	url := fmt.Sprintf("%s/v1/runs/%s/jobs/%s/diff", u.cfg.ServerURL, runID.String(), jobID.String())

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
	if err := httpError(resp, http.StatusCreated, "upload diff"); err != nil {
		return err
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

	// Read bearer token for control plane authentication.
	// Token files often contain trailing newlines or whitespace (e.g., when
	// edited in text editors or generated via echo). Trim to prevent corrupted
	// Authorization headers like "Bearer tok\n" which would cause auth failures.
	tokenFile := bearerTokenPath()
	bearerTokenBytes, err := os.ReadFile(tokenFile)
	if err != nil {
		return nil, fmt.Errorf("read bearer token: %w", err)
	}
	bearerToken := strings.TrimSpace(string(bearerTokenBytes))

	// Wrap transport with bearer token injector
	authenticatedTransport := &bearerTokenTransport{
		base:   transport,
		token:  bearerToken,
		nodeID: cfg.NodeID,
	}

	return &http.Client{
		Transport: authenticatedTransport,
		Timeout:   30 * time.Second,
	}, nil
}

// bearerTokenTransport wraps an http.RoundTripper and adds Authorization and
// PLOY_NODE_UUID headers to all requests.
// Uses domain type (NodeID) for type-safe identification.
type bearerTokenTransport struct {
	base   http.RoundTripper
	token  string
	nodeID types.NodeID // Node ID (NanoID-backed)
}

func (t *bearerTokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clone request to avoid modifying the original
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+t.token)
	req.Header.Set("PLOY_NODE_UUID", t.nodeID.String()) // Convert domain type to string for header
	return t.base.RoundTrip(req)
}
