package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestCompleteBootstrapEnrollment(t *testing.T) {
	ctx, db := newTestStore(t)
	now := time.Now().UTC()

	tests := []struct {
		name        string
		setup       func(t *testing.T, ctx context.Context, db Store) (string, types.NodeID, bool)
		wantErr     error
		wantCreated bool
	}{
		{
			name: "valid token creates node and consumes token",
			setup: func(t *testing.T, ctx context.Context, db Store) (string, types.NodeID, bool) {
				nodeID := types.NodeID(types.NewNodeKey())
				tokenID := "valid-token"
				insertBootstrapTokenForTest(t, ctx, db, tokenID, nodeID)
				return tokenID, nodeID, true
			},
			wantCreated: true,
		},
		{
			name: "missing token row is invalid",
			setup: func(t *testing.T, ctx context.Context, db Store) (string, types.NodeID, bool) {
				return "missing-token", types.NodeID(types.NewNodeKey()), false
			},
			wantErr: ErrBootstrapTokenInvalid,
		},
		{
			name: "revoked token is invalid",
			setup: func(t *testing.T, ctx context.Context, db Store) (string, types.NodeID, bool) {
				nodeID := types.NodeID(types.NewNodeKey())
				tokenID := "revoked-token"
				insertBootstrapTokenForTest(t, ctx, db, tokenID, nodeID)
				if _, err := db.Pool().Exec(ctx, `UPDATE bootstrap_tokens SET revoked_at = now() WHERE token_id = $1`, tokenID); err != nil {
					t.Fatalf("revoke bootstrap token: %v", err)
				}
				return tokenID, nodeID, true
			},
			wantErr: ErrBootstrapTokenInvalid,
		},
		{
			name: "used token is invalid",
			setup: func(t *testing.T, ctx context.Context, db Store) (string, types.NodeID, bool) {
				nodeID := types.NodeID(types.NewNodeKey())
				tokenID := "used-token"
				insertBootstrapTokenForTest(t, ctx, db, tokenID, nodeID)
				if _, err := db.Pool().Exec(ctx, `UPDATE bootstrap_tokens SET used_at = now() WHERE token_id = $1`, tokenID); err != nil {
					t.Fatalf("mark bootstrap token used: %v", err)
				}
				return tokenID, nodeID, true
			},
			wantErr: ErrBootstrapTokenInvalid,
		},
		{
			name: "node mismatch is invalid",
			setup: func(t *testing.T, ctx context.Context, db Store) (string, types.NodeID, bool) {
				storedNodeID := types.NodeID(types.NewNodeKey())
				requestNodeID := types.NodeID(types.NewNodeKey())
				tokenID := "mismatch-token"
				insertBootstrapTokenForTest(t, ctx, db, tokenID, storedNodeID)
				return tokenID, requestNodeID, true
			},
			wantErr: ErrBootstrapTokenInvalid,
		},
		{
			name: "existing node is conflict",
			setup: func(t *testing.T, ctx context.Context, db Store) (string, types.NodeID, bool) {
				node := createTestNode(t, ctx, db)
				tokenID := "existing-node-token"
				insertBootstrapTokenForTest(t, ctx, db, tokenID, node.ID)
				return tokenID, node.ID, true
			},
			wantErr: ErrBootstrapNodeExists,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanTestTables(t, ctx, db)
			tokenID, nodeID, tokenInserted := tt.setup(t, ctx, db)

			err := db.CompleteBootstrapEnrollment(ctx, CompleteBootstrapEnrollmentParams{
				TokenID:         tokenID,
				NodeID:          nodeID,
				CertSerial:      "serial-" + tokenID,
				CertFingerprint: "fingerprint-" + tokenID,
				CertNotBefore:   now,
				CertNotAfter:    now.Add(365 * 24 * time.Hour),
			})
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("CompleteBootstrapEnrollment error = %v, want %v", err, tt.wantErr)
				}
				if tokenInserted {
					assertBootstrapTokenNotConsumed(t, ctx, db, tokenID)
				}
				return
			}
			if err != nil {
				t.Fatalf("CompleteBootstrapEnrollment: %v", err)
			}

			node, err := db.GetNode(ctx, nodeID)
			if err != nil {
				t.Fatalf("GetNode enrolled node: %v", err)
			}
			if node.Name != "node-"+nodeID.String() || node.Concurrency != 1 || node.IpAddress.String() != "0.0.0.0" {
				t.Fatalf("node defaults = %+v", node)
			}
			if node.CertSerial == nil || *node.CertSerial != "serial-"+tokenID {
				t.Fatalf("cert serial = %v", node.CertSerial)
			}
			if node.CertFingerprint == nil || *node.CertFingerprint != "fingerprint-"+tokenID {
				t.Fatalf("cert fingerprint = %v", node.CertFingerprint)
			}
			row, err := db.GetBootstrapToken(ctx, tokenID)
			if err != nil {
				t.Fatalf("GetBootstrapToken: %v", err)
			}
			if !row.UsedAt.Valid || !row.CertIssuedAt.Valid {
				t.Fatalf("token consumption timestamps = used:%v cert:%v", row.UsedAt, row.CertIssuedAt)
			}
		})
	}
}

func insertBootstrapTokenForTest(t *testing.T, ctx context.Context, db Store, tokenID string, nodeID types.NodeID) {
	t.Helper()
	if err := db.InsertBootstrapToken(ctx, InsertBootstrapTokenParams{
		TokenHash: tokenID + "-hash",
		TokenID:   tokenID,
		NodeID:    &nodeID,
		IssuedAt:  pgtype.Timestamptz{Time: time.Now().Add(-time.Minute), Valid: true},
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true},
	}); err != nil {
		t.Fatalf("InsertBootstrapToken(%s): %v", tokenID, err)
	}
}

func assertBootstrapTokenNotConsumed(t *testing.T, ctx context.Context, db Store, tokenID string) {
	t.Helper()
	row, err := db.GetBootstrapToken(ctx, tokenID)
	if err != nil {
		t.Fatalf("GetBootstrapToken(%s): %v", tokenID, err)
	}
	if row.UsedAt.Valid || row.CertIssuedAt.Valid {
		t.Fatalf("bootstrap token %s consumed unexpectedly: used=%v cert=%v", tokenID, row.UsedAt, row.CertIssuedAt)
	}
}
