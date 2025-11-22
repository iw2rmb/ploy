package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// Token types
const (
	TokenTypeAPI       = "api"       // Long-lived API tokens for CLI
	TokenTypeBootstrap = "bootstrap" // Short-lived tokens for node bootstrapping
)

// JWT Claims structure with domain types for cluster identifier.
// ClusterID is validated on unmarshal (non-empty after trimming spaces).
type TokenClaims struct {
	ClusterID domaintypes.ClusterID `json:"cluster_id"`
	Role      string                `json:"role"`              // "cli-admin", "control-plane", "worker"
	TokenType string                `json:"token_type"`        // "api" or "bootstrap"
	NodeID    string                `json:"node_id,omitempty"` // Only for bootstrap tokens
	jwt.RegisteredClaims
}

// GenerateAPIToken creates a long-lived bearer token for CLI usage.
// clusterID is passed as a string and converted to domain type for validation.
func GenerateAPIToken(secret, clusterID, role string, expiresAt time.Time) (string, error) {
	claims := &TokenClaims{
		ClusterID: domaintypes.ClusterID(clusterID),
		Role:      role,
		TokenType: TokenTypeAPI,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ID:        generateTokenID(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// GenerateBootstrapToken creates a short-lived token for node bootstrapping.
// clusterID is passed as a string and converted to domain type for validation.
func GenerateBootstrapToken(secret, clusterID, nodeID string, expiresAt time.Time) (string, error) {
	claims := &TokenClaims{
		ClusterID: domaintypes.ClusterID(clusterID),
		Role:      RoleWorker,
		TokenType: TokenTypeBootstrap,
		NodeID:    nodeID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ID:        generateTokenID(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// ValidateToken verifies and parses a JWT token
func ValidateToken(tokenString, secret string) (*TokenClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &TokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	})

	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}

	claims, ok := token.Claims.(*TokenClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	return claims, nil
}

func generateTokenID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
