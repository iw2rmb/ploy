package gitlab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

// ErrInvalidConfig indicates the GitLab configuration failed validation.
var ErrInvalidConfig = errors.New("gitlab config invalid")

const (
	defaultConfigKey = "config/gitlab/settings"
	maskedValue      = "***redacted***"
)

// Config captures the persisted GitLab integration settings.
type Config struct {
	APIBaseURL      string         `json:"api_base_url"`
	AllowedProjects []string       `json:"allowed_projects"`
	DefaultToken    Token          `json:"default_token"`
	DeployTokens    []Token        `json:"deploy_tokens,omitempty"`
	BranchPolicies  []BranchPolicy `json:"branch_policies,omitempty"`
	RBAC            RBAC           `json:"rbac"`
}

// Token represents a GitLab credential with scoped access.
type Token struct {
	Name      string     `json:"name"`
	Value     string     `json:"value"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// BranchPolicy defines branch-level protections enforced during Mods operations.
type BranchPolicy struct {
	Pattern          string `json:"pattern"`
	Protected        bool   `json:"protected,omitempty"`
	RequireApprovals int    `json:"require_approvals,omitempty"`
}

// RBAC enumerates actors permitted to read or update GitLab credentials.
type RBAC struct {
	Readers  []string `json:"readers"`
	Updaters []string `json:"updaters"`
}

// Value represents a stored configuration payload with revision metadata.
type Value struct {
	Data     string
	Revision int64
}

// Store persists and retrieves GitLab configuration from a KV back-end (etcd).
type Store struct {
	kv  KV
	key string
}

// KV abstracts the subset of etcd operations required by the store.
type KV interface {
	Get(ctx context.Context, key string) (Value, bool, error)
	Put(ctx context.Context, key, value string) (int64, error)
}

// NewStore constructs a Store backed by the provided KV.
func NewStore(kv KV, opts ...StoreOption) *Store {
	cfg := storeConfig{key: defaultConfigKey}
	for _, opt := range opts {
		opt(&cfg)
	}
	return &Store{kv: kv, key: cfg.key}
}

// StoreOption mutates store configuration.
type StoreOption func(*storeConfig)

type storeConfig struct {
	key string
}

// WithKey overrides the default configuration key.
func WithKey(key string) StoreOption {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		trimmed = defaultConfigKey
	}
	return func(cfg *storeConfig) {
		cfg.key = trimmed
	}
}

// Load retrieves the current configuration, if present.
func (s *Store) Load(ctx context.Context) (Config, int64, error) {
	if s == nil || s.kv == nil {
		return Config{}, 0, errors.New("gitlab store not initialised")
	}
	val, ok, err := s.kv.Get(ctx, s.key)
	if err != nil {
		return Config{}, 0, fmt.Errorf("load gitlab config: %w", err)
	}
	if !ok {
		return Config{}, 0, nil
	}
	var cfg Config
	if err := json.Unmarshal([]byte(val.Data), &cfg); err != nil {
		return Config{}, 0, fmt.Errorf("decode gitlab config: %w", err)
	}
	normalised, err := Normalize(cfg)
	if err != nil {
		return Config{}, 0, err
	}
	return normalised, val.Revision, nil
}

// Save validates and stores the supplied configuration.
func (s *Store) Save(ctx context.Context, cfg Config) (int64, error) {
	if s == nil || s.kv == nil {
		return 0, errors.New("gitlab store not initialised")
	}
	normalised, err := Normalize(cfg)
	if err != nil {
		return 0, err
	}
	payload, err := json.Marshal(normalised)
	if err != nil {
		return 0, fmt.Errorf("encode gitlab config: %w", err)
	}
	rev, err := s.kv.Put(ctx, s.key, string(payload))
	if err != nil {
		return 0, fmt.Errorf("persist gitlab config: %w", err)
	}
	return rev, nil
}

// Normalize trims, deduplicates, and validates configuration fields.
func Normalize(cfg Config) (Config, error) {
	normalised := cfg
	normalised.APIBaseURL = strings.TrimSpace(normalised.APIBaseURL)
	if normalised.APIBaseURL == "" {
		return Config{}, fmt.Errorf("%w: api_base_url required", ErrInvalidConfig)
	}
	parsed, err := url.Parse(normalised.APIBaseURL)
	if err != nil {
		return Config{}, fmt.Errorf("%w: invalid api_base_url: %v", ErrInvalidConfig, err)
	}
	if parsed.Scheme != "https" {
		return Config{}, fmt.Errorf("%w: api_base_url must use https", ErrInvalidConfig)
	}
	parsed.Path = strings.TrimSuffix(parsed.Path, "/")
	normalised.APIBaseURL = parsed.String()

	normalised.AllowedProjects = cleanList(normalised.AllowedProjects, true)
	if len(normalised.AllowedProjects) == 0 {
		return Config{}, fmt.Errorf("%w: at least one allowed project required", ErrInvalidConfig)
	}

	var tokenErr error
	normalised.DefaultToken, tokenErr = normaliseToken(normalised.DefaultToken, true)
	if tokenErr != nil {
		return Config{}, tokenErr
	}

	deploy, err := normaliseTokens(normalised.DeployTokens, false)
	if err != nil {
		return Config{}, err
	}
	normalised.DeployTokens = deploy

	if err := normalisePolicies(&normalised); err != nil {
		return Config{}, err
	}

	if err := normaliseRBAC(&normalised); err != nil {
		return Config{}, err
	}

	return normalised, nil
}

// Sanitize masks credential values for display.
func (c Config) Sanitize() Config {
	clone := c
	clone.DefaultToken.Value = maskIfPresent(clone.DefaultToken.Value)
	clone.DeployTokens = maskTokens(clone.DeployTokens)
	return clone
}

func maskIfPresent(value string) string {
	if strings.TrimSpace(value) == "" {
		return value
	}
	return maskedValue
}

func maskTokens(tokens []Token) []Token {
	if len(tokens) == 0 {
		return tokens
	}
	masked := make([]Token, len(tokens))
	for i, t := range tokens {
		masked[i] = t
		masked[i].Value = maskIfPresent(t.Value)
	}
	return masked
}

func normaliseToken(token Token, enforceAPIScope bool) (Token, error) {
	trimmed := token
	trimmed.Name = strings.TrimSpace(trimmed.Name)
	trimmed.Value = strings.TrimSpace(trimmed.Value)
	if trimmed.Name == "" {
		trimmed.Name = "default"
	}
	if trimmed.Value == "" {
		return Token{}, fmt.Errorf("%w: token %q value required", ErrInvalidConfig, trimmed.Name)
	}
	scopes := cleanList(trimmed.Scopes, false)
	if len(scopes) == 0 {
		return Token{}, fmt.Errorf("%w: token %q scopes required", ErrInvalidConfig, trimmed.Name)
	}
	if enforceAPIScope && !contains(scopes, "api") {
		return Token{}, fmt.Errorf("%w: default token must include api scope", ErrInvalidConfig)
	}
	trimmed.Scopes = scopes
	return trimmed, nil
}

func normaliseTokens(tokens []Token, requireAPI bool) ([]Token, error) {
	if len(tokens) == 0 {
		return tokens, nil
	}
	out := make([]Token, len(tokens))
	for i, t := range tokens {
		normalised, err := normaliseToken(t, requireAPI)
		if err != nil {
			return nil, err
		}
		out[i] = normalised
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func normalisePolicies(cfg *Config) error {
	if len(cfg.BranchPolicies) == 0 {
		return nil
	}
	policies := make([]BranchPolicy, 0, len(cfg.BranchPolicies))
	for _, p := range cfg.BranchPolicies {
		pattern := strings.TrimSpace(p.Pattern)
		if pattern == "" {
			return fmt.Errorf("%w: branch policy pattern required", ErrInvalidConfig)
		}
		if p.RequireApprovals < 0 {
			return fmt.Errorf("%w: branch policy approvals must be >= 0", ErrInvalidConfig)
		}
		policies = append(policies, BranchPolicy{
			Pattern:          pattern,
			Protected:        p.Protected,
			RequireApprovals: p.RequireApprovals,
		})
	}
	sort.Slice(policies, func(i, j int) bool {
		return policies[i].Pattern < policies[j].Pattern
	})
	cfg.BranchPolicies = policies
	return nil
}

func normaliseRBAC(cfg *Config) error {
	readers := cleanList(cfg.RBAC.Readers, false)
	updaters := cleanList(cfg.RBAC.Updaters, false)
	if len(updaters) == 0 {
		return fmt.Errorf("%w: at least one updater required", ErrInvalidConfig)
	}
	cfg.RBAC.Readers = readers
	cfg.RBAC.Updaters = updaters
	return nil
}

func cleanList(items []string, requireItems bool) []string {
	if len(items) == 0 {
		if requireItems {
			return []string{}
		}
		return nil
	}
	unique := make(map[string]struct{}, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		unique[trimmed] = struct{}{}
	}
	cleaned := make([]string, 0, len(unique))
	for value := range unique {
		cleaned = append(cleaned, value)
	}
	sort.Strings(cleaned)
	if requireItems && len(cleaned) == 0 {
		return []string{}
	}
	return cleaned
}

func contains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
