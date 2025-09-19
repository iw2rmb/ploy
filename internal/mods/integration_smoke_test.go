package mods

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/common"
)

func TestMods_SuccessfulWorkflowWithMocks(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	workspaceDir := t.TempDir()
	cfg := &ModConfig{
		Version:      "v1alpha1",
		ID:           "test-integration-success",
		TargetRepo:   "https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git",
		TargetBranch: "main",
		BaseRef:      "main",
		Lane:         "C",
		BuildTimeout: "10m",
		Steps:        []ModStep{{Type: "orw-apply", ID: "java-migration", Recipes: []string{"org.openrewrite.java.migrate.UpgradeToJava17"}, RecipeGroup: "org.openrewrite.recipe", RecipeArtifact: "rewrite-migrate-java", RecipeVersion: "3.17.0", MavenPluginVersion: "6.18.0"}},
		SelfHeal:     GetDefaultSelfHealConfig(),
	}
	integrations := NewModIntegrationsWithTestMode("http://localhost:8080", workspaceDir, true)
	runner, err := integrations.CreateConfiguredRunner(cfg)
	if err != nil {
		t.Fatalf("failed to create runner: %v", err)
	}
	oldSubmit := submitAndWaitTerminal
	submitAndWaitTerminal = func(string, time.Duration) error { return nil }
	oldPutFile := putFileFn
	oldPutJSON := putJSONFn
	putFileFn = func(string, string, string, string) error { return nil }
	putJSONFn = func(string, string, []byte) error { return nil }
	defer func() {
		submitAndWaitTerminal = oldSubmit
		putFileFn = oldPutFile
		putJSONFn = oldPutJSON
	}()
	oldValidate := validateJob
	validateJob = func(string) error { return nil }
	mockSubmitter := &MockJobSubmitter{JobResults: map[string]JobResult{}}
	runner.SetJobSubmitter(mockSubmitter)
	runner.SetHealingOrchestrator(NewProdHealingOrchestrator(runner.jobSubmitter, runner))
	_ = os.Setenv("GITLAB_TOKEN", "test-token-for-integration")
	defer func() {
		_ = os.Unsetenv("GITLAB_TOKEN")
		validateJob = oldValidate
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	result, err := runner.Run(ctx)
	if err != nil {
		t.Fatalf("workflow failed unexpectedly: %v", err)
	}
	if result == nil {
		t.Fatal("expected result but got nil")
	}
	if result.BranchName == "" {
		t.Error("expected branch name but got empty string")
	}
	if result.BuildVersion == "" {
		t.Error("expected build version but got empty string")
	}
}

func TestMods_WorkflowWithBuildFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	workspaceDir := t.TempDir()
	cfg := &ModConfig{
		Version:      "v1alpha1",
		ID:           "test-integration-fail",
		TargetRepo:   "https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git",
		TargetBranch: "main",
		BaseRef:      "main",
		Lane:         "C",
		BuildTimeout: "10m",
		Steps:        []ModStep{{Type: "orw-apply", ID: "java-migration", Recipes: []string{"org.openrewrite.java.migrate.UpgradeToJava17"}, RecipeGroup: "org.openrewrite.recipe", RecipeArtifact: "rewrite-migrate-java", RecipeVersion: "3.17.0", MavenPluginVersion: "6.18.0"}},
		SelfHeal:     GetDefaultSelfHealConfig(),
	}
	integrations := NewModIntegrationsWithTestMode("http://localhost:8080", workspaceDir, true)
	failing := &ModIntegrations{ControllerURL: integrations.ControllerURL, WorkDir: integrations.WorkDir, TestMode: true}
	runner, err := failing.CreateConfiguredRunner(cfg)
	if err != nil {
		t.Fatalf("failed to create runner: %v", err)
	}
	runner.SetBuildChecker(NewTestModeBuildChecker(true))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	result, err := runner.Run(ctx)
	if err == nil {
		t.Error("expected error but workflow succeeded")
	}
	if result != nil {
		_ = result // ensure compiled; behavior validated by error check
	}
}

func TestMods_TestModeIntegrationsMocks(t *testing.T) {
	work := t.TempDir()
	integrations := NewModIntegrationsWithTestMode("http://localhost:8080", work, true)
	if bc := integrations.CreateBuildChecker(); bc == nil {
		t.Error("expected build checker but got nil")
	} else {
		ctx := context.Background()
		res, err := bc.CheckBuild(ctx, common.DeployConfig{App: "test-app", Lane: "C", Environment: "dev"})
		if err != nil {
			t.Errorf("mock build checker should not fail: %v", err)
		}
		if res == nil || !res.Success {
			t.Error("mock build checker should return successful result")
		}
	}
	if gp := integrations.CreateGitProvider(); gp == nil {
		t.Error("expected git provider but got nil")
	} else if err := gp.ValidateConfiguration(); err != nil {
		t.Errorf("mock git provider validation should succeed: %v", err)
	}
}
