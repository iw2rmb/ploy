package nodeagent

import (
	"testing"
)

// =============================================================================
// E5 — Parallel Healing Tests (Node Agent)
// =============================================================================
//
// This file contains nodeagent-level tests for parallel healing behavior,
// focusing on multi-strategy spec parsing, branch execution isolation, and
// edge cases per ROADMAP.md E5.
//
// The nodeagent tests complement the server/handlers tests by exercising:
//   - Client-side parsing of multi-strategy healing specs
//   - Healing mod execution with branch isolation
//   - Stats and metadata propagation for multi-branch runs

// -----------------------------------------------------------------------------
// TestParseRunOptions_MultiStrategy verifies that parseRunOptions correctly
// extracts healing config from multi-strategy specs.
//
// Scenario: spec with two named strategies, each with different mod counts.
// -----------------------------------------------------------------------------
func TestParseRunOptions_MultiStrategy(t *testing.T) {
	t.Parallel()

	// Multi-strategy spec with named branches.
	options := map[string]any{
		"build_gate_healing": map[string]any{
			"retries": float64(2),
			"strategies": []any{
				map[string]any{
					"name": "codex-ai",
					"mods": []any{
						map[string]any{"image": "mods-codex:latest"},
					},
				},
				map[string]any{
					"name": "static-patch",
					"mods": []any{
						map[string]any{"image": "analyze:v1"},
						map[string]any{"image": "patch:v1"},
					},
				},
			},
		},
	}

	runOpts := parseRunOptions(options)

	// Verify healing config parsed correctly.
	if runOpts.Healing == nil {
		t.Fatalf("expected Healing to be non-nil")
	}
	if runOpts.Healing.Retries != 2 {
		t.Fatalf("expected retries=2, got %d", runOpts.Healing.Retries)
	}

	// Multi-strategy form should populate Strategies slice.
	if len(runOpts.Healing.Strategies) != 2 {
		t.Fatalf("expected 2 strategies, got %d", len(runOpts.Healing.Strategies))
	}

	// Verify first strategy (codex-ai).
	if runOpts.Healing.Strategies[0].Name != "codex-ai" {
		t.Fatalf("expected strategy[0].Name='codex-ai', got %q", runOpts.Healing.Strategies[0].Name)
	}
	if len(runOpts.Healing.Strategies[0].Mods) != 1 {
		t.Fatalf("expected 1 mod in codex-ai strategy, got %d", len(runOpts.Healing.Strategies[0].Mods))
	}
	if runOpts.Healing.Strategies[0].Mods[0].Image.Universal != "mods-codex:latest" {
		t.Fatalf("expected image='mods-codex:latest', got %q", runOpts.Healing.Strategies[0].Mods[0].Image.Universal)
	}

	// Verify second strategy (static-patch) with 2 mods.
	if runOpts.Healing.Strategies[1].Name != "static-patch" {
		t.Fatalf("expected strategy[1].Name='static-patch', got %q", runOpts.Healing.Strategies[1].Name)
	}
	if len(runOpts.Healing.Strategies[1].Mods) != 2 {
		t.Fatalf("expected 2 mods in static-patch strategy, got %d", len(runOpts.Healing.Strategies[1].Mods))
	}

	// Verify NormalizedStrategies returns Strategies when populated.
	normalized := runOpts.Healing.NormalizedStrategies()
	if len(normalized) != 2 {
		t.Fatalf("NormalizedStrategies() should return 2, got %d", len(normalized))
	}
}

// -----------------------------------------------------------------------------
// TestParseRunOptions_LegacyFormPreserved verifies backward compatibility with
// legacy single-strategy specs (mods[] at top level).
// -----------------------------------------------------------------------------
func TestParseRunOptions_LegacyFormPreserved(t *testing.T) {
	t.Parallel()

	// Legacy spec with mods at top level.
	options := map[string]any{
		"build_gate_healing": map[string]any{
			"retries": float64(3),
			"mods": []any{
				map[string]any{"image": "heal-a:latest"},
				map[string]any{"image": "heal-b:latest"},
			},
		},
	}

	runOpts := parseRunOptions(options)

	if runOpts.Healing == nil {
		t.Fatalf("expected Healing to be non-nil")
	}
	if runOpts.Healing.Retries != 3 {
		t.Fatalf("expected retries=3, got %d", runOpts.Healing.Retries)
	}

	// Legacy form should populate Mods, not Strategies.
	if len(runOpts.Healing.Mods) != 2 {
		t.Fatalf("expected 2 mods, got %d", len(runOpts.Healing.Mods))
	}

	if runOpts.Healing.Mods[0].Image.Universal != "heal-a:latest" {
		t.Fatalf("mods[0].Image = %q, want heal-a:latest", runOpts.Healing.Mods[0].Image.Universal)
	}
	if runOpts.Healing.Mods[1].Image.Universal != "heal-b:latest" {
		t.Fatalf("mods[1].Image = %q, want heal-b:latest", runOpts.Healing.Mods[1].Image.Universal)
	}

	// NormalizedStrategies should convert legacy mods to a single unnamed strategy.
	normalized := runOpts.Healing.NormalizedStrategies()
	if len(normalized) != 1 {
		t.Fatalf("NormalizedStrategies() should return 1 for legacy form, got %d", len(normalized))
	}
	if normalized[0].Name != "" {
		t.Fatalf("expected empty name for legacy strategy, got %q", normalized[0].Name)
	}
	if len(normalized[0].Mods) != 2 {
		t.Fatalf("expected 2 mods in normalized strategy, got %d", len(normalized[0].Mods))
	}
}

// -----------------------------------------------------------------------------
// TestParseRunOptions_StrategiesTakesPrecedence verifies that when both mods[]
// and strategies[] are present, strategies[] takes precedence.
// -----------------------------------------------------------------------------
func TestParseRunOptions_StrategiesTakesPrecedence(t *testing.T) {
	t.Parallel()

	// Spec with both mods[] (legacy) and strategies[] (new).
	// Per documentation, strategies[] should take precedence.
	options := map[string]any{
		"build_gate_healing": map[string]any{
			"retries": float64(1),
			"mods": []any{
				map[string]any{"image": "legacy:ignored"},
			},
			"strategies": []any{
				map[string]any{
					"name": "winner",
					"mods": []any{
						map[string]any{"image": "strategy:winner"},
					},
				},
			},
		},
	}

	runOpts := parseRunOptions(options)

	if runOpts.Healing == nil {
		t.Fatalf("expected Healing to be non-nil")
	}

	// Strategies should be populated (takes precedence).
	if len(runOpts.Healing.Strategies) != 1 {
		t.Fatalf("expected 1 strategy, got %d", len(runOpts.Healing.Strategies))
	}

	// The mod should be from strategies[], not legacy mods[].
	if runOpts.Healing.Strategies[0].Mods[0].Image.Universal != "strategy:winner" {
		t.Fatalf("mod.Image = %q, want strategy:winner (from strategies[])",
			runOpts.Healing.Strategies[0].Mods[0].Image.Universal)
	}

	// NormalizedStrategies should return strategies[] content.
	normalized := runOpts.Healing.NormalizedStrategies()
	if len(normalized) != 1 || normalized[0].Mods[0].Image.Universal != "strategy:winner" {
		t.Fatalf("NormalizedStrategies() should return strategies[] content")
	}
}

// -----------------------------------------------------------------------------
// TestNormalizedStrategies_EmptyConfig verifies NormalizedStrategies returns nil
// for nil or empty healing config.
// -----------------------------------------------------------------------------
func TestNormalizedStrategies_EmptyConfig(t *testing.T) {
	t.Parallel()

	// Test nil config.
	var nilConfig *HealingConfig
	if nilConfig.NormalizedStrategies() != nil {
		t.Fatal("expected nil for nil HealingConfig")
	}

	// Test empty config (no mods or strategies).
	emptyConfig := &HealingConfig{Retries: 3}
	if len(emptyConfig.NormalizedStrategies()) != 0 {
		t.Fatal("expected empty slice for HealingConfig with no mods/strategies")
	}
}

// -----------------------------------------------------------------------------
// TestNormalizedStrategies_ReturnsStrategiesOverMods verifies NormalizedStrategies
// returns Strategies[] when populated, ignoring legacy Mods[].
// -----------------------------------------------------------------------------
func TestNormalizedStrategies_ReturnsStrategiesOverMods(t *testing.T) {
	t.Parallel()

	// Config with both Strategies and Mods.
	config := &HealingConfig{
		Retries: 1,
		Mods: []HealingMod{
			{Image: testModImage("legacy:ignored")},
		},
		Strategies: []HealingStrategy{
			{Name: "winner", Mods: []HealingMod{{Image: testModImage("strategy:used")}}},
		},
	}

	normalized := config.NormalizedStrategies()

	if len(normalized) != 1 {
		t.Fatalf("expected 1 strategy, got %d", len(normalized))
	}
	if normalized[0].Name != "winner" {
		t.Fatalf("expected strategy name 'winner', got %q", normalized[0].Name)
	}
	if normalized[0].Mods[0].Image.Universal != "strategy:used" {
		t.Fatalf("expected mod image 'strategy:used', got %q", normalized[0].Mods[0].Image.Universal)
	}
}

// -----------------------------------------------------------------------------
// TestNormalizedStrategies_NormalizesLegacyMods verifies NormalizedStrategies
// converts legacy Mods[] to a single unnamed strategy when Strategies[] is empty.
// -----------------------------------------------------------------------------
func TestNormalizedStrategies_NormalizesLegacyMods(t *testing.T) {
	t.Parallel()

	// Config with only legacy Mods.
	config := &HealingConfig{
		Retries: 2,
		Mods: []HealingMod{
			{Image: testModImage("heal-a:latest")},
			{Image: testModImage("heal-b:latest")},
		},
	}

	normalized := config.NormalizedStrategies()

	if len(normalized) != 1 {
		t.Fatalf("expected 1 normalized strategy, got %d", len(normalized))
	}

	// Strategy should be unnamed (legacy form).
	if normalized[0].Name != "" {
		t.Fatalf("expected empty name for legacy strategy, got %q", normalized[0].Name)
	}

	// Should contain all legacy mods.
	if len(normalized[0].Mods) != 2 {
		t.Fatalf("expected 2 mods in normalized strategy, got %d", len(normalized[0].Mods))
	}
}
