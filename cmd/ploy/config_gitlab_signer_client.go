package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type httpGitlabSignerClient struct {
	base *url.URL
	http *http.Client
}

func newHTTPGitlabSignerClient(base *url.URL, httpClient *http.Client) *httpGitlabSignerClient {
	clone := *base
	return &httpGitlabSignerClient{
		base: &clone,
		http: httpClient,
	}
}

func (c *httpGitlabSignerClient) RotateSecret(ctx context.Context, req gitlabRotateSecretRequest) (gitlabRotateSecretResult, error) {
	payload := map[string]any{
		"secret":  strings.TrimSpace(req.Secret),
		"api_key": req.APIKey,
		"scopes":  req.Scopes,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return gitlabRotateSecretResult{}, fmt.Errorf("marshal rotate payload: %w", err)
	}

	endpoint := c.endpoint("/v1/gitlab/signer/secrets", nil)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(body))
	if err != nil {
		return gitlabRotateSecretResult{}, fmt.Errorf("build rotate request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return gitlabRotateSecretResult{}, fmt.Errorf("rotate GitLab secret: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 300 {
		return gitlabRotateSecretResult{}, fmt.Errorf("rotate GitLab secret: %w", controlPlaneHTTPError(resp))
	}

	var response struct {
		Secret    string   `json:"secret"`
		Revision  int64    `json:"revision"`
		UpdatedAt string   `json:"updated_at"`
		Scopes    []string `json:"scopes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return gitlabRotateSecretResult{}, fmt.Errorf("decode rotate response: %w", err)
	}

	return gitlabRotateSecretResult{
		Secret:    strings.TrimSpace(response.Secret),
		Revision:  response.Revision,
		UpdatedAt: parseTimestamp(response.UpdatedAt),
		Scopes:    normalizeScopes(response.Scopes),
	}, nil
}

func (c *httpGitlabSignerClient) Status(ctx context.Context, req gitlabSignerStatusRequest) (gitlabSignerStatus, error) {
	query := url.Values{}
	if trimmed := strings.TrimSpace(req.Secret); trimmed != "" {
		query.Set("secret", trimmed)
	}
	endpoint := c.endpoint("/v1/gitlab/signer/status", query)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return gitlabSignerStatus{}, fmt.Errorf("build signer status request: %w", err)
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return gitlabSignerStatus{}, fmt.Errorf("fetch signer status: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return gitlabSignerStatus{}, errors.New("gitlab signer status endpoint unavailable on control plane")
	}
	if resp.StatusCode >= 300 {
		return gitlabSignerStatus{}, fmt.Errorf("fetch signer status: %w", controlPlaneHTTPError(resp))
	}

	var payload struct {
		FeedRevision int64 `json:"feed_revision"`
		Secrets      []struct {
			Secret    string   `json:"secret"`
			Revision  int64    `json:"revision"`
			UpdatedAt string   `json:"updated_at"`
			Scopes    []string `json:"scopes"`
			Audit     struct {
				LastRotation string `json:"last_rotation"`
				Revoked      []struct {
					NodeID    string `json:"node_id"`
					TokenID   string `json:"token_id"`
					Timestamp string `json:"timestamp"`
				} `json:"revoked"`
				Failed []struct {
					NodeID    string `json:"node_id"`
					TokenID   string `json:"token_id"`
					Timestamp string `json:"timestamp"`
					Error     string `json:"error"`
				} `json:"failed"`
			} `json:"audit"`
		} `json:"secrets"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return gitlabSignerStatus{}, fmt.Errorf("decode signer status: %w", err)
	}

	status := gitlabSignerStatus{
		FeedRevision: payload.FeedRevision,
	}
	for _, secret := range payload.Secrets {
		audit := gitlabSignerAudit{
			LastRotation: parseTimestamp(secret.Audit.LastRotation),
		}
		for _, rev := range secret.Audit.Revoked {
			audit.Revocations = append(audit.Revocations, gitlabSignerRevocation{
				NodeID:    strings.TrimSpace(rev.NodeID),
				TokenID:   strings.TrimSpace(rev.TokenID),
				Timestamp: parseTimestamp(rev.Timestamp),
			})
		}
		for _, fail := range secret.Audit.Failed {
			audit.Failures = append(audit.Failures, gitlabSignerFailure{
				NodeID:    strings.TrimSpace(fail.NodeID),
				TokenID:   strings.TrimSpace(fail.TokenID),
				Timestamp: parseTimestamp(fail.Timestamp),
				Error:     strings.TrimSpace(fail.Error),
			})
		}
		status.Secrets = append(status.Secrets, gitlabSignerSecretStatus{
			Name:      strings.TrimSpace(secret.Secret),
			Revision:  secret.Revision,
			RotatedAt: parseTimestamp(secret.UpdatedAt),
			Scopes:    normalizeScopes(secret.Scopes),
			Audit:     audit,
		})
	}
	return status, nil
}

func (c *httpGitlabSignerClient) endpoint(path string, query url.Values) string {
	u := *c.base
	u.Path = strings.TrimSuffix(u.Path, "/") + path
	if query != nil {
		u.RawQuery = query.Encode()
	} else {
		u.RawQuery = ""
	}
	return u.String()
}
