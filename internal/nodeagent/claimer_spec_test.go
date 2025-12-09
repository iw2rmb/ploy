package nodeagent

import (
	"encoding/json"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
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

// TestParseSpec_CanonicalSingleStepFormat verifies that parseSpec correctly extracts
// top-level fields for single-step runs. This is the canonical shape for single-step
// specs (replacing the legacy "mod" object format).
func TestParseSpec_CanonicalSingleStepFormat(t *testing.T) {
	// Canonical single-step format: top-level image, command, env, retain_container.
	specJSON := `{
        "image": "docker.io/test/mod:latest",
        "retain_container": true,
        "env": {"A":"1","B":"2"},
        "command": ["/bin/sh","-c","echo hi"],
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
// canonical shapes (top-level fields for single-step, mods[] for multi-step).
func TestParseSpec_LegacyModObjectIgnored(t *testing.T) {
	// Legacy format: nested "mod" object (no longer supported).
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

	// Verify that "mod" object fields are NOT extracted to top-level.
	// The legacy fallback has been removed; specs must use canonical shapes.
	if _, hasImage := opts["image"]; hasImage {
		t.Fatalf("expected image not to be extracted from legacy mod object")
	}
	if _, hasRetain := opts["retain_container"]; hasRetain {
		t.Fatalf("expected retain_container not to be extracted from legacy mod object")
	}
	if len(env) > 0 {
		t.Fatalf("expected env to be empty (mod.env should not be merged), got: %+v", env)
	}

	// build_gate should still be flattened (it's a top-level field, not part of mod).
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

// TestParseRunOptions_MultiStrategyHealing verifies that parseRunOptions correctly
// parses the canonical multi-strategy healing schema and that strategies[] takes
// precedence when legacy mods[] and strategies[] are both present.
func TestParseRunOptions_MultiStrategyHealing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		specJSON         string
		wantRetries      int
		wantStrategies   int
		wantStratNames   []string
		wantModsPerStrat []int
	}{
		{
			name: "multi_strategy_form_two_branches",
			specJSON: `{
				"build_gate_healing": {
					"retries": 3,
					"strategies": [
						{
							"name": "codex-ai",
							"mods": [
								{"image": "docker.io/test/codex:latest", "command": "fix-with-ai"}
							]
						},
						{
							"name": "static-patch",
							"mods": [
								{"image": "docker.io/test/patcher:latest"},
								{"image": "docker.io/test/validator:latest"}
							]
						}
					]
				}
			}`,
			wantRetries:      3,
			wantStrategies:   2,
			wantStratNames:   []string{"codex-ai", "static-patch"},
			wantModsPerStrat: []int{1, 2},
		},
		{
			name: "strategies_takes_precedence_over_mods",
			specJSON: `{
				"build_gate_healing": {
					"retries": 1,
					"mods": [
						{"image": "docker.io/test/legacy:latest"}
					],
					"strategies": [
						{
							"name": "preferred",
							"mods": [{"image": "docker.io/test/new:latest"}]
						}
					]
				}
			}`,
			wantRetries:      1,
			wantStrategies:   1, // Strategies populated.
			wantStratNames:   []string{"preferred"},
			wantModsPerStrat: []int{1},
		},
		{
			name: "empty_strategy_name_allowed",
			specJSON: `{
				"build_gate_healing": {
					"retries": 1,
					"strategies": [
						{
							"mods": [{"image": "docker.io/test/unnamed:latest"}]
						}
					]
				}
			}`,
			wantRetries:      1,
			wantStrategies:   1,
			wantStratNames:   []string{""}, // Empty name allowed.
			wantModsPerStrat: []int{1},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var raw json.RawMessage = []byte(tc.specJSON)
			opts, _, typedOpts := parseSpec(raw)

			// Verify raw opts contains the healing block.
			if _, ok := opts["build_gate_healing"]; !ok {
				t.Fatal("expected build_gate_healing in raw opts")
			}

			// Verify typed options.
			if typedOpts.Healing == nil {
				t.Fatal("expected Healing config to be parsed")
			}

			if typedOpts.Healing.Retries != tc.wantRetries {
				t.Errorf("Retries: got %d, want %d", typedOpts.Healing.Retries, tc.wantRetries)
			}

			if len(typedOpts.Healing.Strategies) != tc.wantStrategies {
				t.Errorf("Strategies count: got %d, want %d", len(typedOpts.Healing.Strategies), tc.wantStrategies)
			}

			// Verify strategy names.
			for i, wantName := range tc.wantStratNames {
				if i >= len(typedOpts.Healing.Strategies) {
					t.Errorf("missing strategy at index %d", i)
					continue
				}
				if typedOpts.Healing.Strategies[i].Name != wantName {
					t.Errorf("Strategy[%d].Name: got %q, want %q", i, typedOpts.Healing.Strategies[i].Name, wantName)
				}
			}

			// Verify mods per strategy.
			for i, wantMods := range tc.wantModsPerStrat {
				if i >= len(typedOpts.Healing.Strategies) {
					continue
				}
				if len(typedOpts.Healing.Strategies[i].Mods) != wantMods {
					t.Errorf("Strategy[%d].Mods count: got %d, want %d", i, len(typedOpts.Healing.Strategies[i].Mods), wantMods)
				}
			}
		})
	}
}

// TestHealingConfig_NormalizedStrategies verifies that NormalizedStrategies()
// returns configured strategies when present and nil/empty otherwise.
func TestHealingConfig_NormalizedStrategies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		config         *HealingConfig
		wantStrategies int
		wantFirstName  string
		wantFirstMods  int
	}{
		{
			name:           "nil_config_returns_nil",
			config:         nil,
			wantStrategies: 0,
		},
		{
			name: "multi_strategy_returned_directly",
			config: &HealingConfig{
				Retries: 1,
				Strategies: []HealingStrategy{
					{Name: "alpha", Mods: []HealingMod{{Image: contracts.ModImage{Universal: "a:latest"}}}},
					{Name: "beta", Mods: []HealingMod{{Image: contracts.ModImage{Universal: "b:latest"}}}},
				},
			},
			wantStrategies: 2,
			wantFirstName:  "alpha",
			wantFirstMods:  1,
		},
		{
			name: "empty_config_returns_nil",
			config: &HealingConfig{
				Retries: 1,
				// No Mods, no Strategies.
			},
			wantStrategies: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			strategies := tc.config.NormalizedStrategies()

			if len(strategies) != tc.wantStrategies {
				t.Fatalf("NormalizedStrategies count: got %d, want %d", len(strategies), tc.wantStrategies)
			}

			if tc.wantStrategies == 0 {
				return
			}

			if strategies[0].Name != tc.wantFirstName {
				t.Errorf("First strategy name: got %q, want %q", strategies[0].Name, tc.wantFirstName)
			}

			if len(strategies[0].Mods) != tc.wantFirstMods {
				t.Errorf("First strategy mods count: got %d, want %d", len(strategies[0].Mods), tc.wantFirstMods)
			}
		})
	}
}

// TestParseHealingStrategy_ModFields verifies that parseHealingStrategy correctly
// extracts mod fields including image, command, env, and retain_container.
func TestParseHealingStrategy_ModFields(t *testing.T) {
	t.Parallel()

	specJSON := `{
		"build_gate_healing": {
			"retries": 1,
			"strategies": [
				{
					"name": "test-strategy",
					"mods": [
						{
							"image": "docker.io/test/healer:v1",
							"command": "heal.sh --fix",
							"env": {
								"MODE": "aggressive",
								"DEBUG": "true"
							},
							"retain_container": true
						}
					]
				}
			]
		}
	}`

	var raw json.RawMessage = []byte(specJSON)
	_, _, typedOpts := parseSpec(raw)

	if typedOpts.Healing == nil || len(typedOpts.Healing.Strategies) == 0 {
		t.Fatal("expected healing strategies to be parsed")
	}

	strategy := typedOpts.Healing.Strategies[0]
	if strategy.Name != "test-strategy" {
		t.Errorf("Strategy name: got %q, want %q", strategy.Name, "test-strategy")
	}

	if len(strategy.Mods) != 1 {
		t.Fatalf("Strategy mods count: got %d, want 1", len(strategy.Mods))
	}

	mod := strategy.Mods[0]

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
