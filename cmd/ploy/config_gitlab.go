package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	cfgstore "github.com/iw2rmb/ploy/internal/cli/config"
	gitlabcfg "github.com/iw2rmb/ploy/internal/config/gitlab"
)

type gitlabStore interface {
	Load(context.Context) (gitlabcfg.Config, int64, error)
	Save(context.Context, gitlabcfg.Config) (int64, error)
}

type gitlabStoreCloser interface {
	gitlabStore
	Close() error
}

type gitlabSignerClient interface {
	Status(ctx context.Context, req gitlabSignerStatusRequest) (gitlabSignerStatus, error)
	RotateSecret(ctx context.Context, req gitlabRotateSecretRequest) (gitlabRotateSecretResult, error)
}

type gitlabSignerStatusRequest struct {
	Secret string
}

type gitlabSignerStatus struct {
	FeedRevision int64
	Secrets      []gitlabSignerSecretStatus
}

type gitlabSignerSecretStatus struct {
	Name      string
	Revision  int64
	RotatedAt time.Time
	Scopes    []string
	Audit     gitlabSignerAudit
}

type gitlabSignerAudit struct {
	LastRotation time.Time
	Revocations  []gitlabSignerRevocation
	Failures     []gitlabSignerFailure
}

type gitlabSignerRevocation struct {
	NodeID    string
	TokenID   string
	Timestamp time.Time
}

type gitlabSignerFailure struct {
	NodeID    string
	TokenID   string
	Timestamp time.Time
	Error     string
}

type gitlabRotateSecretRequest struct {
	Secret string
	APIKey string
	Scopes []string
}

type gitlabRotateSecretResult struct {
	Secret    string
	Revision  int64
	UpdatedAt time.Time
	Scopes    []string
}

const (
	controlPlaneURLEnv   = "PLOY_CONTROL_PLANE_URL"
	defaultGitlabTimeout = 10 * time.Second
)

var gitlabConfigStoreFactory = func(ctx context.Context) (gitlabStore, error) {
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return nil, err
	}
	return newHTTPGitlabConfigStore(base, httpClient), nil
}

var gitlabSignerClientFactory = func(ctx context.Context) (gitlabSignerClient, error) {
	base, httpClient, err := resolveControlPlaneHTTP(ctx)
	if err != nil {
		return nil, err
	}
	return newHTTPGitlabSignerClient(base, httpClient), nil
}

func handleConfig(args []string, stderr io.Writer) error {
	if len(args) == 0 {
		printConfigUsage(stderr)
		return errors.New("config subcommand required")
	}

	switch args[0] {
	case "gitlab":
		return handleConfigGitlab(args[1:], stderr)
	default:
		printConfigUsage(stderr)
		return fmt.Errorf("unknown config command %q", args[0])
	}
}

func handleConfigGitlab(args []string, stderr io.Writer) error {
	if len(args) == 0 {
		printConfigGitlabUsage(stderr)
		return errors.New("gitlab subcommand required")
	}
	switch args[0] {
	case "show":
		return runGitlabShow(stderr)
	case "set":
		return runGitlabSet(args[1:], stderr)
	case "validate":
		return runGitlabValidate(args[1:], stderr)
	case "status":
		return runGitlabStatus(args[1:], stderr)
	case "rotate":
		return runGitlabRotate(args[1:], stderr)
	default:
		printConfigGitlabUsage(stderr)
		return fmt.Errorf("unknown gitlab subcommand %q", args[0])
	}
}

func runGitlabShow(stderr io.Writer) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultGitlabTimeout)
	defer cancel()
	store, err := gitlabConfigStoreFactory(ctx)
	if err != nil {
		return err
	}
	defer closeGitlabStore(store)
	cfg, revision, err := store.Load(ctx)
	if err != nil {
		return err
	}
	if revision == 0 {
		_, _ = fmt.Fprintln(stderr, "GitLab configuration not set")
		return nil
	}
	sanitized := cfg.Sanitize()
	lines := []string{
		fmt.Sprintf("GitLab configuration revision %d", revision),
		fmt.Sprintf("API base URL: %s", sanitized.APIBaseURL),
		fmt.Sprintf("Allowed projects: %s", strings.Join(sanitized.AllowedProjects, ", ")),
	}
	scopeLine := fmt.Sprintf("Default token scopes: %s", strings.Join(sanitized.DefaultToken.Scopes, ", "))
	lines = append(lines, scopeLine)
	if sanitized.DefaultToken.ExpiresAt != nil {
		lines = append(lines, fmt.Sprintf("Default token expires: %s", sanitized.DefaultToken.ExpiresAt.Format(time.RFC3339)))
	}
	if len(sanitized.RBAC.Readers) > 0 {
		lines = append(lines, fmt.Sprintf("RBAC readers: %s", strings.Join(sanitized.RBAC.Readers, ", ")))
	}
	lines = append(lines, fmt.Sprintf("RBAC updaters: %s", strings.Join(sanitized.RBAC.Updaters, ", ")))
	for _, line := range lines {
		_, _ = fmt.Fprintln(stderr, line)
	}
	return nil
}

func runGitlabSet(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("config gitlab set", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	file := fs.String("file", "", "path to GitLab configuration JSON file")
	if err := fs.Parse(args); err != nil {
		printConfigGitlabUsage(stderr)
		return err
	}
	path := strings.TrimSpace(*file)
	if path == "" {
		printConfigGitlabUsage(stderr)
		return errors.New("config gitlab set requires --file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config file: %w", err)
	}
	var cfg gitlabcfg.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("decode config: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), defaultGitlabTimeout)
	defer cancel()
	store, err := gitlabConfigStoreFactory(ctx)
	if err != nil {
		return err
	}
	defer closeGitlabStore(store)
	revision, err := store.Save(ctx, cfg)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stderr, "GitLab configuration updated (revision %d)\n", revision)
	return nil
}

func runGitlabValidate(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("config gitlab validate", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	file := fs.String("file", "", "path to GitLab configuration JSON file")
	if err := fs.Parse(args); err != nil {
		printConfigGitlabUsage(stderr)
		return err
	}
	path := strings.TrimSpace(*file)
	if path == "" {
		printConfigGitlabUsage(stderr)
		return errors.New("config gitlab validate requires --file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config file: %w", err)
	}
	var cfg gitlabcfg.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("decode config: %w", err)
	}
	if _, err := gitlabcfg.Normalize(cfg); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(stderr, "GitLab configuration is valid")
	return nil
}

func runGitlabStatus(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("config gitlab status", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	secret := fs.String("secret", "", "optional secret name to filter")
	limit := fs.Int("limit", 10, "maximum recent audit events to display per category")
	if err := fs.Parse(args); err != nil {
		printConfigGitlabUsage(stderr)
		return err
	}
	if *limit < 0 {
		printConfigGitlabUsage(stderr)
		return errors.New("limit must be non-negative")
	}
	ctx, cancel := context.WithTimeout(context.Background(), defaultGitlabTimeout)
	defer cancel()
	client, err := gitlabSignerClientFactory(ctx)
	if err != nil {
		return err
	}
	defer closeGitlabSignerClient(client)
	status, err := client.Status(ctx, gitlabSignerStatusRequest{Secret: strings.TrimSpace(*secret)})
	if err != nil {
		return err
	}
	printGitlabSignerStatus(stderr, status, *limit)
	return nil
}

func runGitlabRotate(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("config gitlab rotate", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	secret := fs.String("secret", "", "GitLab secret name to rotate")
	apiKey := fs.String("api-key", "", "GitLab API key (personal access token)")
	var scopeValues multiScopeFlag
	fs.Var(&scopeValues, "scope", "GitLab token scope (repeatable)")
	scopesCSV := fs.String("scopes", "", "comma-separated GitLab token scopes")
	if err := fs.Parse(args); err != nil {
		printConfigGitlabUsage(stderr)
		return err
	}

	trimmedSecret := strings.TrimSpace(*secret)
	trimmedKey := strings.TrimSpace(*apiKey)
	if trimmedSecret == "" || trimmedKey == "" {
		printConfigGitlabUsage(stderr)
		return errors.New("config gitlab rotate requires --secret and --api-key")
	}

	scopes := scopeValues.Values()
	if csv := strings.TrimSpace(*scopesCSV); csv != "" {
		for _, part := range strings.Split(csv, ",") {
			if trimmed := strings.TrimSpace(part); trimmed != "" {
				scopes = append(scopes, trimmed)
			}
		}
	}
	scopes = uniqueStrings(normalizeScopes(scopes))
	if len(scopes) == 0 {
		printConfigGitlabUsage(stderr)
		return errors.New("config gitlab rotate requires at least one --scope")
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultGitlabTimeout)
	defer cancel()
	client, err := gitlabSignerClientFactory(ctx)
	if err != nil {
		return err
	}
	defer closeGitlabSignerClient(client)

	result, err := client.RotateSecret(ctx, gitlabRotateSecretRequest{
		Secret: trimmedSecret,
		APIKey: trimmedKey,
		Scopes: scopes,
	})
	if err != nil {
		return err
	}

	secretLabel := trimmedSecret
	if result.Secret != "" {
		secretLabel = result.Secret
	}
	_, _ = fmt.Fprintf(stderr, "GitLab secret %s rotated (revision %d)\n", secretLabel, result.Revision)
	if !result.UpdatedAt.IsZero() {
		_, _ = fmt.Fprintf(stderr, "  Updated at: %s\n", result.UpdatedAt.UTC().Format(time.RFC3339))
	}
	if len(result.Scopes) == 0 {
		result.Scopes = scopes
	}
	if len(result.Scopes) > 0 {
		_, _ = fmt.Fprintf(stderr, "  Scopes: %s\n", strings.Join(result.Scopes, ", "))
	}
	return nil
}

func printConfigGitlabUsage(w io.Writer) {
	printCommandUsage(w, "config", "gitlab")
}

func printGitlabSignerStatus(w io.Writer, status gitlabSignerStatus, limit int) {
	if limit == 0 {
		limit = -1
	}
	_, _ = fmt.Fprintln(w, "GitLab signer status")
	if status.FeedRevision > 0 {
		_, _ = fmt.Fprintf(w, "Audit feed revision: %d\n", status.FeedRevision)
	}
	if len(status.Secrets) == 0 {
		_, _ = fmt.Fprintln(w, "No GitLab secrets managed by the signer.")
		return
	}

	secrets := append([]gitlabSignerSecretStatus(nil), status.Secrets...)
	sort.Slice(secrets, func(i, j int) bool {
		return strings.ToLower(secrets[i].Name) < strings.ToLower(secrets[j].Name)
	})

	for _, secret := range secrets {
		_, _ = fmt.Fprintf(w, "\nSecret: %s\n", secret.Name)
		if secret.Revision > 0 {
			_, _ = fmt.Fprintf(w, "  Revision: %d\n", secret.Revision)
		}
		if !secret.RotatedAt.IsZero() {
			_, _ = fmt.Fprintf(w, "  Rotated at: %s\n", secret.RotatedAt.UTC().Format(time.RFC3339))
		}
		if len(secret.Scopes) > 0 {
			_, _ = fmt.Fprintf(w, "  Scopes: %s\n", strings.Join(secret.Scopes, ", "))
		}
		printSignerAudit(w, secret.Audit, limit)
	}
}

func printSignerAudit(w io.Writer, audit gitlabSignerAudit, limit int) {
	_, _ = fmt.Fprintln(w, "  Audit:")
	if !audit.LastRotation.IsZero() {
		_, _ = fmt.Fprintf(w, "    Last rotation: %s\n", audit.LastRotation.UTC().Format(time.RFC3339))
	}

	revoked := limitEntries(audit.Revocations, limit)
	if len(revoked) == 0 {
		_, _ = fmt.Fprintln(w, "    Revoked nodes: none recorded")
	} else {
		_, _ = fmt.Fprintln(w, "    Revoked nodes:")
		for _, entry := range revoked {
			ts := ""
			if !entry.Timestamp.IsZero() {
				ts = entry.Timestamp.UTC().Format(time.RFC3339)
			}
			_, _ = fmt.Fprintf(w, "      - %s (token=%s%s)\n", entry.NodeID, entry.TokenID, formatTimestampSuffix(ts))
		}
	}

	failures := limitEntries(audit.Failures, limit)
	if len(failures) == 0 {
		_, _ = fmt.Fprintln(w, "    Revocation failures: none recorded")
	} else {
		_, _ = fmt.Fprintln(w, "    Revocation failures:")
		for _, entry := range failures {
			ts := ""
			if !entry.Timestamp.IsZero() {
				ts = entry.Timestamp.UTC().Format(time.RFC3339)
			}
			errMsg := strings.TrimSpace(entry.Error)
			if errMsg == "" {
				errMsg = "unknown error"
			}
			_, _ = fmt.Fprintf(w, "      - %s (token=%s, error=%s%s)\n", entry.NodeID, entry.TokenID, errMsg, formatTimestampSuffix(ts))
		}
	}
}

func formatTimestampSuffix(ts string) string {
	if ts == "" {
		return ""
	}
	return ", ts=" + ts
}

func closeGitlabSignerClient(client gitlabSignerClient) {
	if closer, ok := client.(interface{ Close() error }); ok && closer != nil {
		_ = closer.Close()
	}
}

type multiScopeFlag struct {
	values []string
}

func (f *multiScopeFlag) String() string {
	return strings.Join(f.values, ",")
}

func (f *multiScopeFlag) Set(value string) error {
	parts := strings.Split(value, ",")
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			f.values = append(f.values, trimmed)
		}
	}
	return nil
}

func (f *multiScopeFlag) Values() []string {
	return append([]string(nil), f.values...)
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func normalizeScopes(scopes []string) []string {
	if len(scopes) == 0 {
		return nil
	}
	result := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		trimmed := strings.TrimSpace(scope)
		if trimmed == "" {
			continue
		}
		result = append(result, trimmed)
	}
	return result
}

func limitEntries[T any](items []T, limit int) []T {
	if limit < 0 || limit >= len(items) {
		return items
	}
	return items[:limit]
}

func resolveControlPlaneHTTP(ctx context.Context) (*url.URL, *http.Client, error) {
	var lastErr error
	endpoint := strings.TrimSpace(os.Getenv(controlPlaneURLEnv))
	var descriptor *cfgstore.Descriptor

	if endpoint == "" {
		cfg, err := resolveIntegrationConfig(ctx)
		if err == nil {
			if trimmed := strings.TrimSpace(cfg.APIEndpoint); trimmed != "" {
				endpoint = trimmed
			}
		} else if !errors.Is(err, errGridClientDisabled) {
			lastErr = err
		}
	}

	if endpoint == "" {
		desc, ok, err := defaultClusterDescriptor()
		if err != nil {
			if lastErr == nil {
				lastErr = err
			}
		} else if ok {
			descriptor = &desc
			if trimmed := strings.TrimSpace(desc.ControlPlaneURL); trimmed != "" {
				endpoint = trimmed
			} else if trimmed := strings.TrimSpace(desc.BeaconURL); trimmed != "" {
				endpoint = trimmed
			}
		}
	}

	if endpoint == "" {
		if lastErr != nil {
			return nil, nil, lastErr
		}
		return nil, nil, errors.New("control plane endpoint not configured; set PLOY_CONTROL_PLANE_URL or connect to a cluster descriptor with a control plane URL")
	}

	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, nil, fmt.Errorf("parse control plane url: %w", err)
	}
	if parsed.Scheme == "" {
		parsed.Scheme = "https"
	}

	httpClient, err := newControlPlaneHTTPClient(parsed, descriptor)
	if err != nil {
		return nil, nil, err
	}
	return parsed, httpClient, nil
}

func defaultClusterDescriptor() (cfgstore.Descriptor, bool, error) {
	descs, err := cfgstore.ListDescriptors()
	if err != nil {
		return cfgstore.Descriptor{}, false, err
	}
	if len(descs) == 0 {
		return cfgstore.Descriptor{}, false, nil
	}
	for _, desc := range descs {
		if desc.Default {
			return desc, true, nil
		}
	}
	if len(descs) == 1 {
		return descs[0], true, nil
	}
	return cfgstore.Descriptor{}, false, errors.New("multiple cluster descriptors found without a default; designate one via 'ploy cluster connect' before using GitLab signer commands")
}

func newControlPlaneHTTPClient(base *url.URL, desc *cfgstore.Descriptor) (*http.Client, error) {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
	}
	if strings.EqualFold(base.Scheme, "https") {
		tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
		if desc != nil {
			if caPath := strings.TrimSpace(desc.CABundlePath); caPath != "" {
				data, err := os.ReadFile(caPath)
				if err != nil {
					return nil, fmt.Errorf("read control plane CA bundle: %w", err)
				}
				pool := x509.NewCertPool()
				if !pool.AppendCertsFromPEM(data) {
					return nil, errors.New("parse control plane CA bundle")
				}
				tlsCfg.RootCAs = pool
			}
		}
		transport.TLSClientConfig = tlsCfg
	}
	client := &http.Client{
		Timeout:   15 * time.Second,
		Transport: transport,
	}
	return client, nil
}

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

func parseTimestamp(value string) time.Time {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return time.Time{}
	}
	if ts, err := time.Parse(time.RFC3339Nano, trimmed); err == nil {
		return ts
	}
	if ts, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return ts
	}
	return time.Time{}
}

func controlPlaneHTTPError(resp *http.Response) error {
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("control plane responded with %s", resp.Status)
	}
	var payload struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &payload); err == nil {
			if msg := strings.TrimSpace(payload.Message); msg != "" {
				return errors.New(msg)
			}
			if msg := strings.TrimSpace(payload.Error); msg != "" {
				return errors.New(msg)
			}
		}
		if msg := strings.TrimSpace(string(data)); msg != "" {
			return errors.New(msg)
		}
	}
	return fmt.Errorf("control plane responded with %s", resp.Status)
}

func closeGitlabStore(store gitlabStore) {
	if closer, ok := store.(gitlabStoreCloser); ok {
		_ = closer.Close()
	}
}

type httpGitlabConfigStore struct {
	base *url.URL
	http *http.Client
}

func newHTTPGitlabConfigStore(base *url.URL, httpClient *http.Client) *httpGitlabConfigStore {
	clone := *base
	return &httpGitlabConfigStore{base: &clone, http: httpClient}
}

func (s *httpGitlabConfigStore) Close() error { return nil }

func (s *httpGitlabConfigStore) Load(ctx context.Context) (gitlabcfg.Config, int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.endpoint("/v1/config/gitlab"), nil)
	if err != nil {
		return gitlabcfg.Config{}, 0, fmt.Errorf("build gitlab config request: %w", err)
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return gitlabcfg.Config{}, 0, fmt.Errorf("fetch gitlab config: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return gitlabcfg.Config{}, 0, nil
	}
	if resp.StatusCode != http.StatusOK {
		return gitlabcfg.Config{}, 0, controlPlaneHTTPError(resp)
	}

	var payload struct {
		Config   gitlabcfg.Config `json:"config"`
		Revision int64            `json:"revision"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return gitlabcfg.Config{}, 0, fmt.Errorf("decode gitlab config response: %w", err)
	}
	return payload.Config, payload.Revision, nil
}

func (s *httpGitlabConfigStore) Save(ctx context.Context, cfg gitlabcfg.Config) (int64, error) {
	_, revision, err := s.Load(ctx)
	if err != nil {
		return 0, err
	}

	request := map[string]any{
		"revision": revision,
		"config":   cfg,
	}
	body, err := json.Marshal(request)
	if err != nil {
		return 0, fmt.Errorf("marshal gitlab config: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, s.endpoint("/v1/config/gitlab"), bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("build gitlab config update request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("update gitlab config: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, controlPlaneHTTPError(resp)
	}

	var payload struct {
		Revision int64 `json:"revision"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return 0, fmt.Errorf("decode gitlab config update response: %w", err)
	}
	return payload.Revision, nil
}

func (s *httpGitlabConfigStore) endpoint(path string) string {
	u := *s.base
	u.Path = strings.TrimSuffix(u.Path, "/") + path
	u.RawQuery = ""
	return u.String()
}
