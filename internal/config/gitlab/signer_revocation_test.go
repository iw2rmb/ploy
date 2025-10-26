package gitlab

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// TestSignerRotateSecretRevokesTokens ensures rotations trigger revocation hooks.
func TestSignerRotateSecretRevokesTokens(t *testing.T) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	etcd, client := newTestEtcd(t)
	defer etcd.Close()
	t.Cleanup(func() {
		_ = client.Close()
	})

	revoker := newRecordingRevoker(nil)
	audit := newRecordingAudit()

	cipher := mustNewAESCipher(t, strings.Repeat("t", 32))
	signer := mustNewSigner(t, client, cipher,
		WithTokenRevoker(revoker),
		WithAuditRecorder(audit),
	)
	t.Cleanup(func() {
		_ = signer.Close()
	})

	if _, err := signer.RotateSecret(ctx, RotateSecretRequest{
		SecretName: "deploy",
		APIKey:     "glpat-initial",
		Scopes:     []string{"api"},
	}); err != nil {
		t.Fatalf("RotateSecret initial: %v", err)
	}

	token, err := signer.IssueToken(ctx, IssueTokenRequest{
		SecretName: "deploy",
		Scopes:     []string{"api"},
		TTL:        3 * time.Minute,
		NodeID:     "node-1",
	})
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	if _, err := signer.RotateSecret(ctx, RotateSecretRequest{
		SecretName: "deploy",
		APIKey:     "glpat-next",
		Scopes:     []string{"api"},
	}); err != nil {
		t.Fatalf("RotateSecret rotate: %v", err)
	}

	report := revoker.lastCall()
	if report == nil {
		t.Fatalf("expected revoker to be invoked")
	}
	if len(report.tokens) != 1 || report.tokens[0].ID != token.TokenID {
		t.Fatalf("expected revoker to receive token id %q, got %+v", token.TokenID, report.tokens)
	}
	if report.secret != "deploy" {
		t.Fatalf("expected secret deploy, got %s", report.secret)
	}

	issued := audit.eventsByAction(AuditActionIssued)
	if len(issued) == 0 {
		t.Fatalf("expected issued audit event recorded")
	}
	revoked := audit.eventsByAction(AuditActionRevoked)
	if len(revoked) != 1 {
		t.Fatalf("expected single revoked audit event, got %d", len(revoked))
	}
	if revokerFail := audit.eventsByAction(AuditActionRevocationFailed); len(revokerFail) != 0 {
		t.Fatalf("unexpected revocation failure events: %+v", revokerFail)
	}
	if rev := revoked[0]; rev.TokenID != token.TokenID || rev.NodeID != "node-1" {
		t.Fatalf("unexpected revoked event payload: %+v", rev)
	}
}

// TestSignerRotateSecretRecordsRevocationFailures records failed revocations for retries.
func TestSignerRotateSecretRecordsRevocationFailures(t *testing.T) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	etcd, client := newTestEtcd(t)
	defer etcd.Close()
	t.Cleanup(func() {
		_ = client.Close()
	})

	cipher := mustNewAESCipher(t, strings.Repeat("u", 32))
	failErr := errors.New("boom")
	revoker := newRecordingRevoker(map[string]error{})
	audit := newRecordingAudit()

	signer := mustNewSigner(t, client, cipher,
		WithTokenRevoker(revoker),
		WithAuditRecorder(audit),
	)
	t.Cleanup(func() {
		_ = signer.Close()
	})

	if _, err := signer.RotateSecret(ctx, RotateSecretRequest{
		SecretName: "deploy",
		APIKey:     "glpat-initial",
		Scopes:     []string{"api"},
	}); err != nil {
		t.Fatalf("RotateSecret initial: %v", err)
	}

	token, err := signer.IssueToken(ctx, IssueTokenRequest{
		SecretName: "deploy",
		Scopes:     []string{"api"},
		TTL:        2 * time.Minute,
		NodeID:     "node-err",
	})
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	revoker.failures[token.TokenID] = failErr

	if _, err := signer.RotateSecret(ctx, RotateSecretRequest{
		SecretName: "deploy",
		APIKey:     "glpat-next",
		Scopes:     []string{"api"},
	}); err != nil {
		t.Fatalf("RotateSecret rotate: %v", err)
	}

	failed := audit.eventsByAction(AuditActionRevocationFailed)
	if len(failed) != 1 {
		t.Fatalf("expected revocation failure event, got %d", len(failed))
	}
	if failed[0].TokenID != token.TokenID || failed[0].NodeID != "node-err" {
		t.Fatalf("unexpected failure event payload: %+v", failed[0])
	}
	if failed[0].Error == "" || !strings.Contains(failed[0].Error, "boom") {
		t.Fatalf("expected failure error containing boom, got %q", failed[0].Error)
	}

	// Clear failure to allow success on next rotation.
	delete(revoker.failures, token.TokenID)

	if _, err := signer.RotateSecret(ctx, RotateSecretRequest{
		SecretName: "deploy",
		APIKey:     "glpat-final",
		Scopes:     []string{"api"},
	}); err != nil {
		t.Fatalf("RotateSecret retry: %v", err)
	}

	revoked := audit.eventsByAction(AuditActionRevoked)
	if len(revoked) == 0 {
		t.Fatalf("expected revoked event after retry")
	}
}
