package handlers

import (
	"encoding/json"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// TestMergeGlobalEnvIntoSpec_EmptyEnv verifies that when no global env vars
// are provided, the spec is returned unchanged.
func TestMergeGlobalEnvIntoSpec_EmptyEnv(t *testing.T) {
	spec := json.RawMessage(`{"foo":"bar"}`)
	result, err := mergeGlobalEnvIntoSpec(spec, nil, domaintypes.JobTypeMod)
	if err != nil {
		t.Fatalf("mergeGlobalEnvIntoSpec: %v", err)
	}
	if string(result) != string(spec) {
		t.Errorf("expected spec unchanged, got %s", result)
	}

	result, err = mergeGlobalEnvIntoSpec(spec, map[string]GlobalEnvVar{}, domaintypes.JobTypeMod)
	if err != nil {
		t.Fatalf("mergeGlobalEnvIntoSpec: %v", err)
	}
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
	_, err := mergeGlobalEnvIntoSpec(nil, env, domaintypes.JobTypeMod)
	if err == nil {
		t.Fatalf("expected error for nil spec, got nil")
	}

	// Test with empty JSON spec.
	result, err := mergeGlobalEnvIntoSpec(json.RawMessage(`{}`), env, domaintypes.JobTypeMod)
	if err != nil {
		t.Fatalf("mergeGlobalEnvIntoSpec: %v", err)
	}
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
}

// TestMergeGlobalEnvIntoSpec_ScopeFiltering verifies that only env vars
// with matching scopes are merged into the spec.
func TestMergeGlobalEnvIntoSpec_ScopeFiltering(t *testing.T) {
	env := map[string]GlobalEnvVar{
		"ALL_KEY":  {Value: "all-value", Scope: domaintypes.GlobalEnvScopeAll, Secret: false},
		"MODS_KEY": {Value: "migs-value", Scope: domaintypes.GlobalEnvScopeMods, Secret: false},
		"HEAL_KEY": {Value: "heal-value", Scope: domaintypes.GlobalEnvScopeHeal, Secret: false},
		"GATE_KEY": {Value: "gate-value", Scope: domaintypes.GlobalEnvScopeGate, Secret: false},
	}

	tests := []struct {
		name       string
		jobType    domaintypes.JobType
		expectKeys []string
		rejectKeys []string
	}{
		{
			name:       "mod job gets all and migs",
			jobType:    domaintypes.JobTypeMod,
			expectKeys: []string{"ALL_KEY", "MODS_KEY"},
			rejectKeys: []string{"HEAL_KEY", "GATE_KEY"},
		},
		{
			name:       "heal job gets all and heal",
			jobType:    domaintypes.JobTypeHeal,
			expectKeys: []string{"ALL_KEY", "HEAL_KEY"},
			rejectKeys: []string{"MODS_KEY", "GATE_KEY"},
		},
		{
			name:       "pre_gate job gets all and gate",
			jobType:    domaintypes.JobTypePreGate,
			expectKeys: []string{"ALL_KEY", "GATE_KEY"},
			rejectKeys: []string{"MODS_KEY", "HEAL_KEY"},
		},
		{
			name:       "re_gate job gets all, heal, and gate",
			jobType:    domaintypes.JobTypeReGate,
			expectKeys: []string{"ALL_KEY", "HEAL_KEY", "GATE_KEY"},
			rejectKeys: []string{"MODS_KEY"},
		},
		{
			name:       "post_gate job gets all, migs, and gate",
			jobType:    domaintypes.JobTypePostGate,
			expectKeys: []string{"ALL_KEY", "MODS_KEY", "GATE_KEY"},
			rejectKeys: []string{"HEAL_KEY"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := mergeGlobalEnvIntoSpec(json.RawMessage(`{}`), env, tt.jobType)
			if err != nil {
				t.Fatalf("mergeGlobalEnvIntoSpec: %v", err)
			}
			var m map[string]any
			if err := json.Unmarshal(result, &m); err != nil {
				t.Fatalf("failed to unmarshal result: %v", err)
			}
			em, _ := m["env"].(map[string]any)

			// Check expected keys are present.
			for _, key := range tt.expectKeys {
				if _, ok := em[key]; !ok {
					t.Errorf("expected key %q to be present for job type %q", key, tt.jobType.String())
				}
			}

			// Check rejected keys are absent.
			for _, key := range tt.rejectKeys {
				if _, ok := em[key]; ok {
					t.Errorf("expected key %q to be absent for job type %q", key, tt.jobType.String())
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

	result, err := mergeGlobalEnvIntoSpec(spec, env, domaintypes.JobTypeMod)
	if err != nil {
		t.Fatalf("mergeGlobalEnvIntoSpec: %v", err)
	}
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

	result, err := mergeGlobalEnvIntoSpec(spec, env, domaintypes.JobTypeMod)
	if err != nil {
		t.Fatalf("mergeGlobalEnvIntoSpec: %v", err)
	}
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

	_, err := mergeGlobalEnvIntoSpec(json.RawMessage(`{invalid`), env, domaintypes.JobTypeMod)
	if err == nil {
		t.Fatalf("expected error, got nil")
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
	result, err := mergeGlobalEnvIntoSpec(json.RawMessage(`{}`), env, domaintypes.JobTypeMod)
	if err != nil {
		t.Fatalf("mergeGlobalEnvIntoSpec: %v", err)
	}
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
	result, err = mergeGlobalEnvIntoSpec(json.RawMessage(`{}`), env, domaintypes.JobTypePreGate)
	if err != nil {
		t.Fatalf("mergeGlobalEnvIntoSpec: %v", err)
	}
	if err := json.Unmarshal(result, &m); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}
	em = m["env"].(map[string]any)

	if _, ok := em["CA_CERTS_PEM_BUNDLE"]; !ok {
		t.Error("expected CA_CERTS_PEM_BUNDLE for pre_gate job")
	}
	if _, ok := em["CODEX_AUTH_JSON"]; ok {
		t.Error("CODEX_AUTH_JSON should not be present for pre_gate job (scope=migs)")
	}
	if _, ok := em["OPENAI_API_KEY"]; ok {
		t.Error("OPENAI_API_KEY should not be present for pre_gate job (scope=migs)")
	}
}
