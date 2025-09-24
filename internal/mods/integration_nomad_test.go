//go:build integration
// +build integration

package mods

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestMods_WorkflowWithNomadJobs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	serviceConfig := RequireServices(t, "nomad")
	workspaceDir := t.TempDir()
	_ = os.Setenv("MODS_SUBMIT", "1")
	_ = os.Setenv("NOMAD_ADDR", serviceConfig.NomadAddr)
	defer func() { _ = os.Unsetenv("MODS_SUBMIT"); _ = os.Unsetenv("NOMAD_ADDR") }()
	integrations := NewModIntegrationsFromEnv(workspaceDir, false)
	t.Run("validate_nomad_operations", func(t *testing.T) { testNomadOperations(t, serviceConfig.NomadAddr) })
	cfg := &ModConfig{Version: "v1alpha1", ID: "test-nomad-real", TargetRepo: "https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git", TargetBranch: "main", BaseRef: "main", Lane: "C", BuildTimeout: "10m", Steps: []ModStep{{Type: "orw-apply", ID: "java-migration", Recipes: []RecipeEntry{recipeEntry("org.openrewrite.java.migrate.UpgradeToJava17", "org.openrewrite.recipe", "rewrite-migrate-java", "3.17.0")}, MavenPluginVersion: "6.18.0"}}, SelfHeal: &SelfHealConfig{Enabled: true, MaxRetries: 2, Cooldown: "5m"}}
	runner, err := integrations.CreateConfiguredRunner(cfg)
	if err != nil {
		t.Fatalf("failed to create runner with Nomad: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	result, err := runner.Run(ctx)
	if err != nil {
		t.Fatalf("workflow with real Nomad failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected result but got nil")
	}
	validateNomadUsage(t, result, serviceConfig)
}
