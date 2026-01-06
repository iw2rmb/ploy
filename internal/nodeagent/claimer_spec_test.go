package nodeagent

import (
	"encoding/json"
	"testing"
)

// TestParseSpec_PassesThroughBuildGateHealing verifies that the node agent
// carries the build_gate.healing block from the spec into Options so that
// executeWithHealing can honor the configured heal → re-gate loop.
func TestParseSpec_PassesThroughBuildGateHealing(t *testing.T) {
	specJSON := `{
	        "steps": [{"image": "docker.io/test/mod:latest"}],
	        "build_gate": {
	            "healing": {
	                "retries": 2,
	                "mod": { "image": "docker.io/test/heal:latest" }
	            }
	        }
	    }`

	var raw json.RawMessage = []byte(specJSON)
	opts, _, _ := parseSpec(raw)

	bgAny, ok := opts["build_gate"]
	if !ok {
		t.Fatalf("expected build_gate present in options")
	}

	bg, ok := bgAny.(map[string]any)
	if !ok {
		t.Fatalf("expected build_gate to be map[string]any, got %T", bgAny)
	}

	healingAny, ok := bg["healing"]
	if !ok {
		t.Fatalf("expected build_gate.healing present in options")
	}

	m, ok := healingAny.(map[string]any)
	if !ok {
		t.Fatalf("expected build_gate.healing to be map[string]any, got %T", healingAny)
	}

	// Check retries
	switch r := m["retries"].(type) {
	case int:
		if r != 2 {
			t.Fatalf("expected retries=2, got %v (%T)", m["retries"], m["retries"])
		}
	case float64:
		if int(r) != 2 {
			t.Fatalf("expected retries=2, got %v (%T)", m["retries"], m["retries"])
		}
	default:
		t.Fatalf("expected retries=2, got %v (%T)", m["retries"], m["retries"])
	}

	mod, ok := m["mod"].(map[string]any)
	if !ok {
		t.Fatalf("expected mod object, got %T", m["mod"])
	}
	if img, _ := mod["image"].(string); img != "docker.io/test/heal:latest" {
		t.Fatalf("expected heal image docker.io/test/heal:latest, got %v", img)
	}
}

// TestParseSpec_CanonicalSingleStepFormat verifies that parseSpec correctly extracts
// flattened top-level fields for single-step runs from steps[0].
func TestParseSpec_CanonicalSingleStepFormat(t *testing.T) {
	specJSON := `{
        "steps": [{
            "image": "docker.io/test/mod:latest",
            "retain_container": true,
            "command": ["/bin/sh","-c","echo hi"]
        }],
        "env": {"A":"1","B":"2"},
        "build_gate": {"enabled": false, "profile": "java-maven"}
    }`
	var raw json.RawMessage = []byte(specJSON)
	opts, env, _ := parseSpec(raw)

	// Verify top-level fields are extracted.
	if opts["image"] != "docker.io/test/mod:latest" {
		t.Fatalf("image not extracted: %v", opts["image"])
	}
	if rc, _ := opts["retain_container"].(bool); !rc {
		t.Fatalf("retain_container not extracted")
	}
	if env["A"] != "1" || env["B"] != "2" {
		t.Fatalf("env not extracted: %+v", env)
	}

	// Verify build_gate is flattened.
	if en, ok := opts["build_gate_enabled"].(bool); !ok || en != false {
		t.Fatalf("build_gate_enabled not flattened: %v", opts["build_gate_enabled"])
	}
	if pr, ok := opts["build_gate_profile"].(string); !ok || pr != "java-maven" {
		t.Fatalf("build_gate_profile not flattened: %v", opts["build_gate_profile"])
	}
}

// TestParseSpec_LegacyModObjectIgnored verifies that the legacy "mod" object format
// is no longer processed by parseSpec. Specs using "mod" must be migrated to
// canonical shapes (steps[]).
func TestParseSpec_LegacyModObjectIgnored(t *testing.T) {
	// Legacy format: nested "mod" object (no longer supported).
	specJSON := `{
        "steps": [{"image": "docker.io/test/required:latest"}],
        "mod": {
            "image": "docker.io/test/mod:latest",
            "retain_container": true,
            "env": {"A":"1","B":"2"},
            "command": ["/bin/sh","-c","echo hi"]
        },
        "build_gate": {"enabled": false, "profile": "java-maven"}
    }`
	var raw json.RawMessage = []byte(specJSON)
	opts, env, _ := parseSpec(raw)

	// Legacy "mod" object is rejected by the canonical parser; parseSpec returns empty maps.
	if _, hasImage := opts["image"]; hasImage {
		t.Fatalf("expected image not to be extracted from legacy mod object")
	}
	if _, hasRetain := opts["retain_container"]; hasRetain {
		t.Fatalf("expected retain_container not to be extracted from legacy mod object")
	}
	if len(env) > 0 {
		t.Fatalf("expected env to be empty (mod.env should not be merged), got: %+v", env)
	}
}

// TestParseSpec_PreservesStepsArray verifies that parseSpec preserves the steps[]
// array for multi-step runs without modification. The mods[] array represents
// sequential transformation steps that share global gate/healing policy.
func TestParseSpec_PreservesStepsArray(t *testing.T) {
	t.Parallel()

	// Spec with multi-step steps[] array (3 steps with different images and env).
	specJSON := `{
		"steps": [
			{
				"image": "docker.io/test/mod-step1:latest",
				"env": {"STEP": "1", "TARGET": "java8"},
				"retain_container": false
			},
			{
				"image": "docker.io/test/mod-step2:latest",
				"command": ["migrate.sh", "--verbose"],
				"env": {"STEP": "2", "TARGET": "java11"}
			},
			{
				"image": "docker.io/test/mod-step3:latest",
				"command": "finalize.sh",
				"env": {"STEP": "3"}
			}
		],
		"build_gate": {
			"enabled": true,
			"profile": "auto",
			"healing": {
				"retries": 1,
				"mod": {"image": "docker.io/test/healer:latest"}
			}
		}
	}`

	var raw json.RawMessage = []byte(specJSON)
	opts, _, _ := parseSpec(raw)

	// Verify steps[] array is present in opts.
	stepsRaw, ok := opts["steps"]
	if !ok {
		t.Fatalf("expected steps array in options, got none")
	}

	stepsSlice, ok := stepsRaw.([]any)
	if !ok {
		t.Fatalf("expected steps to be []any, got %T", stepsRaw)
	}

	if len(stepsSlice) != 3 {
		t.Fatalf("expected 3 steps in array, got %d", len(stepsSlice))
	}

	// Verify first mod entry is preserved correctly.
	mod0, ok := stepsSlice[0].(map[string]any)
	if !ok {
		t.Fatalf("expected steps[0] to be map[string]any, got %T", stepsSlice[0])
	}
	if img, _ := mod0["image"].(string); img != "docker.io/test/mod-step1:latest" {
		t.Errorf("expected steps[0].image=docker.io/test/mod-step1:latest, got %v", img)
	}
	if env0, ok := mod0["env"].(map[string]any); ok {
		if step, _ := env0["STEP"].(string); step != "1" {
			t.Errorf("expected steps[0].env.STEP=1, got %v", step)
		}
		if target, _ := env0["TARGET"].(string); target != "java8" {
			t.Errorf("expected steps[0].env.TARGET=java8, got %v", target)
		}
	} else {
		t.Errorf("expected steps[0].env to be present")
	}

	// Verify second mod entry has command array preserved.
	mod1, ok := stepsSlice[1].(map[string]any)
	if !ok {
		t.Fatalf("expected steps[1] to be map[string]any, got %T", stepsSlice[1])
	}
	if img, _ := mod1["image"].(string); img != "docker.io/test/mod-step2:latest" {
		t.Errorf("expected steps[1].image=docker.io/test/mod-step2:latest, got %v", img)
	}
	if cmdArr, ok := mod1["command"].([]any); ok {
		if len(cmdArr) != 2 || cmdArr[0] != "migrate.sh" || cmdArr[1] != "--verbose" {
			t.Errorf("expected steps[1].command=[migrate.sh --verbose], got %v", cmdArr)
		}
	} else {
		t.Errorf("expected steps[1].command to be array, got %T", mod1["command"])
	}

	// Verify third mod entry has shell command preserved.
	mod2, ok := stepsSlice[2].(map[string]any)
	if !ok {
		t.Fatalf("expected steps[2] to be map[string]any, got %T", stepsSlice[2])
	}
	if cmd, _ := mod2["command"].(string); cmd != "finalize.sh" {
		t.Errorf("expected steps[2].command=finalize.sh, got %v", cmd)
	}

	// Verify build_gate and build_gate.healing are preserved (global policy).
	if en, ok := opts["build_gate_enabled"].(bool); !ok || !en {
		t.Errorf("expected build_gate_enabled=true, got %v", opts["build_gate_enabled"])
	}
	bgAny, ok := opts["build_gate"]
	if !ok {
		t.Errorf("expected build_gate to be preserved")
	} else if bg, ok := bgAny.(map[string]any); !ok {
		t.Errorf("expected build_gate to be map[string]any, got %T", bgAny)
	} else if healingAny, ok := bg["healing"]; !ok {
		t.Errorf("expected build_gate.healing to be preserved")
	} else if healing, ok := healingAny.(map[string]any); !ok {
		t.Errorf("expected build_gate.healing to be map[string]any, got %T", healingAny)
	} else {
		switch retries := healing["retries"].(type) {
		case int:
			if retries != 1 {
				t.Errorf("expected build_gate.healing.retries=1, got %v", retries)
			}
		case float64:
			if int(retries) != 1 {
				t.Errorf("expected build_gate.healing.retries=1, got %v", retries)
			}
		default:
			t.Errorf("expected build_gate.healing.retries to be numeric, got %T", healing["retries"])
		}
	}
}

// TestParseRunOptions_HealingSingleMod verifies that parseRunOptions correctly
// parses the canonical single-mod healing schema.
func TestParseRunOptions_HealingSingleMod(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		specJSON    string
		wantRetries int
		wantImage   string
	}{
		{
			name: "single_mod_healing",
			specJSON: `{
				"steps": [{"image": "docker.io/test/mod:latest"}],
				"build_gate": {
					"healing": {
						"retries": 3,
						"mod": {
							"image": "docker.io/test/codex:latest",
							"command": "fix-with-ai"
						}
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
			opts, _, typedOpts := parseSpec(raw)

			// Verify raw opts contains the healing block.
			bgAny, ok := opts["build_gate"]
			if !ok {
				t.Fatal("expected build_gate in raw opts")
			}
			bg, ok := bgAny.(map[string]any)
			if !ok {
				t.Fatalf("expected build_gate to be map[string]any, got %T", bgAny)
			}
			if _, ok := bg["healing"]; !ok {
				t.Fatal("expected build_gate.healing in raw opts")
			}

			// Verify typed options.
			if typedOpts.Healing == nil {
				t.Fatal("expected Healing config to be parsed")
			}

			if typedOpts.Healing.Retries != tc.wantRetries {
				t.Errorf("Retries: got %d, want %d", typedOpts.Healing.Retries, tc.wantRetries)
			}

			if typedOpts.Healing.Mod.Image.Universal != tc.wantImage {
				t.Errorf("Healing mod image: got %q, want %q", typedOpts.Healing.Mod.Image.Universal, tc.wantImage)
			}
		})
	}
}

// TestParseHealingMod_ModFields verifies that healing mod parsing correctly
// extracts mod fields including image, command, env, and retain_container.
func TestParseHealingMod_ModFields(t *testing.T) {
	t.Parallel()

	specJSON := `{
		"steps": [{"image": "docker.io/test/mod:latest"}],
		"build_gate": {
			"healing": {
				"retries": 1,
				"mod": {
					"image": "docker.io/test/healer:v1",
					"command": "heal.sh --fix",
					"env": {
						"MODE": "aggressive",
						"DEBUG": "true"
					},
					"retain_container": true
				}
			}
		}
	}`

	var raw json.RawMessage = []byte(specJSON)
	_, _, typedOpts := parseSpec(raw)

	if typedOpts.Healing == nil {
		t.Fatal("expected healing config to be parsed")
	}

	mod := typedOpts.Healing.Mod

	// Verify image.
	if mod.Image.Universal != "docker.io/test/healer:v1" {
		t.Errorf("Mod image: got %q, want %q", mod.Image.Universal, "docker.io/test/healer:v1")
	}

	// Verify command (shell form).
	if mod.Command.Shell != "heal.sh --fix" {
		t.Errorf("Mod command: got %q, want %q", mod.Command.Shell, "heal.sh --fix")
	}

	// Verify env.
	if mod.Env["MODE"] != "aggressive" {
		t.Errorf("Mod env MODE: got %q, want %q", mod.Env["MODE"], "aggressive")
	}
	if mod.Env["DEBUG"] != "true" {
		t.Errorf("Mod env DEBUG: got %q, want %q", mod.Env["DEBUG"], "true")
	}

	// Verify retain_container.
	if !mod.RetainContainer {
		t.Error("Mod retain_container: got false, want true")
	}
}
