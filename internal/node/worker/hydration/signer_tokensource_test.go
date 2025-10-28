package hydration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestSignerTokenSourceIssuesAndCachesToken(t *testing.T) {
	t.Helper()
	var configCalls atomic.Int32
	var tokenCalls atomic.Int32
	expiry := time.Now().Add(30 * time.Minute).UTC().Format(time.RFC3339Nano)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/config/gitlab":
			configCalls.Add(1)
			writeJSON(t, w, map[string]any{
				"config": map[string]any{
					"default_token": map[string]any{
						"name":   "default-ci",
						"scopes": []string{"read_repository", "api"},
					},
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/gitlab/signer/tokens":
			tokenCalls.Add(1)
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode token payload: %v", err)
			}
			if payload["secret"] != "default-ci" {
				t.Fatalf("expected secret default-ci, got %v", payload["secret"])
			}
			scopes, _ := payload["scopes"].([]any)
			if len(scopes) == 0 {
				t.Fatalf("expected scopes in payload")
			}
			writeJSON(t, w, map[string]any{
				"token":      "glpat-xyz",
				"expires_at": expiry,
				"issued_at":  time.Now().UTC().Format(time.RFC3339Nano),
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	source, err := NewSignerTokenSource(SignerTokenSourceOptions{
		BaseURL:    srv.URL,
		NodeID:     "node-1",
		HTTPClient: srv.Client(),
		TTL:        15 * time.Minute,
	})
	if err != nil {
		t.Fatalf("NewSignerTokenSource error: %v", err)
	}

	token, err := source.IssueToken(context.Background(), contracts.RepoMaterialization{URL: "https://gitlab.example.com/group/project.git"})
	if err != nil {
		t.Fatalf("IssueToken error: %v", err)
	}
	if token.Value != "glpat-xyz" {
		t.Fatalf("unexpected token value: %s", token.Value)
	}
	if configCalls.Load() != 1 || tokenCalls.Load() != 1 {
		t.Fatalf("unexpected call counts config=%d token=%d", configCalls.Load(), tokenCalls.Load())
	}

	// Second call should reuse cached token.
	token2, err := source.IssueToken(context.Background(), contracts.RepoMaterialization{URL: "https://gitlab.example.com/group/project.git"})
	if err != nil {
		t.Fatalf("IssueToken cached error: %v", err)
	}
	if token2.Value != token.Value {
		t.Fatalf("cached token mismatch: %s", token2.Value)
	}
	if tokenCalls.Load() != 1 {
		t.Fatalf("token endpoint invoked multiple times: %d", tokenCalls.Load())
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, payload any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatalf("encode json: %v", err)
	}
}
