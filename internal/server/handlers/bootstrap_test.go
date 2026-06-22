package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const bootstrapTestSecret = "bootstrap-test-secret"

type bootstrapStore struct {
	store.Store

	getNode struct {
		val store.Node
		err error
	}
	insertBootstrapToken struct {
		called int
		arg    store.InsertBootstrapTokenParams
		err    error
	}
	getBootstrapToken struct {
		val store.GetBootstrapTokenRow
		err error
	}
	completeBootstrapEnrollment struct {
		called int
		arg    store.CompleteBootstrapEnrollmentParams
		err    error
	}
}

func (m *bootstrapStore) GetNode(ctx context.Context, id domaintypes.NodeID) (store.Node, error) {
	return m.getNode.val, m.getNode.err
}

func (m *bootstrapStore) InsertBootstrapToken(ctx context.Context, arg store.InsertBootstrapTokenParams) error {
	m.insertBootstrapToken.called++
	m.insertBootstrapToken.arg = arg
	return m.insertBootstrapToken.err
}

func (m *bootstrapStore) GetBootstrapToken(ctx context.Context, tokenID string) (store.GetBootstrapTokenRow, error) {
	return m.getBootstrapToken.val, m.getBootstrapToken.err
}

func (m *bootstrapStore) CompleteBootstrapEnrollment(ctx context.Context, arg store.CompleteBootstrapEnrollmentParams) error {
	m.completeBootstrapEnrollment.called++
	m.completeBootstrapEnrollment.arg = arg
	return m.completeBootstrapEnrollment.err
}

func TestCreateBootstrapTokenHandler_NodeEnrollmentTarget(t *testing.T) {
	tests := []struct {
		name       string
		setupStore func(nodeID domaintypes.NodeID) *bootstrapStore
		wantStatus int
		wantInsert bool
	}{
		{
			name: "future node id succeeds and inserts token",
			setupStore: func(domaintypes.NodeID) *bootstrapStore {
				st := &bootstrapStore{}
				st.getNode.err = pgx.ErrNoRows
				return st
			},
			wantStatus: http.StatusOK,
			wantInsert: true,
		},
		{
			name: "existing node id returns conflict and does not insert",
			setupStore: func(nodeID domaintypes.NodeID) *bootstrapStore {
				st := &bootstrapStore{}
				st.getNode.val = store.Node{ID: nodeID}
				return st
			},
			wantStatus: http.StatusConflict,
			wantInsert: false,
		},
		{
			name: "node lookup error returns server error and does not insert",
			setupStore: func(domaintypes.NodeID) *bootstrapStore {
				st := &bootstrapStore{}
				st.getNode.err = errors.New("database unavailable")
				return st
			},
			wantStatus: http.StatusInternalServerError,
			wantInsert: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodeID := domaintypes.NodeID(domaintypes.NewNodeKey())
			st := tt.setupStore(nodeID)
			body := bytes.NewBufferString(`{"node_id":"` + nodeID.String() + `","expires_in_minutes":5}`)
			req := httptest.NewRequest(http.MethodPost, "/v1/bootstrap/tokens", body)
			rr := httptest.NewRecorder()

			createBootstrapTokenHandler(st, bootstrapTestSecret).ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body: %s", rr.Code, tt.wantStatus, rr.Body.String())
			}
			if gotInsert := st.insertBootstrapToken.called > 0; gotInsert != tt.wantInsert {
				t.Fatalf("insert called = %v, want %v", gotInsert, tt.wantInsert)
			}
			if !tt.wantInsert {
				return
			}
			if st.insertBootstrapToken.arg.NodeID == nil || *st.insertBootstrapToken.arg.NodeID != nodeID {
				t.Fatalf("insert node_id = %v, want %s", st.insertBootstrapToken.arg.NodeID, nodeID)
			}
			var resp struct {
				Token     string             `json:"token"`
				NodeID    domaintypes.NodeID `json:"node_id"`
				ExpiresAt time.Time          `json:"expires_at"`
			}
			if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if resp.Token == "" || resp.NodeID != nodeID || resp.ExpiresAt.IsZero() {
				t.Fatalf("unexpected response: %+v", resp)
			}
		})
	}
}

func TestValidateBootstrapToken(t *testing.T) {
	nodeID := domaintypes.NodeID(domaintypes.NewNodeKey())
	tokenString, claims := mustBootstrapTokenForHandlers(t, nodeID)
	otherNodeID := domaintypes.NodeID(domaintypes.NewNodeKey())

	tests := []struct {
		name       string
		row        store.GetBootstrapTokenRow
		rowErr     error
		wantErrSub string
	}{
		{
			name: "unrevoked stored token succeeds",
			row:  store.GetBootstrapTokenRow{NodeID: &nodeID},
		},
		{
			name:       "missing token row is rejected",
			rowErr:     pgx.ErrNoRows,
			wantErrSub: "token not found or invalid",
		},
		{
			name:       "revoked token is rejected",
			row:        store.GetBootstrapTokenRow{NodeID: &nodeID, RevokedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}},
			wantErrSub: "token revoked",
		},
		{
			name:       "used token is rejected",
			row:        store.GetBootstrapTokenRow{NodeID: &nodeID, UsedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}},
			wantErrSub: "token already used",
		},
		{
			name:       "node id mismatch is rejected",
			row:        store.GetBootstrapTokenRow{NodeID: &otherNodeID},
			wantErrSub: "token not found or invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &bootstrapStore{}
			st.getBootstrapToken.val = tt.row
			st.getBootstrapToken.err = tt.rowErr
			req := httptest.NewRequest(http.MethodPost, "/v1/pki/bootstrap", nil)
			req.Header.Set("Authorization", "Bearer "+tokenString)

			gotClaims, err := validateBootstrapToken(req, st, bootstrapTestSecret)
			if tt.wantErrSub != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrSub) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErrSub)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateBootstrapToken error: %v", err)
			}
			if gotClaims.ID != claims.ID || gotClaims.NodeID != nodeID {
				t.Fatalf("claims = %+v, want id %s node %s", gotClaims, claims.ID, nodeID)
			}
		})
	}
}

func mustBootstrapTokenForHandlers(t *testing.T, nodeID domaintypes.NodeID) (string, *auth.TokenClaims) {
	t.Helper()
	tokenString, err := auth.GenerateBootstrapToken(bootstrapTestSecret, nodeID, time.Now().Add(15*time.Minute))
	if err != nil {
		t.Fatalf("GenerateBootstrapToken: %v", err)
	}
	claims, err := auth.ValidateToken(tokenString, bootstrapTestSecret)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	return tokenString, claims
}
