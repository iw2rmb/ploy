//go:build integration
// +build integration

package mods

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestMods_WorkflowWithConsulKV(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	serviceConfig := RequireServices(t, "consul")
	_ = os.Setenv("CONSUL_HTTP_ADDR", serviceConfig.ConsulAddr)
	defer func() { _ = os.Unsetenv("CONSUL_HTTP_ADDR") }()
	workspaceDir := t.TempDir()
	integrations := NewModIntegrationsWithTestMode("http://localhost:8080", workspaceDir, false)
	t.Run("validate_consul_operations", func(t *testing.T) { testConsulOperations(t, serviceConfig.ConsulAddr) })
	cfg := &ModConfig{Version: "v1alpha1", ID: "test-consul-real", TargetRepo: "https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git", TargetBranch: "main", BaseRef: "main", Lane: "C", BuildTimeout: "10m", Steps: []ModStep{{Type: "orw-apply", ID: "java-migration", Recipes: []RecipeEntry{recipeEntry("org.openrewrite.java.migrate.UpgradeToJava17", "org.openrewrite.recipe", "rewrite-migrate-java", "3.17.0")}, MavenPluginVersion: "6.18.0"}}, SelfHeal: &SelfHealConfig{Enabled: true, MaxRetries: 1, Cooldown: "15m"}}
	runner, err := integrations.CreateConfiguredRunner(cfg)
	if err != nil {
		t.Fatalf("failed to create runner with Consul: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	result, err := runner.Run(ctx)
	if err != nil {
		t.Fatalf("workflow with real Consul failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected result but got nil")
	}
	validateConsulUsage(t, result, serviceConfig)
}
