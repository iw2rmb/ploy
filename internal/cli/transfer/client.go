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

// Commit notifies the control plane that the slot finished transferring successfully.
func (c Client) Commit(ctx context.Context, slotID string, req CommitRequest) error {
	endpoint := fmt.Sprintf("/v1/transfers/%s/commit", strings.TrimSpace(slotID))
	return c.do(ctx, http.MethodPost, endpoint, req, nil)
}

// Abort releases the slot without committing the transfer.
func (c Client) Abort(ctx context.Context, slotID string) error {
	endpoint := fmt.Sprintf("/v1/transfers/%s/abort", strings.TrimSpace(slotID))
	return c.do(ctx, http.MethodPost, endpoint, map[string]any{}, nil)
}

func (c Client) requestSlot(ctx context.Context, endpoint string, payload any) (Slot, error) {
	var slot Slot
	if err := c.do(ctx, http.MethodPost, endpoint, payload, &slot); err != nil {
		return Slot{}, err
	}
	return slot, nil
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
	defer resp.Body.Close()
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
