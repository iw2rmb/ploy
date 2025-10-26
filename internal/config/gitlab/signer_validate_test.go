package gitlab

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestSignerValidateToken confirms claims match the originally issued token.
func TestSignerValidateToken(t *testing.T) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	etcd, client := newTestEtcd(t)
	defer etcd.Close()
	t.Cleanup(func() { _ = client.Close() })

	cipher := mustNewAESCipher(t, strings.Repeat("v", 32))
	signer := mustNewSigner(t, client, cipher)
	t.Cleanup(func() { _ = signer.Close() })

	if _, err := signer.RotateSecret(ctx, RotateSecretRequest{
		SecretName: "deploy",
		APIKey:     "glpat-validate",
		Scopes:     []string{"api", "read_repository"},
	}); err != nil {
		t.Fatalf("RotateSecret: %v", err)
	}

	token, err := signer.IssueToken(ctx, IssueTokenRequest{
		SecretName: "deploy",
		Scopes:     []string{"api"},
		NodeID:     "node-validate",
	})
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	claims, err := signer.ValidateToken(ctx, token.Value)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if claims.SecretName != "deploy" {
		t.Fatalf("unexpected secret: %s", claims.SecretName)
	}
	if claims.TokenID == "" {
		t.Fatalf("expected token id populated")
	}
	if diff := claims.IssuedAt.Sub(token.IssuedAt); diff > time.Second || diff < -time.Second {
		t.Fatalf("expected issued_at within 1s, want %s got %s", token.IssuedAt, claims.IssuedAt)
	}
	if diff := claims.ExpiresAt.Sub(token.ExpiresAt); diff > time.Second || diff < -time.Second {
		t.Fatalf("expected expires_at within 1s, want %s got %s", token.ExpiresAt, claims.ExpiresAt)
	}
	if len(claims.Scopes) != 1 || claims.Scopes[0] != "api" {
		t.Fatalf("unexpected scopes: %+v", claims.Scopes)
	}

	mutation := []byte(token.Value)
	if len(mutation) == 0 {
		t.Fatal("token value empty")
	}
	last := len(mutation) - 1
	if mutation[last] == 'A' {
		mutation[last] = 'B'
	} else {
		mutation[last] = 'A'
	}
	if _, err := signer.ValidateToken(ctx, string(mutation)); err == nil {
		t.Fatalf("expected tampered token validation to fail")
	}
}
