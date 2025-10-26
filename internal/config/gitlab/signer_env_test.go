package gitlab

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

// TestNewSignerFromEnv ensures environment overrides apply expected TTL limits.
func TestNewSignerFromEnv(t *testing.T) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	etcd, client := newTestEtcd(t)
	defer etcd.Close()
	t.Cleanup(func() {
		_ = client.Close()
	})

	key := strings.Repeat("e", 32)
	encodedKey := base64.StdEncoding.EncodeToString([]byte(key))
	t.Setenv("PLOY_GITLAB_SIGNER_AES_KEY", encodedKey)
	t.Setenv("PLOY_GITLAB_SIGNER_DEFAULT_TTL", "6m")
	t.Setenv("PLOY_GITLAB_SIGNER_MAX_TTL", "1h")

	signer, err := NewSignerFromEnv(client)
	if err != nil {
		t.Fatalf("NewSignerFromEnv: %v", err)
	}
	t.Cleanup(func() {
		_ = signer.Close()
	})

	if _, err := signer.RotateSecret(ctx, RotateSecretRequest{
		SecretName: "env-secret",
		APIKey:     "glpat-env",
		Scopes:     []string{"api"},
	}); err != nil {
		t.Fatalf("RotateSecret: %v", err)
	}

	token, err := signer.IssueToken(ctx, IssueTokenRequest{
		SecretName: "env-secret",
		NodeID:     "node-env",
	})
	if err != nil {
		t.Fatalf("IssueToken default ttl: %v", err)
	}

	actualTTL := token.ExpiresAt.Sub(token.IssuedAt)
	expected := 6 * time.Minute
	if delta := actualTTL - expected; delta < -time.Second || delta > time.Second {
		t.Fatalf("expected ttl ~6m, got %s", actualTTL)
	}

	if _, err := signer.IssueToken(ctx, IssueTokenRequest{
		SecretName: "env-secret",
		TTL:        2 * time.Hour,
		NodeID:     "node-env",
	}); err == nil {
		t.Fatalf("expected IssueToken to reject ttl above max")
	}
}
