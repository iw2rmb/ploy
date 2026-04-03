// claimer_global_env_test.go verifies that global environment variables injected
// into job specs by the server arrive intact in containers and gate jobs.
//
// This test file covers the env propagation path:
//
//	spec JSON → parseSpec → StartRunRequest.Env → buildManifestFromRequest → manifest.Env
//
// The tests ensure that:
//   - Global env vars (e.g., CODEX_AUTH_JSON, PLOY_CA_CERTS, OPENAI_API_KEY)
//     pass through the full claim → manifest pipeline without filtering.
//   - Per-run env values override global env when both are present.
//   - Gate manifests (buildGateManifestFromRequest) preserve env vars.
//   - Multi-step runs (steps[]) merge base env with step-specific env.
//
// These tests complement the server-side spec_utils_global_env_test.go which
// verifies mergeGlobalEnvIntoSpec semantics. Together they form an end-to-end
// contract for the global env feature (see docs/envs/README.md#Global Env Configuration).
package nodeagent

import (
	"encoding/json"
	"testing"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// TestParseSpec_GlobalEnvFromServerClaim verifies that parseSpec correctly
// extracts global environment variables injected by the server into the
// spec's "env" block. This is the first step in the env propagation chain.
//
// Global env vars are injected by the server via mergeGlobalEnvIntoSpec in
// nodes_claim.go and should arrive in the node agent's env map unchanged.
func TestParseSpec_GlobalEnvFromServerClaim(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		spec    json.RawMessage
		wantEnv map[string]string
	}{
		{
			name: "global_env_vars_extracted",
			spec: json.RawMessage(`{
				"job_id": "` + testKSUID + `",
				"steps": [{"image": "docker.io/test/mig:latest"}],
				"envs": {
					"PLOY_CA_CERTS": "-----BEGIN CERTIFICATE-----\nMIIBkTCC...\n-----END CERTIFICATE-----",
					"CODEX_AUTH_JSON": "{\"api_key\":\"sk-xxx\",\"org_id\":\"org-yyy\"}",
					"OPENAI_API_KEY": "sk-openai-test-key-12345"
				}
			}`),
			wantEnv: map[string]string{
				"PLOY_CA_CERTS":   "-----BEGIN CERTIFICATE-----\nMIIBkTCC...\n-----END CERTIFICATE-----",
				"CODEX_AUTH_JSON": `{"api_key":"sk-xxx","org_id":"org-yyy"}`,
				"OPENAI_API_KEY":  "sk-openai-test-key-12345",
			},
		},
		{
			// parseSpec only extracts top-level env. Nested/unknown blocks must not
			// be merged into the container env map.
			name: "nested_env_not_merged",
			spec: json.RawMessage(`{
					"steps": [{"image": "docker.io/test/mig:latest"}],
					"envs": {
						"GLOBAL_VAR": "global_value",
						"SHARED_VAR": "top_level_value"
					},
					"ignored": {
						"image": "test/mig:latest",
						"env": {
							"MIG_VAR": "mig_value",
							"SHARED_VAR": "mig_ignored"
						}
					}
				}`),
			wantEnv: map[string]string{
				// Only top-level env is extracted; mig.env is ignored.
				"GLOBAL_VAR": "global_value",
				"SHARED_VAR": "top_level_value",
				// MIG_VAR is NOT present because mig.env is not processed.
			},
		},
		{
			name: "empty_env_values_preserved",
			spec: json.RawMessage(`{
				"steps": [{"image": "docker.io/test/mig:latest"}],
				"envs": {
					"EMPTY_VAR": "",
					"WHITESPACE_VAR": "   "
				}
			}`),
			wantEnv: map[string]string{
				"EMPTY_VAR":      "",
				"WHITESPACE_VAR": "   ",
			},
		},
		{
			name: "multiline_cert_bundle_preserved",
			spec: json.RawMessage(`{
				"steps": [{"image": "docker.io/test/mig:latest"}],
				"envs": {
					"PLOY_CA_CERTS": "-----BEGIN CERTIFICATE-----\nMIIBkT...\n-----END CERTIFICATE-----\n-----BEGIN CERTIFICATE-----\nMIICaT...\n-----END CERTIFICATE-----"
				}
			}`),
			wantEnv: map[string]string{
				"PLOY_CA_CERTS": "-----BEGIN CERTIFICATE-----\nMIIBkT...\n-----END CERTIFICATE-----\n-----BEGIN CERTIFICATE-----\nMIICaT...\n-----END CERTIFICATE-----",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			env, _, _ := parseSpec(tc.spec)

			// Verify all expected env vars are present with correct values.
			for key, wantVal := range tc.wantEnv {
				gotVal, ok := env[key]
				if !ok {
					t.Errorf("env missing key %q", key)
					continue
				}
				if gotVal != wantVal {
					t.Errorf("env[%q] = %q, want %q", key, gotVal, wantVal)
				}
			}

			// Verify no unexpected keys are present (exact match).
			if len(env) != len(tc.wantEnv) {
				t.Errorf("env has %d keys, want %d", len(env), len(tc.wantEnv))
			}
		})
	}
}

// TestGlobalEnvPropagation_SpecToManifest verifies the complete env propagation
// chain from spec JSON → parseSpec → StartRunRequest → buildManifestFromRequest.
//
// This end-to-end test ensures that global env vars injected by the server
// arrive intact in the final StepManifest used by the container runtime.
func TestGlobalEnvPropagation_SpecToManifest(t *testing.T) {
	t.Parallel()

	// Simulate a spec with global env vars injected by the server.
	specJSON := json.RawMessage(`{
		"job_id": "` + testKSUID + `",
		"steps": [{"image": "docker.io/test/mig:latest"}],
		"gitlab_pat": "glpat-test-token",
		"envs": {
			"PLOY_CA_CERTS": "-----BEGIN CERTIFICATE-----\ntest-cert\n-----END CERTIFICATE-----",
			"CODEX_AUTH_JSON": "{\"token\":\"test-codex-token\"}",
			"OPENAI_API_KEY": "sk-test-openai-key",
			"CUSTOM_GLOBAL_VAR": "custom_value"
		}
	}`)

	// Step 1: Parse the spec (simulates claimer_spec.go).
	env, typedOpts, _ := parseSpec(specJSON)

	// Step 2: Build StartRunRequest (simulates claimer.go/execution*.go).
	req := StartRunRequest{
		RunID:        types.RunID("run-e2e-env-test"),
		JobID:        types.JobID(testKSUID),
		RepoURL:      types.RepoURL("https://gitlab.com/test/repo.git"),
		BaseRef:      types.GitRef("main"),
		TargetRef:    types.GitRef("feature/global-env"),
		TypedOptions: typedOpts,
		Env:          env, // Global env vars flow here.
	}

	// Step 3: Build manifest (simulates manifest.go).
	// Pass MigStackUnknown explicitly to indicate tests operate without stack detection.
	manifest, err := buildManifestFromRequest(req, typedOpts, 0, contracts.MigStackUnknown)
	if err != nil {
		t.Fatalf("buildManifestFromRequest() error: %v", err)
	}

	// Verify all global env vars are present in the manifest.
	expectedEnv := map[string]string{
		"PLOY_CA_CERTS":     "-----BEGIN CERTIFICATE-----\ntest-cert\n-----END CERTIFICATE-----",
		"CODEX_AUTH_JSON":   `{"token":"test-codex-token"}`,
		"OPENAI_API_KEY":    "sk-test-openai-key",
		"CUSTOM_GLOBAL_VAR": "custom_value",
	}

	for key, wantVal := range expectedEnv {
		gotVal, ok := manifest.Envs[key]
		if !ok {
			t.Errorf("manifest.Env missing key %q", key)
			continue
		}
		if gotVal != wantVal {
			t.Errorf("manifest.Envs[%q] = %q, want %q", key, gotVal, wantVal)
		}
	}
}

// TestGlobalEnvPropagation_GateManifest verifies that global env vars are
// preserved in gate manifests built via buildGateManifestFromRequest.
//
// Gate jobs (pre_gate, post_gate, re_gate) use a separate manifest builder
// that sanitizes stack-aware image configuration but should preserve env vars.
func TestGlobalEnvPropagation_GateManifest(t *testing.T) {
	t.Parallel()

	// Spec with global env vars and stack-aware image map.
	// Note: build_gate is specified as a nested object and is consumed via typed options.
	specJSON := json.RawMessage(`{
		"steps": [
			{
				"image": {
					"java-maven": "docker.io/test/maven-mig:latest",
					"java-gradle": "docker.io/test/gradle-mig:latest"
				}
			}
		],
		"build_gate": {
			"enabled": true
		},
		"envs": {
			"PLOY_CA_CERTS": "gate-test-cert-bundle",
			"CODEX_AUTH_JSON": "gate-codex-auth",
			"GATE_SPECIFIC_VAR": "gate_value"
		}
	}`)

	env, typedOpts, _ := parseSpec(specJSON)

	req := StartRunRequest{
		RunID:        types.RunID("run-gate-env-test"),
		JobID:        types.JobID("job-gate-env-test"),
		RepoURL:      types.RepoURL("https://gitlab.com/test/repo.git"),
		BaseRef:      types.GitRef("main"),
		TypedOptions: typedOpts,
		Env:          env,
	}

	// Build gate manifest (should not fail on stack-aware image map).
	gateManifest, err := buildGateManifestFromRequest(req, typedOpts)
	if err != nil {
		t.Fatalf("buildGateManifestFromRequest() error: %v", err)
	}

	// Verify global env vars are preserved in gate manifest.
	expectedEnv := map[string]string{
		"PLOY_CA_CERTS":     "gate-test-cert-bundle",
		"CODEX_AUTH_JSON":   "gate-codex-auth",
		"GATE_SPECIFIC_VAR": "gate_value",
	}

	for key, wantVal := range expectedEnv {
		gotVal, ok := gateManifest.Envs[key]
		if !ok {
			t.Errorf("gateManifest.Env missing key %q", key)
			continue
		}
		if gotVal != wantVal {
			t.Errorf("gateManifest.Envs[%q] = %q, want %q", key, gotVal, wantVal)
		}
	}

	// Verify gate settings are correct.
	if gateManifest.Gate == nil {
		t.Fatal("expected Gate spec to be set")
	}
	if !gateManifest.Gate.Enabled {
		t.Error("expected Gate.Enabled=true")
	}

	// The Docker-based gate executor reads env vars from Gate.Env (not StepManifest.Env).
	// Gate manifests must mirror job env vars into Gate.Env so build images (e.g. Gradle)
	// can consume injected settings like PLOY_GRADLE_BUILD_CACHE_*.
	for key, wantVal := range expectedEnv {
		gotVal, ok := gateManifest.Gate.Env[key]
		if !ok {
			t.Errorf("gateManifest.Gate.Env missing key %q", key)
			continue
		}
		if gotVal != wantVal {
			t.Errorf("gateManifest.Gate.Env[%q] = %q, want %q", key, gotVal, wantVal)
		}
	}
}

// TestGlobalEnvPropagation_MultiStepRun verifies that global env vars are
// correctly merged with step-specific env in multi-step runs (steps[] array).
//
// The merge semantics are:
//  1. Base env (req.Env) provides global defaults.
//  2. Step-specific env (steps[i].env) overrides on conflict.
func TestGlobalEnvPropagation_MultiStepRun(t *testing.T) {
	t.Parallel()

	specJSON := json.RawMessage(`{
		"envs": {
			"GLOBAL_VAR": "global_value",
			"SHARED_VAR": "global_default",
			"PLOY_CA_CERTS": "global-cert-bundle"
		},
		"steps": [
			{
				"image": "step0-mig:latest",
				"envs": {
					"STEP_VAR": "step0_value",
					"SHARED_VAR": "step0_override"
				}
			},
			{
				"image": "step1-mig:latest",
				"envs": {
					"STEP_VAR": "step1_value"
				}
			}
		]
	}`)

	env, typedOpts, _ := parseSpec(specJSON)

	req := StartRunRequest{
		RunID:        types.RunID("run-multi-step-env"),
		JobID:        types.JobID("job-multi-step-env"),
		RepoURL:      types.RepoURL("https://gitlab.com/test/repo.git"),
		TypedOptions: typedOpts,
		Env:          env, // Global env from spec.
	}

	// Build manifest for step 0 (should have step override).
	// Pass MigStackUnknown explicitly to indicate tests operate without stack detection.
	manifest0, err := buildManifestFromRequest(req, typedOpts, 0, contracts.MigStackUnknown)
	if err != nil {
		t.Fatalf("buildManifestFromRequest(step=0) error: %v", err)
	}

	// Verify step 0 env merge.
	if manifest0.Envs["GLOBAL_VAR"] != "global_value" {
		t.Errorf("step0: GLOBAL_VAR=%q, want global_value", manifest0.Envs["GLOBAL_VAR"])
	}
	if manifest0.Envs["STEP_VAR"] != "step0_value" {
		t.Errorf("step0: STEP_VAR=%q, want step0_value", manifest0.Envs["STEP_VAR"])
	}
	if manifest0.Envs["SHARED_VAR"] != "step0_override" {
		t.Errorf("step0: SHARED_VAR=%q, want step0_override (step env wins)", manifest0.Envs["SHARED_VAR"])
	}
	if manifest0.Envs["PLOY_CA_CERTS"] != "global-cert-bundle" {
		t.Errorf("step0: PLOY_CA_CERTS=%q, want global-cert-bundle", manifest0.Envs["PLOY_CA_CERTS"])
	}

	// Build manifest for step 1 (should not have step0 override).
	// Pass MigStackUnknown explicitly to indicate tests operate without stack detection.
	manifest1, err := buildManifestFromRequest(req, typedOpts, 1, contracts.MigStackUnknown)
	if err != nil {
		t.Fatalf("buildManifestFromRequest(step=1) error: %v", err)
	}

	// Verify step 1 env merge (SHARED_VAR should be global default).
	if manifest1.Envs["GLOBAL_VAR"] != "global_value" {
		t.Errorf("step1: GLOBAL_VAR=%q, want global_value", manifest1.Envs["GLOBAL_VAR"])
	}
	if manifest1.Envs["STEP_VAR"] != "step1_value" {
		t.Errorf("step1: STEP_VAR=%q, want step1_value", manifest1.Envs["STEP_VAR"])
	}
	if manifest1.Envs["SHARED_VAR"] != "global_default" {
		t.Errorf("step1: SHARED_VAR=%q, want global_default (no step override)", manifest1.Envs["SHARED_VAR"])
	}
}

// TestGlobalEnvPropagation_HealingManifest verifies that global env vars from
// req.Env are merged into healing manifests as a base layer, with mig.Env
// overriding on collision.
//
// The merge order is: req.Env (base) → mig.Env (override) → repo metadata.
// This ensures target-based global env propagated via the claim spec reaches
// heal job containers.
func TestGlobalEnvPropagation_HealingManifest(t *testing.T) {
	t.Parallel()

	req := StartRunRequest{
		RunID:     types.RunID("run-healing-env-test"),
		JobID:     types.JobID("job-healing-env-test"),
		RepoURL:   types.RepoURL("https://gitlab.com/test/repo.git"),
		BaseRef:   types.GitRef("main"),
		TargetRef: types.GitRef("feature/healing"),
		Env: map[string]string{
			"GLOBAL_VAR":    "global_value",
			"PLOY_CA_CERTS": "global-cert-bundle",
			"SHARED_VAR":    "from_req",
		},
	}

	// Healing mig with its own env that overrides req.Env on collision.
	healingMig := MigContainerSpec{
		Image: testJobImage("codex:latest"),
		Env: map[string]string{
			"PLOY_CA_CERTS":    "healing-cert-bundle",
			"CODEX_AUTH_JSON":  `{"healing":"auth"}`,
			"HEALING_SPECIFIC": "healing_value",
		},
	}

	// Pass MigStackUnknown explicitly to indicate tests operate without stack detection.
	manifest, err := buildHealingManifest(req, healingMig, 0, "", contracts.MigStackUnknown)
	if err != nil {
		t.Fatalf("buildHealingManifest() error: %v", err)
	}

	// Verify env vars are preserved with correct precedence.
	expectedEnv := map[string]string{
		"GLOBAL_VAR":       "global_value",        // from req.Env (no override)
		"PLOY_CA_CERTS":    "healing-cert-bundle", // mig.Env overrides req.Env
		"CODEX_AUTH_JSON":  `{"healing":"auth"}`,  // from mig.Env only
		"HEALING_SPECIFIC": "healing_value",       // from mig.Env only
		"SHARED_VAR":       "from_req",            // from req.Env (no mig override)
		"PLOY_REPO_URL":    "https://gitlab.com/test/repo.git",
		"PLOY_BASE_REF":    "main",
		"PLOY_TARGET_REF":  "feature/healing",
	}

	for key, wantVal := range expectedEnv {
		gotVal, ok := manifest.Envs[key]
		if !ok {
			t.Errorf("healingManifest.Env missing key %q", key)
			continue
		}
		if gotVal != wantVal {
			t.Errorf("healingManifest.Env[%q] = %q, want %q", key, gotVal, wantVal)
		}
	}
}

// TestGlobalEnvPropagation_NoFiltering verifies that no env key filtering
// occurs in the propagation chain. Any valid string key should pass through.
func TestGlobalEnvPropagation_NoFiltering(t *testing.T) {
	t.Parallel()

	// Test various env key patterns that might be incorrectly filtered.
	specJSON := json.RawMessage(`{
		"steps": [{"image": "docker.io/test/mig:latest"}],
		"envs": {
			"NORMAL_KEY": "value1",
			"lowercase_key": "value2",
			"MixedCase_Key": "value3",
			"KEY_WITH_123_NUMBERS": "value4",
			"_UNDERSCORE_PREFIX": "value5",
			"DOUBLE__UNDERSCORE": "value6",
			"PLOY_INTERNAL_VAR": "value7",
			"DOCKER_HOST": "value8",
			"PATH": "value9"
		}
	}`)

	env, typedOpts, _ := parseSpec(specJSON)

	req := StartRunRequest{
		RunID:        types.RunID("run-no-filter-test"),
		JobID:        types.JobID("job-no-filter-test"),
		RepoURL:      types.RepoURL("https://gitlab.com/test/repo.git"),
		TypedOptions: typedOpts,
		Env:          env,
	}

	// Pass MigStackUnknown explicitly to indicate tests operate without stack detection.
	manifest, err := buildManifestFromRequest(req, typedOpts, 0, contracts.MigStackUnknown)
	if err != nil {
		t.Fatalf("buildManifestFromRequest() error: %v", err)
	}

	// Verify all keys are present without filtering.
	expectedKeys := []string{
		"NORMAL_KEY",
		"lowercase_key",
		"MixedCase_Key",
		"KEY_WITH_123_NUMBERS",
		"_UNDERSCORE_PREFIX",
		"DOUBLE__UNDERSCORE",
		"PLOY_INTERNAL_VAR",
		"DOCKER_HOST",
		"PATH",
	}

	for _, key := range expectedKeys {
		if _, ok := manifest.Envs[key]; !ok {
			t.Errorf("manifest.Env missing key %q (filtered incorrectly)", key)
		}
	}

	// Verify total count matches.
	if len(manifest.Envs) != len(expectedKeys) {
		t.Errorf("manifest.Envs has %d keys, want %d", len(manifest.Envs), len(expectedKeys))
	}
}
