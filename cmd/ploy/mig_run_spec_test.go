package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBuildSpecPayloadCLIOverrides verifies that CLI flags take precedence over
// spec file values when both are provided (envs, image, gitlab_domain, etc.).
func TestBuildSpecPayloadCLIOverrides(t *testing.T) {
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "test.yaml")
	specContent := `
steps:
  - image: docker.io/test/mig:v1
envs:
  KEY1: from_spec
  KEY2: value2
gitlab_domain: gitlab.com
`
	if err := os.WriteFile(specPath, []byte(specContent), 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}

	migEnvs := []string{"KEY1=from_cli", "KEY3=new_value"}
	payload, err := buildSpecPayload(
		context.Background(),
		nil,
		nil,
		specPath,
		migEnvs,
		"docker.io/test/mig:v2",
		true,
		"",
		"glpat-test",
		"gitlab.example.com",
		true,
		false,
	)
	if err != nil {
		t.Fatalf("buildSpecPayload error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	steps, ok := result["steps"].([]any)
	if !ok || len(steps) != 1 {
		t.Fatalf("expected steps[0] in payload, got %v", result["steps"])
	}
	step0, ok := steps[0].(map[string]any)
	if !ok {
		t.Fatalf("expected steps[0] to be map, got %T", steps[0])
	}
	if img, ok := step0["image"].(string); !ok || img != "docker.io/test/mig:v2" {
		t.Errorf("expected steps[0].image=docker.io/test/mig:v2 (CLI override), got %v", step0["image"])
	}

	if envs, ok := result["envs"].(map[string]any); ok {
		if v, ok := envs["KEY1"].(string); !ok || v != "from_cli" {
			t.Errorf("expected envs.KEY1=from_cli (CLI override), got %v", envs["KEY1"])
		}
		if v, ok := envs["KEY2"].(string); !ok || v != "value2" {
			t.Errorf("expected envs.KEY2=value2 (from spec), got %v", envs["KEY2"])
		}
		if v, ok := envs["KEY3"].(string); !ok || v != "new_value" {
			t.Errorf("expected envs.KEY3=new_value (CLI override), got %v", envs["KEY3"])
		}
	} else {
		t.Errorf("expected envs in payload")
	}

	if _, exists := step0["retain_container"]; exists {
		t.Errorf("expected steps[0].retain_container to be absent")
	}

	if domain, ok := result["gitlab_domain"].(string); !ok || domain != "gitlab.example.com" {
		t.Errorf("expected gitlab_domain=gitlab.example.com (CLI override), got %v", result["gitlab_domain"])
	}

	if pat, ok := result["gitlab_pat"].(string); !ok || pat != "glpat-test" {
		t.Errorf("expected gitlab_pat=glpat-test (CLI), got %v", result["gitlab_pat"])
	}

	if success, ok := result["mr_on_success"].(bool); !ok || !success {
		t.Errorf("expected mr_on_success=true (CLI), got %v", result["mr_on_success"])
	}
}

// TestBuildSpecPayloadNoSpec verifies that buildSpecPayload works correctly
// when no spec file is provided, constructing the payload solely from CLI flags.
func TestBuildSpecPayloadNoSpec(t *testing.T) {
	payload, err := buildSpecPayload(
		context.Background(),
		nil,
		nil,
		"",
		[]string{"KEY1=value1"},
		"docker.io/test/mig:latest",
		false,
		"",
		"",
		"",
		false,
		false,
	)
	if err != nil {
		t.Fatalf("buildSpecPayload error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	steps, ok := result["steps"].([]any)
	if !ok || len(steps) != 1 {
		t.Fatalf("expected steps[0] in payload, got %v", result["steps"])
	}
	step0, ok := steps[0].(map[string]any)
	if !ok {
		t.Fatalf("expected steps[0] to be map, got %T", steps[0])
	}
	if img, ok := step0["image"].(string); !ok || img != "docker.io/test/mig:latest" {
		t.Errorf("expected steps[0].image=docker.io/test/mig:latest, got %v", step0["image"])
	}

	if envs, ok := result["envs"].(map[string]any); ok {
		if v, ok := envs["KEY1"].(string); !ok || v != "value1" {
			t.Errorf("expected envs.KEY1=value1, got %v", envs["KEY1"])
		}
	} else {
		t.Errorf("expected envs in payload")
	}
}

// TestBuildSpecPayloadEmpty verifies that buildSpecPayload returns nil when
// no spec file or CLI overrides are provided (empty payload case).
func TestBuildSpecPayloadEmpty(t *testing.T) {
	payload, err := buildSpecPayload(context.Background(), nil, nil, "", nil, "", false, "", "", "", false, false)
	if err != nil {
		t.Fatalf("buildSpecPayload error: %v", err)
	}

	if payload != nil {
		t.Errorf("expected nil payload when no spec or overrides, got %v", payload)
	}
}

// TestBuildSpecPayloadGitLabDomainDefaulting verifies the precedence and defaulting
// logic for gitlab_domain: CLI overrides spec; PAT presence triggers gitlab.com default.
func TestBuildSpecPayloadGitLabDomainDefaulting(t *testing.T) {
	tests := []struct {
		name          string
		specContent   string
		gitlabPAT     string
		gitlabDomain  string
		wantDomain    string
		wantDomainSet bool
	}{
		{
			name:          "PAT provided, no domain in CLI or spec - defaults to gitlab.com",
			specContent:   "",
			gitlabPAT:     "glpat-test",
			gitlabDomain:  "",
			wantDomain:    "gitlab.com",
			wantDomainSet: true,
		},
		{
			name:          "PAT and domain both provided in CLI - uses CLI domain",
			specContent:   "",
			gitlabPAT:     "glpat-test",
			gitlabDomain:  "gitlab.example.com",
			wantDomain:    "gitlab.example.com",
			wantDomainSet: true,
		},
		{
			name:          "PAT in CLI, domain in spec - CLI domain empty, spec preserved",
			specContent:   "gitlab_domain: gitlab.spec.com\n",
			gitlabPAT:     "glpat-test",
			gitlabDomain:  "",
			wantDomain:    "gitlab.spec.com",
			wantDomainSet: true,
		},
		{
			name:          "PAT in CLI, domain in spec - CLI domain overrides spec",
			specContent:   "gitlab_domain: gitlab.spec.com\n",
			gitlabPAT:     "glpat-test",
			gitlabDomain:  "gitlab.cli.com",
			wantDomain:    "gitlab.cli.com",
			wantDomainSet: true,
		},
		{
			name:          "No PAT provided - domain not set even if empty",
			specContent:   "",
			gitlabPAT:     "",
			gitlabDomain:  "",
			wantDomain:    "",
			wantDomainSet: false,
		},
		{
			name: "PAT in spec, no CLI override - defaults to gitlab.com",
			specContent: `gitlab_pat: glpat-from-spec
`,
			gitlabPAT:     "",
			gitlabDomain:  "",
			wantDomain:    "gitlab.com",
			wantDomainSet: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var specFile string
			if tt.specContent != "" {
				tmpDir := t.TempDir()
				specFile = filepath.Join(tmpDir, "test.yaml")
				if err := os.WriteFile(specFile, []byte(tt.specContent), 0o644); err != nil {
					t.Fatalf("write spec file: %v", err)
				}
			}

			migImage := ""
			if tt.gitlabPAT != "" || strings.TrimSpace(tt.specContent) != "" || tt.gitlabDomain != "" {
				migImage = "docker.io/test/mig:latest"
			}
			payload, err := buildSpecPayload(
				context.Background(),
				nil,
				nil,
				specFile,
				nil,
				migImage,
				false,
				"",
				tt.gitlabPAT,
				tt.gitlabDomain,
				false,
				false,
			)
			if err != nil {
				t.Fatalf("buildSpecPayload error: %v", err)
			}

			if payload == nil && !tt.wantDomainSet {
				return
			}

			var result map[string]any
			if err := json.Unmarshal(payload, &result); err != nil {
				t.Fatalf("unmarshal payload: %v", err)
			}

			domain, exists := result["gitlab_domain"].(string)
			if tt.wantDomainSet {
				if !exists {
					t.Errorf("expected gitlab_domain to be set, but it was not present")
				} else if domain != tt.wantDomain {
					t.Errorf("expected gitlab_domain=%s, got %s", tt.wantDomain, domain)
				}
			} else {
				if exists {
					t.Errorf("expected gitlab_domain not to be set, but got %s", domain)
				}
			}
		})
	}
}

// TestBuildSpecPayloadGitLabDomainDefaultingWithMRFlags verifies MR creation flags
// work correctly with gitlab_domain defaulting.
func TestBuildSpecPayloadGitLabDomainDefaultingWithMRFlags(t *testing.T) {
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "test.yaml")
	specContent := `
steps:
  - image: docker.io/test/mig:latest
envs:
  KEY1: value1
`
	if err := os.WriteFile(specPath, []byte(specContent), 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}

	payload, err := buildSpecPayload(
		context.Background(),
		nil,
		nil,
		specPath,
		nil,
		"",
		false,
		"",
		"glpat-test-123",
		"",
		false,
		true,
	)
	if err != nil {
		t.Fatalf("buildSpecPayload error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	if pat, ok := result["gitlab_pat"].(string); !ok || pat != "glpat-test-123" {
		t.Errorf("expected gitlab_pat=glpat-test-123, got %v", result["gitlab_pat"])
	}
	if domain, ok := result["gitlab_domain"].(string); !ok || domain != "gitlab.com" {
		t.Errorf("expected gitlab_domain=gitlab.com (defaulted), got %v", result["gitlab_domain"])
	}
	if mrFail, ok := result["mr_on_fail"].(bool); !ok || !mrFail {
		t.Errorf("expected mr_on_fail=true, got %v", result["mr_on_fail"])
	}
	if mrSuccess, ok := result["mr_on_success"].(bool); ok && mrSuccess {
		t.Errorf("expected mr_on_success=false or not present, got true")
	}

	steps, ok := result["steps"].([]any)
	if !ok || len(steps) != 1 {
		t.Fatalf("expected steps[0] in payload, got %v", result["steps"])
	}
	step0, ok := steps[0].(map[string]any)
	if !ok {
		t.Fatalf("expected steps[0] to be map, got %T", steps[0])
	}
	if img, ok := step0["image"].(string); !ok || img != "docker.io/test/mig:latest" {
		t.Errorf("expected steps[0].image from spec to be preserved, got %v", step0["image"])
	}
}

// TestBuildSpecPayloadConfigOverlayMergePrecedence verifies that local config.yaml
// overlay is applied with correct precedence: overlay < spec < CLI.
func TestBuildSpecPayloadConfigOverlayMergePrecedence(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("PLOY_CONFIG_HOME", configHome)

	cfgContent := `
defaults:
  job:
    mig:
      envs:
        FROM_OVERLAY: overlay_val
        SHARED: from_overlay
`
	if err := os.WriteFile(filepath.Join(configHome, "config.yaml"), []byte(cfgContent), 0o644); err != nil {
		t.Fatal(err)
	}

	specDir := t.TempDir()
	specPath := filepath.Join(specDir, "test.yaml")
	specContent := `
steps:
  - image: docker.io/test/mig:latest
envs:
  FROM_SPEC: spec_val
  SHARED: from_spec
`
	if err := os.WriteFile(specPath, []byte(specContent), 0o644); err != nil {
		t.Fatal(err)
	}

	payload, err := buildSpecPayload(
		context.Background(), nil, nil,
		specPath, nil, "", false, "", "", "", false, false,
	)
	if err != nil {
		t.Fatalf("buildSpecPayload error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	envs, ok := result["envs"].(map[string]any)
	if !ok {
		t.Fatal("expected envs in payload")
	}
	// Spec wins over overlay for same key.
	if envs["SHARED"] != "from_spec" {
		t.Errorf("SHARED = %v, want from_spec (spec > overlay)", envs["SHARED"])
	}
	// Overlay key appears when not in spec.
	if envs["FROM_OVERLAY"] != "overlay_val" {
		t.Errorf("FROM_OVERLAY = %v, want overlay_val", envs["FROM_OVERLAY"])
	}
	// Spec key preserved.
	if envs["FROM_SPEC"] != "spec_val" {
		t.Errorf("FROM_SPEC = %v, want spec_val", envs["FROM_SPEC"])
	}
}

// TestBuildSpecPayloadConfigOverlayNoFile verifies that missing config.yaml
// does not cause errors (returns zero overlay).
func TestBuildSpecPayloadConfigOverlayNoFile(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("PLOY_CONFIG_HOME", configHome)

	specDir := t.TempDir()
	specPath := filepath.Join(specDir, "test.yaml")
	specContent := `
steps:
  - image: docker.io/test/mig:latest
envs:
  KEY: value
`
	if err := os.WriteFile(specPath, []byte(specContent), 0o644); err != nil {
		t.Fatal(err)
	}

	payload, err := buildSpecPayload(
		context.Background(), nil, nil,
		specPath, nil, "", false, "", "", "", false, false,
	)
	if err != nil {
		t.Fatalf("buildSpecPayload error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	envs := result["envs"].(map[string]any)
	if envs["KEY"] != "value" {
		t.Errorf("KEY = %v, want value", envs["KEY"])
	}
}

// TestApplyConfigOverlayInPlace_RouterInheritsGatePhase verifies that router
// containers receive the pre_gate config overlay section.
func TestApplyConfigOverlayInPlace_RouterInheritsGatePhase(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("PLOY_CONFIG_HOME", configHome)

	cfgContent := `
defaults:
  job:
    pre_gate:
      envs:
        GATE_KEY: gate_val
`
	if err := os.WriteFile(filepath.Join(configHome, "config.yaml"), []byte(cfgContent), 0o644); err != nil {
		t.Fatal(err)
	}

	spec := map[string]any{
		"steps": []any{
			map[string]any{"image": "docker.io/test/mig:latest"},
		},
		"build_gate": map[string]any{
			"router": map[string]any{
				"image": "docker.io/test/router:latest",
			},
		},
	}

	if err := applyConfigOverlayInPlace(spec); err != nil {
		t.Fatalf("applyConfigOverlayInPlace: %v", err)
	}

	router := spec["build_gate"].(map[string]any)["router"].(map[string]any)
	envs, ok := router["envs"].(map[string]any)
	if !ok || envs["GATE_KEY"] != "gate_val" {
		t.Errorf("router envs = %v, expected GATE_KEY=gate_val from pre_gate overlay", envs)
	}
}

// TestApplyConfigOverlayInPlace_HealingGetsHealSection verifies that healing
// containers receive the heal config overlay section.
func TestApplyConfigOverlayInPlace_HealingGetsHealSection(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("PLOY_CONFIG_HOME", configHome)

	cfgContent := `
defaults:
  job:
    heal:
      envs:
        HEAL_KEY: heal_val
`
	if err := os.WriteFile(filepath.Join(configHome, "config.yaml"), []byte(cfgContent), 0o644); err != nil {
		t.Fatal(err)
	}

	spec := map[string]any{
		"steps": []any{
			map[string]any{"image": "docker.io/test/mig:latest"},
		},
		"build_gate": map[string]any{
			"healing": map[string]any{
				"by_error_kind": map[string]any{
					"infra": map[string]any{
						"image": "docker.io/test/healer:latest",
					},
				},
			},
		},
	}

	if err := applyConfigOverlayInPlace(spec); err != nil {
		t.Fatalf("applyConfigOverlayInPlace: %v", err)
	}

	infra := spec["build_gate"].(map[string]any)["healing"].(map[string]any)["by_error_kind"].(map[string]any)["infra"].(map[string]any)
	envs, ok := infra["envs"].(map[string]any)
	if !ok || envs["HEAL_KEY"] != "heal_val" {
		t.Errorf("healing envs = %v, expected HEAL_KEY=heal_val from heal overlay", envs)
	}
}

// TestDeriveActiveGatePhase verifies that deriveActiveGatePhase returns the
// correct gate phase based on the spec's build_gate.pre/post configuration.
func TestDeriveActiveGatePhase(t *testing.T) {
	tests := []struct {
		name string
		spec map[string]any
		want string
	}{
		{
			name: "no build_gate defaults to pre_gate",
			spec: map[string]any{},
			want: "pre_gate",
		},
		{
			name: "build_gate with pre configured returns pre_gate",
			spec: map[string]any{
				"build_gate": map[string]any{
					"pre": map[string]any{"target": "build"},
				},
			},
			want: "pre_gate",
		},
		{
			name: "build_gate with only post configured returns post_gate",
			spec: map[string]any{
				"build_gate": map[string]any{
					"post": map[string]any{"target": "unit"},
				},
			},
			want: "post_gate",
		},
		{
			name: "build_gate with both pre and post returns pre_gate",
			spec: map[string]any{
				"build_gate": map[string]any{
					"pre":  map[string]any{"target": "build"},
					"post": map[string]any{"target": "unit"},
				},
			},
			want: "pre_gate",
		},
		{
			name: "build_gate with no pre or post defaults to pre_gate",
			spec: map[string]any{
				"build_gate": map[string]any{
					"enabled": true,
				},
			},
			want: "pre_gate",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deriveActiveGatePhase(tt.spec)
			if got != tt.want {
				t.Errorf("deriveActiveGatePhase() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestApplyConfigOverlayInPlace_RouterDerivesActiveGatePhase verifies that
// router overlay selection uses the active gate phase derived from the spec
// build_gate config, not a hardcoded value.
func TestApplyConfigOverlayInPlace_RouterDerivesActiveGatePhase(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("PLOY_CONFIG_HOME", configHome)

	cfgContent := `
defaults:
  job:
    pre_gate:
      envs:
        PHASE_KEY: pre_gate_val
    post_gate:
      envs:
        PHASE_KEY: post_gate_val
`
	if err := os.WriteFile(filepath.Join(configHome, "config.yaml"), []byte(cfgContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Spec with only post gate configured → router should get post_gate overlay.
	spec := map[string]any{
		"steps": []any{
			map[string]any{"image": "docker.io/test/mig:latest"},
		},
		"build_gate": map[string]any{
			"post": map[string]any{"target": "unit"},
			"router": map[string]any{
				"image": "docker.io/test/router:latest",
			},
		},
	}

	if err := applyConfigOverlayInPlace(spec); err != nil {
		t.Fatalf("applyConfigOverlayInPlace: %v", err)
	}

	router := spec["build_gate"].(map[string]any)["router"].(map[string]any)
	envs, ok := router["envs"].(map[string]any)
	if !ok || envs["PHASE_KEY"] != "post_gate_val" {
		t.Errorf("router envs = %v, expected PHASE_KEY=post_gate_val from post_gate overlay (active gate phase)", envs)
	}
}

// TestBuildSpecPayloadDeterministicCanonicalSnapshot verifies that repeated
// builds of the same spec with the same overlay produce identical JSON output,
// ensuring deterministic canonical output.
func TestBuildSpecPayloadDeterministicCanonicalSnapshot(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("PLOY_CONFIG_HOME", configHome)

	cfgContent := `
defaults:
  job:
    mig:
      envs:
        FROM_OVERLAY: overlay_val
        Z_KEY: z_val
        A_KEY: a_val
`
	if err := os.WriteFile(filepath.Join(configHome, "config.yaml"), []byte(cfgContent), 0o644); err != nil {
		t.Fatal(err)
	}

	specDir := t.TempDir()
	specPath := filepath.Join(specDir, "test.yaml")
	specContent := `
steps:
  - image: docker.io/test/mig:latest
envs:
  FROM_SPEC: spec_val
  M_KEY: m_val
`
	if err := os.WriteFile(specPath, []byte(specContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Build the spec payload twice and verify identical output.
	var payloads [2][]byte
	for i := range payloads {
		payload, err := buildSpecPayload(
			context.Background(), nil, nil,
			specPath, nil, "", false, "", "", "", false, false,
		)
		if err != nil {
			t.Fatalf("run %d: buildSpecPayload error: %v", i, err)
		}
		payloads[i] = payload
	}

	if string(payloads[0]) != string(payloads[1]) {
		t.Errorf("non-deterministic output:\nrun 0: %s\nrun 1: %s", payloads[0], payloads[1])
	}

	// Verify the canonical shape includes all expected keys.
	var result map[string]any
	if err := json.Unmarshal(payloads[0], &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	envs, ok := result["envs"].(map[string]any)
	if !ok {
		t.Fatal("expected envs in payload")
	}
	wantKeys := map[string]string{
		"FROM_SPEC":    "spec_val",
		"FROM_OVERLAY": "overlay_val",
		"Z_KEY":        "z_val",
		"A_KEY":        "a_val",
		"M_KEY":        "m_val",
	}
	for k, v := range wantKeys {
		if envs[k] != v {
			t.Errorf("envs[%s] = %v, want %v", k, envs[k], v)
		}
	}

	steps, ok := result["steps"].([]any)
	if !ok || len(steps) != 1 {
		t.Fatal("expected exactly one step in canonical output")
	}
	step0 := steps[0].(map[string]any)
	if step0["image"] != "docker.io/test/mig:latest" {
		t.Errorf("step image = %v, want docker.io/test/mig:latest", step0["image"])
	}
}

// TestBuildSpecPayloadServerLocalSpecMergePrecedence verifies the three-layer
// merge order: server defaults < local config.yaml < spec values. At CLI compile
// time, server defaults are not applied (that happens at claim time), but the
// local < spec precedence must be correct so server-side merge produces the
// expected final result.
func TestBuildSpecPayloadServerLocalSpecMergePrecedence(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("PLOY_CONFIG_HOME", configHome)

	// Local overlay: provides base env values that spec should override.
	cfgContent := `
defaults:
  job:
    mig:
      envs:
        LOCAL_KEY: local_val
        SHARED: from_local
    heal:
      envs:
        HEAL_LOCAL: heal_local_val
    pre_gate:
      envs:
        GATE_LOCAL: gate_local_val
`
	if err := os.WriteFile(filepath.Join(configHome, "config.yaml"), []byte(cfgContent), 0o644); err != nil {
		t.Fatal(err)
	}

	specDir := t.TempDir()
	specPath := filepath.Join(specDir, "test.yaml")
	specContent := `
steps:
  - image: docker.io/test/mig:latest
    envs:
      STEP_SHARED: from_spec
envs:
  SPEC_KEY: spec_val
  SHARED: from_spec
build_gate:
  pre:
    target: build
  router:
    image: docker.io/test/router:latest
  healing:
    by_error_kind:
      infra:
        image: docker.io/test/healer:latest
`
	if err := os.WriteFile(specPath, []byte(specContent), 0o644); err != nil {
		t.Fatal(err)
	}

	payload, err := buildSpecPayload(
		context.Background(), nil, nil,
		specPath, nil, "", false, "", "", "", false, false,
	)
	if err != nil {
		t.Fatalf("buildSpecPayload error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Top-level envs: spec wins for SHARED, local key preserved.
	envs := result["envs"].(map[string]any)
	if envs["SHARED"] != "from_spec" {
		t.Errorf("top-level SHARED = %v, want from_spec (spec > local)", envs["SHARED"])
	}
	if envs["LOCAL_KEY"] != "local_val" {
		t.Errorf("top-level LOCAL_KEY = %v, want local_val", envs["LOCAL_KEY"])
	}
	if envs["SPEC_KEY"] != "spec_val" {
		t.Errorf("top-level SPEC_KEY = %v, want spec_val", envs["SPEC_KEY"])
	}

	// Step-level: mig overlay applied, spec step envs preserved.
	steps := result["steps"].([]any)
	step0 := steps[0].(map[string]any)
	stepEnvs := step0["envs"].(map[string]any)
	if stepEnvs["STEP_SHARED"] != "from_spec" {
		t.Errorf("step STEP_SHARED = %v, want from_spec", stepEnvs["STEP_SHARED"])
	}

	// Router: pre_gate overlay applied (since build_gate.pre is configured).
	bg := result["build_gate"].(map[string]any)
	router := bg["router"].(map[string]any)
	routerEnvs, ok := router["envs"].(map[string]any)
	if !ok || routerEnvs["GATE_LOCAL"] != "gate_local_val" {
		t.Errorf("router GATE_LOCAL = %v, want gate_local_val from pre_gate overlay", routerEnvs)
	}

	// Healing: heal overlay applied.
	healing := bg["healing"].(map[string]any)
	infra := healing["by_error_kind"].(map[string]any)["infra"].(map[string]any)
	healEnvs, ok := infra["envs"].(map[string]any)
	if !ok || healEnvs["HEAL_LOCAL"] != "heal_local_val" {
		t.Errorf("healing HEAL_LOCAL = %v, want heal_local_val from heal overlay", healEnvs)
	}
}
