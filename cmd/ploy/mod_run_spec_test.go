package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestBuildSpecPayloadCLIOverrides verifies that CLI flags take precedence over
// spec file values when both are provided (env, image, gitlab_domain, etc.).
func TestBuildSpecPayloadCLIOverrides(t *testing.T) {
	// Create a temporary YAML spec file with some defaults
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "test.yaml")
	specContent := `
image: docker.io/test/mod:v1
env:
  KEY1: from_spec
  KEY2: value2
gitlab_domain: gitlab.com
`
	if err := os.WriteFile(specPath, []byte(specContent), 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}

	// CLI overrides should take precedence
	modEnvs := []string{"KEY1=from_cli", "KEY3=new_value"}
	payload, err := buildSpecPayload(
		specPath,
		modEnvs,
		"docker.io/test/mod:v2", // override image
		true,                    // retain
		"",
		"glpat-test",         // gitlab_pat
		"gitlab.example.com", // override domain
		true,                 // mr_success
		false,                // mr_fail
	)
	if err != nil {
		t.Fatalf("buildSpecPayload error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	// Verify CLI override for image
	if img, ok := result["image"].(string); !ok || img != "docker.io/test/mod:v2" {
		t.Errorf("expected image=docker.io/test/mod:v2 (CLI override), got %v", result["image"])
	}

	// Verify CLI override for env
	if env, ok := result["env"].(map[string]any); ok {
		if v, ok := env["KEY1"].(string); !ok || v != "from_cli" {
			t.Errorf("expected env.KEY1=from_cli (CLI override), got %v", env["KEY1"])
		}
		if v, ok := env["KEY2"].(string); !ok || v != "value2" {
			t.Errorf("expected env.KEY2=value2 (from spec), got %v", env["KEY2"])
		}
		if v, ok := env["KEY3"].(string); !ok || v != "new_value" {
			t.Errorf("expected env.KEY3=new_value (CLI override), got %v", env["KEY3"])
		}
	} else {
		t.Errorf("expected env in payload")
	}

	// Verify CLI override for retain_container
	if retain, ok := result["retain_container"].(bool); !ok || !retain {
		t.Errorf("expected retain_container=true (CLI override), got %v", result["retain_container"])
	}

	// Verify CLI override for gitlab_domain
	if domain, ok := result["gitlab_domain"].(string); !ok || domain != "gitlab.example.com" {
		t.Errorf("expected gitlab_domain=gitlab.example.com (CLI override), got %v", result["gitlab_domain"])
	}

	// Verify gitlab_pat from CLI
	if pat, ok := result["gitlab_pat"].(string); !ok || pat != "glpat-test" {
		t.Errorf("expected gitlab_pat=glpat-test (CLI), got %v", result["gitlab_pat"])
	}

	// Verify mr_on_success from CLI
	if success, ok := result["mr_on_success"].(bool); !ok || !success {
		t.Errorf("expected mr_on_success=true (CLI), got %v", result["mr_on_success"])
	}
}

// TestBuildSpecPayloadNoSpec verifies that buildSpecPayload works correctly
// when no spec file is provided, constructing the payload solely from CLI flags.
func TestBuildSpecPayloadNoSpec(t *testing.T) {
	// No spec file, only CLI flags
	payload, err := buildSpecPayload(
		"",
		[]string{"KEY1=value1"},
		"docker.io/test/mod:latest",
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

	// Verify CLI-only values
	if img, ok := result["image"].(string); !ok || img != "docker.io/test/mod:latest" {
		t.Errorf("expected image=docker.io/test/mod:latest, got %v", result["image"])
	}

	if env, ok := result["env"].(map[string]any); ok {
		if v, ok := env["KEY1"].(string); !ok || v != "value1" {
			t.Errorf("expected env.KEY1=value1, got %v", env["KEY1"])
		}
	} else {
		t.Errorf("expected env in payload")
	}
}

// TestBuildSpecPayloadEmpty verifies that buildSpecPayload returns nil when
// no spec file or CLI overrides are provided (empty payload case).
func TestBuildSpecPayloadEmpty(t *testing.T) {
	// No spec file and no CLI overrides
	payload, err := buildSpecPayload("", nil, "", false, "", "", "", false, false)
	if err != nil {
		t.Fatalf("buildSpecPayload error: %v", err)
	}

	if payload != nil {
		t.Errorf("expected nil payload when no spec or overrides, got %v", payload)
	}
}

// heal-on-build back-compat behavior has been removed; specs must configure
// build_gate_healing explicitly when needed.

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

			payload, err := buildSpecPayload(
				specFile,
				nil,
				"",
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

			// When no PAT and no domain, payload might be nil
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

// TestBuildSpecPayloadGitLabDomainDefaultingWithMRFlags is an integration test
// verifying that MR creation flags work correctly with gitlab_domain defaulting.
// Simulates: ploy mod run --spec test.yaml --gitlab-pat glpat-xxx --mr-fail
func TestBuildSpecPayloadGitLabDomainDefaultingWithMRFlags(t *testing.T) {
	// Integration test: verify MR creation flags work correctly with domain defaulting.
	// This simulates a real-world scenario where a user provides a PAT and MR flags
	// without explicitly specifying the domain, expecting it to default to gitlab.com.
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "test.yaml")
	specContent := `
image: docker.io/test/mod:latest
env:
  KEY1: value1
`
	if err := os.WriteFile(specPath, []byte(specContent), 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}

	// Simulate user running: ploy mod run --spec test.yaml --gitlab-pat glpat-xxx --mr-fail
	payload, err := buildSpecPayload(
		specPath,
		nil,              // no additional env
		"",               // no image override
		false,            // no retain
		"",               // no command
		"glpat-test-123", // PAT provided
		"",               // domain NOT specified - should default to gitlab.com
		false,            // mr_on_success=false
		true,             // mr_on_fail=true
	)
	if err != nil {
		t.Fatalf("buildSpecPayload error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	// Verify gitlab_pat is present
	if pat, ok := result["gitlab_pat"].(string); !ok || pat != "glpat-test-123" {
		t.Errorf("expected gitlab_pat=glpat-test-123, got %v", result["gitlab_pat"])
	}

	// Verify gitlab_domain defaults to gitlab.com
	if domain, ok := result["gitlab_domain"].(string); !ok || domain != "gitlab.com" {
		t.Errorf("expected gitlab_domain=gitlab.com (defaulted), got %v", result["gitlab_domain"])
	}

	// Verify mr_on_fail is set
	if mrFail, ok := result["mr_on_fail"].(bool); !ok || !mrFail {
		t.Errorf("expected mr_on_fail=true, got %v", result["mr_on_fail"])
	}

	// Verify mr_on_success is NOT set (should not be present or false)
	if mrSuccess, ok := result["mr_on_success"].(bool); ok && mrSuccess {
		t.Errorf("expected mr_on_success=false or not present, got true")
	}

	// Verify other spec values are preserved
	if img, ok := result["image"].(string); !ok || img != "docker.io/test/mod:latest" {
		t.Errorf("expected image from spec to be preserved, got %v", result["image"])
	}
}

// TestBuildSpecPayloadCLIOverridesWithModSection verifies that when the spec uses
// the canonical `mod` section, CLI overrides (image, env, command, retain) are
// correctly applied inside the `mod` section without creating duplicate top-level keys.
func TestBuildSpecPayloadCLIOverridesWithModSection(t *testing.T) {
	// Spec uses canonical `mod` section. CLI overrides must apply inside `mod`.
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "mod-overrides.yaml")
	spec := `
mod:
  image: docker.io/test/mod:v1
  env:
    KEY1: from_spec
    KEY2: value2
  retain_container: false
`
	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	payload, err := buildSpecPayload(
		specPath,
		[]string{"KEY1=from_cli", "KEY3=new_value"}, // env overrides
		"docker.io/test/mod:v2",                     // image override
		true,                                        // retain
		`["/bin/sh","-c","echo hi"]`,                // command override (JSON array)
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
		t.Fatalf("unmarshal: %v", err)
	}

	mod, ok := result["mod"].(map[string]any)
	if !ok {
		t.Fatalf("expected mod section in payload")
	}

	// Image override inside mod
	if img, ok := mod["image"].(string); !ok || img != "docker.io/test/mod:v2" {
		t.Errorf("expected mod.image override, got %v", mod["image"])
	}

	// Env merged inside mod
	env, ok := mod["env"].(map[string]any)
	if !ok {
		t.Fatalf("expected mod.env map")
	}
	if v := env["KEY1"]; v != "from_cli" {
		t.Errorf("expected mod.env.KEY1=from_cli, got %v", v)
	}
	if v := env["KEY2"]; v != "value2" {
		t.Errorf("expected mod.env.KEY2=value2, got %v", v)
	}
	if v := env["KEY3"]; v != "new_value" {
		t.Errorf("expected mod.env.KEY3=new_value, got %v", v)
	}

	// Retain inside mod
	if retain, ok := mod["retain_container"].(bool); !ok || !retain {
		t.Errorf("expected mod.retain_container=true, got %v", mod["retain_container"])
	}

	// Command override inside mod as array
	if cmd, ok := mod["command"].([]any); !ok || len(cmd) != 3 {
		t.Errorf("expected mod.command array len 3, got %T (%v)", mod["command"], mod["command"])
	}

	// Ensure no duplicate top-level keys when mod section exists
	if _, ok := result["image"]; ok {
		t.Errorf("did not expect top-level image when mod is present")
	}
	if _, ok := result["env"]; ok {
		t.Errorf("did not expect top-level env when mod is present")
	}
	if _, ok := result["retain_container"]; ok {
		t.Errorf("did not expect top-level retain_container when mod is present")
	}
}
