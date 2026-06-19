package contracts

import (
	"strings"
	"testing"
)

func TestParseMigSpecJSON_SingleStep(t *testing.T) {
	input := `{
		"steps": [{
			"image": "ghcr.io/iw2rmb/ploy/mig:latest",
			"command": "echo hello",
			"envs": {"FOO": "bar", "BAZ": "qux"},
			"options": {"mount_docker_socket": true}
		}],
		"build_gate": {}
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
	if !step.Options.MountDockerSocket {
		t.Errorf("options.mount_docker_socket = false, want true")
	}

	// Verify build_gate.
	if spec.BuildGate == nil {
		t.Fatal("build_gate is nil")
	}
	if spec.BuildGate.Disabled {
		t.Errorf("build_gate.disabled = true, want false")
	}

}

// TestParseMigSpecJSON_MultiStep tests parsing multi-step spec JSON.
func TestParseMigSpecJSON_MultiStep(t *testing.T) {
	input := `{
		"steps": [
			{"name": "step-1", "image": "ghcr.io/iw2rmb/ploy/mig1:latest", "command": ["echo", "step1"], "envs": {"STEP": "1"}},
			{"name": "step-2", "image": "ghcr.io/iw2rmb/ploy/mig2:latest", "envs": {"STEP": "2"}}
		],
		"build_gate": {"disabled": true}
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
	if spec.BuildGate == nil || !spec.BuildGate.Disabled {
		t.Errorf("build_gate.disabled should be true")
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

// TestParseMigSpecJSON_RootMetadata tests parsing of optional root metadata fields.
func TestParseMigSpecJSON_RootMetadata(t *testing.T) {
	input := `{
		"apiVersion": "ploy.mig/v1alpha1",
		"name": "upgrade-java_17.v1",
		"description": "Upgrade Java projects to release 17",
		"steps": [{
			"image": "ghcr.io/iw2rmb/ploy/mig:latest",
			"command": "echo hello",
			"envs": {"FOO": "bar"}
		}],
		"build_gate": {}
	}`

	spec, err := ParseMigSpecJSON([]byte(input))
	if err != nil {
		t.Fatalf("ParseMigSpecJSON failed: %v", err)
	}

	if spec.APIVersion != "ploy.mig/v1alpha1" {
		t.Errorf("apiVersion = %q, want %q", spec.APIVersion, "ploy.mig/v1alpha1")
	}
	if spec.Name != "upgrade-java_17.v1" {
		t.Errorf("name = %q, want %q", spec.Name, "upgrade-java_17.v1")
	}
	if spec.Description != "Upgrade Java projects to release 17" {
		t.Errorf("description = %q, want %q", spec.Description, "Upgrade Java projects to release 17")
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

func TestParseMigSpecJSON_SchemaValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr []string
	}{
		{
			name: "missing steps with extra root field",
			input: `{
				"mig": {
					"image": "ghcr.io/iw2rmb/ploy/mig:latest",
					"command": "echo hello"
				}
			}`,
			wantErr: []string{
				"missing property 'steps'",
				"additional properties 'mig' not allowed",
			},
		},
		{
			name: "unknown nested build gate field",
			input: `{
				"steps": [{"image": "ghcr.io/iw2rmb/ploy/mig:latest"}],
				"build_gate": {"enabled": true}
			}`,
			wantErr: []string{
				"build_gate: additional properties 'enabled' not allowed",
			},
		},
		{
			name: "unknown step option field",
			input: `{
				"steps": [{
					"image": "ghcr.io/iw2rmb/ploy/mig:latest",
					"options": {"docker_socket": true}
				}]
			}`,
			wantErr: []string{
				"steps[0].options: additional properties 'docker_socket' not allowed",
			},
		},
		{
			name: "root name rejects uppercase",
			input: `{
				"name": "UpgradeJava",
				"steps": [{"image": "ghcr.io/iw2rmb/ploy/mig:latest"}]
			}`,
			wantErr: []string{
				"name: 'UpgradeJava' does not match pattern '^[0-9a-z._-]+$'",
			},
		},
		{
			name: "root name rejects empty string",
			input: `{
				"name": "",
				"steps": [{"image": "ghcr.io/iw2rmb/ploy/mig:latest"}]
			}`,
			wantErr: []string{
				"name: '' does not match pattern '^[0-9a-z._-]+$'",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseMigSpecJSON([]byte(tt.input))
			if err == nil {
				t.Fatal("expected schema validation error")
			}
			for _, want := range tt.wantErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error = %q, want to contain %q", err.Error(), want)
				}
			}
		})
	}
}

// TestParseMigSpecJSON_InvalidJSON tests error handling for invalid JSON.
func TestParseMigSpecJSON_InvalidJSON(t *testing.T) {
	_, err := ParseMigSpecJSON([]byte(`{invalid json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
