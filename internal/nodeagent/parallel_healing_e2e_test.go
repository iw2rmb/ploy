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

// Single-strategy healing must use build_gate_healing.strategies with one entry;
// legacy mods[]-only healing specs are no longer supported.

// -----------------------------------------------------------------------------
// TestParseRunOptions_StrategiesTakesPrecedence verifies that when both mods[]
// and strategies[] are present, strategies[] takes precedence.
// -----------------------------------------------------------------------------
func TestParseRunOptions_StrategiesTakesPrecedence(t *testing.T) {
	t.Parallel()

	// Spec with both legacy mods[] and canonical strategies[].
	// Per documentation, strategies[] should take precedence and legacy mods[] is ignored.
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

	// The mod should be from strategies[], not the top-level mods[] list.
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

// NormalizedStrategies now returns only the configured Strategies slice.
