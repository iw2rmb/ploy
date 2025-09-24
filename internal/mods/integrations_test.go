package mods

import (
	"testing"
)

func TestCreateConfiguredRunner_WiresDefaultModules(t *testing.T) {
	cfg := &ModConfig{
		Version:      "v1alpha1",
		ID:           "unit-modules",
		TargetRepo:   "https://example.com/repo.git",
		TargetBranch: "main",
		BaseRef:      "main",
		Lane:         "C",
		Steps: []ModStep{{
			Type:    "orw-apply",
			ID:      "s1",
			Engine:  "openrewrite",
			Recipes: []RecipeEntry{recipeEntry("org.openrewrite.java.migrate.UpgradeToJava17", "org.openrewrite.recipe", "rewrite-migrate-java", "3.17.0")},
		}},
		SelfHeal: GetDefaultSelfHealConfig(),
	}
	integ := NewModIntegrationsFromEnv(t.TempDir(), false)
	r, err := integ.CreateConfiguredRunner(cfg)
	if err != nil {
		t.Fatalf("CreateConfiguredRunner: %v", err)
	}
	if r.repoManager == nil {
		t.Fatalf("repoManager not wired")
	}
	if r.mrManager == nil {
		t.Fatalf("mrManager not wired")
	}
	if r.buildGate == nil {
		t.Fatalf("buildGate not wired")
	}
	if r.transformExec == nil {
		t.Fatalf("transformExec not wired")
	}
	if r.healer == nil {
		t.Fatalf("healer not wired")
	}
}
