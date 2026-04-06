package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

const testSecret = "test-secret-key-for-jwt-signing-at-least-32-chars-long"

// TestGenerateAPIToken_ClaimsAndRoles validates token generation for all roles,
// verifying all required claims are present and correct.
func TestGenerateAPIToken_ClaimsAndRoles(t *testing.T) {
	clusterID := "prod-cluster"
	expiresAt := time.Now().Add(365 * 24 * time.Hour)

	for _, role := range []Role{RoleControlPlane, RoleWorker, RoleCLIAdmin} {
		t.Run(string(role), func(t *testing.T) {
			tokenString, err := GenerateAPIToken(testSecret, clusterID, string(role), expiresAt)
			if err != nil {
				t.Fatalf("GenerateAPIToken error: %v", err)
			}
			if tokenString == "" {
				t.Fatal("expected non-empty token string")
			}

			claims, err := ValidateToken(tokenString, testSecret)
			if err != nil {
				t.Fatalf("ValidateToken error: %v", err)
			}

			if claims.ClusterID.String() != clusterID {
				t.Errorf("ClusterID=%s want %s", claims.ClusterID, clusterID)
			}
			if claims.Role != string(role) {
				t.Errorf("Role=%s want %s", claims.Role, role)
			}
			if claims.TokenType != TokenTypeAPI {
				t.Errorf("TokenType=%s want %s", claims.TokenType, TokenTypeAPI)
			}
			if claims.ID == "" {
				t.Error("expected token ID to be set")
			}
			if claims.IssuedAt == nil {
				t.Error("expected IssuedAt to be set")
			}
			if claims.ExpiresAt == nil {
				t.Error("expected ExpiresAt to be set")
			}
			if !claims.NodeID.IsZero() {
				t.Errorf("expected NodeID to be empty for API token, got %s", claims.NodeID.String())
			}
			expiryDiff := claims.ExpiresAt.Time.Sub(expiresAt).Abs()
			if expiryDiff > time.Minute {
				t.Errorf("expiry time differs by %v, expected within 1 minute", expiryDiff)
			}
		})
	}
}

func TestGenerateAPIToken_UniqueTokenIDs(t *testing.T) {
	expiresAt := time.Now().Add(24 * time.Hour)
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		tokenString, err := GenerateAPIToken(testSecret, "test-cluster", string(RoleControlPlane), expiresAt)
		if err != nil {
			t.Fatalf("GenerateAPIToken error: %v", err)
		}
		claims, err := ValidateToken(tokenString, testSecret)
		if err != nil {
			t.Fatalf("ValidateToken error: %v", err)
		}
		if ids[claims.ID] {
			t.Errorf("duplicate token ID: %s", claims.ID)
		}
		ids[claims.ID] = true
		if claims.ID == "" {
			t.Error("generateTokenID returned empty string")
		}
		if strings.ContainsAny(claims.ID, "+/=") {
			t.Errorf("token ID contains invalid characters (should be URL-safe base64): %s", claims.ID)
		}
	}
}

func TestGenerateBootstrapToken(t *testing.T) {
	clusterID := "test-cluster"
	nodeID := domaintypes.NodeID(domaintypes.NewNodeKey())
	expiresAt := time.Now().Add(15 * time.Minute)

	tokenString, err := GenerateBootstrapToken(testSecret, clusterID, nodeID, expiresAt)
	if err != nil {
		t.Fatalf("GenerateBootstrapToken error: %v", err)
	}
	if tokenString == "" {
		t.Fatal("expected non-empty token string")
	}

	claims, err := ValidateToken(tokenString, testSecret)
	if err != nil {
		t.Fatalf("ValidateToken error: %v", err)
	}

	if claims.ClusterID.String() != clusterID {
		t.Errorf("ClusterID=%s want %s", claims.ClusterID, clusterID)
	}
	if claims.TokenType != TokenTypeBootstrap {
		t.Errorf("TokenType=%s want %s", claims.TokenType, TokenTypeBootstrap)
	}
	if claims.NodeID != nodeID {
		t.Errorf("NodeID=%s want %s", claims.NodeID, nodeID)
	}
	if claims.Role != string(RoleWorker) {
		t.Errorf("Role=%s want %s", claims.Role, RoleWorker)
	}

	// Verify expiration is within expected range (15 minutes +/- 1 minute tolerance).
	actualDuration := claims.ExpiresAt.Sub(claims.IssuedAt.Time)
	diff := actualDuration - 15*time.Minute
	if diff.Abs() > time.Minute {
		t.Errorf("token duration=%v want approximately 15m", actualDuration)
	}
}

func TestValidateToken_Errors(t *testing.T) {
	tests := []struct {
		name      string
		token     string
		secret    string
		wantSubst string // substring expected in error message, empty = any error
	}{
		{"empty", "", testSecret, ""},
		{"invalid base64", "not-a-valid-jwt-token", testSecret, ""},
		{"missing signature", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0", testSecret, ""},
		{"corrupted", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.###.###", testSecret, ""},
		{"wrong secret", "", "secret-two-12345678901234567890", "signature is invalid"},
		{"expired token", "", testSecret, "expired"},
	}

	// Pre-generate tokens for the secret/expiry cases.
	validToken, err := GenerateAPIToken(testSecret, "test-cluster", string(RoleControlPlane), time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("GenerateAPIToken error: %v", err)
	}
	expiredToken, err := GenerateAPIToken(testSecret, "test-cluster", string(RoleControlPlane), time.Now().Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("GenerateAPIToken error: %v", err)
	}
	// Token signed with a different secret.
	wrongSecretToken, err := GenerateAPIToken("secret-one-12345678901234567890", "test-cluster", string(RoleControlPlane), time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("GenerateAPIToken error: %v", err)
	}

	// Fill in pre-generated tokens.
	tests[4].token = wrongSecretToken
	tests[5].token = expiredToken

	// Also ensure a valid token parses fine.
	claims, err := ValidateToken(validToken, testSecret)
	if err != nil {
		t.Fatalf("ValidateToken error on valid token: %v", err)
	}
	if claims == nil {
		t.Fatal("expected non-nil claims")
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidateToken(tt.token, tt.secret)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if tt.wantSubst != "" && !strings.Contains(err.Error(), tt.wantSubst) {
				t.Errorf("expected error containing %q, got: %v", tt.wantSubst, err)
			}
		})
	}
}

func TestValidateToken_WrongSigningMethod(t *testing.T) {
	claims := &TokenClaims{
		ClusterID: "test-cluster",
		Role:      string(RoleControlPlane),
		TokenType: TokenTypeAPI,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ID:        "test-id",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	tokenString, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("create test token: %v", err)
	}

	if _, err = ValidateToken(tokenString, testSecret); err == nil {
		t.Error("expected error for wrong signing method, got nil")
	}
}
