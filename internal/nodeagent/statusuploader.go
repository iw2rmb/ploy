package nodeagent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// StatusUploader uploads terminal status and stats to the control-plane server.
type StatusUploader struct {
	cfg    Config
	client *http.Client
}

// NewStatusUploader creates a new status uploader.
func NewStatusUploader(cfg Config) (*StatusUploader, error) {
	client, err := createHTTPClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("create http client: %w", err)
	}

	return &StatusUploader{
		cfg:    cfg,
		client: client,
	}, nil
}

// UploadStatus uploads terminal status and stats to the server.
func (u *StatusUploader) UploadStatus(ctx context.Context, runID, status string, reason *string, stats map[string]interface{}) error {
	// Build request payload.
	payload := map[string]interface{}{
		"run_id": runID,
		"status": status,
	}

	if reason != nil {
		payload["reason"] = *reason
	}

	if stats != nil {
		payload["stats"] = stats
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	// Construct URL.
	url := fmt.Sprintf("%s/v1/nodes/%s/complete", u.cfg.ServerURL, u.cfg.NodeID)

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
	if resp.StatusCode != http.StatusNoContent {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed: status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}
