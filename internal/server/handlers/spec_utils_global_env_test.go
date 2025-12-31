package handlers

import (
	"encoding/json"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// TestScopeMatches tests the scope matching logic that determines whether
// a global env var should be injected based on job type and env var scope.
func TestScopeMatches(t *testing.T) {
	tests := []struct {
		name     string
		jobType  string
		scope    string
		expected bool
	}{
		// "all" scope matches every job type.
		{name: "all scope with mod job", jobType: "mod", scope: "all", expected: true},
		{name: "all scope with heal job", jobType: "heal", scope: "all", expected: true},
		{name: "all scope with pre_gate job", jobType: "pre_gate", scope: "all", expected: true},
		{name: "all scope with re_gate job", jobType: "re_gate", scope: "all", expected: true},
		{name: "all scope with post_gate job", jobType: "post_gate", scope: "all", expected: true},

		// "mods" scope matches mod and post_gate jobs.
		{name: "mods scope with mod job", jobType: "mod", scope: "mods", expected: true},
		{name: "mods scope with post_gate job", jobType: "post_gate", scope: "mods", expected: true},
		{name: "mods scope with heal job", jobType: "heal", scope: "mods", expected: false},
		{name: "mods scope with pre_gate job", jobType: "pre_gate", scope: "mods", expected: false},
		{name: "mods scope with re_gate job", jobType: "re_gate", scope: "mods", expected: false},

		// "heal" scope matches heal and re_gate jobs.
		{name: "heal scope with heal job", jobType: "heal", scope: "heal", expected: true},
		{name: "heal scope with re_gate job", jobType: "re_gate", scope: "heal", expected: true},
		{name: "heal scope with mod job", jobType: "mod", scope: "heal", expected: false},
		{name: "heal scope with pre_gate job", jobType: "pre_gate", scope: "heal", expected: false},
		{name: "heal scope with post_gate job", jobType: "post_gate", scope: "heal", expected: false},

		// "gate" scope matches all gate-related jobs.
		{name: "gate scope with pre_gate job", jobType: "pre_gate", scope: "gate", expected: true},
		{name: "gate scope with re_gate job", jobType: "re_gate", scope: "gate", expected: true},
		{name: "gate scope with post_gate job", jobType: "post_gate", scope: "gate", expected: true},
		{name: "gate scope with mod job", jobType: "mod", scope: "gate", expected: false},
		{name: "gate scope with heal job", jobType: "heal", scope: "gate", expected: false},

		// Unknown scopes should not match.
		{name: "unknown scope", jobType: "mod", scope: "unknown", expected: false},
		{name: "empty scope", jobType: "mod", scope: "", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scopeMatches(tt.jobType, tt.scope)
			if got != tt.expected {
				t.Errorf("scopeMatches(%q, %q) = %v, want %v",
					tt.jobType, tt.scope, got, tt.expected)
			}
		})
	}
}

// TestMergeGlobalEnvIntoSpec_EmptyEnv verifies that when no global env vars
// are provided, the spec is returned unchanged.
func TestMergeGlobalEnvIntoSpec_EmptyEnv(t *testing.T) {
	spec := json.RawMessage(`{"foo":"bar"}`)
	result := mergeGlobalEnvIntoSpec(spec, nil, "mod")
	if string(result) != string(spec) {
		t.Errorf("expected spec unchanged, got %s", result)
	}

	result = mergeGlobalEnvIntoSpec(spec, map[string]GlobalEnvVar{}, "mod")
	if string(result) != string(spec) {
		t.Errorf("expected spec unchanged, got %s", result)
	}
}

// TestMergeGlobalEnvIntoSpec_EmptySpec verifies that merging into an empty/nil
// spec correctly creates the env map.
func TestMergeGlobalEnvIntoSpec_EmptySpec(t *testing.T) {
	env := map[string]GlobalEnvVar{
		"API_KEY": {Value: "secret123", Scope: domaintypes.GlobalEnvScopeAll, Secret: true},
	}

	// Test with nil spec.
	result := mergeGlobalEnvIntoSpec(nil, env, "mod")
	var m map[string]any
	if err := json.Unmarshal(result, &m); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}
	em, ok := m["env"].(map[string]any)
	if !ok {
		t.Fatalf("expected env map in result, got %v", m)
	}
	if em["API_KEY"] != "secret123" {
		t.Errorf("expected API_KEY=secret123, got %v", em["API_KEY"])
	}

	// Test with empty JSON spec.
	result = mergeGlobalEnvIntoSpec(json.RawMessage(`{}`), env, "mod")
	if err := json.Unmarshal(result, &m); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}
	em, ok = m["env"].(map[string]any)
	if !ok {
		t.Fatalf("expected env map in result, got %v", m)
	}
	if em["API_KEY"] != "secret123" {
		t.Errorf("expected API_KEY=secret123, got %v", em["API_KEY"])
	}
}

// TestMergeGlobalEnvIntoSpec_ScopeFiltering verifies that only env vars
// with matching scopes are merged into the spec.
func TestMergeGlobalEnvIntoSpec_ScopeFiltering(t *testing.T) {
	env := map[string]GlobalEnvVar{
		"ALL_KEY":  {Value: "all-value", Scope: domaintypes.GlobalEnvScopeAll, Secret: false},
		"MODS_KEY": {Value: "mods-value", Scope: domaintypes.GlobalEnvScopeMods, Secret: false},
		"HEAL_KEY": {Value: "heal-value", Scope: domaintypes.GlobalEnvScopeHeal, Secret: false},
		"GATE_KEY": {Value: "gate-value", Scope: domaintypes.GlobalEnvScopeGate, Secret: false},
	}

	tests := []struct {
		name       string
		jobType    string
		expectKeys []string
		rejectKeys []string
	}{
		{
			name:       "mod job gets all and mods",
			jobType:    "mod",
			expectKeys: []string{"ALL_KEY", "MODS_KEY"},
			rejectKeys: []string{"HEAL_KEY", "GATE_KEY"},
		},
		{
			name:       "heal job gets all and heal",
			jobType:    "heal",
			expectKeys: []string{"ALL_KEY", "HEAL_KEY"},
			rejectKeys: []string{"MODS_KEY", "GATE_KEY"},
		},
		{
			name:       "pre_gate job gets all and gate",
			jobType:    "pre_gate",
			expectKeys: []string{"ALL_KEY", "GATE_KEY"},
			rejectKeys: []string{"MODS_KEY", "HEAL_KEY"},
		},
		{
			name:       "re_gate job gets all, heal, and gate",
			jobType:    "re_gate",
			expectKeys: []string{"ALL_KEY", "HEAL_KEY", "GATE_KEY"},
			rejectKeys: []string{"MODS_KEY"},
		},
		{
			name:       "post_gate job gets all, mods, and gate",
			jobType:    "post_gate",
			expectKeys: []string{"ALL_KEY", "MODS_KEY", "GATE_KEY"},
			rejectKeys: []string{"HEAL_KEY"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mergeGlobalEnvIntoSpec(json.RawMessage(`{}`), env, tt.jobType)
			var m map[string]any
			if err := json.Unmarshal(result, &m); err != nil {
				t.Fatalf("failed to unmarshal result: %v", err)
			}
			em, _ := m["env"].(map[string]any)

			// Check expected keys are present.
			for _, key := range tt.expectKeys {
				if _, ok := em[key]; !ok {
					t.Errorf("expected key %q to be present for job type %q", key, tt.jobType)
				}
			}

			// Check rejected keys are absent.
			for _, key := range tt.rejectKeys {
				if _, ok := em[key]; ok {
					t.Errorf("expected key %q to be absent for job type %q", key, tt.jobType)
				}
			}
		})
	}
}

// TestMergeGlobalEnvIntoSpec_PerRunEnvPrecedence verifies that per-run env vars
// in the spec take precedence over global env vars (existing keys are not overwritten).
func TestMergeGlobalEnvIntoSpec_PerRunEnvPrecedence(t *testing.T) {
	// Spec already has API_KEY set to a different value.
	spec := json.RawMessage(`{"env":{"API_KEY":"per-run-value","OTHER":"existing"}}`)

	env := map[string]GlobalEnvVar{
		"API_KEY": {Value: "global-value", Scope: domaintypes.GlobalEnvScopeAll, Secret: true},
		"NEW_KEY": {Value: "new-value", Scope: domaintypes.GlobalEnvScopeAll, Secret: false},
	}

	result := mergeGlobalEnvIntoSpec(spec, env, "mod")
	var m map[string]any
	if err := json.Unmarshal(result, &m); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}
	em, ok := m["env"].(map[string]any)
	if !ok {
		t.Fatalf("expected env map in result, got %v", m)
	}

	// Per-run value should win.
	if em["API_KEY"] != "per-run-value" {
		t.Errorf("expected API_KEY=per-run-value (per-run precedence), got %v", em["API_KEY"])
	}

	// Existing per-run env should be preserved.
	if em["OTHER"] != "existing" {
		t.Errorf("expected OTHER=existing, got %v", em["OTHER"])
	}

	// New global key should be added.
	if em["NEW_KEY"] != "new-value" {
		t.Errorf("expected NEW_KEY=new-value, got %v", em["NEW_KEY"])
	}
}

// TestMergeGlobalEnvIntoSpec_PreservesOtherSpecFields verifies that merging
// global env does not clobber other fields in the spec.
func TestMergeGlobalEnvIntoSpec_PreservesOtherSpecFields(t *testing.T) {
	spec := json.RawMessage(`{"repo":"github.com/test","timeout":300,"env":{"EXISTING":"yes"}}`)

	env := map[string]GlobalEnvVar{
		"CA_CERTS_PEM_BUNDLE": {Value: "-----BEGIN CERT-----\n...", Scope: domaintypes.GlobalEnvScopeAll, Secret: true},
	}

	result := mergeGlobalEnvIntoSpec(spec, env, "mod")
	var m map[string]any
	if err := json.Unmarshal(result, &m); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	// Verify other spec fields are preserved.
	if m["repo"] != "github.com/test" {
		t.Errorf("expected repo field preserved, got %v", m["repo"])
	}
	if m["timeout"] != float64(300) { // JSON numbers unmarshal as float64.
		t.Errorf("expected timeout=300, got %v", m["timeout"])
	}

	// Verify env fields.
	em := m["env"].(map[string]any)
	if em["EXISTING"] != "yes" {
		t.Errorf("expected EXISTING=yes, got %v", em["EXISTING"])
	}
	if em["CA_CERTS_PEM_BUNDLE"] != "-----BEGIN CERT-----\n..." {
		t.Errorf("expected CA_CERTS_PEM_BUNDLE to be set")
	}
}

// TestMergeGlobalEnvIntoSpec_InvalidJSON verifies that invalid JSON specs
// are handled gracefully by creating a new map.
func TestMergeGlobalEnvIntoSpec_InvalidJSON(t *testing.T) {
	env := map[string]GlobalEnvVar{
		"KEY": {Value: "value", Scope: domaintypes.GlobalEnvScopeAll, Secret: false},
	}

	// Invalid JSON should not cause a crash; global env should still be injected.
	result := mergeGlobalEnvIntoSpec(json.RawMessage(`{invalid`), env, "mod")
	var m map[string]any
	if err := json.Unmarshal(result, &m); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}
	em, _ := m["env"].(map[string]any)
	if em["KEY"] != "value" {
		t.Errorf("expected KEY=value even with invalid input spec, got %v", em["KEY"])
	}
}

// TestMergeGlobalEnvIntoSpec_CommonGlobalEnvKeys tests injection of commonly
// used global env keys like CA_CERTS_PEM_BUNDLE and CODEX_AUTH_JSON.
func TestMergeGlobalEnvIntoSpec_CommonGlobalEnvKeys(t *testing.T) {
	env := map[string]GlobalEnvVar{
		"CA_CERTS_PEM_BUNDLE": {Value: "-----BEGIN CERTIFICATE-----\nMIID...", Scope: domaintypes.GlobalEnvScopeAll, Secret: true},
		"CODEX_AUTH_JSON":     {Value: `{"token":"xxx"}`, Scope: domaintypes.GlobalEnvScopeMods, Secret: true},
		"OPENAI_API_KEY":      {Value: "sk-...", Scope: domaintypes.GlobalEnvScopeMods, Secret: true},
	}

	// Test mod job should receive all three keys.
	result := mergeGlobalEnvIntoSpec(json.RawMessage(`{}`), env, "mod")
	var m map[string]any
	if err := json.Unmarshal(result, &m); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}
	em := m["env"].(map[string]any)

	if _, ok := em["CA_CERTS_PEM_BUNDLE"]; !ok {
		t.Error("expected CA_CERTS_PEM_BUNDLE for mod job")
	}
	if _, ok := em["CODEX_AUTH_JSON"]; !ok {
		t.Error("expected CODEX_AUTH_JSON for mod job")
	}
	if _, ok := em["OPENAI_API_KEY"]; !ok {
		t.Error("expected OPENAI_API_KEY for mod job")
	}

	// Test pre_gate job should only receive CA_CERTS_PEM_BUNDLE (scope=all).
	result = mergeGlobalEnvIntoSpec(json.RawMessage(`{}`), env, "pre_gate")
	if err := json.Unmarshal(result, &m); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}
	em = m["env"].(map[string]any)

	if _, ok := em["CA_CERTS_PEM_BUNDLE"]; !ok {
		t.Error("expected CA_CERTS_PEM_BUNDLE for pre_gate job")
	}
	if _, ok := em["CODEX_AUTH_JSON"]; ok {
		t.Error("CODEX_AUTH_JSON should not be present for pre_gate job (scope=mods)")
	}
	if _, ok := em["OPENAI_API_KEY"]; ok {
		t.Error("OPENAI_API_KEY should not be present for pre_gate job (scope=mods)")
	}
}
