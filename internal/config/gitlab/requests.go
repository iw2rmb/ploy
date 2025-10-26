package gitlab

import "time"

// RotateSecretRequest stores or rotates a GitLab API key.
type RotateSecretRequest struct {
	SecretName string
	APIKey     string
	Scopes     []string
}

// RotateSecretResult returns revision metadata for persisted secrets.
type RotateSecretResult struct {
	Revision  int64
	UpdatedAt time.Time
}

// IssueTokenRequest specifies parameters for a short-lived token.
type IssueTokenRequest struct {
	SecretName string
	Scopes     []string
	TTL        time.Duration
	NodeID     string
}

// SignedToken captures the issued token and metadata.
type SignedToken struct {
	SecretName string
	Value      string
	Scopes     []string
	IssuedAt   time.Time
	ExpiresAt  time.Time
	TokenID    string
}

// TokenClaims captures validated token metadata.
type TokenClaims struct {
	SecretName string
	TokenID    string
	Scopes     []string
	IssuedAt   time.Time
	ExpiresAt  time.Time
}
