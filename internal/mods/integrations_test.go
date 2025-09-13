package mods

import (
	"testing"
)

func TestCreateConfiguredRunner_WiresDefaultModules(t *testing.T) {
	cfg := &TransflowConfig{
		Version:      "v1alpha1",
		ID:           "unit-modules",
		TargetRepo:   "https://example.com/repo.git",
		TargetBranch: "main",
		BaseRef:      "main",
		Lane:         "C",
		Steps: []TransflowStep{{
			Type:    "orw-apply",
			ID:      "s1",
			Engine:  "openrewrite",
			Recipes: []string{"org.openrewrite.java.migrate.UpgradeToJava17"},
		}},
		SelfHeal: GetDefaultSelfHealConfig(),
	}
	integ := NewTransflowIntegrationsWithTestMode("http://controller/v1", t.TempDir(), false)
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
