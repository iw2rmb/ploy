package contracts

import (
	"testing"
)

func TestParseMigSpecJSON_SingleStep(t *testing.T) {
	input := `{
		"steps": [{
			"image": "ghcr.io/iw2rmb/ploy/mig:latest",
			"command": "echo hello",
			"envs": {"FOO": "bar", "BAZ": "qux"}
		}],
		"build_gate": {"enabled": true},
		"gitlab_pat": "secret",
		"gitlab_domain": "gitlab.com",
		"mr_on_success": true,
		"mr_on_fail": false
	}`

	spec, err := ParseMigSpecJSON([]byte(input))
	if err != nil {
		t.Fatalf("ParseMigSpecJSON failed: %v", err)
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
	if step.Image.Universal != "ghcr.io/iw2rmb/ploy/mig:latest" {
		t.Errorf("image = %q, want %q", step.Image.Universal, "ghcr.io/iw2rmb/ploy/mig:latest")
	}

	// Verify envs.
	if step.Envs["FOO"] != "bar" {
		t.Errorf("envs[FOO] = %q, want %q", step.Envs["FOO"], "bar")
	}
	if step.Envs["BAZ"] != "qux" {
		t.Errorf("envs[BAZ] = %q, want %q", step.Envs["BAZ"], "qux")
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

// TestParseMigSpecJSON_MultiStep tests parsing multi-step spec JSON.
func TestParseMigSpecJSON_MultiStep(t *testing.T) {
	input := `{
		"steps": [
			{"name": "step-1", "image": "ghcr.io/iw2rmb/ploy/mig1:latest", "command": ["echo", "step1"], "envs": {"STEP": "1"}},
			{"name": "step-2", "image": "ghcr.io/iw2rmb/ploy/mig2:latest", "envs": {"STEP": "2"}}
		],
		"build_gate": {
			"enabled": true,
			"heal": {
				"retries": 3,
				"image": "ghcr.io/iw2rmb/ploy/java-17-codex-amata-maven:latest",
				"command": "fix-it",
				"envs": {"PROMPT": "fix the build"}
			}
		}
	}`

	spec, err := ParseMigSpecJSON([]byte(input))
	if err != nil {
		t.Fatalf("ParseMigSpecJSON failed: %v", err)
	}

	// Verify multi-step detection.
	if len(spec.Steps) != 2 {
		t.Fatalf("len(steps) = %d, want 2", len(spec.Steps))
	}

	// Verify first step.
	mig1 := spec.Steps[0]
	if mig1.Name != "step-1" {
		t.Errorf("steps[0].name = %q, want %q", mig1.Name, "step-1")
	}
	if mig1.Image.Universal != "ghcr.io/iw2rmb/ploy/mig1:latest" {
		t.Errorf("steps[0].image = %q, want %q", mig1.Image.Universal, "ghcr.io/iw2rmb/ploy/mig1:latest")
	}
	// Command is exec array form.
	if len(mig1.Command.Exec) != 2 || mig1.Command.Exec[0] != "echo" || mig1.Command.Exec[1] != "step1" {
		t.Errorf("steps[0].command.Exec = %v, want [echo, step1]", mig1.Command.Exec)
	}
	if mig1.Envs["STEP"] != "1" {
		t.Errorf("steps[0].envs[STEP] = %q, want %q", mig1.Envs["STEP"], "1")
	}

	// Verify second step.
	mig2 := spec.Steps[1]
	if mig2.Name != "step-2" {
		t.Errorf("steps[1].name = %q, want %q", mig2.Name, "step-2")
	}

	// Verify heal.
	if spec.BuildGate == nil || spec.BuildGate.Heal == nil {
		t.Fatal("build_gate.heal is nil")
	}
	if spec.BuildGate.Heal.Retries != 3 {
		t.Errorf("build_gate.heal.retries = %d, want 3", spec.BuildGate.Heal.Retries)
	}
	if spec.BuildGate.Heal.Image.Universal != "ghcr.io/iw2rmb/ploy/java-17-codex-amata-maven:latest" {
		t.Errorf("build_gate.heal.image = %q, want %q",
			spec.BuildGate.Heal.Image.Universal, "ghcr.io/iw2rmb/ploy/java-17-codex-amata-maven:latest")
	}
	if spec.BuildGate.Heal.Command.Shell != "fix-it" {
		t.Errorf("build_gate.heal.command = %q, want %q",
			spec.BuildGate.Heal.Command.Shell, "fix-it")
	}
}

func TestParseMigSpecJSON_StackSpecificImage(t *testing.T) {
	input := `{
		"steps": [{
			"image": {
				"default": "ghcr.io/iw2rmb/ploy/mig:default",
				"java-maven": "ghcr.io/iw2rmb/ploy/mig:maven",
				"java-gradle": "ghcr.io/iw2rmb/ploy/mig:gradle"
			}
		}]
	}`

	spec, err := ParseMigSpecJSON([]byte(input))
	if err != nil {
		t.Fatalf("ParseMigSpecJSON failed: %v", err)
	}

	if spec.Steps[0].Image.IsUniversal() {
		t.Errorf("expected stack-specific image, got universal")
	}
	if !spec.Steps[0].Image.IsStackSpecific() {
		t.Errorf("expected IsStackSpecific() = true")
	}

	// Verify resolution.
	img, err := spec.Steps[0].Image.ResolveImage(MigStackJavaMaven)
	if err != nil {
		t.Fatalf("ResolveImage(java-maven) failed: %v", err)
	}
	if img != "ghcr.io/iw2rmb/ploy/mig:maven" {
		t.Errorf("ResolveImage(java-maven) = %q, want %q", img, "ghcr.io/iw2rmb/ploy/mig:maven")
	}

	// Verify default fallback.
	img, err = spec.Steps[0].Image.ResolveImage(MigStackUnknown)
	if err != nil {
		t.Fatalf("ResolveImage(unknown) failed: %v", err)
	}
	if img != "ghcr.io/iw2rmb/ploy/mig:default" {
		t.Errorf("ResolveImage(unknown) = %q, want %q", img, "ghcr.io/iw2rmb/ploy/mig:default")
	}
}

// TestParseMigSpecJSON_APIVersionAndKind tests parsing of optional metadata fields.
// These fields are informational (typically from YAML manifests converted to JSON).
func TestParseMigSpecJSON_APIVersionAndKind(t *testing.T) {
	input := `{
		"apiVersion": "ploy.mig/v1alpha1",
		"kind": "MigRunSpec",
		"steps": [{
			"image": "ghcr.io/iw2rmb/ploy/mig:latest",
			"command": "echo hello",
			"envs": {"FOO": "bar"}
		}],
		"build_gate": {"enabled": true}
	}`

	spec, err := ParseMigSpecJSON([]byte(input))
	if err != nil {
		t.Fatalf("ParseMigSpecJSON failed: %v", err)
	}

	if spec.APIVersion != "ploy.mig/v1alpha1" {
		t.Errorf("apiVersion = %q, want %q", spec.APIVersion, "ploy.mig/v1alpha1")
	}
	if spec.Kind != "MigRunSpec" {
		t.Errorf("kind = %q, want %q", spec.Kind, "MigRunSpec")
	}
	if spec.Steps[0].Image.Universal != "ghcr.io/iw2rmb/ploy/mig:latest" {
		t.Errorf("image = %q, want %q", spec.Steps[0].Image.Universal, "ghcr.io/iw2rmb/ploy/mig:latest")
	}
	if spec.Steps[0].Command.Shell != "echo hello" {
		t.Errorf("command = %q, want %q", spec.Steps[0].Command.Shell, "echo hello")
	}
}

// TestParseMigSpecJSON_Empty tests empty input handling.
func TestParseMigSpecJSON_Empty(t *testing.T) {
	_, err := ParseMigSpecJSON(nil)
	if err == nil {
		t.Fatal("expected error for empty input")
	}
	if want := "steps: required"; err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

// TestParseMigSpecJSON_ValidationError tests validation errors.
func TestParseMigSpecJSON_ValidationError(t *testing.T) {
	// Step without image.
	input := `{"steps": [{"name": "test"}]}`
	_, err := ParseMigSpecJSON([]byte(input))
	if err == nil {
		t.Fatal("expected validation error for mig without image")
	}
	if want := "steps[0].image: required"; err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

// TestParseMigSpecJSON_HealingValidation tests healing spec validation.

func TestMigSpec_ArtifactFields(t *testing.T) {
	input := `{
		"steps": [{"image": "test:latest"}],
		"artifact_name": "my-bundle",
		"artifact_paths": ["output/", "logs/app.log"]
	}`

	spec, err := ParseMigSpecJSON([]byte(input))
	if err != nil {
		t.Fatalf("ParseMigSpecJSON failed: %v", err)
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

func TestParseMigSpecJSON_RequiresStepsEvenWithExtraFields(t *testing.T) {
	input := `{
		"mig": {
			"image": "ghcr.io/iw2rmb/ploy/mig:latest",
			"command": "echo hello"
		}
	}`

	_, err := ParseMigSpecJSON([]byte(input))
	if err == nil {
		t.Fatal("expected error for missing steps")
	}
	wantErr := "steps: required"
	if err.Error() != wantErr {
		t.Errorf("error = %q, want %q", err.Error(), wantErr)
	}
}

// TestParseMigSpecJSON_InvalidJSON tests error handling for invalid JSON.
func TestParseMigSpecJSON_InvalidJSON(t *testing.T) {
	_, err := ParseMigSpecJSON([]byte(`{invalid json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
