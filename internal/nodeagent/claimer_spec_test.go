package nodeagent

import (
	"encoding/json"
	"testing"
)

// TestParseSpec_PassesThroughBuildGateHealing verifies that the node agent
// carries the build_gate_healing block from the spec into Options so that
// executeWithHealing can honor the configured heal → re-gate loop.
func TestParseSpec_PassesThroughBuildGateHealing(t *testing.T) {
	specJSON := `{
        "build_gate_healing": {
            "retries": 2,
            "mods": [ { "image": "docker.io/test/heal:latest" } ]
        }
    }`

	var raw json.RawMessage = []byte(specJSON)
	opts, _, _ := parseSpec(raw)

	v, ok := opts["build_gate_healing"]
	if !ok {
		t.Fatalf("expected build_gate_healing present in options")
	}

	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("expected build_gate_healing to be map[string]any, got %T", v)
	}

	// Check retries
	r, ok := m["retries"].(float64)
	if !ok || int(r) != 2 {
		t.Fatalf("expected retries=2, got %v (%T)", m["retries"], m["retries"])
	}

	mods, ok := m["mods"].([]any)
	if !ok || len(mods) != 1 {
		t.Fatalf("expected mods array with 1 element, got %v", m["mods"])
	}
	mod0, ok := mods[0].(map[string]any)
	if !ok {
		t.Fatalf("expected mods[0] to be map, got %T", mods[0])
	}
	if img, _ := mod0["image"].(string); img != "docker.io/test/heal:latest" {
		t.Fatalf("expected heal image docker.io/test/heal:latest, got %v", img)
	}
}

func TestParseSpec_FlattensModAndBuildGate(t *testing.T) {
	specJSON := `{
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
	if opts["image"] != "docker.io/test/mod:latest" {
		t.Fatalf("image not flattened: %v", opts["image"])
	}
	if rc, _ := opts["retain_container"].(bool); !rc {
		t.Fatalf("retain_container not flattened")
	}
	if env["A"] != "1" || env["B"] != "2" {
		t.Fatalf("env not flattened: %+v", env)
	}
	if en, ok := opts["build_gate_enabled"].(bool); !ok || en != false {
		t.Fatalf("build_gate_enabled not flattened: %v", opts["build_gate_enabled"])
	}
	if pr, ok := opts["build_gate_profile"].(string); !ok || pr != "java-maven" {
		t.Fatalf("build_gate_profile not flattened: %v", opts["build_gate_profile"])
	}
}

// TestParseSpec_PreservesModsArray verifies that parseSpec preserves the mods[]
// array for multi-step runs without modification. The mods[] array represents
// sequential transformation steps that share global gate/healing policy.
func TestParseSpec_PreservesModsArray(t *testing.T) {
	t.Parallel()

	// Spec with multi-step mods[] array (3 steps with different images and env).
	specJSON := `{
		"mods": [
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
		"build_gate": {"enabled": true, "profile": "auto"},
		"build_gate_healing": {
			"retries": 1,
			"mods": [{"image": "docker.io/test/healer:latest"}]
		}
	}`

	var raw json.RawMessage = []byte(specJSON)
	opts, _, _ := parseSpec(raw)

	// Verify mods[] array is present in opts.
	modsRaw, ok := opts["mods"]
	if !ok {
		t.Fatalf("expected mods array in options, got none")
	}

	modsSlice, ok := modsRaw.([]any)
	if !ok {
		t.Fatalf("expected mods to be []any, got %T", modsRaw)
	}

	if len(modsSlice) != 3 {
		t.Fatalf("expected 3 mods in array, got %d", len(modsSlice))
	}

	// Verify first mod entry is preserved correctly.
	mod0, ok := modsSlice[0].(map[string]any)
	if !ok {
		t.Fatalf("expected mods[0] to be map[string]any, got %T", modsSlice[0])
	}
	if img, _ := mod0["image"].(string); img != "docker.io/test/mod-step1:latest" {
		t.Errorf("expected mods[0].image=docker.io/test/mod-step1:latest, got %v", img)
	}
	if env0, ok := mod0["env"].(map[string]any); ok {
		if step, _ := env0["STEP"].(string); step != "1" {
			t.Errorf("expected mods[0].env.STEP=1, got %v", step)
		}
		if target, _ := env0["TARGET"].(string); target != "java8" {
			t.Errorf("expected mods[0].env.TARGET=java8, got %v", target)
		}
	} else {
		t.Errorf("expected mods[0].env to be present")
	}

	// Verify second mod entry has command array preserved.
	mod1, ok := modsSlice[1].(map[string]any)
	if !ok {
		t.Fatalf("expected mods[1] to be map[string]any, got %T", modsSlice[1])
	}
	if img, _ := mod1["image"].(string); img != "docker.io/test/mod-step2:latest" {
		t.Errorf("expected mods[1].image=docker.io/test/mod-step2:latest, got %v", img)
	}
	if cmdArr, ok := mod1["command"].([]any); ok {
		if len(cmdArr) != 2 || cmdArr[0] != "migrate.sh" || cmdArr[1] != "--verbose" {
			t.Errorf("expected mods[1].command=[migrate.sh --verbose], got %v", cmdArr)
		}
	} else {
		t.Errorf("expected mods[1].command to be array, got %T", mod1["command"])
	}

	// Verify third mod entry has shell command preserved.
	mod2, ok := modsSlice[2].(map[string]any)
	if !ok {
		t.Fatalf("expected mods[2] to be map[string]any, got %T", modsSlice[2])
	}
	if cmd, _ := mod2["command"].(string); cmd != "finalize.sh" {
		t.Errorf("expected mods[2].command=finalize.sh, got %v", cmd)
	}

	// Verify build_gate and build_gate_healing are preserved (global policy).
	if en, ok := opts["build_gate_enabled"].(bool); !ok || !en {
		t.Errorf("expected build_gate_enabled=true, got %v", opts["build_gate_enabled"])
	}
	if healing, ok := opts["build_gate_healing"].(map[string]any); !ok {
		t.Errorf("expected build_gate_healing to be preserved")
	} else if retries, _ := healing["retries"].(float64); int(retries) != 1 {
		t.Errorf("expected build_gate_healing.retries=1, got %v", retries)
	}
}
