package handlers

import (
	"encoding/json"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestApplyGlobalEnvMutator(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		spec       json.RawMessage
		env        map[string]GlobalEnvVar
		jobType    domaintypes.JobType
		wantErr    bool
		expectKeys []string
		rejectKeys []string
		checkEnv   map[string]string // exact value checks
	}{
		{
			name:    "nil env leaves spec unchanged",
			spec:    json.RawMessage(`{"foo":"bar"}`),
			env:     nil,
			jobType: domaintypes.JobTypeMod,
		},
		{
			name:    "empty env leaves spec unchanged",
			spec:    json.RawMessage(`{"foo":"bar"}`),
			env:     map[string]GlobalEnvVar{},
			jobType: domaintypes.JobTypeMod,
		},
		{
			name:    "nil spec returns error",
			spec:    nil,
			env:     map[string]GlobalEnvVar{"API_KEY": {Value: "secret123", Scope: domaintypes.GlobalEnvScopeAll, Secret: true}},
			jobType: domaintypes.JobTypeMod,
			wantErr: true,
		},
		{
			name:       "empty spec creates env map",
			spec:       json.RawMessage(`{}`),
			env:        map[string]GlobalEnvVar{"API_KEY": {Value: "secret123", Scope: domaintypes.GlobalEnvScopeAll, Secret: true}},
			jobType:    domaintypes.JobTypeMod,
			expectKeys: []string{"API_KEY"},
			checkEnv:   map[string]string{"API_KEY": "secret123"},
		},
		{
			name:       "mig job gets all and mods scope",
			spec:       json.RawMessage(`{}`),
			env:        scopeTestEnv(),
			jobType:    domaintypes.JobTypeMod,
			expectKeys: []string{"ALL_KEY", "MODS_KEY"},
			rejectKeys: []string{"HEAL_KEY", "GATE_KEY"},
		},
		{
			name:       "heal job gets all and heal scope",
			spec:       json.RawMessage(`{}`),
			env:        scopeTestEnv(),
			jobType:    domaintypes.JobTypeHeal,
			expectKeys: []string{"ALL_KEY", "HEAL_KEY"},
			rejectKeys: []string{"MODS_KEY", "GATE_KEY"},
		},
		{
			name:       "pre_gate job gets all and gate scope",
			spec:       json.RawMessage(`{}`),
			env:        scopeTestEnv(),
			jobType:    domaintypes.JobTypePreGate,
			expectKeys: []string{"ALL_KEY", "GATE_KEY"},
			rejectKeys: []string{"MODS_KEY", "HEAL_KEY"},
		},
		{
			name:       "re_gate job gets all, heal, and gate scope",
			spec:       json.RawMessage(`{}`),
			env:        scopeTestEnv(),
			jobType:    domaintypes.JobTypeReGate,
			expectKeys: []string{"ALL_KEY", "HEAL_KEY", "GATE_KEY"},
			rejectKeys: []string{"MODS_KEY"},
		},
		{
			name:       "post_gate job gets all, mods, and gate scope",
			spec:       json.RawMessage(`{}`),
			env:        scopeTestEnv(),
			jobType:    domaintypes.JobTypePostGate,
			expectKeys: []string{"ALL_KEY", "MODS_KEY", "GATE_KEY"},
			rejectKeys: []string{"HEAL_KEY"},
		},
		{
			name:    "per-run env takes precedence over global",
			spec:    json.RawMessage(`{"env":{"API_KEY":"per-run-value","OTHER":"existing"}}`),
			env:     map[string]GlobalEnvVar{"API_KEY": {Value: "global-value", Scope: domaintypes.GlobalEnvScopeAll}, "NEW_KEY": {Value: "new-value", Scope: domaintypes.GlobalEnvScopeAll}},
			jobType: domaintypes.JobTypeMod,
			checkEnv: map[string]string{
				"API_KEY": "per-run-value",
				"OTHER":   "existing",
				"NEW_KEY": "new-value",
			},
		},
		{
			name:    "preserves other spec fields",
			spec:    json.RawMessage(`{"repo":"github.com/test","timeout":300,"env":{"EXISTING":"yes"}}`),
			env:     map[string]GlobalEnvVar{"CA_CERTS_PEM_BUNDLE": {Value: "-----BEGIN CERT-----\n...", Scope: domaintypes.GlobalEnvScopeAll, Secret: true}},
			jobType: domaintypes.JobTypeMod,
			checkEnv: map[string]string{
				"EXISTING":           "yes",
				"CA_CERTS_PEM_BUNDLE": "-----BEGIN CERT-----\n...",
			},
		},
		{
			name:    "invalid JSON returns error",
			spec:    json.RawMessage(`{invalid`),
			env:     map[string]GlobalEnvVar{"KEY": {Value: "value", Scope: domaintypes.GlobalEnvScopeAll}},
			jobType: domaintypes.JobTypeMod,
			wantErr: true,
		},
		{
			name:    "common global keys for mig",
			spec:    json.RawMessage(`{}`),
			env: map[string]GlobalEnvVar{
				"CA_CERTS_PEM_BUNDLE": {Value: "-----BEGIN CERTIFICATE-----\nMIID...", Scope: domaintypes.GlobalEnvScopeAll, Secret: true},
				"CODEX_AUTH_JSON":     {Value: `{"token":"xxx"}`, Scope: domaintypes.GlobalEnvScopeMods, Secret: true},
				"OPENAI_API_KEY":      {Value: "sk-...", Scope: domaintypes.GlobalEnvScopeMods, Secret: true},
			},
			jobType:    domaintypes.JobTypeMod,
			expectKeys: []string{"CA_CERTS_PEM_BUNDLE", "CODEX_AUTH_JSON", "OPENAI_API_KEY"},
		},
		{
			name:    "common global keys for pre_gate",
			spec:    json.RawMessage(`{}`),
			env: map[string]GlobalEnvVar{
				"CA_CERTS_PEM_BUNDLE": {Value: "-----BEGIN CERTIFICATE-----\nMIID...", Scope: domaintypes.GlobalEnvScopeAll, Secret: true},
				"CODEX_AUTH_JSON":     {Value: `{"token":"xxx"}`, Scope: domaintypes.GlobalEnvScopeMods, Secret: true},
				"OPENAI_API_KEY":      {Value: "sk-...", Scope: domaintypes.GlobalEnvScopeMods, Secret: true},
			},
			jobType:    domaintypes.JobTypePreGate,
			expectKeys: []string{"CA_CERTS_PEM_BUNDLE"},
			rejectKeys: []string{"CODEX_AUTH_JSON", "OPENAI_API_KEY"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m, err := parseSpecObjectStrict(tt.spec)
			if err != nil {
				if tt.wantErr {
					return
				}
				t.Fatalf("parseSpecObjectStrict: %v", err)
			}

			if err := applyGlobalEnvMutator(m, tt.env, tt.jobType); err != nil {
				if tt.wantErr {
					return
				}
				t.Fatalf("applyGlobalEnvMutator: %v", err)
			}
			if tt.wantErr {
				t.Fatalf("expected error, got nil")
			}

			em, _ := m["env"].(map[string]any)

			for _, key := range tt.expectKeys {
				if _, ok := em[key]; !ok {
					t.Errorf("expected key %q to be present", key)
				}
			}
			for _, key := range tt.rejectKeys {
				if _, ok := em[key]; ok {
					t.Errorf("expected key %q to be absent", key)
				}
			}
			for key, want := range tt.checkEnv {
				if em == nil {
					t.Fatalf("env map is nil, expected key %q=%q", key, want)
				}
				if got := em[key]; got != want {
					t.Errorf("env[%q] = %v, want %q", key, got, want)
				}
			}
		})
	}
}

func scopeTestEnv() map[string]GlobalEnvVar {
	return map[string]GlobalEnvVar{
		"ALL_KEY":  {Value: "all-value", Scope: domaintypes.GlobalEnvScopeAll},
		"MODS_KEY": {Value: "migs-value", Scope: domaintypes.GlobalEnvScopeMods},
		"HEAL_KEY": {Value: "heal-value", Scope: domaintypes.GlobalEnvScopeHeal},
		"GATE_KEY": {Value: "gate-value", Scope: domaintypes.GlobalEnvScopeGate},
	}
}
