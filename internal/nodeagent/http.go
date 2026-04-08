package nodeagent

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/iw2rmb/ploy/internal/cli/httpx"
	types "github.com/iw2rmb/ploy/internal/domain/types"
	wfbackoff "github.com/iw2rmb/ploy/internal/workflow/backoff"
)

// baseUploader provides common HTTP client functionality for uploaders and fetchers.
type baseUploader struct {
	cfg    Config
	client *http.Client
}

type getJobStatusResponse struct {
	JobID  types.JobID `json:"job_id"`
	Status string      `json:"status"`
}

func newBaseUploader(cfg Config) (*baseUploader, error) {
	client, err := createHTTPClient(cfg)
	if err != nil {
		return nil, err
	}
	return &baseUploader{cfg: cfg, client: client}, nil
}

// postJSON sends a JSON POST request and checks the expected status code.
// On success, the caller is responsible for closing resp.Body.
func (b *baseUploader) postJSON(ctx context.Context, apiPath string, payload any, expectedStatus int, action string) (*http.Response, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	u := MustBuildURL(b.cfg.ServerURL, apiPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	if err := httpx.CheckStatus(resp, expectedStatus, action); err != nil {
		_ = resp.Body.Close()
		return nil, err
	}
	return resp, nil
}

type postJSONRetryMode int

const (
	postJSONRetryModeDefault postJSONRetryMode = iota
	postJSONRetryModeStartupReconcile
)

func classifyPostJSONStatus(mode postJSONRetryMode, statusCode int) (success bool, retry bool) {
	switch {
	case statusCode == http.StatusOK || statusCode == http.StatusNoContent:
		return true, false
	case mode == postJSONRetryModeStartupReconcile && statusCode == http.StatusConflict:
		// Startup reconciliation replays terminal completion and must be idempotent.
		return true, false
	case statusCode >= 500 && statusCode < 600:
		return false, true
	default:
		return false, false
	}
}

// postJSONWithRetry sends a JSON POST request with exponential backoff.
// Accepts 200 and 204 as success in all modes. In startup reconcile mode,
// 409 is also treated as success (idempotent replay).
func (b *baseUploader) postJSONWithRetry(ctx context.Context, apiPath string, payload any, action string, mode postJSONRetryMode) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	u := MustBuildURL(b.cfg.ServerURL, apiPath)
	policy := wfbackoff.StatusUploaderPolicy()
	logger := slog.Default()
	attempt := 0

	uploadOp := func() error {
		attempt++
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
		if err != nil {
			return backoff.Permanent(fmt.Errorf("create request: %w", err))
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := b.client.Do(req)
		if err != nil {
			logger.Warn(action+" request failed, retrying", "attempt", attempt, "error", err)
			return fmt.Errorf("send request: %w", err)
		}
		respBody, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		success, retry := classifyPostJSONStatus(mode, resp.StatusCode)
		if success {
			return nil
		}
		if retry {
			logger.Warn(action+" received 5xx, retrying", "attempt", attempt, "status_code", resp.StatusCode)
			return fmt.Errorf("%s failed: status %d: %s", action, resp.StatusCode, string(respBody))
		}
		return backoff.Permanent(fmt.Errorf("%s failed: status %d: %s", action, resp.StatusCode, string(respBody)))
	}

	return wfbackoff.RunWithBackoff(ctx, policy, logger, uploadOp)
}

// UploadJobStatus uploads terminal status and stats to the job-level endpoint.
func (b *baseUploader) UploadJobStatus(ctx context.Context, jobID types.JobID, status string, exitCode *int32, stats types.RunStats, repoSHAOut ...string) error {
	shaOut := ""
	if len(repoSHAOut) > 0 {
		shaOut = strings.TrimSpace(repoSHAOut[0])
	}
	return b.postJSONWithRetry(
		ctx,
		fmt.Sprintf("/v1/jobs/%s/complete", jobID),
		buildJobStatusPayload(status, exitCode, stats, shaOut),
		"upload job status",
		postJSONRetryModeDefault,
	)
}

// UploadActionStatus uploads terminal status and stats to the action-level endpoint.
func (b *baseUploader) UploadActionStatus(ctx context.Context, actionID types.JobID, status string, stats types.RunStats) error {
	return b.postJSONWithRetry(
		ctx,
		fmt.Sprintf("/v1/actions/%s/complete", actionID),
		map[string]any{
			"status": status,
			"stats":  stats,
		},
		"upload action status",
		postJSONRetryModeDefault,
	)
}

// UploadJobStatusReconcile uploads terminal status during startup crash
// reconciliation. This mode treats 409 conflicts as successful idempotent replay.
func (b *baseUploader) UploadJobStatusReconcile(ctx context.Context, jobID types.JobID, status string, exitCode *int32, stats types.RunStats, repoSHAOut ...string) error {
	shaOut := ""
	if len(repoSHAOut) > 0 {
		shaOut = strings.TrimSpace(repoSHAOut[0])
	}
	return b.postJSONWithRetry(
		ctx,
		fmt.Sprintf("/v1/jobs/%s/complete", jobID),
		buildJobStatusPayload(status, exitCode, stats, shaOut),
		"upload reconciled job status",
		postJSONRetryModeStartupReconcile,
	)
}

func buildJobStatusPayload(status string, exitCode *int32, stats types.RunStats, repoSHAOut string) map[string]any {
	payload := map[string]any{"status": status}
	if exitCode != nil {
		payload["exit_code"] = *exitCode
	}
	if stats != nil {
		payload["stats"] = stats
	}
	if repoSHAOut != "" {
		payload["repo_sha_out"] = repoSHAOut
	}
	return payload
}

// GetJobStatus returns canonical control-plane status for a claimed job.
func (b *baseUploader) GetJobStatus(ctx context.Context, jobID types.JobID) (string, error) {
	u := MustBuildURL(b.cfg.ServerURL, fmt.Sprintf("/v1/jobs/%s/status", jobID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	resp, err := b.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("get job status failed: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload getJobStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	status := strings.TrimSpace(payload.Status)
	if status == "" {
		return "", errors.New("get job status failed: empty status")
	}
	return status, nil
}

// BuildURL resolves a base URL and a path-only reference, preserving scheme/host.
func BuildURL(base, p string) (string, error) {
	bu, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("parse base url: %w", err)
	}
	pu, err := url.Parse(p)
	if err != nil {
		return "", fmt.Errorf("parse path: %w", err)
	}
	if pu.IsAbs() || pu.Scheme != "" || pu.Host != "" || pu.User != nil {
		return "", fmt.Errorf("path must not include scheme or host")
	}
	return bu.ResolveReference(pu).String(), nil
}

// MustBuildURL is like BuildURL but panics on error.
func MustBuildURL(base, p string) string {
	u, err := BuildURL(base, p)
	if err != nil {
		panic(fmt.Sprintf("MustBuildURL: %v", err))
	}
	return u
}

const (
	MaxUploadSize = 10 << 20 // 10 MiB

	// SoftUploadSize is the threshold at which log chunks are flushed. The 64-byte
	// margin accounts for gzip footer overhead when finalizing a chunk, ensuring
	// the closed stream stays under MaxUploadSize.
	SoftUploadSize = MaxUploadSize - 64
)

// ErrPayloadTooLarge is returned when compressed data exceeds MaxUploadSize.
var ErrPayloadTooLarge = errors.New("payload exceeds size cap")

func validateUploadSize(data []byte, dataType string) error {
	if len(data) > MaxUploadSize {
		return fmt.Errorf("%s exceeds size cap: %d > %d bytes: %w",
			dataType, len(data), MaxUploadSize, ErrPayloadTooLarge)
	}
	return nil
}

// getBytesFromURL sends a GET request to fullURL and returns the response body on 200 OK.
// Returns an error if the request fails, if the server returns a non-200 status, or if reading fails.
func (b *baseUploader) getBytesFromURL(ctx context.Context, fullURL, action string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer httpx.DrainAndClose(resp)
	if err := httpx.CheckStatus(resp, http.StatusOK, action); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	return data, nil
}

// postJSONBytes sends pre-marshaled JSON bytes as a POST request to u.
// Returns nil if the response status is 200 or 201. Drains and closes the response body.
func postJSONBytes(ctx context.Context, client *http.Client, u string, body []byte, action string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer httpx.DrainAndClose(resp)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		msg := httpx.ReadErrorMessage(resp.Body, resp.Status, httpx.MaxErrorBodyBytes)
		return fmt.Errorf("%s failed: status %d: %s", action, resp.StatusCode, msg)
	}
	return nil
}

func gzipCompress(data []byte, dataType string) ([]byte, error) {
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)

	if _, err := gzWriter.Write(data); err != nil {
		return nil, fmt.Errorf("gzip %s: %w", dataType, err)
	}
	if err := gzWriter.Close(); err != nil {
		return nil, fmt.Errorf("finalize gzip %s: %w", dataType, err)
	}

	compressed := buf.Bytes()
	if err := validateUploadSize(compressed, dataType); err != nil {
		return nil, err
	}

	return compressed, nil
}

// createHTTPClient creates an HTTP client with bearer token authentication.
func createHTTPClient(cfg Config) (*http.Client, error) {
	transport := &http.Transport{
		MaxIdleConns:        10,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}

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

	tokenFile := bearerTokenPath()
	bearerTokenBytes, err := os.ReadFile(tokenFile)
	if err != nil {
		return nil, fmt.Errorf("read bearer token: %w", err)
	}
	bearerToken := strings.TrimSpace(string(bearerTokenBytes))

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
type bearerTokenTransport struct {
	base   http.RoundTripper
	token  string
	nodeID types.NodeID
}

func (t *bearerTokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}

	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}

	// Avoid Request.Clone() here; in our node runtime this path has shown
	// unstable behavior in Go's map cloning internals under load.
	reqCopy := new(http.Request)
	*reqCopy = *req
	if req.Header != nil {
		reqCopy.Header = req.Header.Clone()
	} else {
		reqCopy.Header = make(http.Header, 2)
	}

	reqCopy.Header.Set("Authorization", "Bearer "+t.token)
	reqCopy.Header.Set("PLOY_NODE_UUID", t.nodeID.String())
	return base.RoundTrip(reqCopy)
}
