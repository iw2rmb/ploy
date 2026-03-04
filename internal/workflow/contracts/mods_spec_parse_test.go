package contracts

import (
	"strings"
	"testing"
)

func TestParseModsSpecJSON_SingleStep(t *testing.T) {
	input := `{
		"steps": [{
			"image": "docker.io/user/mig:latest",
			"command": "echo hello",
			"env": {"FOO": "bar", "BAZ": "qux"}
		}],
		"build_gate": {"enabled": true},
		"gitlab_pat": "secret",
		"gitlab_domain": "gitlab.com",
		"mr_on_success": true,
		"mr_on_fail": false
	}`

	spec, err := ParseModsSpecJSON([]byte(input))
	if err != nil {
		t.Fatalf("ParseModsSpecJSON failed: %v", err)
	}

	if len(spec.Steps) != 1 {
		t.Fatalf("len(steps) = %d, want 1", len(spec.Steps))
	}
	step := spec.Steps[0]

	// Verify command (shell form).
	if step.Command.Shell != "echo hello" {
		t.Errorf("command.Shell = %q, want %q", step.Command.Shell, "echo hello")
	}
	expected := []string{"/bin/sh", "-c", "echo hello"}
	got := step.Command.ToSlice()
	if len(got) != len(expected) {
		t.Errorf("Command.ToSlice() = %v, want %v", got, expected)
	}

	// Verify image (universal form).
	if step.Image.Universal != "docker.io/user/mig:latest" {
		t.Errorf("image = %q, want %q", step.Image.Universal, "docker.io/user/mig:latest")
	}

	// Verify env.
	if step.Env["FOO"] != "bar" {
		t.Errorf("env[FOO] = %q, want %q", step.Env["FOO"], "bar")
	}
	if step.Env["BAZ"] != "qux" {
		t.Errorf("env[BAZ] = %q, want %q", step.Env["BAZ"], "qux")
	}

	// Verify build_gate.
	if spec.BuildGate == nil {
		t.Fatal("build_gate is nil")
	}
	if !spec.BuildGate.Enabled {
		t.Errorf("build_gate.enabled = false, want true")
	}

	// Verify GitLab integration.
	if spec.GitLabPAT != "secret" {
		t.Errorf("gitlab_pat = %q, want %q", spec.GitLabPAT, "secret")
	}
	if spec.GitLabDomain != "gitlab.com" {
		t.Errorf("gitlab_domain = %q, want %q", spec.GitLabDomain, "gitlab.com")
	}
	if spec.MROnSuccess == nil || !*spec.MROnSuccess {
		t.Errorf("mr_on_success = %v, want true", spec.MROnSuccess)
	}
	if spec.MROnFail == nil || *spec.MROnFail {
		t.Errorf("mr_on_fail = %v, want false", spec.MROnFail)
	}
}

// TestParseModsSpecJSON_MultiStep tests parsing multi-step spec JSON.
func TestParseModsSpecJSON_MultiStep(t *testing.T) {
	input := `{
		"steps": [
			{"name": "step-1", "image": "docker.io/user/mod1:latest", "command": ["echo", "step1"], "env": {"STEP": "1"}, "always": true},
			{"name": "step-2", "image": "docker.io/user/mod2:latest", "env": {"STEP": "2"}, "always": false}
		],
		"build_gate": {
			"enabled": true,
			"healing": {
				"by_error_kind": {
					"infra": {
						"retries": 3,
						"image": "docker.io/user/codex:latest",
						"command": "fix-it",
						"env": {"PROMPT": "fix the build"}
					}
				}
			},
			"router": {
				"image": "docker.io/user/router:latest"
			}
		}
	}`

	spec, err := ParseModsSpecJSON([]byte(input))
	if err != nil {
		t.Fatalf("ParseModsSpecJSON failed: %v", err)
	}

	// Verify multi-step detection.
	if len(spec.Steps) != 2 {
		t.Fatalf("len(steps) = %d, want 2", len(spec.Steps))
	}

	// Verify first step.
	mod1 := spec.Steps[0]
	if mod1.Name != "step-1" {
		t.Errorf("steps[0].name = %q, want %q", mod1.Name, "step-1")
	}
	if mod1.Image.Universal != "docker.io/user/mod1:latest" {
		t.Errorf("steps[0].image = %q, want %q", mod1.Image.Universal, "docker.io/user/mod1:latest")
	}
	// Command is exec array form.
	if len(mod1.Command.Exec) != 2 || mod1.Command.Exec[0] != "echo" || mod1.Command.Exec[1] != "step1" {
		t.Errorf("steps[0].command.Exec = %v, want [echo, step1]", mod1.Command.Exec)
	}
	if mod1.Env["STEP"] != "1" {
		t.Errorf("steps[0].env[STEP] = %q, want %q", mod1.Env["STEP"], "1")
	}
	if !mod1.Always {
		t.Errorf("steps[0].always = false, want true")
	}

	// Verify second step.
	mod2 := spec.Steps[1]
	if mod2.Name != "step-2" {
		t.Errorf("steps[1].name = %q, want %q", mod2.Name, "step-2")
	}
	if mod2.Always {
		t.Errorf("steps[1].always = true, want false")
	}

	// Verify healing.
	if spec.BuildGate == nil || spec.BuildGate.Healing == nil {
		t.Fatal("build_gate.healing is nil")
	}
	infra, ok := spec.BuildGate.Healing.ByErrorKind["infra"]
	if !ok {
		t.Fatal("build_gate.healing.by_error_kind.infra is missing")
	}
	if infra.Retries != 3 {
		t.Errorf("build_gate.healing.by_error_kind.infra.retries = %d, want 3", infra.Retries)
	}
	if infra.Image.Universal != "docker.io/user/codex:latest" {
		t.Errorf("build_gate.healing.by_error_kind.infra.image = %q, want %q",
			infra.Image.Universal, "docker.io/user/codex:latest")
	}
	if infra.Command.Shell != "fix-it" {
		t.Errorf("build_gate.healing.by_error_kind.infra.command = %q, want %q",
			infra.Command.Shell, "fix-it")
	}
	if spec.BuildGate.Router == nil {
		t.Fatal("build_gate.router is nil")
	}
	if spec.BuildGate.Router.Image.Universal != "docker.io/user/router:latest" {
		t.Errorf("build_gate.router.image = %q, want %q",
			spec.BuildGate.Router.Image.Universal, "docker.io/user/router:latest")
	}
}

func TestParseModsSpecJSON_RetainContainerForbidden(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name: "step retain forbidden",
			input: `{
				"steps": [{"image": "docker.io/user/mig:latest", "retain_container": true}]
			}`,
			wantErr: "steps[0].retain_container: forbidden",
		},
		{
			name: "healing retain forbidden",
			input: `{
				"steps": [{"image": "docker.io/user/mig:latest"}],
				"build_gate": {
					"healing": {"by_error_kind": {"infra": {"image": "docker.io/user/heal:latest", "retain_container": true}}},
					"router": {"image": "docker.io/user/router:latest"}
				}
			}`,
			wantErr: "build_gate.healing.by_error_kind.infra.retain_container: forbidden",
		},
		{
			name: "router retain forbidden",
			input: `{
				"steps": [{"image": "docker.io/user/mig:latest"}],
				"build_gate": {
					"router": {"image": "docker.io/user/router:latest", "retain_container": true}
				}
			}`,
			wantErr: "build_gate.router.retain_container: forbidden",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseModsSpecJSON([]byte(tt.input))
			if err == nil {
				t.Fatal("expected error")
			}
			if err.Error() != tt.wantErr {
				t.Fatalf("error = %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestParseModsSpecJSON_StackSpecificImage(t *testing.T) {
	input := `{
		"steps": [{
			"image": {
				"default": "docker.io/user/mig:default",
				"java-maven": "docker.io/user/mig:maven",
				"java-gradle": "docker.io/user/mig:gradle"
			}
		}]
	}`

	spec, err := ParseModsSpecJSON([]byte(input))
	if err != nil {
		t.Fatalf("ParseModsSpecJSON failed: %v", err)
	}

	if spec.Steps[0].Image.IsUniversal() {
		t.Errorf("expected stack-specific image, got universal")
	}
	if !spec.Steps[0].Image.IsStackSpecific() {
		t.Errorf("expected IsStackSpecific() = true")
	}

	// Verify resolution.
	img, err := spec.Steps[0].Image.ResolveImage(ModStackJavaMaven)
	if err != nil {
		t.Fatalf("ResolveImage(java-maven) failed: %v", err)
	}
	if img != "docker.io/user/mig:maven" {
		t.Errorf("ResolveImage(java-maven) = %q, want %q", img, "docker.io/user/mig:maven")
	}

	// Verify default fallback.
	img, err = spec.Steps[0].Image.ResolveImage(ModStackUnknown)
	if err != nil {
		t.Fatalf("ResolveImage(unknown) failed: %v", err)
	}
	if img != "docker.io/user/mig:default" {
		t.Errorf("ResolveImage(unknown) = %q, want %q", img, "docker.io/user/mig:default")
	}
}

// TestParseModsSpecJSON_APIVersionAndKind tests parsing of optional metadata fields.
// These fields are informational (typically from YAML manifests converted to JSON).
func TestParseModsSpecJSON_APIVersionAndKind(t *testing.T) {
	input := `{
		"apiVersion": "ploy.mig/v1alpha1",
		"kind": "MigRunSpec",
		"steps": [{
			"image": "docker.io/user/mig:latest",
			"command": "echo hello",
			"env": {"FOO": "bar"}
		}],
		"build_gate": {"enabled": true}
	}`

	spec, err := ParseModsSpecJSON([]byte(input))
	if err != nil {
		t.Fatalf("ParseModsSpecJSON failed: %v", err)
	}

	if spec.APIVersion != "ploy.mig/v1alpha1" {
		t.Errorf("apiVersion = %q, want %q", spec.APIVersion, "ploy.mig/v1alpha1")
	}
	if spec.Kind != "MigRunSpec" {
		t.Errorf("kind = %q, want %q", spec.Kind, "MigRunSpec")
	}
	if spec.Steps[0].Image.Universal != "docker.io/user/mig:latest" {
		t.Errorf("image = %q, want %q", spec.Steps[0].Image.Universal, "docker.io/user/mig:latest")
	}
	if spec.Steps[0].Command.Shell != "echo hello" {
		t.Errorf("command = %q, want %q", spec.Steps[0].Command.Shell, "echo hello")
	}
}

// TestParseModsSpecJSON_Empty tests empty input handling.
func TestParseModsSpecJSON_Empty(t *testing.T) {
	_, err := ParseModsSpecJSON(nil)
	if err == nil {
		t.Fatal("expected error for empty input")
	}
	if want := "steps: required"; err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestParseModsSpecJSON_ModIndexForbidden(t *testing.T) {
	input := `{"mod_index":0,"steps":[{"image":"docker.io/user/mig:latest"}]}`
	_, err := ParseModsSpecJSON([]byte(input))
	if err == nil {
		t.Fatal("expected error for mod_index")
	}
	if !strings.Contains(err.Error(), "mod_index: forbidden") {
		t.Fatalf("expected mod_index forbidden error, got %q", err.Error())
	}
}

// TestParseModsSpecJSON_ValidationError tests validation errors.
func TestParseModsSpecJSON_ValidationError(t *testing.T) {
	// Step without image.
	input := `{"steps": [{"name": "test"}]}`
	_, err := ParseModsSpecJSON([]byte(input))
	if err == nil {
		t.Fatal("expected validation error for mig without image")
	}
	if want := "steps[0].image: required"; err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

// TestParseModsSpecJSON_HealingValidation tests healing spec validation.

func TestModsSpec_ArtifactFields(t *testing.T) {
	input := `{
		"steps": [{"image": "test:latest"}],
		"artifact_name": "my-bundle",
		"artifact_paths": ["output/", "logs/app.log"]
	}`

	spec, err := ParseModsSpecJSON([]byte(input))
	if err != nil {
		t.Fatalf("ParseModsSpecJSON failed: %v", err)
	}

	if spec.ArtifactName != "my-bundle" {
		t.Errorf("artifact_name = %q, want %q", spec.ArtifactName, "my-bundle")
	}
	if len(spec.ArtifactPaths) != 2 {
		t.Fatalf("len(artifact_paths) = %d, want 2", len(spec.ArtifactPaths))
	}
	if spec.ArtifactPaths[0] != "output/" {
		t.Errorf("artifact_paths[0] = %q, want %q", spec.ArtifactPaths[0], "output/")
	}
}

func TestParseModsSpecJSON_RequiresStepsEvenWithExtraFields(t *testing.T) {
	input := `{
		"mig": {
			"image": "docker.io/user/mig:latest",
			"command": "echo hello"
		}
	}`

	_, err := ParseModsSpecJSON([]byte(input))
	if err == nil {
		t.Fatal("expected error for missing steps")
	}
	wantErr := "steps: required"
	if err.Error() != wantErr {
		t.Errorf("error = %q, want %q", err.Error(), wantErr)
	}
}

// TestParseModsSpecJSON_InvalidJSON tests error handling for invalid JSON.
func TestParseModsSpecJSON_InvalidJSON(t *testing.T) {
	_, err := ParseModsSpecJSON([]byte(`{invalid json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
