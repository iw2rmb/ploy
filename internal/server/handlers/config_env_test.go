package handlers

import (
	"net/http"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/config"
)

// TestConfigEnvListReturnsAllEntries verifies that GET /v1/config/env
// returns all key+target pairs sorted by key then target, with secret values redacted.
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

	// Verify sorted order by key.
	if resp[0].Key != "API_KEY" || resp[1].Key != "CA_CERTS_PEM_BUNDLE" {
		t.Errorf("entries not sorted: got %v, %v", resp[0].Key, resp[1].Key)
	}

	// Non-secret entry includes value and target.
	if resp[0].Value != "sk-abc123" {
		t.Errorf("non-secret value = %q, want %q", resp[0].Value, "sk-abc123")
	}
	if resp[0].Target != "steps" {
		t.Errorf("non-secret target = %q, want %q", resp[0].Target, "steps")
	}

	// Secret entry has redacted (empty) value.
	if resp[1].Value != "" {
		t.Errorf("secret value = %q, want empty", resp[1].Value)
	}
	if resp[1].Target != "gates" {
		t.Errorf("secret target = %q, want %q", resp[1].Target, "gates")
	}
}

// TestConfigEnvListMultiTarget verifies that list returns separate entries
// for the same key with different targets.
func TestConfigEnvListMultiTarget(t *testing.T) {
	holder := NewConfigHolder(emptyGitLabConfig(), nil)
	holder.SetGlobalEnvVar("SHARED_KEY", GlobalEnvVar{Value: "val-gates", Target: domaintypes.GlobalEnvTargetGates, Secret: false})
	holder.SetGlobalEnvVar("SHARED_KEY", GlobalEnvVar{Value: "val-steps", Target: domaintypes.GlobalEnvTargetSteps, Secret: false})

	handler := listGlobalEnvHandler(holder)
	rr := doRequest(t, handler, http.MethodGet, "/v1/config/env", nil)

	assertStatus(t, rr, http.StatusOK)

	resp := decodeBody[[]globalEnvListItem](t, rr)

	if len(resp) != 2 {
		t.Fatalf("got %d entries, want 2", len(resp))
	}

	// Sorted by target within key.
	if resp[0].Target != "gates" || resp[1].Target != "steps" {
		t.Errorf("targets = [%q, %q], want [gates, steps]", resp[0].Target, resp[1].Target)
	}
	if resp[0].Value != "val-gates" || resp[1].Value != "val-steps" {
		t.Errorf("values = [%q, %q], want [val-gates, val-steps]", resp[0].Value, resp[1].Value)
	}
}

// TestConfigEnvGetReturnsEntry verifies GET /v1/config/env/{key}
// returns full value (including for secrets) for admin access.
func TestConfigEnvGetReturnsEntry(t *testing.T) {
	holder := NewConfigHolder(emptyGitLabConfig(), map[string]GlobalEnvVar{
		"CODEX_AUTH_JSON": {Value: `{"token":"secret"}`, Target: domaintypes.GlobalEnvTargetSteps, Secret: true},
	})

	handler := getGlobalEnvHandler(holder)

	rr := doRequest(t, handler, http.MethodGet, "/v1/config/env/CODEX_AUTH_JSON", nil, "key", "CODEX_AUTH_JSON")

	assertStatus(t, rr, http.StatusOK)

	resp := decodeBody[globalEnvResponse](t, rr)

	if resp.Key != "CODEX_AUTH_JSON" {
		t.Errorf("Key = %q, want %q", resp.Key, "CODEX_AUTH_JSON")
	}
	if resp.Value != `{"token":"secret"}` {
		t.Errorf("Value = %q, want %q", resp.Value, `{"token":"secret"}`)
	}
	if resp.Target != "steps" {
		t.Errorf("Target = %q, want %q", resp.Target, "steps")
	}
	if !resp.Secret {
		t.Errorf("Secret = %v, want true", resp.Secret)
	}
}

// TestConfigEnvGetWithTargetSelector verifies GET /v1/config/env/{key}?target=
// returns the correct entry when multiple targets exist.
func TestConfigEnvGetWithTargetSelector(t *testing.T) {
	holder := NewConfigHolder(emptyGitLabConfig(), nil)
	holder.SetGlobalEnvVar("MULTI", GlobalEnvVar{Value: "gates-val", Target: domaintypes.GlobalEnvTargetGates, Secret: false})
	holder.SetGlobalEnvVar("MULTI", GlobalEnvVar{Value: "steps-val", Target: domaintypes.GlobalEnvTargetSteps, Secret: false})

	handler := getGlobalEnvHandler(holder)

	rr := doRequest(t, handler, http.MethodGet, "/v1/config/env/MULTI?target=steps", nil, "key", "MULTI")

	assertStatus(t, rr, http.StatusOK)

	resp := decodeBody[globalEnvResponse](t, rr)

	if resp.Value != "steps-val" {
		t.Errorf("Value = %q, want %q", resp.Value, "steps-val")
	}
	if resp.Target != "steps" {
		t.Errorf("Target = %q, want %q", resp.Target, "steps")
	}
}

// TestConfigEnvGetAmbiguityError verifies GET /v1/config/env/{key} returns 409
// when multiple targets exist and no target selector is provided.
func TestConfigEnvGetAmbiguityError(t *testing.T) {
	holder := NewConfigHolder(emptyGitLabConfig(), nil)
	holder.SetGlobalEnvVar("MULTI", GlobalEnvVar{Value: "gates-val", Target: domaintypes.GlobalEnvTargetGates, Secret: false})
	holder.SetGlobalEnvVar("MULTI", GlobalEnvVar{Value: "steps-val", Target: domaintypes.GlobalEnvTargetSteps, Secret: false})

	handler := getGlobalEnvHandler(holder)

	rr := doRequest(t, handler, http.MethodGet, "/v1/config/env/MULTI", nil, "key", "MULTI")

	assertStatus(t, rr, http.StatusConflict)
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
		"target": "gates",
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

	// Verify response uses target field.
	resp := decodeBody[globalEnvResponse](t, rr)
	if resp.Target != "gates" {
		t.Errorf("response Target = %q, want %q", resp.Target, "gates")
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
		"value":  "test-value",
		"target": "steps",
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
		"value":  "test",
		"target": "invalid-target",
	}

	rr := doRequest(t, handler, http.MethodPut, "/v1/config/env/TEST_KEY", reqBody, "key", "TEST_KEY")

	assertStatus(t, rr, http.StatusBadRequest)

	// Store should not be called.
	if st.upsertGlobalEnv.called {
		t.Error("store.UpsertGlobalEnv should not be called for invalid target")
	}
}

// TestConfigEnvPutMultiTarget verifies that PUT with different targets for the same key
// creates separate entries in the holder.
func TestConfigEnvPutMultiTarget(t *testing.T) {
	st := &configStore{}
	holder := NewConfigHolder(emptyGitLabConfig(), nil)

	putHandler := putGlobalEnvHandler(holder, st)

	// PUT with gates target.
	rr := doRequest(t, putHandler, http.MethodPut, "/v1/config/env/SHARED", map[string]any{
		"value":  "gates-val",
		"target": "gates",
		"secret": false,
	}, "key", "SHARED")
	assertStatus(t, rr, http.StatusOK)

	// PUT same key with steps target.
	rr = doRequest(t, putHandler, http.MethodPut, "/v1/config/env/SHARED", map[string]any{
		"value":  "steps-val",
		"target": "steps",
		"secret": false,
	}, "key", "SHARED")
	assertStatus(t, rr, http.StatusOK)

	// Verify both entries exist.
	entries := holder.GetGlobalEnvEntries("SHARED")
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
}

// TestConfigEnvDeleteRemovesEntry verifies DELETE /v1/config/env/{key}?target=
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

// TestConfigEnvDeleteInfersTarget verifies DELETE /v1/config/env/{key} without
// ?target= succeeds when only one target exists for the key.
func TestConfigEnvDeleteInfersTarget(t *testing.T) {
	st := &configStore{}
	holder := NewConfigHolder(emptyGitLabConfig(), map[string]GlobalEnvVar{
		"SINGLE": {Value: "val", Target: domaintypes.GlobalEnvTargetNodes, Secret: false},
	})

	handler := deleteGlobalEnvHandler(holder, st)

	rr := doRequest(t, handler, http.MethodDelete, "/v1/config/env/SINGLE", nil, "key", "SINGLE")

	assertStatus(t, rr, http.StatusNoContent)

	if !st.deleteGlobalEnv.called {
		t.Error("store.DeleteGlobalEnv was not called")
	}
	if st.deleteGlobalEnv.params.Target != "nodes" {
		t.Errorf("inferred target = %q, want %q", st.deleteGlobalEnv.params.Target, "nodes")
	}
}

// TestConfigEnvDeleteAmbiguityError verifies DELETE /v1/config/env/{key}
// returns 409 when multiple targets exist and no target is specified.
func TestConfigEnvDeleteAmbiguityError(t *testing.T) {
	st := &configStore{}
	holder := NewConfigHolder(emptyGitLabConfig(), nil)
	holder.SetGlobalEnvVar("MULTI", GlobalEnvVar{Value: "a", Target: domaintypes.GlobalEnvTargetGates, Secret: false})
	holder.SetGlobalEnvVar("MULTI", GlobalEnvVar{Value: "b", Target: domaintypes.GlobalEnvTargetSteps, Secret: false})

	handler := deleteGlobalEnvHandler(holder, st)

	rr := doRequest(t, handler, http.MethodDelete, "/v1/config/env/MULTI", nil, "key", "MULTI")

	assertStatus(t, rr, http.StatusConflict)

	// Store should not be called.
	if st.deleteGlobalEnv.called {
		t.Error("store.DeleteGlobalEnv should not be called for ambiguous delete")
	}
}

// TestConfigEnvDeleteNonexistentKey verifies DELETE for non-existent key returns 204.
func TestConfigEnvDeleteNonexistentKey(t *testing.T) {
	st := &configStore{}
	holder := NewConfigHolder(emptyGitLabConfig(), nil)

	handler := deleteGlobalEnvHandler(holder, st)

	rr := doRequest(t, handler, http.MethodDelete, "/v1/config/env/GHOST", nil, "key", "GHOST")

	assertStatus(t, rr, http.StatusNoContent)

	// Store should not be called for non-existent key.
	if st.deleteGlobalEnv.called {
		t.Error("store.DeleteGlobalEnv should not be called for non-existent key")
	}
}

// TestConfigEnvRoundTrip verifies PUT followed by GET returns identical values
// using target-based wire format.
func TestConfigEnvRoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		key    string
		value  string
		target string
		secret bool
	}{
		{
			name:   "CA bundle",
			key:    "CA_CERTS_PEM_BUNDLE",
			value:  "-----BEGIN CERTIFICATE-----\nMIIB...\n-----END CERTIFICATE-----",
			target: "gates",
			secret: true,
		},
		{
			name:   "non-secret API key",
			key:    "PUBLIC_API_KEY",
			value:  "pk_live_abc123",
			target: "steps",
			secret: false,
		},
		{
			name:   "codex auth JSON",
			key:    "CODEX_AUTH_JSON",
			value:  `{"api_key":"sk-...","org_id":"org-123"}`,
			target: "server",
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
				"target": tt.target,
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
			if resp.Target != tt.target {
				t.Errorf("Target = %q, want %q", resp.Target, tt.target)
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

	reqBody := map[string]any{"value": "test", "target": "gates"}

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
