//go:build integration
// +build integration

package mods

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestMods_WorkflowWithGitLabAPI(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	serviceConfig := RequireServices(t, "gitlab")
	_ = os.Setenv("GITLAB_URL", serviceConfig.GitLabURL)
	_ = os.Setenv("GITLAB_TOKEN", serviceConfig.GitLabToken)
	defer func() { _ = os.Unsetenv("GITLAB_URL"); _ = os.Unsetenv("GITLAB_TOKEN") }()
	workspaceDir := t.TempDir()
	integrations := NewModIntegrationsFromEnv(workspaceDir, false)
	t.Run("validate_gitlab_operations", func(t *testing.T) { testGitLabOperations(t, serviceConfig.GitLabURL, serviceConfig.GitLabToken) })
	cfg := &ModConfig{Version: "v1alpha1", ID: "test-gitlab-real", TargetRepo: "https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git", TargetBranch: "main", BaseRef: "main", Lane: "C", BuildTimeout: "10m", Steps: []ModStep{{Type: "orw-apply", ID: "java-migration", Recipes: []RecipeEntry{recipeEntry("org.openrewrite.java.migrate.UpgradeToJava17", "org.openrewrite.recipe", "rewrite-migrate-java", "3.17.0")}, MavenPluginVersion: "6.18.0"}}, SelfHeal: &SelfHealConfig{Enabled: true, MaxRetries: 1, Cooldown: "15m"}}
	runner, err := integrations.CreateConfiguredRunner(cfg)
	if err != nil {
		t.Fatalf("failed to create runner with GitLab: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Minute)
	defer cancel()
	result, err := runner.Run(ctx)
	if err != nil {
		t.Fatalf("workflow with real GitLab failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected result but got nil")
	}
	validateGitLabUsage(t, result, serviceConfig)
}
