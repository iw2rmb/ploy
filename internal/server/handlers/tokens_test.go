package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/store"
	"github.com/jackc/pgx/v5/pgtype"
)

type tokenStore struct {
	store.Store
	insertAPIToken mockCall[store.InsertAPITokenParams, struct{}]
	listAPITokens  mockResult[[]store.ListAPITokensRow]
}

func (m *tokenStore) InsertAPIToken(ctx context.Context, params store.InsertAPITokenParams) error {
	_, err := m.insertAPIToken.record(params)
	return err
}

func (m *tokenStore) ListAPITokens(ctx context.Context) ([]store.ListAPITokensRow, error) {
	return m.listAPITokens.ret()
}

func TestCreateAPITokenRequiresUsernameForControlPlane(t *testing.T) {
	st := &tokenStore{}
	handler := createAPITokenHandler(st, "test-secret-12345678901234567890")

	rr := doRequest(t, handler, http.MethodPost, "/v1/tokens", map[string]any{
		"role": "control-plane",
	})

	assertStatus(t, rr, http.StatusBadRequest)
	if st.insertAPIToken.called {
		t.Fatal("InsertAPIToken should not be called")
	}
}

func TestCreateAPITokenStoresUsername(t *testing.T) {
	st := &tokenStore{}
	handler := createAPITokenHandler(st, "test-secret-12345678901234567890")

	rr := doRequest(t, handler, http.MethodPost, "/v1/tokens", map[string]any{
		"role":     "control-plane",
		"username": "alice",
	})

	assertStatus(t, rr, http.StatusOK)
	if !st.insertAPIToken.called {
		t.Fatal("InsertAPIToken was not called")
	}
	if st.insertAPIToken.params.Username == nil || *st.insertAPIToken.params.Username != "alice" {
		t.Fatalf("stored username = %v, want alice", st.insertAPIToken.params.Username)
	}
	var resp struct {
		Username *string `json:"username"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Username == nil || *resp.Username != "alice" {
		t.Fatalf("response username = %v, want alice", resp.Username)
	}
}

func TestListAPITokensIncludesUsername(t *testing.T) {
	username := "alice"
	st := &tokenStore{
		listAPITokens: mockResult[[]store.ListAPITokensRow]{
			val: []store.ListAPITokensRow{{
				TokenID:   "token-1",
				Role:      "control-plane",
				Username:  &username,
				IssuedAt:  pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
				ExpiresAt: pgtype.Timestamptz{Time: time.Now().UTC().Add(time.Hour), Valid: true},
			}},
		},
	}

	rr := doRequest(t, listAPITokensHandler(st), http.MethodGet, "/v1/tokens", nil)

	assertStatus(t, rr, http.StatusOK)
	var resp struct {
		Tokens []struct {
			Username *string `json:"username"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Tokens) != 1 || resp.Tokens[0].Username == nil || *resp.Tokens[0].Username != username {
		t.Fatalf("tokens = %+v, want username %q", resp.Tokens, username)
	}
}
