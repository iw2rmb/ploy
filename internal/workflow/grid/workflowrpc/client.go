package workflowrpc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/aster"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/manifests"
)

const submitPath = "/v1/workflows/rpc/runs:submit"

// Options configures the Workflow RPC client.
type Options struct {
	Endpoint   string
	HTTPClient *http.Client
}

// Client wraps the Workflow RPC endpoint.
type Client struct {
	endpoint   *url.URL
	httpClient *http.Client
}

// NewClient constructs a Workflow RPC client.
func NewClient(opts Options) (*Client, error) {
	endpoint := strings.TrimSpace(opts.Endpoint)
	if endpoint == "" {
		return nil, fmt.Errorf("workflow rpc endpoint is required")
	}

	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse workflow rpc endpoint: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("workflow rpc endpoint must include scheme and host")
	}

	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &Client{endpoint: parsed, httpClient: httpClient}, nil
}

// Submit issues a workflow stage submission against Grid's Workflow RPC.
func (c *Client) Submit(ctx context.Context, req SubmitRequest) (SubmitResponse, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return SubmitResponse{}, fmt.Errorf("encode workflow rpc request: %w", err)
	}

	endpoint := c.endpoint.ResolveReference(&url.URL{Path: submitPath})
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(data))
	if err != nil {
		return SubmitResponse{}, fmt.Errorf("create workflow rpc request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return SubmitResponse{}, fmt.Errorf("invoke workflow rpc endpoint: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return SubmitResponse{}, fmt.Errorf("read workflow rpc response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet := strings.TrimSpace(string(body))
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		return SubmitResponse{}, &HTTPError{StatusCode: resp.StatusCode, Body: snippet}
	}

	var out SubmitResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return SubmitResponse{}, fmt.Errorf("decode workflow rpc response: %w", err)
	}
	return out, nil
}

// HTTPError describes an HTTP failure returned by the Workflow RPC endpoint.
type HTTPError struct {
	StatusCode int
	Body       string
}

// Error implements the error interface.
func (e *HTTPError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.Body) == "" {
		return fmt.Sprintf("workflow rpc returned status %d", e.StatusCode)
	}
	return fmt.Sprintf("workflow rpc returned status %d: %s", e.StatusCode, e.Body)
}

// Retryable reports whether the HTTP error is retryable.
func (e *HTTPError) Retryable() bool {
	if e == nil {
		return false
	}
	return e.StatusCode >= 500 && e.StatusCode < 600
}

// AsHTTPError extracts an HTTPError from an error value if available.
func AsHTTPError(err error) (*HTTPError, bool) {
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		return httpErr, true
	}
	return nil, false
}

// SubmitRequest is the payload sent to the Workflow RPC endpoint.
type SubmitRequest struct {
	SchemaVersion string                   `json:"schema_version"`
	Ticket        contracts.WorkflowTicket `json:"ticket"`
	Stage         Stage                    `json:"stage"`
}

// SubmitResponse is the payload returned by the Workflow RPC endpoint.
type SubmitResponse struct {
	Status    string     `json:"status"`
	Message   string     `json:"message"`
	Retryable bool       `json:"retryable"`
	Stage     Stage      `json:"stage"`
	Artifacts []Artifact `json:"artifacts"`
}

// Stage represents the workflow stage envelope exchanged with Grid.
type Stage struct {
	Name         string      `json:"name"`
	Kind         string      `json:"kind"`
	Lane         string      `json:"lane"`
	Dependencies []string    `json:"dependencies,omitempty"`
	CacheKey     string      `json:"cache_key,omitempty"`
	Constraints  Constraints `json:"constraints"`
	Aster        Aster       `json:"aster"`
	Job          JobSpec     `json:"job"`
}

// Constraints encapsulates manifest constraints for a stage.
type Constraints struct {
	Manifest manifests.Compilation `json:"manifest"`
}

// Aster captures stage-level Aster metadata.
type Aster struct {
	Enabled bool             `json:"enabled"`
	Toggles []string         `json:"toggles"`
	Bundles []aster.Metadata `json:"bundles"`
}

// JobSpec represents the job payload associated with a stage.
type JobSpec struct {
	Image     string            `json:"image"`
	Command   []string          `json:"command"`
	Env       map[string]string `json:"env"`
	Resources Resources         `json:"resources"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// Resources captures resource hints for the job payload.
type Resources struct {
	CPU    string `json:"cpu,omitempty"`
	Memory string `json:"memory,omitempty"`
	Disk   string `json:"disk,omitempty"`
	GPU    string `json:"gpu,omitempty"`
}

// Artifact represents the artifact envelope returned by Workflow RPC.
type Artifact struct {
	Name        string `json:"name"`
	ArtifactCID string `json:"artifact_cid"`
	Digest      string `json:"digest"`
	MediaType   string `json:"media_type"`
}
