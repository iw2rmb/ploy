package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "test-secret-key-for-jwt-signing-at-least-32-chars-long"

func TestGenerateAPIToken_Success(t *testing.T) {
	clusterID := "test-cluster"
	role := RoleControlPlane
	expiresAt := time.Now().Add(24 * time.Hour)

	tokenString, err := GenerateAPIToken(testSecret, clusterID, role, expiresAt)
	if err != nil {
		t.Fatalf("GenerateAPIToken error: %v", err)
	}

	if tokenString == "" {
		t.Fatal("expected non-empty token string")
	}

	// Verify token can be parsed
	claims, err := ValidateToken(tokenString, testSecret)
	if err != nil {
		t.Fatalf("ValidateToken error: %v", err)
	}

	if claims.ClusterID != clusterID {
		t.Errorf("ClusterID=%s want %s", claims.ClusterID, clusterID)
	}
	if claims.Role != role {
		t.Errorf("Role=%s want %s", claims.Role, role)
	}
	if claims.TokenType != TokenTypeAPI {
		t.Errorf("TokenType=%s want %s", claims.TokenType, TokenTypeAPI)
	}
}

func TestGenerateAPIToken_ContainsCorrectClaims(t *testing.T) {
	clusterID := "prod-cluster"
	role := RoleCLIAdmin
	expiresAt := time.Now().Add(365 * 24 * time.Hour)

	tokenString, err := GenerateAPIToken(testSecret, clusterID, role, expiresAt)
	if err != nil {
		t.Fatalf("GenerateAPIToken error: %v", err)
	}

	claims, err := ValidateToken(tokenString, testSecret)
	if err != nil {
		t.Fatalf("ValidateToken error: %v", err)
	}

	// Verify all required claims are present
	if claims.ClusterID != clusterID {
		t.Errorf("ClusterID=%s want %s", claims.ClusterID, clusterID)
	}
	if claims.Role != role {
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

	// Verify NodeID is not set for API tokens
	if claims.NodeID != "" {
		t.Errorf("expected NodeID to be empty for API token, got %s", claims.NodeID)
	}

	// Verify expiration is approximately correct (within 1 minute tolerance)
	expiryDiff := claims.ExpiresAt.Time.Sub(expiresAt).Abs()
	if expiryDiff > time.Minute {
		t.Errorf("expiry time differs by %v, expected within 1 minute", expiryDiff)
	}
}

func TestGenerateAPIToken_UniqueTokenIDs(t *testing.T) {
	// Generate multiple tokens and verify they have unique IDs
	clusterID := "test-cluster"
	role := RoleControlPlane
	expiresAt := time.Now().Add(24 * time.Hour)

	tokens := make(map[string]bool)
	for i := 0; i < 10; i++ {
		tokenString, err := GenerateAPIToken(testSecret, clusterID, role, expiresAt)
		if err != nil {
			t.Fatalf("GenerateAPIToken error: %v", err)
		}

		claims, err := ValidateToken(tokenString, testSecret)
		if err != nil {
			t.Fatalf("ValidateToken error: %v", err)
		}

		if tokens[claims.ID] {
			t.Errorf("duplicate token ID: %s", claims.ID)
		}
		tokens[claims.ID] = true
	}
}

func TestGenerateBootstrapToken_Success(t *testing.T) {
	clusterID := "test-cluster"
	nodeID := "node-123"
	expiresAt := time.Now().Add(15 * time.Minute)

	tokenString, err := GenerateBootstrapToken(testSecret, clusterID, nodeID, expiresAt)
	if err != nil {
		t.Fatalf("GenerateBootstrapToken error: %v", err)
	}

	if tokenString == "" {
		t.Fatal("expected non-empty token string")
	}

	// Verify token can be parsed
	claims, err := ValidateToken(tokenString, testSecret)
	if err != nil {
		t.Fatalf("ValidateToken error: %v", err)
	}

	if claims.ClusterID != clusterID {
		t.Errorf("ClusterID=%s want %s", claims.ClusterID, clusterID)
	}
	if claims.TokenType != TokenTypeBootstrap {
		t.Errorf("TokenType=%s want %s", claims.TokenType, TokenTypeBootstrap)
	}
}

func TestGenerateBootstrapToken_IncludesNodeID(t *testing.T) {
	clusterID := "test-cluster"
	nodeID := "550e8400-e29b-41d4-a716-446655440000"
	expiresAt := time.Now().Add(15 * time.Minute)

	tokenString, err := GenerateBootstrapToken(testSecret, clusterID, nodeID, expiresAt)
	if err != nil {
		t.Fatalf("GenerateBootstrapToken error: %v", err)
	}

	claims, err := ValidateToken(tokenString, testSecret)
	if err != nil {
		t.Fatalf("ValidateToken error: %v", err)
	}

	if claims.NodeID != nodeID {
		t.Errorf("NodeID=%s want %s", claims.NodeID, nodeID)
	}

	// Bootstrap tokens should always have worker role
	if claims.Role != RoleWorker {
		t.Errorf("Role=%s want %s", claims.Role, RoleWorker)
	}
}

func TestGenerateBootstrapToken_ShortLived(t *testing.T) {
	clusterID := "test-cluster"
	nodeID := "node-123"
	expiresAt := time.Now().Add(15 * time.Minute)

	tokenString, err := GenerateBootstrapToken(testSecret, clusterID, nodeID, expiresAt)
	if err != nil {
		t.Fatalf("GenerateBootstrapToken error: %v", err)
	}

	claims, err := ValidateToken(tokenString, testSecret)
	if err != nil {
		t.Fatalf("ValidateToken error: %v", err)
	}

	// Verify expiration is within expected range (15 minutes ± 1 minute tolerance)
	expectedDuration := 15 * time.Minute
	actualDuration := claims.ExpiresAt.Time.Sub(claims.IssuedAt.Time)
	diff := actualDuration - expectedDuration
	if diff.Abs() > time.Minute {
		t.Errorf("token duration=%v want approximately %v", actualDuration, expectedDuration)
	}
}

func TestValidateToken_ValidToken(t *testing.T) {
	clusterID := "test-cluster"
	role := RoleControlPlane
	expiresAt := time.Now().Add(24 * time.Hour)

	tokenString, err := GenerateAPIToken(testSecret, clusterID, role, expiresAt)
	if err != nil {
		t.Fatalf("GenerateAPIToken error: %v", err)
	}

	claims, err := ValidateToken(tokenString, testSecret)
	if err != nil {
		t.Errorf("ValidateToken error: %v", err)
	}
	if claims == nil {
		t.Fatal("expected non-nil claims")
	}
}

func TestValidateToken_InvalidSignature(t *testing.T) {
	clusterID := "test-cluster"
	role := RoleControlPlane
	expiresAt := time.Now().Add(24 * time.Hour)

	tokenString, err := GenerateAPIToken(testSecret, clusterID, role, expiresAt)
	if err != nil {
		t.Fatalf("GenerateAPIToken error: %v", err)
	}

	// Try to validate with wrong secret
	wrongSecret := "wrong-secret-key"
	_, err = ValidateToken(tokenString, wrongSecret)
	if err == nil {
		t.Error("expected error for wrong secret, got nil")
	}
	if !strings.Contains(err.Error(), "signature is invalid") {
		t.Errorf("expected signature error, got: %v", err)
	}
}

func TestValidateToken_ExpiredToken(t *testing.T) {
	clusterID := "test-cluster"
	role := RoleControlPlane
	expiresAt := time.Now().Add(-1 * time.Hour) // Already expired

	tokenString, err := GenerateAPIToken(testSecret, clusterID, role, expiresAt)
	if err != nil {
		t.Fatalf("GenerateAPIToken error: %v", err)
	}

	_, err = ValidateToken(tokenString, testSecret)
	if err == nil {
		t.Error("expected error for expired token, got nil")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Errorf("expected expiration error, got: %v", err)
	}
}

func TestValidateToken_MalformedToken(t *testing.T) {
	malformedTokens := []struct {
		name  string
		token string
	}{
		{"empty", ""},
		{"invalid base64", "not-a-valid-jwt-token"},
		{"missing signature", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0"},
		{"corrupted", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.###.###"},
	}

	for _, tt := range malformedTokens {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidateToken(tt.token, testSecret)
			if err == nil {
				t.Errorf("expected error for malformed token, got nil")
			}
		})
	}
}

func TestValidateToken_WrongSecret(t *testing.T) {
	clusterID := "test-cluster"
	role := RoleControlPlane
	expiresAt := time.Now().Add(24 * time.Hour)

	// Generate token with one secret
	secret1 := "secret-one-12345678901234567890"
	tokenString, err := GenerateAPIToken(secret1, clusterID, role, expiresAt)
	if err != nil {
		t.Fatalf("GenerateAPIToken error: %v", err)
	}

	// Try to validate with different secret
	secret2 := "secret-two-12345678901234567890"
	_, err = ValidateToken(tokenString, secret2)
	if err == nil {
		t.Error("expected error for wrong secret, got nil")
	}
}

func TestValidateToken_WrongSigningMethod(t *testing.T) {
	// Manually create a token with RS256 instead of HS256
	claims := &TokenClaims{
		ClusterID: "test-cluster",
		Role:      RoleControlPlane,
		TokenType: TokenTypeAPI,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ID:        "test-id",
		},
	}

	// Create token with wrong signing method (None)
	token := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	tokenString, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("create test token: %v", err)
	}

	// Try to validate - should fail due to wrong signing method
	_, err = ValidateToken(tokenString, testSecret)
	if err == nil {
		t.Error("expected error for wrong signing method, got nil")
	}
}

func TestGenerateTokenID_Uniqueness(t *testing.T) {
	// Generate multiple token IDs and verify uniqueness
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := generateTokenID()
		if ids[id] {
			t.Errorf("duplicate token ID generated: %s", id)
		}
		ids[id] = true

		// Verify ID is properly base64 encoded
		if id == "" {
			t.Error("generateTokenID returned empty string")
		}
		if strings.ContainsAny(id, "+/=") {
			t.Errorf("token ID contains invalid characters (should be URL-safe base64): %s", id)
		}
	}
}

func TestTokenClaims_AllRoles(t *testing.T) {
	roles := []string{RoleControlPlane, RoleWorker, RoleCLIAdmin}
	clusterID := "test-cluster"
	expiresAt := time.Now().Add(24 * time.Hour)

	for _, role := range roles {
		t.Run(role, func(t *testing.T) {
			tokenString, err := GenerateAPIToken(testSecret, clusterID, role, expiresAt)
			if err != nil {
				t.Fatalf("GenerateAPIToken error: %v", err)
			}

			claims, err := ValidateToken(tokenString, testSecret)
			if err != nil {
				t.Fatalf("ValidateToken error: %v", err)
			}

			if claims.Role != role {
				t.Errorf("Role=%s want %s", claims.Role, role)
			}
		})
	}
}
