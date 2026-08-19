package api

import (
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// CreateAPITokenRequest is the canonical request for POST /v1/tokens.
type CreateAPITokenRequest struct {
	Role          string `json:"role"`
	Username      string `json:"username,omitempty"`
	Description   string `json:"description,omitempty"`
	ExpiresInDays int    `json:"expires_in_days"`
}

// CreateAPITokenResponse is the canonical response from POST /v1/tokens.
type CreateAPITokenResponse struct {
	Token     string    `json:"token"`
	TokenID   string    `json:"token_id"`
	Role      string    `json:"role"`
	Username  *string   `json:"username,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
	Warning   string    `json:"warning"`
}

// APITokenListItem is one API token in the list response.
type APITokenListItem struct {
	TokenID     string     `json:"token_id"`
	Role        string     `json:"role"`
	Username    *string    `json:"username,omitempty"`
	Description *string    `json:"description,omitempty"`
	IssuedAt    time.Time  `json:"issued_at"`
	ExpiresAt   time.Time  `json:"expires_at"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
	CreatedBy   *string    `json:"created_by,omitempty"`
}

// APITokenListResponse is the canonical response from GET /v1/tokens.
type APITokenListResponse struct {
	Tokens []APITokenListItem `json:"tokens"`
}

// CreateBootstrapTokenRequest is the canonical request for POST /v1/bootstrap/tokens.
type CreateBootstrapTokenRequest struct {
	NodeID           domaintypes.NodeID `json:"node_id"`
	ExpiresInMinutes int                `json:"expires_in_minutes"`
}

// CreateBootstrapTokenResponse is the canonical response from POST /v1/bootstrap/tokens.
type CreateBootstrapTokenResponse struct {
	Token     string             `json:"token"`
	NodeID    domaintypes.NodeID `json:"node_id"`
	ExpiresAt time.Time          `json:"expires_at"`
}
