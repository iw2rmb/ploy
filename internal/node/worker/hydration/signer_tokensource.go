package hydration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// SignerTokenSourceOptions configure the GitLab signer-backed token source.
type SignerTokenSourceOptions struct {
	BaseURL    string
	NodeID     string
	HTTPClient *http.Client
	Secret     string
	Scopes     []string
	TTL        time.Duration
}

// SignerTokenSource issues GitLab tokens via the control-plane signer endpoints.
type SignerTokenSource struct {
	baseURL    string
	nodeID     string
	httpClient *http.Client
	ttl        time.Duration

	mu        sync.Mutex
	secret    string
	scopes    []string
	cached    Token
	cacheTime time.Time
}

// NewSignerTokenSource constructs a token source backed by the GitLab signer API.
func NewSignerTokenSource(opts SignerTokenSourceOptions) (*SignerTokenSource, error) {
	if opts.HTTPClient == nil {
		return nil, fmt.Errorf("hydration: signer token source http client required")
	}
	base := strings.TrimSpace(opts.BaseURL)
	if base == "" {
		return nil, fmt.Errorf("hydration: signer token source base url required")
	}
	node := strings.TrimSpace(opts.NodeID)
	if node == "" {
		return nil, fmt.Errorf("hydration: signer token source node id required")
	}
	ttl := opts.TTL
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	scopes := make([]string, len(opts.Scopes))
	copy(scopes, opts.Scopes)
	return &SignerTokenSource{
		baseURL:    base,
		nodeID:     node,
		httpClient: opts.HTTPClient,
		ttl:        ttl,
		secret:     strings.TrimSpace(opts.Secret),
		scopes:     scopes,
	}, nil
}

// IssueToken acquires or reuses a short-lived GitLab token.
func (s *SignerTokenSource) IssueToken(ctx context.Context, repo contracts.RepoMaterialization) (Token, error) {
	_ = repo
	now := time.Now()
	s.mu.Lock()
	if s.cached.Valid(now) {
		token := s.cached
		s.mu.Unlock()
		return token, nil
	}
	secret := s.secret
	scopes := append([]string(nil), s.scopes...)
	s.mu.Unlock()

	if secret == "" {
		cfg, err := s.fetchConfig(ctx)
		if err != nil {
			return Token{}, err
		}
		secret = cfg.Secret
		scopes = cfg.Scopes
	}
	if len(scopes) == 0 {
		scopes = []string{"read_repository"}
	}

	token, err := s.requestToken(ctx, secret, scopes)
	if err != nil {
		return Token{}, err
	}

	s.mu.Lock()
	s.secret = secret
	s.scopes = scopes
	s.cached = token
	s.cacheTime = now
	s.mu.Unlock()
	return token, nil
}

type gitlabConfigSnapshot struct {
	Secret string
	Scopes []string
}

func (s *SignerTokenSource) fetchConfig(ctx context.Context) (gitlabConfigSnapshot, error) {
	endpoint, err := s.resolve("/v1/config/gitlab")
	if err != nil {
		return gitlabConfigSnapshot{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return gitlabConfigSnapshot{}, err
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return gitlabConfigSnapshot{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return gitlabConfigSnapshot{}, fmt.Errorf("hydration: gitlab config request returned %s", resp.Status)
	}
	var payload struct {
		Config struct {
			DefaultToken struct {
				Name   string   `json:"name"`
				Scopes []string `json:"scopes"`
			} `json:"default_token"`
		} `json:"config"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return gitlabConfigSnapshot{}, fmt.Errorf("hydration: decode gitlab config: %w", err)
	}
	secret := strings.TrimSpace(payload.Config.DefaultToken.Name)
	if secret == "" {
		return gitlabConfigSnapshot{}, errors.New("hydration: gitlab config missing default token name")
	}
	scopes := make([]string, 0, len(payload.Config.DefaultToken.Scopes))
	for _, scope := range payload.Config.DefaultToken.Scopes {
		trimmed := strings.TrimSpace(scope)
		if trimmed != "" {
			scopes = append(scopes, trimmed)
		}
	}
	return gitlabConfigSnapshot{Secret: secret, Scopes: scopes}, nil
}

func (s *SignerTokenSource) requestToken(ctx context.Context, secret string, scopes []string) (Token, error) {
	endpoint, err := s.resolve("/v1/gitlab/signer/tokens")
	if err != nil {
		return Token{}, err
	}
	payload := map[string]any{
		"secret":      secret,
		"scopes":      scopes,
		"ttl_seconds": int64(s.ttl.Seconds()),
		"node_id":     s.nodeID,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Token{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Token{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return Token{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return Token{}, fmt.Errorf("hydration: gitlab token request returned %s", resp.Status)
	}
	var payloadResp struct {
		Token     string   `json:"token"`
		ExpiresAt string   `json:"expires_at"`
		IssuedAt  string   `json:"issued_at"`
		Scopes    []string `json:"scopes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payloadResp); err != nil {
		return Token{}, fmt.Errorf("hydration: decode gitlab token response: %w", err)
	}
	value := strings.TrimSpace(payloadResp.Token)
	if value == "" {
		return Token{}, errors.New("hydration: signer returned empty token")
	}
	expires, err := parseTime(payloadResp.ExpiresAt)
	if err != nil {
		return Token{}, err
	}
	if expires.IsZero() {
		expires = time.Now().Add(s.ttl)
	}
	return Token{Value: value, ExpiresAt: expires}, nil
}

func (s *SignerTokenSource) resolve(pathSuffix string) (string, error) {
	base, err := url.Parse(s.baseURL)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(pathSuffix, "http://") || strings.HasPrefix(pathSuffix, "https://") {
		return pathSuffix, nil
	}
	base.Path = strings.TrimSuffix(base.Path, "/") + pathSuffix
	return base.String(), nil
}

func parseTime(value string) (time.Time, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return time.Time{}, nil
	}
	if ts, err := time.Parse(time.RFC3339Nano, trimmed); err == nil {
		return ts, nil
	}
	if ts, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return ts, nil
	}
	return time.Time{}, fmt.Errorf("hydration: parse timestamp %q", value)
}
