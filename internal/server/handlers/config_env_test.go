package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/config"
)

// TestConfigEnvListReturnsAllEntries verifies that GET /v1/config/env
// returns all entries sorted by key, with secret values redacted.
func TestConfigEnvListReturnsAllEntries(t *testing.T) {
	holder := NewConfigHolder(emptyGitLabConfig(), map[string]GlobalEnvVar{
		"CA_CERTS_PEM_BUNDLE": {Value: "-----BEGIN CERTIFICATE-----\n...", Scope: domaintypes.GlobalEnvScopeAll, Secret: true},
		"API_KEY":             {Value: "sk-abc123", Scope: domaintypes.GlobalEnvScopeMods, Secret: false},
	})

	handler := listGlobalEnvHandler(holder)
	req := httptest.NewRequest(http.MethodGet, "/v1/config/env", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp []globalEnvListItem
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

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
		"CODEX_AUTH_JSON": {Value: `{"token":"secret"}`, Scope: domaintypes.GlobalEnvScopeMods, Secret: true},
	})

	handler := getGlobalEnvHandler(holder)

	// Create a request with path value set.
	req := httptest.NewRequest(http.MethodGet, "/v1/config/env/CODEX_AUTH_JSON", nil)
	req.SetPathValue("key", "CODEX_AUTH_JSON")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp globalEnvResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	// Value should be returned for admin access.
	if resp.Key != "CODEX_AUTH_JSON" {
		t.Errorf("Key = %q, want %q", resp.Key, "CODEX_AUTH_JSON")
	}
	if resp.Value != `{"token":"secret"}` {
		t.Errorf("Value = %q, want %q", resp.Value, `{"token":"secret"}`)
	}
	if resp.Scope != "migs" {
		t.Errorf("Scope = %q, want %q", resp.Scope, "migs")
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
	req := httptest.NewRequest(http.MethodGet, "/v1/config/env/NONEXISTENT", nil)
	req.SetPathValue("key", "NONEXISTENT")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

// TestConfigEnvPutUpsertsEntry verifies PUT /v1/config/env/{key}
// persists to store and updates the holder.
func TestConfigEnvPutUpsertsEntry(t *testing.T) {
	st := &mockStore{}
	holder := NewConfigHolder(emptyGitLabConfig(), nil)

	handler := putGlobalEnvHandler(holder, st)

	reqBody := map[string]any{
		"value":  "-----BEGIN CERTIFICATE-----\n...",
		"scope":  "all",
		"secret": true,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPut, "/v1/config/env/CA_CERTS_PEM_BUNDLE", bytes.NewReader(body))
	req.SetPathValue("key", "CA_CERTS_PEM_BUNDLE")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}

	// Verify store was called.
	if !st.upsertGlobalEnvCalled {
		t.Error("store.UpsertGlobalEnv was not called")
	}
	if st.upsertGlobalEnvParams.Key != "CA_CERTS_PEM_BUNDLE" {
		t.Errorf("store Key = %q, want %q", st.upsertGlobalEnvParams.Key, "CA_CERTS_PEM_BUNDLE")
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
	st := &mockStore{}
	holder := NewConfigHolder(emptyGitLabConfig(), nil)

	handler := putGlobalEnvHandler(holder, st)

	// Omit "secret" field — should default to true.
	reqBody := map[string]any{
		"value": "test-value",
		"scope": "migs",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPut, "/v1/config/env/TEST_KEY", bytes.NewReader(body))
	req.SetPathValue("key", "TEST_KEY")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	// Verify secret defaults to true.
	if !st.upsertGlobalEnvParams.Secret {
		t.Error("store Secret = false, want true (default)")
	}

	v, _ := holder.GetGlobalEnvVar("TEST_KEY")
	if !v.Secret {
		t.Error("holder Secret = false, want true")
	}
}

// TestConfigEnvPutInvalidScope verifies that invalid scope values return 400.
func TestConfigEnvPutInvalidScope(t *testing.T) {
	st := &mockStore{}
	holder := NewConfigHolder(emptyGitLabConfig(), nil)

	handler := putGlobalEnvHandler(holder, st)

	reqBody := map[string]any{
		"value": "test",
		"scope": "invalid-scope",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPut, "/v1/config/env/TEST_KEY", bytes.NewReader(body))
	req.SetPathValue("key", "TEST_KEY")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}

	// Store should not be called.
	if st.upsertGlobalEnvCalled {
		t.Error("store.UpsertGlobalEnv should not be called for invalid scope")
	}
}

// TestConfigEnvDeleteRemovesEntry verifies DELETE /v1/config/env/{key}
// removes from store and holder.
func TestConfigEnvDeleteRemovesEntry(t *testing.T) {
	st := &mockStore{}
	holder := NewConfigHolder(emptyGitLabConfig(), map[string]GlobalEnvVar{
		"OLD_KEY": {Value: "old-value", Scope: domaintypes.GlobalEnvScopeAll, Secret: false},
	})

	handler := deleteGlobalEnvHandler(holder, st)

	req := httptest.NewRequest(http.MethodDelete, "/v1/config/env/OLD_KEY", nil)
	req.SetPathValue("key", "OLD_KEY")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNoContent)
	}

	// Verify store was called.
	if !st.deleteGlobalEnvCalled {
		t.Error("store.DeleteGlobalEnv was not called")
	}
	if st.deleteGlobalEnvParam != "OLD_KEY" {
		t.Errorf("store Key = %q, want %q", st.deleteGlobalEnvParam, "OLD_KEY")
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
			scope:  "all",
			secret: true,
		},
		{
			name:   "non-secret API key",
			key:    "PUBLIC_API_KEY",
			value:  "pk_live_abc123",
			scope:  "gate",
			secret: false,
		},
		{
			name:   "codex auth JSON",
			key:    "CODEX_AUTH_JSON",
			value:  `{"api_key":"sk-...","org_id":"org-123"}`,
			scope:  "migs",
			secret: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &mockStore{}
			holder := NewConfigHolder(emptyGitLabConfig(), nil)

			// PUT the entry.
			putHandler := putGlobalEnvHandler(holder, st)
			reqBody := map[string]any{
				"value":  tt.value,
				"scope":  tt.scope,
				"secret": tt.secret,
			}
			body, _ := json.Marshal(reqBody)

			putReq := httptest.NewRequest(http.MethodPut, "/v1/config/env/"+tt.key, bytes.NewReader(body))
			putReq.SetPathValue("key", tt.key)
			putReq.Header.Set("Content-Type", "application/json")
			putRR := httptest.NewRecorder()
			putHandler.ServeHTTP(putRR, putReq)

			if putRR.Code != http.StatusOK {
				t.Fatalf("PUT status = %d, want %d", putRR.Code, http.StatusOK)
			}

			// GET the entry.
			getHandler := getGlobalEnvHandler(holder)
			getReq := httptest.NewRequest(http.MethodGet, "/v1/config/env/"+tt.key, nil)
			getReq.SetPathValue("key", tt.key)
			getRR := httptest.NewRecorder()
			getHandler.ServeHTTP(getRR, getReq)

			if getRR.Code != http.StatusOK {
				t.Fatalf("GET status = %d, want %d", getRR.Code, http.StatusOK)
			}

			var resp globalEnvResponse
			if err := json.NewDecoder(getRR.Body).Decode(&resp); err != nil {
				t.Fatalf("decode GET response: %v", err)
			}

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
	st := &mockStore{}
	holder := NewConfigHolder(emptyGitLabConfig(), nil)

	handler := putGlobalEnvHandler(holder, st)
	req := httptest.NewRequest(http.MethodPut, "/v1/config/env/TEST", bytes.NewReader([]byte("not json")))
	req.SetPathValue("key", "TEST")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

// TestConfigEnvPutStoreError verifies that store errors return 500.
func TestConfigEnvPutStoreError(t *testing.T) {
	st := &mockStore{
		upsertGlobalEnvErr: errMockDatabase,
	}
	holder := NewConfigHolder(emptyGitLabConfig(), nil)

	handler := putGlobalEnvHandler(holder, st)

	reqBody := map[string]any{"value": "test", "scope": "all"}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPut, "/v1/config/env/TEST", bytes.NewReader(body))
	req.SetPathValue("key", "TEST")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}

	// Holder should not be updated on store failure.
	if _, ok := holder.GetGlobalEnvVar("TEST"); ok {
		t.Error("holder should not contain TEST after store failure")
	}
}

// TestConfigEnvDeleteStoreError verifies that store errors return 500.
func TestConfigEnvDeleteStoreError(t *testing.T) {
	st := &mockStore{
		deleteGlobalEnvErr: errMockDatabase,
	}
	holder := NewConfigHolder(emptyGitLabConfig(), map[string]GlobalEnvVar{
		"OLD_KEY": {Value: "val", Scope: domaintypes.GlobalEnvScopeAll, Secret: false},
	})

	handler := deleteGlobalEnvHandler(holder, st)

	req := httptest.NewRequest(http.MethodDelete, "/v1/config/env/OLD_KEY", nil)
	req.SetPathValue("key", "OLD_KEY")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}

	// Holder should not be updated on store failure.
	if _, ok := holder.GetGlobalEnvVar("OLD_KEY"); !ok {
		t.Error("holder should still contain OLD_KEY after store failure")
	}
}

// Helper to create empty GitLab config for tests.
func emptyGitLabConfig() config.GitLabConfig {
	return config.GitLabConfig{}
}
