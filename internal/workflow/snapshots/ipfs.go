package snapshots

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type IPFSGatewayOptions struct {
	Pin        bool
	HTTPClient *http.Client
}

type ipfsGatewayPublisher struct {
	base   *url.URL
	client *http.Client
	opts   IPFSGatewayOptions
}

func NewIPFSGatewayPublisher(endpoint string, opts IPFSGatewayOptions) (ArtifactPublisher, error) {
	trimmed := strings.TrimSpace(endpoint)
	if trimmed == "" {
		return nil, fmt.Errorf("ipfs gateway endpoint required")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("parse ipfs gateway endpoint: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("ipfs gateway endpoint must include scheme and host")
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	base := *parsed
	base.RawQuery = ""
	base.Fragment = ""
	return &ipfsGatewayPublisher{
		base:   &base,
		client: client,
		opts:   opts,
	}, nil
}

func (p *ipfsGatewayPublisher) Publish(ctx context.Context, data []byte) (string, error) {
	if len(data) == 0 {
		return "", fmt.Errorf("artifact payload empty")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "artifact.json")
	if err != nil {
		return "", fmt.Errorf("create multipart form: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return "", fmt.Errorf("write artifact payload: %w", err)
	}
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("finalise multipart payload: %w", err)
	}

	endpoint := p.base.ResolveReference(&url.URL{Path: "/api/v0/add"})
	query := endpoint.Query()
	if p.opts.Pin {
		query.Set("pin", "true")
	}
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), body)
	if err != nil {
		return "", fmt.Errorf("build ipfs publish request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("publish artifact to ipfs: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("publish artifact to ipfs: unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read ipfs response: %w", err)
	}

	cid, err := extractCID(payload)
	if err != nil {
		return "", err
	}
	return cid, nil
}

func extractCID(payload []byte) (string, error) {
	lines := bytes.Split(payload, []byte("\n"))
	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var parsed struct {
			Hash string `json:"Hash"`
			CID  string `json:"Cid"`
		}
		if err := json.Unmarshal(line, &parsed); err != nil {
			continue
		}
		cid := strings.TrimSpace(parsed.Hash)
		if cid == "" {
			cid = strings.TrimSpace(parsed.CID)
		}
		if cid != "" {
			return cid, nil
		}
	}
	trimmed := strings.TrimSpace(string(payload))
	if trimmed == "" {
		trimmed = "<empty>"
	}
	return "", fmt.Errorf("publish artifact to ipfs: response missing cid: %s", trimmed)
}
