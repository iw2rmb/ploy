package nodeagent

import (
	"encoding/json"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// TestParseSpec_PassesThroughBuildGateHeal verifies that the node agent
// extracts the build_gate.heal block into typed RunOptions so discrete healing
// jobs can honor the configured heal → re-gate loop.
func TestParseSpec_PassesThroughBuildGateHeal(t *testing.T) {
	specJSON := `{
	        "steps": [{"image": "docker.io/test/mig:latest"}],
	        "build_gate": {
	            "heal": {
	                "retries": 2,
	                "image": "docker.io/test/heal:latest"
	            }
	        }
	    }`

	var raw json.RawMessage = []byte(specJSON)
	_, typedOpts, _ := parseSpec(raw)

	if typedOpts.Healing == nil {
		t.Fatal("expected Healing config to be parsed")
	}
	if typedOpts.Healing.Retries != 2 {
		t.Fatalf("expected retries=2, got %d", typedOpts.Healing.Retries)
	}
	if typedOpts.Healing.Mig.Image.Universal != "docker.io/test/heal:latest" {
		t.Fatalf("expected heal image docker.io/test/heal:latest, got %q", typedOpts.Healing.Mig.Image.Universal)
	}
}

func TestParseSpec_CanonicalSingleStepFormat(t *testing.T) {
	specJSON := `{
        "steps": [{
            "image": "docker.io/test/mig:latest",
            "command": ["/bin/sh","-c","echo hi"]
        }],
        "envs": {"A":"1","B":"2"},
        "build_gate": {"enabled": false}
    }`
	var raw json.RawMessage = []byte(specJSON)
	env, typedOpts, _ := parseSpec(raw)

	// Verify execution options are extracted.
	img, err := typedOpts.Execution.Image.ResolveImage(contracts.MigStackUnknown)
	if err != nil {
		t.Fatalf("unexpected image resolve error: %v", err)
	}
	if img != "docker.io/test/mig:latest" {
		t.Fatalf("Execution.Image = %q, want docker.io/test/mig:latest", img)
	}
	wantExec := []string{"/bin/sh", "-c", "echo hi"}
	gotExec := typedOpts.Execution.Command.ToSlice()
	if len(gotExec) != len(wantExec) {
		t.Fatalf("Execution.Command.ToSlice length = %d, want %d", len(gotExec), len(wantExec))
	}
	for i, want := range wantExec {
		if gotExec[i] != want {
			t.Fatalf("Execution.Command.ToSlice[%d] = %q, want %q", i, gotExec[i], want)
		}
	}
	if env["A"] != "1" || env["B"] != "2" {
		t.Fatalf("env not extracted: %+v", env)
	}

	// Verify build gate options.
	if typedOpts.BuildGate.Enabled {
		t.Fatalf("BuildGate.Enabled = true, want false")
	}
}

func TestParseSpec_IgnoresUnknownTopLevelMigObject(t *testing.T) {
	specJSON := `{
        "steps": [{
            "image": "docker.io/test/required:latest",
            "command": "echo from steps",
            "envs": {"A":"1","B":"2"}
        }],
        "mig": {
            "image": "docker.io/test/ignored:latest",
            "retain_container": true,
            "env": {"A":"999","C":"should-not-merge"},
            "command": "echo ignored"
        },
        "build_gate": {"enabled": false}
    }`
	var raw json.RawMessage = []byte(specJSON)
	env, typedOpts, _ := parseSpec(raw)

	// steps[0] should drive single-step extraction; unknown top-level "mig" is ignored.
	img, err := typedOpts.Execution.Image.ResolveImage(contracts.MigStackUnknown)
	if err != nil {
		t.Fatalf("unexpected image resolve error: %v", err)
	}
	if img != "docker.io/test/required:latest" {
		t.Fatalf("expected image from steps[0], got %q", img)
	}
	if typedOpts.Execution.Command.Shell != "echo from steps" {
		t.Fatalf("expected command from steps[0], got %q", typedOpts.Execution.Command.Shell)
	}
	if env["A"] != "1" || env["B"] != "2" {
		t.Fatalf("expected env from steps[0] to be merged, got: %+v", env)
	}
	if _, ok := env["C"]; ok {
		t.Fatalf("expected env from top-level mig to be ignored, got: %+v", env)
	}
}

// TestParseSpec_PreservesStepsArray verifies that parseSpec preserves the steps[]
// array for multi-step runs without modification. The migs[] array represents
// sequential transformation steps that share global gate/healing policy.
func TestParseSpec_PreservesStepsArray(t *testing.T) {
	t.Parallel()

	// Spec with multi-step steps[] array (3 steps with different images and env).
	specJSON := `{
		"steps": [
			{
				"image": "docker.io/test/mig-step1:latest",
				"envs": {"STEP": "1", "TARGET": "java8"}
			},
			{
				"image": "docker.io/test/mig-step2:latest",
				"command": ["migrate.sh", "--verbose"],
				"envs": {"STEP": "2", "TARGET": "java11"}
			},
			{
				"image": "docker.io/test/mig-step3:latest",
				"command": "finalize.sh",
				"envs": {"STEP": "3"}
			}
		],
		"build_gate": {
			"enabled": true,
			"heal": {
				"retries": 1,
				"image": "docker.io/test/healer:latest"
			}
		}
	}`

	var raw json.RawMessage = []byte(specJSON)
	_, typedOpts, _ := parseSpec(raw)

	if len(typedOpts.Steps) != 3 {
		t.Fatalf("expected 3 steps in typed options, got %d", len(typedOpts.Steps))
	}

	// Verify first step entry.
	step0Img, err := typedOpts.Steps[0].Image.ResolveImage(contracts.MigStackUnknown)
	if err != nil {
		t.Fatalf("unexpected steps[0] image resolve error: %v", err)
	}
	if step0Img != "docker.io/test/mig-step1:latest" {
		t.Errorf("expected steps[0].image=docker.io/test/mig-step1:latest, got %q", step0Img)
	}
	if typedOpts.Steps[0].Env["STEP"] != "1" {
		t.Errorf("expected steps[0].env.STEP=1, got %q", typedOpts.Steps[0].Env["STEP"])
	}
	if typedOpts.Steps[0].Env["TARGET"] != "java8" {
		t.Errorf("expected steps[0].env.TARGET=java8, got %q", typedOpts.Steps[0].Env["TARGET"])
	}

	// Verify second mig entry has command array preserved.
	step1Img, err := typedOpts.Steps[1].Image.ResolveImage(contracts.MigStackUnknown)
	if err != nil {
		t.Fatalf("unexpected steps[1] image resolve error: %v", err)
	}
	if step1Img != "docker.io/test/mig-step2:latest" {
		t.Errorf("expected steps[1].image=docker.io/test/mig-step2:latest, got %q", step1Img)
	}
	wantStep1Cmd := []string{"migrate.sh", "--verbose"}
	gotStep1Cmd := typedOpts.Steps[1].Command.ToSlice()
	if len(gotStep1Cmd) != len(wantStep1Cmd) {
		t.Fatalf("expected steps[1].command length=%d, got %d", len(wantStep1Cmd), len(gotStep1Cmd))
	}
	for i, want := range wantStep1Cmd {
		if gotStep1Cmd[i] != want {
			t.Fatalf("expected steps[1].command[%d]=%q, got %q", i, want, gotStep1Cmd[i])
		}
	}

	// Verify third mig entry has shell command preserved.
	step2Img, err := typedOpts.Steps[2].Image.ResolveImage(contracts.MigStackUnknown)
	if err != nil {
		t.Fatalf("unexpected steps[2] image resolve error: %v", err)
	}
	if step2Img != "docker.io/test/mig-step3:latest" {
		t.Errorf("expected steps[2].image=docker.io/test/mig-step3:latest, got %q", step2Img)
	}
	if typedOpts.Steps[2].Command.Shell != "finalize.sh" {
		t.Errorf("expected steps[2].command=finalize.sh, got %q", typedOpts.Steps[2].Command.Shell)
	}

	// Verify build gate and healing are preserved (global policy).
	if !typedOpts.BuildGate.Enabled {
		t.Errorf("expected BuildGate.Enabled=true, got false")
	}
	if typedOpts.Healing == nil {
		t.Fatalf("expected Healing config to be parsed")
	}
	if typedOpts.Healing.Retries != 1 {
		t.Errorf("expected Healing.Retries=1, got %d", typedOpts.Healing.Retries)
	}
	if typedOpts.Healing.Mig.Image.Universal != "docker.io/test/healer:latest" {
		t.Errorf("expected Healing.Mig.Image=docker.io/test/healer:latest, got %q", typedOpts.Healing.Mig.Image.Universal)
	}
}

func TestParseSpec_HealingSingleMig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		specJSON    string
		wantRetries int
		wantImage   string
	}{
		{
			name: "single_mig_healing",
			specJSON: `{
				"steps": [{"image": "docker.io/test/mig:latest"}],
				"build_gate": {
					"heal": {
						"retries": 3,
						"image": "docker.io/test/codex:latest",
						"command": "fix-with-ai"
					}
				}
			}`,
			wantRetries: 3,
			wantImage:   "docker.io/test/codex:latest",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var raw json.RawMessage = []byte(tc.specJSON)
			_, typedOpts, _ := parseSpec(raw)

			// Verify typed options.
			if typedOpts.Healing == nil {
				t.Fatal("expected Healing config to be parsed")
			}

			if typedOpts.Healing.Retries != tc.wantRetries {
				t.Errorf("Retries: got %d, want %d", typedOpts.Healing.Retries, tc.wantRetries)
			}

			if typedOpts.Healing.Mig.Image.Universal != tc.wantImage {
				t.Errorf("Healing mig image: got %q, want %q", typedOpts.Healing.Mig.Image.Universal, tc.wantImage)
			}
		})
	}
}

func TestParseHealingMig_MigFields(t *testing.T) {
	t.Parallel()

	specJSON := `{
		"steps": [{"image": "docker.io/test/mig:latest"}],
		"build_gate": {
			"heal": {
				"retries": 1,
				"image": "docker.io/test/healer:v1",
				"command": "heal.sh --fix",
				"envs": {
					"MODE": "aggressive",
					"DEBUG": "true"
				}
			}
		}
	}`

	var raw json.RawMessage = []byte(specJSON)
	_, typedOpts, _ := parseSpec(raw)

	if typedOpts.Healing == nil {
		t.Fatal("expected healing config to be parsed")
	}

	mig := typedOpts.Healing.Mig

	// Verify image.
	if mig.Image.Universal != "docker.io/test/healer:v1" {
		t.Errorf("Mig image: got %q, want %q", mig.Image.Universal, "docker.io/test/healer:v1")
	}

	// Verify command (shell form).
	if mig.Command.Shell != "heal.sh --fix" {
		t.Errorf("Mig command: got %q, want %q", mig.Command.Shell, "heal.sh --fix")
	}

	// Verify env.
	if mig.Env["MODE"] != "aggressive" {
		t.Errorf("Mig env MODE: got %q, want %q", mig.Env["MODE"], "aggressive")
	}
	if mig.Env["DEBUG"] != "true" {
		t.Errorf("Mig env DEBUG: got %q, want %q", mig.Env["DEBUG"], "true")
	}
}

// TestParseSpec_ProducesTypedOptions_SingleStepExecArray is a regression test
// for the bug where single-step specs with exec-array commands (e.g., ["a","b"])
// would drop into empty command due to the previous map-bridge conversion path,
// which forced command exec arrays through []any and required manual conversion.
//
// This test verifies that TypedOptions.Execution.Command.Exec is populated
// correctly for single-step specs with exec-array commands.
func TestParseSpec_ProducesTypedOptions_SingleStepExecArray(t *testing.T) {
	t.Parallel()

	// Single-step spec with exec-array command (not shell string).
	// This is the canonical format that was previously broken.
	specJSON := `{
		"steps": [{
			"image": "docker.io/test/mig:latest",
			"command": ["echo", "hello", "world"]
		}]
	}`

	var raw json.RawMessage = []byte(specJSON)
	_, typedOpts, _ := parseSpec(raw)

	// Verify that Execution.Command.Exec is populated (regression test).
	// Before the fix, this would be empty because []any != []string.
	wantExec := []string{"echo", "hello", "world"}
	if len(typedOpts.Execution.Command.Exec) != len(wantExec) {
		t.Fatalf("Execution.Command.Exec: got length %d, want %d (values: %v)",
			len(typedOpts.Execution.Command.Exec), len(wantExec), typedOpts.Execution.Command.Exec)
	}
	for i, want := range wantExec {
		if typedOpts.Execution.Command.Exec[i] != want {
			t.Errorf("Execution.Command.Exec[%d]: got %q, want %q",
				i, typedOpts.Execution.Command.Exec[i], want)
		}
	}

	// Verify Shell is empty (exec-array takes precedence).
	if typedOpts.Execution.Command.Shell != "" {
		t.Errorf("Execution.Command.Shell: got %q, want empty", typedOpts.Execution.Command.Shell)
	}

	// Verify ToSlice returns the exec array directly (not wrapped in shell).
	slice := typedOpts.Execution.Command.ToSlice()
	if len(slice) != len(wantExec) {
		t.Fatalf("ToSlice(): got length %d, want %d", len(slice), len(wantExec))
	}
	for i, want := range wantExec {
		if slice[i] != want {
			t.Errorf("ToSlice()[%d]: got %q, want %q", i, slice[i], want)
		}
	}
}
