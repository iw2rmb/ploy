package gitlab

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"
)

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

func TestSignerRotationWatchers(t *testing.T) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	etcd, client := newTestEtcd(t)
	defer etcd.Close()
	t.Cleanup(func() {
		_ = client.Close()
	})

	cipher := mustNewAESCipher(t, strings.Repeat("r", 32))
	signer := mustNewSigner(t, client, cipher)
	t.Cleanup(func() {
		_ = signer.Close()
	})

	sub := signer.SubscribeRotations()
	t.Cleanup(sub.Close)

	result, err := signer.RotateSecret(ctx, RotateSecretRequest{
		SecretName: "deploy-key",
		APIKey:     "glpat-second",
		Scopes:     []string{"read_repository"},
	})
	if err != nil {
		t.Fatalf("RotateSecret: %v", err)
	}
	if result.Revision == 0 {
		t.Fatalf("expected revision for rotation result")
	}

	select {
	case evt := <-sub.C:
		if evt.SecretName != "deploy-key" {
			t.Fatalf("expected secret deploy-key, got %s", evt.SecretName)
		}
		if evt.Revision != result.Revision {
			t.Fatalf("expected revision %d, got %d", result.Revision, evt.Revision)
		}
		if evt.UpdatedAt.IsZero() {
			t.Fatalf("expected UpdatedAt populated")
		}
	case <-ctx.Done():
		t.Fatalf("did not receive rotation event before timeout")
	}
}

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
	}); err == nil {
		t.Fatalf("expected IssueToken to reject unknown scopes")
	}
}

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
	}); err == nil {
		t.Fatalf("expected IssueToken to reject ttl above max")
	}
}

// --- helpers ---

func newTestEtcd(t *testing.T) (*embed.Etcd, *clientv3.Client) {
	t.Helper()
	dir := t.TempDir()
	cfg := embed.NewConfig()
	cfg.Dir = dir

	clientURL := mustParseURL("http://127.0.0.1:0")
	peerURL := mustParseURL("http://127.0.0.1:0")

	cfg.ListenClientUrls = []url.URL{clientURL}
	cfg.ListenPeerUrls = []url.URL{peerURL}
	cfg.AdvertiseClientUrls = []url.URL{clientURL}
	cfg.AdvertisePeerUrls = []url.URL{peerURL}
	cfg.Name = "signer"
	cfg.InitialCluster = fmt.Sprintf("%s=%s", cfg.Name, peerURL.String())
	cfg.ClusterState = embed.ClusterStateFlagNew
	cfg.InitialClusterToken = "signer-test"
	cfg.Logger = "zap"
	cfg.LogLevel = "panic"
	cfg.LogOutputs = []string{filepath.Join(dir, "etcd.log")}

	etcd, err := embed.StartEtcd(cfg)
	if err != nil {
		t.Fatalf("start embedded etcd: %v", err)
	}

	select {
	case <-etcd.Server.ReadyNotify():
	case <-time.After(10 * time.Second):
		t.Fatalf("timed out waiting for etcd readiness")
	}

	endpoint := etcd.Clients[0].Addr().String()
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{endpoint},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("create etcd client: %v", err)
	}

	return etcd, client
}

func mustParseURL(raw string) url.URL {
	parsed, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	return *parsed
}

func mustNewAESCipher(t *testing.T, key string) Cipher {
	t.Helper()
	c, err := NewAESCipher([]byte(key))
	if err != nil {
		t.Fatalf("new cipher: %v", err)
	}
	return c
}

func mustNewSigner(t *testing.T, client *clientv3.Client, cipher Cipher, opts ...SignerOption) *Signer {
	t.Helper()
	signer, err := NewSigner(client, cipher, opts...)
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	return signer
}
