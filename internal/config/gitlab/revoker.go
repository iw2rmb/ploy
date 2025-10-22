package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
)

type httpTokenRevoker struct {
	baseURL string
	client  *http.Client
	token   string
}

// NewHTTPTokenRevoker constructs a GitLab API-backed token revoker.
func NewHTTPTokenRevoker(baseURL, adminToken string, client *http.Client) TokenRevoker {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if client == nil {
		client = http.DefaultClient
	}
	return &httpTokenRevoker{
		baseURL: trimmed,
		client:  client,
		token:   strings.TrimSpace(adminToken),
	}
}

func (r *httpTokenRevoker) Revoke(ctx context.Context, secret string, tokens []RevocableToken) RevocationReport {
	report := RevocationReport{}
	if len(tokens) == 0 {
		return report
	}

	ids := make([]string, len(tokens))
	for i, tok := range tokens {
		ids[i] = tok.ID
	}

	if len(ids) > 1 && r.token != "" && r.baseURL != "" {
		if err := r.postBulk(ctx, ids); err == nil {
			report.Revoked = append(report.Revoked, tokens...)
			return report
		}
	}

	for _, tok := range tokens {
		err := r.postSingle(ctx, tok.ID)
		if err != nil {
			report.Failed = append(report.Failed, RevocationFailure{Token: tok, Err: err})
			continue
		}
		report.Revoked = append(report.Revoked, tok)
	}
	return report
}

func (r *httpTokenRevoker) postBulk(ctx context.Context, ids []string) error {
	if r.baseURL == "" || r.token == "" {
		return fmt.Errorf("gitlab revoker: bulk revocation requires base url and admin token")
	}
	payload := map[string][]string{"token_ids": ids}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	endpoint := r.baseURL + "/api/v4/personal_access_tokens/revoke"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("PRIVATE-TOKEN", r.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	data, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("gitlab revoker: bulk revoke %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
}

func (r *httpTokenRevoker) postSingle(ctx context.Context, id string) error {
	if id == "" || r.baseURL == "" || r.token == "" {
		return fmt.Errorf("gitlab revoker: invalid single revoke parameters")
	}
	endpoint := r.baseURL + path.Join("/api/v4/personal_access_tokens/", id, "revoke")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("PRIVATE-TOKEN", r.token)

	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	data, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("gitlab revoker: revoke %s %d: %s", id, resp.StatusCode, strings.TrimSpace(string(data)))
}
