package handlers

import (
	"net/http"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/config"
)

// TestConfigEnvListReturnsAllEntries verifies that GET /v1/config/env
// returns all entries sorted by key, with secret values redacted.
func TestConfigEnvListReturnsAllEntries(t *testing.T) {
	holder := NewConfigHolder(emptyGitLabConfig(), map[string]GlobalEnvVar{
		"CA_CERTS_PEM_BUNDLE": {Value: "-----BEGIN CERTIFICATE-----\n...", Target: domaintypes.GlobalEnvTargetGates, Secret: true},
		"API_KEY":             {Value: "sk-abc123", Target: domaintypes.GlobalEnvTargetSteps, Secret: false},
	})

	handler := listGlobalEnvHandler(holder)
	rr := doRequest(t, handler, http.MethodGet, "/v1/config/env", nil)

	assertStatus(t, rr, http.StatusOK)

	resp := decodeBody[[]globalEnvListItem](t, rr)

	if len(resp) != 2 {
		t.Fatalf("got %d entries, want 2", len(resp))
	}

	// Verify sorted order.
	if resp[0].Key != "API_KEY" || resp[1].Key != "CA_CERTS_PEM_BUNDLE" {
		t.Errorf("entries not sorted: got %v, %v", resp[0].Key, resp[1].Key)
	}

	// Non-secret entry includes value.
	if resp[0].Value != "sk-abc123" {
		t.Errorf("non-secret value = %q, want %q", resp[0].Value, "sk-abc123")
	}

	// Secret entry has redacted (empty) value.
	if resp[1].Value != "" {
		t.Errorf("secret value = %q, want empty", resp[1].Value)
	}
}

// TestConfigEnvGetReturnsEntry verifies GET /v1/config/env/{key}
// returns full value (including for secrets) for admin access.
func TestConfigEnvGetReturnsEntry(t *testing.T) {
	holder := NewConfigHolder(emptyGitLabConfig(), map[string]GlobalEnvVar{
		"CODEX_AUTH_JSON": {Value: `{"token":"secret"}`, Target: domaintypes.GlobalEnvTargetSteps, Secret: true},
	})

	handler := getGlobalEnvHandler(holder)

	// Create a request with path value set.
	rr := doRequest(t, handler, http.MethodGet, "/v1/config/env/CODEX_AUTH_JSON", nil, "key", "CODEX_AUTH_JSON")

	assertStatus(t, rr, http.StatusOK)

	resp := decodeBody[globalEnvResponse](t, rr)

	// Value should be returned for admin access.
	if resp.Key != "CODEX_AUTH_JSON" {
		t.Errorf("Key = %q, want %q", resp.Key, "CODEX_AUTH_JSON")
	}
	if resp.Value != `{"token":"secret"}` {
		t.Errorf("Value = %q, want %q", resp.Value, `{"token":"secret"}`)
	}
	if resp.Scope != "steps" {
		t.Errorf("Scope = %q, want %q", resp.Scope, "steps")
	}
	if !resp.Secret {
		t.Errorf("Secret = %v, want true", resp.Secret)
	}
}

// TestConfigEnvGetNotFound verifies GET /v1/config/env/{key} returns 404
// when the key does not exist.
func TestConfigEnvGetNotFound(t *testing.T) {
	holder := NewConfigHolder(emptyGitLabConfig(), nil)

	handler := getGlobalEnvHandler(holder)
	rr := doRequest(t, handler, http.MethodGet, "/v1/config/env/NONEXISTENT", nil, "key", "NONEXISTENT")

	assertStatus(t, rr, http.StatusNotFound)
}

// TestConfigEnvPutUpsertsEntry verifies PUT /v1/config/env/{key}
// persists to store and updates the holder.
func TestConfigEnvPutUpsertsEntry(t *testing.T) {
	st := &configStore{}
	holder := NewConfigHolder(emptyGitLabConfig(), nil)

	handler := putGlobalEnvHandler(holder, st)

	reqBody := map[string]any{
		"value":  "-----BEGIN CERTIFICATE-----\n...",
		"scope":  "gates",
		"secret": true,
	}

	rr := doRequest(t, handler, http.MethodPut, "/v1/config/env/CA_CERTS_PEM_BUNDLE", reqBody, "key", "CA_CERTS_PEM_BUNDLE")

	assertStatus(t, rr, http.StatusOK)

	// Verify store was called.
	if !st.upsertGlobalEnv.called {
		t.Error("store.UpsertGlobalEnv was not called")
	}
	if st.upsertGlobalEnv.params.Key != "CA_CERTS_PEM_BUNDLE" {
		t.Errorf("store Key = %q, want %q", st.upsertGlobalEnv.params.Key, "CA_CERTS_PEM_BUNDLE")
	}

	// Verify holder was updated.
	v, ok := holder.GetGlobalEnvVar("CA_CERTS_PEM_BUNDLE")
	if !ok {
		t.Fatal("holder does not contain CA_CERTS_PEM_BUNDLE")
	}
	if v.Value != "-----BEGIN CERTIFICATE-----\n..." {
		t.Errorf("holder Value = %q", v.Value)
	}
}

// TestConfigEnvPutDefaultsSecretToTrue verifies that secret defaults to true
// when not explicitly set in the request.
func TestConfigEnvPutDefaultsSecretToTrue(t *testing.T) {
	st := &configStore{}
	holder := NewConfigHolder(emptyGitLabConfig(), nil)

	handler := putGlobalEnvHandler(holder, st)

	// Omit "secret" field — should default to true.
	reqBody := map[string]any{
		"value": "test-value",
		"scope": "steps",
	}

	rr := doRequest(t, handler, http.MethodPut, "/v1/config/env/TEST_KEY", reqBody, "key", "TEST_KEY")

	assertStatus(t, rr, http.StatusOK)

	// Verify secret defaults to true.
	if !st.upsertGlobalEnv.params.Secret {
		t.Error("store Secret = false, want true (default)")
	}

	v, _ := holder.GetGlobalEnvVar("TEST_KEY")
	if !v.Secret {
		t.Error("holder Secret = false, want true")
	}
}

// TestConfigEnvPutInvalidTarget verifies that invalid target values return 400.
func TestConfigEnvPutInvalidTarget(t *testing.T) {
	st := &configStore{}
	holder := NewConfigHolder(emptyGitLabConfig(), nil)

	handler := putGlobalEnvHandler(holder, st)

	reqBody := map[string]any{
		"value": "test",
		"scope": "invalid-target",
	}

	rr := doRequest(t, handler, http.MethodPut, "/v1/config/env/TEST_KEY", reqBody, "key", "TEST_KEY")

	assertStatus(t, rr, http.StatusBadRequest)

	// Store should not be called.
	if st.upsertGlobalEnv.called {
		t.Error("store.UpsertGlobalEnv should not be called for invalid target")
	}
}

// TestConfigEnvDeleteRemovesEntry verifies DELETE /v1/config/env/{key}
// removes from store and holder.
func TestConfigEnvDeleteRemovesEntry(t *testing.T) {
	st := &configStore{}
	holder := NewConfigHolder(emptyGitLabConfig(), map[string]GlobalEnvVar{
		"OLD_KEY": {Value: "old-value", Target: domaintypes.GlobalEnvTargetGates, Secret: false},
	})

	handler := deleteGlobalEnvHandler(holder, st)

	rr := doRequest(t, handler, http.MethodDelete, "/v1/config/env/OLD_KEY?target=gates", nil, "key", "OLD_KEY")

	assertStatus(t, rr, http.StatusNoContent)

	// Verify store was called.
	if !st.deleteGlobalEnv.called {
		t.Error("store.DeleteGlobalEnv was not called")
	}
	if st.deleteGlobalEnv.params.Key != "OLD_KEY" {
		t.Errorf("store Key = %q, want %q", st.deleteGlobalEnv.params.Key, "OLD_KEY")
	}
	if st.deleteGlobalEnv.params.Target != "gates" {
		t.Errorf("store Target = %q, want %q", st.deleteGlobalEnv.params.Target, "gates")
	}

	// Verify holder was updated.
	if _, ok := holder.GetGlobalEnvVar("OLD_KEY"); ok {
		t.Error("holder still contains OLD_KEY after delete")
	}
}

// TestConfigEnvRoundTrip verifies PUT followed by GET returns identical values.
func TestConfigEnvRoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		key    string
		value  string
		scope  string
		secret bool
	}{
		{
			name:   "CA bundle",
			key:    "CA_CERTS_PEM_BUNDLE",
			value:  "-----BEGIN CERTIFICATE-----\nMIIB...\n-----END CERTIFICATE-----",
			scope:  "gates",
			secret: true,
		},
		{
			name:   "non-secret API key",
			key:    "PUBLIC_API_KEY",
			value:  "pk_live_abc123",
			scope:  "steps",
			secret: false,
		},
		{
			name:   "codex auth JSON",
			key:    "CODEX_AUTH_JSON",
			value:  `{"api_key":"sk-...","org_id":"org-123"}`,
			scope:  "server",
			secret: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &configStore{}
			holder := NewConfigHolder(emptyGitLabConfig(), nil)

			// PUT the entry.
			putRR := doRequest(t, putGlobalEnvHandler(holder, st), http.MethodPut, "/v1/config/env/"+tt.key, map[string]any{
				"value":  tt.value,
				"scope":  tt.scope,
				"secret": tt.secret,
			}, "key", tt.key)
			assertStatus(t, putRR, http.StatusOK)

			// GET the entry.
			getRR := doRequest(t, getGlobalEnvHandler(holder), http.MethodGet, "/v1/config/env/"+tt.key, nil, "key", tt.key)
			assertStatus(t, getRR, http.StatusOK)

			resp := decodeBody[globalEnvResponse](t, getRR)

			if resp.Key != tt.key {
				t.Errorf("Key = %q, want %q", resp.Key, tt.key)
			}
			if resp.Value != tt.value {
				t.Errorf("Value = %q, want %q", resp.Value, tt.value)
			}
			if resp.Scope != tt.scope {
				t.Errorf("Scope = %q, want %q", resp.Scope, tt.scope)
			}
			if resp.Secret != tt.secret {
				t.Errorf("Secret = %v, want %v", resp.Secret, tt.secret)
			}
		})
	}
}

// TestConfigEnvPutInvalidJSON verifies that malformed JSON returns 400.
func TestConfigEnvPutInvalidJSON(t *testing.T) {
	st := &configStore{}
	holder := NewConfigHolder(emptyGitLabConfig(), nil)

	handler := putGlobalEnvHandler(holder, st)
	rr := doRequest(t, handler, http.MethodPut, "/v1/config/env/TEST", "not json", "key", "TEST")

	assertStatus(t, rr, http.StatusBadRequest)
}

// TestConfigEnvPutStoreError verifies that store errors return 500.
func TestConfigEnvPutStoreError(t *testing.T) {
	st := &configStore{}
	st.upsertGlobalEnv.err = errMockDatabase
	holder := NewConfigHolder(emptyGitLabConfig(), nil)

	handler := putGlobalEnvHandler(holder, st)

	reqBody := map[string]any{"value": "test", "scope": "gates"}

	rr := doRequest(t, handler, http.MethodPut, "/v1/config/env/TEST", reqBody, "key", "TEST")

	assertStatus(t, rr, http.StatusInternalServerError)

	// Holder should not be updated on store failure.
	if _, ok := holder.GetGlobalEnvVar("TEST"); ok {
		t.Error("holder should not contain TEST after store failure")
	}
}

// TestConfigEnvDeleteStoreError verifies that store errors return 500.
func TestConfigEnvDeleteStoreError(t *testing.T) {
	st := &configStore{}
	st.deleteGlobalEnv.err = errMockDatabase
	holder := NewConfigHolder(emptyGitLabConfig(), map[string]GlobalEnvVar{
		"OLD_KEY": {Value: "val", Target: domaintypes.GlobalEnvTargetGates, Secret: false},
	})

	handler := deleteGlobalEnvHandler(holder, st)

	rr := doRequest(t, handler, http.MethodDelete, "/v1/config/env/OLD_KEY?target=gates", nil, "key", "OLD_KEY")

	assertStatus(t, rr, http.StatusInternalServerError)

	// Holder should not be updated on store failure.
	if _, ok := holder.GetGlobalEnvVar("OLD_KEY"); !ok {
		t.Error("holder should still contain OLD_KEY after store failure")
	}
}

// Helper to create empty GitLab config for tests.
func emptyGitLabConfig() config.GitLabConfig {
	return config.GitLabConfig{}
}
