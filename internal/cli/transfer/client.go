package transfer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

// Client wraps HTTP access to the control-plane transfer endpoints.
type Client struct {
	BaseURL    *url.URL
	HTTPClient *http.Client
}

// UploadSlotRequest reserves an upload slot for a job.
type UploadSlotRequest struct {
	JobID  string `json:"job_id"`
	Stage  string `json:"stage,omitempty"`
	Kind   string `json:"kind"`
	NodeID string `json:"node_id"`
	Size   int64  `json:"size,omitempty"`
	Digest string `json:"digest,omitempty"`
}

// DownloadSlotRequest reserves a download slot for a job artifact.
type DownloadSlotRequest struct {
	JobID      string `json:"job_id"`
	Kind       string `json:"kind,omitempty"`
	ArtifactID string `json:"artifact_id,omitempty"`
	NodeID     string `json:"node_id,omitempty"`
}

// Slot describes a reserved transfer slot.
type Slot struct {
	ID         string    `json:"slot_id"`
	Kind       string    `json:"kind"`
	JobID      string    `json:"job_id"`
	NodeID     string    `json:"node_id"`
	RemotePath string    `json:"remote_path"`
	MaxSize    int64     `json:"max_size"`
	ExpiresAt  time.Time `json:"expires_at"`
	Digest     string    `json:"digest,omitempty"`
}

// CommitRequest finalises a slot after the transfer completes.
type CommitRequest struct {
	Size   int64  `json:"size"`
	Digest string `json:"digest"`
}

// UploadSlot requests a new upload slot for the provided job metadata.
func (c Client) UploadSlot(ctx context.Context, req UploadSlotRequest) (Slot, error) {
	return c.requestSlot(ctx, "/v1/transfers/upload", req)
}

// DownloadSlot requests a download slot for the supplied artifact metadata.
func (c Client) DownloadSlot(ctx context.Context, req DownloadSlotRequest) (Slot, error) {
	return c.requestSlot(ctx, "/v1/transfers/download", req)
}

// emptyRequest is used for endpoints that accept an empty JSON body.
type emptyRequest struct{}

// emptyResponse is used for endpoints that return no meaningful JSON body.
type emptyResponse struct{}

// Commit notifies the control plane that the slot finished transferring successfully.
func (c Client) Commit(ctx context.Context, slotID string, req CommitRequest) error {
	endpoint := fmt.Sprintf("/v1/transfers/%s/commit", strings.TrimSpace(slotID))
	_, err := doReq[CommitRequest, emptyResponse](ctx, c, http.MethodPost, endpoint, req)
	return err
}

// Abort releases the slot without committing the transfer.
func (c Client) Abort(ctx context.Context, slotID string) error {
	endpoint := fmt.Sprintf("/v1/transfers/%s/abort", strings.TrimSpace(slotID))
	_, err := doReq[emptyRequest, emptyResponse](ctx, c, http.MethodPost, endpoint, emptyRequest{})
	return err
}

func (c Client) requestSlot(ctx context.Context, endpoint string, payload any) (Slot, error) {
	switch p := payload.(type) {
	case UploadSlotRequest:
		return doReq[UploadSlotRequest, Slot](ctx, c, http.MethodPost, endpoint, p)
	case DownloadSlotRequest:
		return doReq[DownloadSlotRequest, Slot](ctx, c, http.MethodPost, endpoint, p)
	default:
		return Slot{}, fmt.Errorf("transfer: unsupported request type: %T", payload)
	}
}

// doReq is a generic HTTP helper that provides compile-time request/response typing.
func doReq[TReq any, TRes any](ctx context.Context, c Client, method, endpoint string, payload TReq) (TRes, error) {
	var zero TRes
	var out any = &zero
	// Skip decoding for empty responses.
	if _, ok := any(zero).(emptyResponse); ok {
		out = nil
	}
	if err := c.do(ctx, method, endpoint, payload, out); err != nil {
		return zero, err
	}
	return zero, nil
}

func (c Client) do(ctx context.Context, method, endpoint string, payload any, out any) error {
	base := c.BaseURL
	if base == nil {
		return errors.New("transfer: base URL required")
	}
	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	body := bytes.NewReader(nil)
	var buf bytes.Buffer
	if payload != nil {
		enc := json.NewEncoder(&buf)
		if err := enc.Encode(payload); err != nil {
			return fmt.Errorf("transfer: encode payload: %w", err)
		}
		body = bytes.NewReader(buf.Bytes())
	}
	reqURL := *base
	reqURL.Path = path.Join(strings.TrimSuffix(base.Path, "/"), strings.TrimPrefix(endpoint, "/"))
	request, err := http.NewRequestWithContext(ctx, method, reqURL.String(), body)
	if err != nil {
		return fmt.Errorf("transfer: build request: %w", err)
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	resp, err := httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("transfer: do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		if len(data) == 0 {
			data = []byte(resp.Status)
		}
		return fmt.Errorf("transfer: request failed: %s", strings.TrimSpace(string(data)))
	}
	if out != nil {
		dec := json.NewDecoder(resp.Body)
		if err := dec.Decode(out); err != nil {
			return fmt.Errorf("transfer: decode response: %w", err)
		}
	}
	return nil
}
