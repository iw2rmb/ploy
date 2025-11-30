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
		"job_id": "job-abc-123",
	}

	runOpts := parseRunOptions(opts)

	if runOpts.ServerMetadata.JobID != "job-abc-123" {
		t.Errorf("expected job_id=job-abc-123, got %q", runOpts.ServerMetadata.JobID)
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
		"job_id": "job-xyz"
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
	if typedOpts.ServerMetadata.JobID != "job-xyz" {
		t.Errorf("expected typed job_id=job-xyz, got %q", typedOpts.ServerMetadata.JobID)
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

// TestParseRunOptions_MultiStepMods verifies that parseRunOptions correctly
// extracts the Steps slice from mods[] array in multi-step run specs.
// For multi-step runs, RunOptions.Steps is populated; for single-step runs,
// Steps remains empty and Execution options are used.
func TestParseRunOptions_MultiStepMods(t *testing.T) {
	t.Parallel()

	// Multi-step spec with 3 mods entries.
	opts := map[string]any{
		"mods": []any{
			map[string]any{
				"image":   "docker.io/test/step1:v1",
				"command": "migrate-java8.sh",
				"env": map[string]any{
					"STEP":   "1",
					"TARGET": "java8",
				},
				"retain_container": false,
			},
			map[string]any{
				"image":   "docker.io/test/step2:v1",
				"command": []any{"/bin/sh", "-c", "migrate-java11.sh"},
				"env": map[string]any{
					"STEP":   "2",
					"TARGET": "java11",
				},
				"retain_container": true,
			},
			map[string]any{
				"image": "docker.io/test/step3:v1",
				"env": map[string]any{
					"STEP": "3",
				},
			},
		},
	}

	runOpts := parseRunOptions(opts)

	// Verify Steps slice is populated.
	if len(runOpts.Steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(runOpts.Steps))
	}

	// Verify first step.
	step0 := runOpts.Steps[0]
	if step0.Image != "docker.io/test/step1:v1" {
		t.Errorf("expected steps[0].image=docker.io/test/step1:v1, got %q", step0.Image)
	}
	if step0.Command.Shell != "migrate-java8.sh" {
		t.Errorf("expected steps[0].command.shell=migrate-java8.sh, got %q", step0.Command.Shell)
	}
	if step0.Env["STEP"] != "1" {
		t.Errorf("expected steps[0].env.STEP=1, got %q", step0.Env["STEP"])
	}
	if step0.Env["TARGET"] != "java8" {
		t.Errorf("expected steps[0].env.TARGET=java8, got %q", step0.Env["TARGET"])
	}
	if step0.RetainContainer {
		t.Errorf("expected steps[0].retain_container=false, got %v", step0.RetainContainer)
	}

	// Verify second step (command as exec array).
	step1 := runOpts.Steps[1]
	if step1.Image != "docker.io/test/step2:v1" {
		t.Errorf("expected steps[1].image=docker.io/test/step2:v1, got %q", step1.Image)
	}
	want := []string{"/bin/sh", "-c", "migrate-java11.sh"}
	if len(step1.Command.Exec) != len(want) {
		t.Fatalf("expected steps[1].command.exec length=%d, got %d", len(want), len(step1.Command.Exec))
	}
	for i, v := range want {
		if step1.Command.Exec[i] != v {
			t.Errorf("expected steps[1].command.exec[%d]=%q, got %q", i, v, step1.Command.Exec[i])
		}
	}
	if step1.Env["STEP"] != "2" {
		t.Errorf("expected steps[1].env.STEP=2, got %q", step1.Env["STEP"])
	}
	if !step1.RetainContainer {
		t.Errorf("expected steps[1].retain_container=true, got %v", step1.RetainContainer)
	}

	// Verify third step (no command specified).
	step2 := runOpts.Steps[2]
	if step2.Image != "docker.io/test/step3:v1" {
		t.Errorf("expected steps[2].image=docker.io/test/step3:v1, got %q", step2.Image)
	}
	if !step2.Command.IsEmpty() {
		t.Errorf("expected steps[2].command to be empty, got shell=%q exec=%v", step2.Command.Shell, step2.Command.Exec)
	}
	if step2.Env["STEP"] != "3" {
		t.Errorf("expected steps[2].env.STEP=3, got %q", step2.Env["STEP"])
	}
}

// TestParseRunOptions_EmptyModsArray verifies that an empty mods[] array
// results in empty Steps slice (not nil).
func TestParseRunOptions_EmptyModsArray(t *testing.T) {
	t.Parallel()

	opts := map[string]any{
		"mods": []any{},
	}

	runOpts := parseRunOptions(opts)

	// Empty mods[] should not populate Steps (len=0, not nil).
	if len(runOpts.Steps) != 0 {
		t.Errorf("expected empty steps slice, got %d steps", len(runOpts.Steps))
	}
}

// TestParseRunOptions_SingleStepHasNoSteps verifies that single-step runs
// (using "mod" or top-level fields) do NOT populate RunOptions.Steps.
// For single-step runs, Execution options are used instead of Steps.
func TestParseRunOptions_SingleStepHasNoSteps(t *testing.T) {
	t.Parallel()

	// Single-step spec (image/command at top-level, no mods[] array).
	opts := map[string]any{
		"image":   "docker.io/test/single:v1",
		"command": "run-single.sh",
	}

	runOpts := parseRunOptions(opts)

	// Verify Steps is empty (single-step format uses Execution instead).
	if len(runOpts.Steps) != 0 {
		t.Errorf("expected empty steps for single-step run, got %d steps", len(runOpts.Steps))
	}

	// Verify Execution options are populated.
	if runOpts.Execution.Image != "docker.io/test/single:v1" {
		t.Errorf("expected execution.image=docker.io/test/single:v1, got %q", runOpts.Execution.Image)
	}
	if runOpts.Execution.Command.Shell != "run-single.sh" {
		t.Errorf("expected execution.command.shell=run-single.sh, got %q", runOpts.Execution.Command.Shell)
	}
}

// TestParseSpec_MultiStepProducesTypedSteps verifies that parseSpec correctly
// produces typed RunOptions.Steps for multi-step specs with mods[] array.
func TestParseSpec_MultiStepProducesTypedSteps(t *testing.T) {
	t.Parallel()

	specJSON := `{
		"mods": [
			{
				"image": "docker.io/test/step-a:v1",
				"command": "step-a.sh",
				"env": {"KEY": "value-a"}
			},
			{
				"image": "docker.io/test/step-b:v1",
				"command": ["step-b.sh", "--flag"],
				"env": {"KEY": "value-b"},
				"retain_container": true
			}
		],
		"build_gate": {"enabled": true, "profile": "auto"}
	}`

	var raw json.RawMessage = []byte(specJSON)
	_, _, typedOpts := parseSpec(raw)

	// Verify typed Steps are populated.
	if len(typedOpts.Steps) != 2 {
		t.Fatalf("expected 2 typed steps, got %d", len(typedOpts.Steps))
	}

	// Verify first step.
	if typedOpts.Steps[0].Image != "docker.io/test/step-a:v1" {
		t.Errorf("expected steps[0].image=docker.io/test/step-a:v1, got %q", typedOpts.Steps[0].Image)
	}
	if typedOpts.Steps[0].Command.Shell != "step-a.sh" {
		t.Errorf("expected steps[0].command.shell=step-a.sh, got %q", typedOpts.Steps[0].Command.Shell)
	}
	if typedOpts.Steps[0].Env["KEY"] != "value-a" {
		t.Errorf("expected steps[0].env.KEY=value-a, got %q", typedOpts.Steps[0].Env["KEY"])
	}

	// Verify second step.
	if typedOpts.Steps[1].Image != "docker.io/test/step-b:v1" {
		t.Errorf("expected steps[1].image=docker.io/test/step-b:v1, got %q", typedOpts.Steps[1].Image)
	}
	want := []string{"step-b.sh", "--flag"}
	if len(typedOpts.Steps[1].Command.Exec) != len(want) {
		t.Fatalf("expected steps[1].command.exec length=%d, got %d", len(want), len(typedOpts.Steps[1].Command.Exec))
	}
	for i, v := range want {
		if typedOpts.Steps[1].Command.Exec[i] != v {
			t.Errorf("expected steps[1].command.exec[%d]=%q, got %q", i, v, typedOpts.Steps[1].Command.Exec[i])
		}
	}
	if !typedOpts.Steps[1].RetainContainer {
		t.Errorf("expected steps[1].retain_container=true, got %v", typedOpts.Steps[1].RetainContainer)
	}

	// Verify build gate is also parsed.
	if !typedOpts.BuildGate.Enabled {
		t.Errorf("expected build_gate.enabled=true")
	}
	if typedOpts.BuildGate.Profile != "auto" {
		t.Errorf("expected build_gate.profile=auto, got %q", typedOpts.BuildGate.Profile)
	}
}
