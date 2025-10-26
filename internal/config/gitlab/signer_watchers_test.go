package gitlab

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestSignerRotationWatchers verifies that rotation events reach subscribers.
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
