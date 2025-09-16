//go:build integration
// +build integration

package mods

import (
	"context"
	"testing"
	"time"
)

func TestMods_WorkflowWithSeaweedFS(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	serviceConfig := RequireServices(t, "seaweedfs")
	workspaceDir := t.TempDir()
	integrations := NewModIntegrationsWithTestMode("http://localhost:8080", workspaceDir, false)
	t.Run("validate_seaweedfs_operations", func(t *testing.T) { testSeaweedFSOperations(t, serviceConfig.SeaweedFSFiler) })
	cfg := &ModConfig{Version: "v1alpha1", ID: "test-seaweedfs", TargetRepo: "https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git", TargetBranch: "main", BaseRef: "main", Lane: "C", BuildTimeout: "10m", Steps: []ModStep{{Type: "orw-apply", ID: "java-migration", Recipes: []string{"org.openrewrite.java.migrate.UpgradeToJava17"}, RecipeGroup: "org.openrewrite.recipe", RecipeArtifact: "rewrite-migrate-java", RecipeVersion: "3.17.0", MavenPluginVersion: "6.18.0"}}, SelfHeal: &SelfHealConfig{Enabled: true, MaxRetries: 1, Cooldown: "15m"}}
	runner, err := integrations.CreateConfiguredRunner(cfg)
	if err != nil {
		t.Fatalf("failed to create runner with services: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	result, err := runner.Run(ctx)
	if err != nil {
		t.Fatalf("workflow with SeaweedFS failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected result but got nil")
	}
	validateServiceUsage(t, result, serviceConfig)
}
