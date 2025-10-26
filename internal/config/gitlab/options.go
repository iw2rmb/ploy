package gitlab

import (
	"strings"
	"time"
)

// SignerOption customises signer configuration.
type SignerOption func(*signerConfig)

type signerConfig struct {
	prefix     string
	defaultTTL time.Duration
	maxTTL     time.Duration
	now        func() time.Time
	revoker    TokenRevoker
	audit      AuditRecorder
}

// WithPrefix overrides the etcd prefix used to store secrets.
func WithPrefix(prefix string) SignerOption {
	return func(cfg *signerConfig) {
		cfg.prefix = strings.TrimSpace(prefix)
	}
}

// WithDefaultTTL overrides the default token TTL when not supplied by callers.
func WithDefaultTTL(ttl time.Duration) SignerOption {
	return func(cfg *signerConfig) {
		if ttl > 0 {
			cfg.defaultTTL = ttl
		}
	}
}

// WithMaxTTL overrides the maximum TTL allowed for issued tokens.
func WithMaxTTL(ttl time.Duration) SignerOption {
	return func(cfg *signerConfig) {
		if ttl > 0 {
			cfg.maxTTL = ttl
		}
	}
}

// WithNow injects a custom clock for testing.
func WithNow(now func() time.Time) SignerOption {
	return func(cfg *signerConfig) {
		if now != nil {
			cfg.now = now
		}
	}
}

// WithTokenRevoker overrides the GitLab token revoker used during rotations.
func WithTokenRevoker(revoker TokenRevoker) SignerOption {
	return func(cfg *signerConfig) {
		if revoker != nil {
			cfg.revoker = revoker
		}
	}
}

// WithAuditRecorder overrides the audit recorder used for rotation events.
func WithAuditRecorder(recorder AuditRecorder) SignerOption {
	return func(cfg *signerConfig) {
		if recorder != nil {
			cfg.audit = recorder
		}
	}
}
