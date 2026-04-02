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
		env        map[string][]GlobalEnvVar
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
			jobType: domaintypes.JobTypeMig,
		},
		{
			name:    "empty env leaves spec unchanged",
			spec:    json.RawMessage(`{"foo":"bar"}`),
			env:     map[string][]GlobalEnvVar{},
			jobType: domaintypes.JobTypeMig,
		},
		{
			name:    "nil spec returns error",
			spec:    nil,
			env:     map[string][]GlobalEnvVar{"API_KEY": {{Value: "secret123", Target: domaintypes.GlobalEnvTargetSteps, Secret: true}}},
			jobType: domaintypes.JobTypeMig,
			wantErr: true,
		},
		{
			name:       "empty spec creates env map",
			spec:       json.RawMessage(`{}`),
			env:        map[string][]GlobalEnvVar{"API_KEY": {{Value: "secret123", Target: domaintypes.GlobalEnvTargetSteps, Secret: true}}},
			jobType:    domaintypes.JobTypeMig,
			expectKeys: []string{"API_KEY"},
			checkEnv:   map[string]string{"API_KEY": "secret123"},
		},
		{
			name:       "mig job gets steps target",
			spec:       json.RawMessage(`{}`),
			env:        targetTestEnv(),
			jobType:    domaintypes.JobTypeMig,
			expectKeys: []string{"STEPS_KEY"},
			rejectKeys: []string{"GATES_KEY"},
		},
		{
			name:       "heal job gets steps target",
			spec:       json.RawMessage(`{}`),
			env:        targetTestEnv(),
			jobType:    domaintypes.JobTypeHeal,
			expectKeys: []string{"STEPS_KEY"},
			rejectKeys: []string{"GATES_KEY"},
		},
		{
			name:       "pre_gate job gets gates target",
			spec:       json.RawMessage(`{}`),
			env:        targetTestEnv(),
			jobType:    domaintypes.JobTypePreGate,
			expectKeys: []string{"GATES_KEY"},
			rejectKeys: []string{"STEPS_KEY"},
		},
		{
			name:       "re_gate job gets gates target",
			spec:       json.RawMessage(`{}`),
			env:        targetTestEnv(),
			jobType:    domaintypes.JobTypeReGate,
			expectKeys: []string{"GATES_KEY"},
			rejectKeys: []string{"STEPS_KEY"},
		},
		{
			name:       "post_gate job gets gates target",
			spec:       json.RawMessage(`{}`),
			env:        targetTestEnv(),
			jobType:    domaintypes.JobTypePostGate,
			expectKeys: []string{"GATES_KEY"},
			rejectKeys: []string{"STEPS_KEY"},
		},
		{
			name:    "per-run env takes precedence over global",
			spec:    json.RawMessage(`{"env":{"API_KEY":"per-run-value","OTHER":"existing"}}`),
			env:     map[string][]GlobalEnvVar{"API_KEY": {{Value: "global-value", Target: domaintypes.GlobalEnvTargetSteps}}, "NEW_KEY": {{Value: "new-value", Target: domaintypes.GlobalEnvTargetSteps}}},
			jobType: domaintypes.JobTypeMig,
			checkEnv: map[string]string{
				"API_KEY": "per-run-value",
				"OTHER":   "existing",
				"NEW_KEY": "new-value",
			},
		},
		{
			name:    "preserves other spec fields",
			spec:    json.RawMessage(`{"repo":"github.com/test","timeout":300,"env":{"EXISTING":"yes"}}`),
			env:     map[string][]GlobalEnvVar{"PLOY_CA_CERTS": {{Value: "-----BEGIN CERT-----\n...", Target: domaintypes.GlobalEnvTargetSteps, Secret: true}}},
			jobType: domaintypes.JobTypeMig,
			checkEnv: map[string]string{
				"EXISTING":      "yes",
				"PLOY_CA_CERTS": "-----BEGIN CERT-----\n...",
			},
		},
		{
			name:    "invalid JSON returns error",
			spec:    json.RawMessage(`{invalid`),
			env:     map[string][]GlobalEnvVar{"KEY": {{Value: "value", Target: domaintypes.GlobalEnvTargetSteps}}},
			jobType: domaintypes.JobTypeMig,
			wantErr: true,
		},
		{
			name: "common global keys for mig",
			spec: json.RawMessage(`{}`),
			env: map[string][]GlobalEnvVar{
				"PLOY_CA_CERTS":   {{Value: "-----BEGIN CERTIFICATE-----\nMIID...", Target: domaintypes.GlobalEnvTargetSteps, Secret: true}},
				"CODEX_AUTH_JSON": {{Value: `{"token":"xxx"}`, Target: domaintypes.GlobalEnvTargetSteps, Secret: true}},
				"OPENAI_API_KEY":  {{Value: "sk-...", Target: domaintypes.GlobalEnvTargetSteps, Secret: true}},
			},
			jobType:    domaintypes.JobTypeMig,
			expectKeys: []string{"PLOY_CA_CERTS", "CODEX_AUTH_JSON", "OPENAI_API_KEY"},
		},
		{
			name: "common global keys for pre_gate with gates target",
			spec: json.RawMessage(`{}`),
			env: map[string][]GlobalEnvVar{
				"PLOY_CA_CERTS":   {{Value: "-----BEGIN CERTIFICATE-----\nMIID...", Target: domaintypes.GlobalEnvTargetGates, Secret: true}},
				"CODEX_AUTH_JSON": {{Value: `{"token":"xxx"}`, Target: domaintypes.GlobalEnvTargetSteps, Secret: true}},
				"OPENAI_API_KEY":  {{Value: "sk-...", Target: domaintypes.GlobalEnvTargetSteps, Secret: true}},
			},
			jobType:    domaintypes.JobTypePreGate,
			expectKeys: []string{"PLOY_CA_CERTS"},
			rejectKeys: []string{"CODEX_AUTH_JSON", "OPENAI_API_KEY"},
		},
		// --- Step 4: target-aware merge precedence tests ---
		{
			name: "nodes-target provides fallback for mig job",
			spec: json.RawMessage(`{}`),
			env: map[string][]GlobalEnvVar{
				"SHARED_KEY": {{Value: "nodes-val", Target: domaintypes.GlobalEnvTargetNodes}},
			},
			jobType:  domaintypes.JobTypeMig,
			checkEnv: map[string]string{"SHARED_KEY": "nodes-val"},
		},
		{
			name: "nodes-target provides fallback for gate job",
			spec: json.RawMessage(`{}`),
			env: map[string][]GlobalEnvVar{
				"SHARED_KEY": {{Value: "nodes-val", Target: domaintypes.GlobalEnvTargetNodes}},
			},
			jobType:  domaintypes.JobTypePreGate,
			checkEnv: map[string]string{"SHARED_KEY": "nodes-val"},
		},
		{
			name: "job-target overrides nodes-target on key collision",
			spec: json.RawMessage(`{}`),
			env: map[string][]GlobalEnvVar{
				"SHARED_KEY": {
					{Value: "nodes-val", Target: domaintypes.GlobalEnvTargetNodes},
					{Value: "steps-val", Target: domaintypes.GlobalEnvTargetSteps},
				},
			},
			jobType:  domaintypes.JobTypeMig,
			checkEnv: map[string]string{"SHARED_KEY": "steps-val"},
		},
		{
			name: "gates-target overrides nodes-target for gate job",
			spec: json.RawMessage(`{}`),
			env: map[string][]GlobalEnvVar{
				"SHARED_KEY": {
					{Value: "nodes-val", Target: domaintypes.GlobalEnvTargetNodes},
					{Value: "gates-val", Target: domaintypes.GlobalEnvTargetGates},
				},
			},
			jobType:  domaintypes.JobTypePostGate,
			checkEnv: map[string]string{"SHARED_KEY": "gates-val"},
		},
		{
			name: "per-run env overrides both job-target and nodes-target",
			spec: json.RawMessage(`{"env":{"SHARED_KEY":"per-run-val"}}`),
			env: map[string][]GlobalEnvVar{
				"SHARED_KEY": {
					{Value: "nodes-val", Target: domaintypes.GlobalEnvTargetNodes},
					{Value: "steps-val", Target: domaintypes.GlobalEnvTargetSteps},
				},
			},
			jobType:  domaintypes.JobTypeMig,
			checkEnv: map[string]string{"SHARED_KEY": "per-run-val"},
		},
		{
			name: "server-target not injected into jobs",
			spec: json.RawMessage(`{}`),
			env: map[string][]GlobalEnvVar{
				"SERVER_ONLY": {{Value: "server-val", Target: domaintypes.GlobalEnvTargetServer}},
			},
			jobType:    domaintypes.JobTypeMig,
			rejectKeys: []string{"SERVER_ONLY"},
		},
		{
			name: "nodes-target with no matching job-target",
			spec: json.RawMessage(`{}`),
			env: map[string][]GlobalEnvVar{
				"STEPS_KEY": {{Value: "steps-val", Target: domaintypes.GlobalEnvTargetSteps}},
				"NODES_KEY": {{Value: "nodes-val", Target: domaintypes.GlobalEnvTargetNodes}},
			},
			jobType:    domaintypes.JobTypePreGate,
			expectKeys: []string{"NODES_KEY"},
			rejectKeys: []string{"STEPS_KEY"},
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

func targetTestEnv() map[string][]GlobalEnvVar {
	return map[string][]GlobalEnvVar{
		"GATES_KEY": {{Value: "gates-value", Target: domaintypes.GlobalEnvTargetGates}},
		"STEPS_KEY": {{Value: "steps-value", Target: domaintypes.GlobalEnvTargetSteps}},
	}
}
