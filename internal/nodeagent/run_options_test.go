package nodeagent

import (
	"encoding/json"
	"testing"
)

// TestParseRunOptions_BuildGate verifies that build gate options are correctly parsed.
func TestParseRunOptions_BuildGate(t *testing.T) {
	t.Parallel()

	opts := map[string]any{
		"build_gate_enabled": true,
		"build_gate_profile": "java-maven",
	}

	runOpts := parseRunOptions(opts)

	if !runOpts.BuildGate.Enabled {
		t.Errorf("expected build_gate_enabled=true, got %v", runOpts.BuildGate.Enabled)
	}
	if runOpts.BuildGate.Profile != "java-maven" {
		t.Errorf("expected build_gate_profile=java-maven, got %q", runOpts.BuildGate.Profile)
	}
}

// TestParseRunOptions_HealingConfig verifies that healing configuration is correctly parsed.
func TestParseRunOptions_HealingConfig(t *testing.T) {
	t.Parallel()

	opts := map[string]any{
		"build_gate_healing": map[string]any{
			"retries": float64(3), // JSON unmarshals numbers as float64
			"mods": []any{
				map[string]any{
					"image":   "docker.io/test/heal:v1",
					"command": "heal.sh",
					"env": map[string]any{
						"MODE": "auto",
					},
					"retain_container": true,
				},
			},
		},
	}

	runOpts := parseRunOptions(opts)

	if runOpts.Healing == nil {
		t.Fatal("expected healing config to be parsed")
	}
	if runOpts.Healing.Retries != 3 {
		t.Errorf("expected retries=3, got %d", runOpts.Healing.Retries)
	}
	if len(runOpts.Healing.Mods) != 1 {
		t.Fatalf("expected 1 healing mod, got %d", len(runOpts.Healing.Mods))
	}

	mod := runOpts.Healing.Mods[0]
	if mod.Image != "docker.io/test/heal:v1" {
		t.Errorf("expected image=docker.io/test/heal:v1, got %q", mod.Image)
	}
	if mod.Command.Shell != "heal.sh" {
		t.Errorf("expected command.shell=heal.sh, got %q", mod.Command.Shell)
	}
	if mod.Env["MODE"] != "auto" {
		t.Errorf("expected env MODE=auto, got %q", mod.Env["MODE"])
	}
	if !mod.RetainContainer {
		t.Errorf("expected retain_container=true, got %v", mod.RetainContainer)
	}
}

// TestParseRunOptions_HealingWithArrayCommand verifies that healing mod commands
// can be parsed from JSON arrays.
func TestParseRunOptions_HealingWithArrayCommand(t *testing.T) {
	t.Parallel()

	opts := map[string]any{
		"build_gate_healing": map[string]any{
			"retries": 1,
			"mods": []any{
				map[string]any{
					"image":   "docker.io/test/heal:v2",
					"command": []any{"/bin/sh", "-c", "echo healing"},
				},
			},
		},
	}

	runOpts := parseRunOptions(opts)

	if runOpts.Healing == nil {
		t.Fatal("expected healing config to be parsed")
	}
	if len(runOpts.Healing.Mods) != 1 {
		t.Fatalf("expected 1 healing mod, got %d", len(runOpts.Healing.Mods))
	}

	mod := runOpts.Healing.Mods[0]
	want := []string{"/bin/sh", "-c", "echo healing"}
	if len(mod.Command.Exec) != len(want) {
		t.Fatalf("expected command.exec length=%d, got %d", len(want), len(mod.Command.Exec))
	}
	for i, v := range want {
		if mod.Command.Exec[i] != v {
			t.Errorf("expected command.exec[%d]=%q, got %q", i, v, mod.Command.Exec[i])
		}
	}
}

// TestParseRunOptions_MRWiring verifies that MR wiring options are correctly parsed.
func TestParseRunOptions_MRWiring(t *testing.T) {
	t.Parallel()

	opts := map[string]any{
		"gitlab_pat":    "glpat-test-token",
		"gitlab_domain": "gitlab.example.com",
		"mr_on_success": true,
		"mr_on_fail":    false,
	}

	runOpts := parseRunOptions(opts)

	if runOpts.MRWiring.GitLabPAT != "glpat-test-token" {
		t.Errorf("expected gitlab_pat=glpat-test-token, got %q", runOpts.MRWiring.GitLabPAT)
	}
	if runOpts.MRWiring.GitLabDomain != "gitlab.example.com" {
		t.Errorf("expected gitlab_domain=gitlab.example.com, got %q", runOpts.MRWiring.GitLabDomain)
	}
	if !runOpts.MRWiring.MROnSuccess {
		t.Errorf("expected mr_on_success=true, got %v", runOpts.MRWiring.MROnSuccess)
	}
	if runOpts.MRWiring.MROnFail {
		t.Errorf("expected mr_on_fail=false, got %v", runOpts.MRWiring.MROnFail)
	}
}

// TestParseRunOptions_Execution verifies that execution options are correctly parsed.
func TestParseRunOptions_Execution(t *testing.T) {
	t.Parallel()

	t.Run("with shell command", func(t *testing.T) {
		opts := map[string]any{
			"image":            "ubuntu:22.04",
			"command":          "echo hello",
			"retain_container": true,
		}

		runOpts := parseRunOptions(opts)

		if runOpts.Execution.Image != "ubuntu:22.04" {
			t.Errorf("expected image=ubuntu:22.04, got %q", runOpts.Execution.Image)
		}
		if runOpts.Execution.Command.Shell != "echo hello" {
			t.Errorf("expected command.shell='echo hello', got %q", runOpts.Execution.Command.Shell)
		}
		if !runOpts.Execution.RetainContainer {
			t.Errorf("expected retain_container=true, got %v", runOpts.Execution.RetainContainer)
		}
	})

	t.Run("with exec array command", func(t *testing.T) {
		opts := map[string]any{
			"image":   "ubuntu:22.04",
			"command": []string{"/bin/ls", "-la"},
		}

		runOpts := parseRunOptions(opts)

		want := []string{"/bin/ls", "-la"}
		if len(runOpts.Execution.Command.Exec) != len(want) {
			t.Fatalf("expected command.exec length=%d, got %d", len(want), len(runOpts.Execution.Command.Exec))
		}
		for i, v := range want {
			if runOpts.Execution.Command.Exec[i] != v {
				t.Errorf("expected command.exec[%d]=%q, got %q", i, v, runOpts.Execution.Command.Exec[i])
			}
		}
	})
}

// TestParseRunOptions_ServerMetadata verifies that server metadata options are correctly parsed.
func TestParseRunOptions_ServerMetadata(t *testing.T) {
	t.Parallel()

	opts := map[string]any{
		"stage_id": "stage-abc-123",
	}

	runOpts := parseRunOptions(opts)

	if runOpts.ServerMetadata.StageID != "stage-abc-123" {
		t.Errorf("expected stage_id=stage-abc-123, got %q", runOpts.ServerMetadata.StageID)
	}
}

// TestParseRunOptions_Artifacts verifies that artifact options are correctly parsed.
func TestParseRunOptions_Artifacts(t *testing.T) {
	t.Parallel()

	opts := map[string]any{
		"artifact_name": "my-bundle.tar.gz",
	}

	runOpts := parseRunOptions(opts)

	if runOpts.Artifacts.Name != "my-bundle.tar.gz" {
		t.Errorf("expected artifact_name=my-bundle.tar.gz, got %q", runOpts.Artifacts.Name)
	}
}

// TestParseSpec_ProducesTypedOptions verifies that parseSpec returns typed RunOptions.
func TestParseSpec_ProducesTypedOptions(t *testing.T) {
	t.Parallel()

	specJSON := `{
		"image": "docker.io/test/mod:latest",
		"command": "run-test.sh",
		"retain_container": true,
		"build_gate": {
			"enabled": false,
			"profile": "java-auto"
		},
		"gitlab_pat": "glpat-secret",
		"mr_on_success": true,
		"stage_id": "stage-xyz"
	}`

	var raw json.RawMessage = []byte(specJSON)
	opts, _, typedOpts := parseSpec(raw)

	// Verify raw opts map is populated (backward compatibility).
	if opts["image"] != "docker.io/test/mod:latest" {
		t.Errorf("raw opts image mismatch")
	}

	// Verify typed options are populated.
	if typedOpts.Execution.Image != "docker.io/test/mod:latest" {
		t.Errorf("expected typed image=docker.io/test/mod:latest, got %q", typedOpts.Execution.Image)
	}
	if typedOpts.Execution.Command.Shell != "run-test.sh" {
		t.Errorf("expected typed command.shell=run-test.sh, got %q", typedOpts.Execution.Command.Shell)
	}
	if !typedOpts.Execution.RetainContainer {
		t.Errorf("expected typed retain_container=true")
	}
	if typedOpts.BuildGate.Enabled {
		t.Errorf("expected typed build_gate.enabled=false")
	}
	if typedOpts.BuildGate.Profile != "java-auto" {
		t.Errorf("expected typed build_gate.profile=java-auto, got %q", typedOpts.BuildGate.Profile)
	}
	if typedOpts.MRWiring.GitLabPAT != "glpat-secret" {
		t.Errorf("expected typed gitlab_pat=glpat-secret, got %q", typedOpts.MRWiring.GitLabPAT)
	}
	if !typedOpts.MRWiring.MROnSuccess {
		t.Errorf("expected typed mr_on_success=true")
	}
	if typedOpts.ServerMetadata.StageID != "stage-xyz" {
		t.Errorf("expected typed stage_id=stage-xyz, got %q", typedOpts.ServerMetadata.StageID)
	}
}

// TestHealingCommand_ToSlice verifies command conversion to slice.
func TestHealingCommand_ToSlice(t *testing.T) {
	t.Parallel()

	t.Run("shell command", func(t *testing.T) {
		cmd := HealingCommand{Shell: "echo test"}
		result := cmd.ToSlice()
		want := []string{"/bin/sh", "-c", "echo test"}
		if len(result) != len(want) {
			t.Fatalf("expected length=%d, got %d", len(want), len(result))
		}
		for i, v := range want {
			if result[i] != v {
				t.Errorf("expected result[%d]=%q, got %q", i, v, result[i])
			}
		}
	})

	t.Run("exec array", func(t *testing.T) {
		cmd := HealingCommand{Exec: []string{"/bin/ls", "-la"}}
		result := cmd.ToSlice()
		want := []string{"/bin/ls", "-la"}
		if len(result) != len(want) {
			t.Fatalf("expected length=%d, got %d", len(want), len(result))
		}
		for i, v := range want {
			if result[i] != v {
				t.Errorf("expected result[%d]=%q, got %q", i, v, result[i])
			}
		}
	})

	t.Run("empty command", func(t *testing.T) {
		cmd := HealingCommand{}
		result := cmd.ToSlice()
		if result != nil {
			t.Errorf("expected nil for empty command, got %v", result)
		}
	})

	t.Run("exec takes precedence over shell", func(t *testing.T) {
		cmd := HealingCommand{
			Shell: "echo shell",
			Exec:  []string{"/bin/exec"},
		}
		result := cmd.ToSlice()
		if len(result) != 1 || result[0] != "/bin/exec" {
			t.Errorf("expected exec to take precedence, got %v", result)
		}
	})
}

// TestExecutionCommand_ToSlice verifies command conversion to slice.
func TestExecutionCommand_ToSlice(t *testing.T) {
	t.Parallel()

	t.Run("shell command", func(t *testing.T) {
		cmd := ExecutionCommand{Shell: "echo test"}
		result := cmd.ToSlice()
		want := []string{"/bin/sh", "-c", "echo test"}
		if len(result) != len(want) {
			t.Fatalf("expected length=%d, got %d", len(want), len(result))
		}
		for i, v := range want {
			if result[i] != v {
				t.Errorf("expected result[%d]=%q, got %q", i, v, result[i])
			}
		}
	})

	t.Run("exec array", func(t *testing.T) {
		cmd := ExecutionCommand{Exec: []string{"/bin/ls", "-la"}}
		result := cmd.ToSlice()
		want := []string{"/bin/ls", "-la"}
		if len(result) != len(want) {
			t.Fatalf("expected length=%d, got %d", len(want), len(result))
		}
		for i, v := range want {
			if result[i] != v {
				t.Errorf("expected result[%d]=%q, got %q", i, v, result[i])
			}
		}
	})

	t.Run("empty command", func(t *testing.T) {
		cmd := ExecutionCommand{}
		result := cmd.ToSlice()
		if result != nil {
			t.Errorf("expected nil for empty command, got %v", result)
		}
	})
}
