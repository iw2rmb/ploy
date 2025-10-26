package gitlab

import (
	"context"
	"sync"
	"time"
)

// RotationEvent captures a secret rotation observed via etcd.
type RotationEvent struct {
	SecretName string
	Revision   int64
	UpdatedAt  time.Time
}

// RotationSubscription delivers rotation events to subscribers.
type RotationSubscription struct {
	C <-chan RotationEvent

	closeOnce sync.Once
	closeFn   func()
}

// Close terminates the subscription and releases resources.
func (s *RotationSubscription) Close() {
	s.closeOnce.Do(func() {
		if s.closeFn != nil {
			s.closeFn()
		}
	})
}

// AuditAction enumerates rotation audit activity types.
type AuditAction string

const (
	// AuditActionIssued records token issuance to a node.
	AuditActionIssued AuditAction = "issued"
	// AuditActionRevoked records successful token revocation.
	AuditActionRevoked AuditAction = "revoked"
	// AuditActionRevocationFailed records a revocation failure.
	AuditActionRevocationFailed AuditAction = "revocation_failed"
)

// AuditEvent captures rotation audit metadata for observability pipelines.
type AuditEvent struct {
	Action     AuditAction
	SecretName string
	NodeID     string
	TokenID    string
	Timestamp  time.Time
	ExpiresAt  time.Time
	Error      string
}

// AuditRecorder consumes rotation audit events.
type AuditRecorder interface {
	Record(event AuditEvent)
}

// NoopAuditRecorder returns an audit recorder that discards events.
func NoopAuditRecorder() AuditRecorder { return noopAuditRecorder{} }

type noopAuditRecorder struct{}

func (noopAuditRecorder) Record(AuditEvent) {}

// RevocableToken describes a token subject to GitLab API revocation.
type RevocableToken struct {
	ID     string
	NodeID string
}

// RevocationFailure tracks a revocation error for a specific token.
type RevocationFailure struct {
	Token RevocableToken
	Err   error
}

// RevocationReport summarises revocation outcomes.
type RevocationReport struct {
	Revoked []RevocableToken
	Failed  []RevocationFailure
}

// TokenRevoker revokes GitLab tokens via the GitLab API.
type TokenRevoker interface {
	Revoke(ctx context.Context, secret string, tokens []RevocableToken) RevocationReport
}

// NoopTokenRevoker returns a revoker that performs no operations.
func NoopTokenRevoker() TokenRevoker { return noopTokenRevoker{} }

type noopTokenRevoker struct{}

func (noopTokenRevoker) Revoke(context.Context, string, []RevocableToken) RevocationReport {
	return RevocationReport{}
}
