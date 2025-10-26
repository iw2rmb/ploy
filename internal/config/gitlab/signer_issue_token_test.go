package gitlab

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestSignerRotateAndIssueToken exercises the happy-path rotation and issuance flow.
func TestSignerRotateAndIssueToken(t *testing.T) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	etcd, client := newTestEtcd(t)
	defer etcd.Close()
	t.Cleanup(func() {
		_ = client.Close()
	})

	now := time.Date(2025, time.October, 21, 15, 0, 0, 0, time.UTC)
	cipher := mustNewAESCipher(t, strings.Repeat("k", 32))
	signer := mustNewSigner(t, client, cipher,
		WithNow(func() time.Time { return now }),
		WithDefaultTTL(15*time.Minute),
	)
	t.Cleanup(func() {
		_ = signer.Close()
	})

	if _, err := signer.RotateSecret(ctx, RotateSecretRequest{
		SecretName: "runner-api",
		APIKey:     "glpat-example-123",
		Scopes:     []string{"api", "read_repository"},
	}); err != nil {
		t.Fatalf("RotateSecret: %v", err)
	}

	token, err := signer.IssueToken(ctx, IssueTokenRequest{
		SecretName: "runner-api",
		Scopes:     []string{"read_repository"},
		TTL:        7 * time.Minute,
		NodeID:     "node-a",
	})
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	if token.Value == "" {
		t.Fatalf("expected token value")
	}
	if !token.IssuedAt.Equal(now) {
		t.Fatalf("expected IssuedAt to equal now, got %s", token.IssuedAt)
	}
	expectedExpiry := now.Add(7 * time.Minute)
	if !token.ExpiresAt.Equal(expectedExpiry) {
		t.Fatalf("expected expiry %s, got %s", expectedExpiry, token.ExpiresAt)
	}
	if len(token.Scopes) != 1 || token.Scopes[0] != "read_repository" {
		t.Fatalf("expected scopes [read_repository], got %+v", token.Scopes)
	}
	if strings.TrimSpace(token.TokenID) == "" {
		t.Fatalf("expected token id populated")
	}

	resp, err := client.Get(ctx, SecretsPrefix+"runner-api")
	if err != nil {
		t.Fatalf("etcd get stored secret: %v", err)
	}
	if len(resp.Kvs) != 1 {
		t.Fatalf("expected stored secret in etcd, got %d keys", len(resp.Kvs))
	}
	var stored struct {
		Ciphertext string `json:"ciphertext"`
	}
	if err := json.Unmarshal(resp.Kvs[0].Value, &stored); err != nil {
		t.Fatalf("decode stored secret: %v", err)
	}
	if strings.Contains(stored.Ciphertext, "glpat-example-123") {
		t.Fatalf("expected ciphertext to hide plaintext API key")
	}
}

// TestSignerIssueTokenValidatesScopes ensures IssueToken rejects scopes that were never granted.
func TestSignerIssueTokenValidatesScopes(t *testing.T) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	etcd, client := newTestEtcd(t)
	defer etcd.Close()
	t.Cleanup(func() {
		_ = client.Close()
	})

	cipher := mustNewAESCipher(t, strings.Repeat("s", 32))
	signer := mustNewSigner(t, client, cipher)
	t.Cleanup(func() {
		_ = signer.Close()
	})

	if _, err := signer.RotateSecret(ctx, RotateSecretRequest{
		SecretName: "limited",
		APIKey:     "glpat-limited",
		Scopes:     []string{"read_repository"},
	}); err != nil {
		t.Fatalf("RotateSecret: %v", err)
	}

	if _, err := signer.IssueToken(ctx, IssueTokenRequest{
		SecretName: "limited",
		Scopes:     []string{"write_repository"},
		TTL:        5 * time.Minute,
		NodeID:     "node-scope",
	}); err == nil {
		t.Fatalf("expected IssueToken to reject unknown scopes")
	}
}
