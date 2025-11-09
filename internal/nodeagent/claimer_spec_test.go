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
	opts, _ := parseSpec(raw)

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
	opts, env := parseSpec(raw)
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
