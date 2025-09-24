//go:build integration
// +build integration

package mods

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestMods_WorkflowWithAllRealServices(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	serviceConfig := RequireServices(t, "consul", "nomad", "seaweedfs", "gitlab")
	_ = os.Setenv("MODS_SUBMIT", "1")
	_ = os.Setenv("NOMAD_ADDR", serviceConfig.NomadAddr)
	_ = os.Setenv("CONSUL_HTTP_ADDR", serviceConfig.ConsulAddr)
	_ = os.Setenv("GITLAB_URL", serviceConfig.GitLabURL)
	_ = os.Setenv("GITLAB_TOKEN", serviceConfig.GitLabToken)
	defer func() {
		_ = os.Unsetenv("MODS_SUBMIT")
		_ = os.Unsetenv("NOMAD_ADDR")
		_ = os.Unsetenv("CONSUL_HTTP_ADDR")
		_ = os.Unsetenv("GITLAB_URL")
		_ = os.Unsetenv("GITLAB_TOKEN")
	}()
	workspaceDir := t.TempDir()
	integrations := NewModIntegrationsFromEnv(workspaceDir, false)
	t.Run("validate_all_service_operations", func(t *testing.T) {
		testSeaweedFSOperations(t, serviceConfig.SeaweedFSFiler, serviceConfig.SeaweedFSMaster)
		testNomadOperations(t, serviceConfig.NomadAddr)
		testConsulOperations(t, serviceConfig.ConsulAddr)
		testGitLabOperations(t, serviceConfig.GitLabURL, serviceConfig.GitLabToken)
	})
	cfg := &ModConfig{Version: "v1alpha1", ID: "test-all-services-real", TargetRepo: "https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git", TargetBranch: "main", BaseRef: "main", Lane: "C", BuildTimeout: "15m", Steps: []ModStep{{Type: "orw-apply", ID: "java-migration", Recipes: []RecipeEntry{recipeEntry("org.openrewrite.java.migrate.UpgradeToJava17", "org.openrewrite.recipe", "rewrite-migrate-java", "3.17.0")}, MavenPluginVersion: "6.18.0"}}, SelfHeal: &SelfHealConfig{Enabled: true, MaxRetries: 3, Cooldown: "10m"}}
	runner, err := integrations.CreateConfiguredRunner(cfg)
	if err != nil {
		t.Fatalf("failed to create runner with all services: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	result, err := runner.Run(ctx)
	if err != nil {
		t.Fatalf("workflow with all real services failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected result but got nil")
	}
	validateAllServicesUsage(t, result, serviceConfig)
}
