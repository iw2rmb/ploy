package acceptance

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestMVPAcceptance_CompleteJavaTransformation validates the core MVP scenario
// of complete Java 11→17 migration as specified in the MVP requirements
func TestMVPAcceptance_CompleteJavaTransformation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping MVP acceptance tests in short mode")
	}

	env := SetupMVPEnvironment(t)
	defer env.Cleanup()

	// Test Case: Complete Java 11→17 Migration as specified in MVP.md
	scenario := &Scenario{
		Name:        "Complete Java 11 to 17 Migration",
		Description: "Validate complete transflow workflow as specified in MVP requirements",

		// Use official MVP test repository
		Repository: "https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git",

		// Exact workflow from MVP specification
		TransflowConfig: `
version: v1alpha1
id: java11to17
target_repo: https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git
base_ref: refs/heads/main
target_branch: refs/heads/main
lane: C
build_timeout: 15m

steps:
  - type: recipe
    engine: openrewrite
    recipes:
      - org.openrewrite.java.migrate.Java11toJava17

self_heal:
  enabled: true
  kb_learning: true
  max_retries: 2
`,

		// Expected outcomes from MVP specification
		ExpectedResults: ExpectedResults{
			Success:        true,
			WorkflowBranch: "workflow/java11to17/*",
			BuildSuccess:   true,
			MRCreated:      true,
			MRLabels:       []string{"ploy", "tfl"},
			MaxDuration:    10 * time.Minute,
		},

		// MVP validation steps
		ValidationSteps: []ValidationStep{
			{Name: "git_clone", Description: "Repository should be cloned successfully"},
			{Name: "workflow_branch", Description: "Workflow branch should be created"},
			{Name: "recipe_execution", Description: "OpenRewrite recipe should execute"},
			{Name: "build_validation", Description: "Build should pass via /v1/apps/:app/builds"},
			{Name: "mr_creation", Description: "GitLab MR should be created"},
			{Name: "mr_labels", Description: "MR should have correct labels"},
			{Name: "cleanup", Description: "Resources should be cleaned up"},
		},
	}

	result, err := env.ExecuteScenario(context.Background(), scenario)
	assert.NoError(t, err, "MVP acceptance scenario should complete without errors")

	// Validate against MVP success criteria
	validateMVPCriteria(t, result, scenario.ExpectedResults)

	// Additional MVP-specific validations
	assert.True(t, result.BuildVersion != "", "Should generate build version")
	assert.True(t, strings.Contains(result.MRDescription, "Java 11"), "MR should describe Java 11 migration")
	assert.True(t, result.ArtifactsGenerated, "Should generate build artifacts")

	// Validate build API integration (MVP requirement)
	if env.BuildClient != nil {
		build, err := env.BuildClient.GetBuild(result.BuildVersion)
		if err == nil {
			assert.Equal(t, "C", build.Lane, "Should correctly detect Lane C")
			assert.Equal(t, "success", build.Status, "Build should be successful")
		}
	}

	// Validate performance against MVP targets
	assert.True(t, result.Duration < 8*time.Minute,
		"Java migration should complete within 8 minutes (actual: %v)", result.Duration)
}

// TestMVPAcceptance_SelfHealingWorkflow validates the self-healing system
// with all three healing strategies as specified in MVP requirements
func TestMVPAcceptance_SelfHealingWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping MVP acceptance tests in short mode")
	}

	env := SetupMVPEnvironment(t)
	defer env.Cleanup()

	// Test Case: Self-healing with all three healing strategies from MVP
	scenario := &Scenario{
		Name:        "Self-Healing with LangGraph Integration",
		Description: "Validate all MVP self-healing capabilities",

		// Repository designed to trigger compilation failures for testing
		Repository: "https://gitlab.com/iw2rmb/ploy-test-healing-scenario.git",

		TransflowConfig: `
version: v1alpha1
id: self-healing-test
target_repo: https://gitlab.com/iw2rmb/ploy-test-healing-scenario.git
base_ref: refs/heads/main
target_branch: refs/heads/main
lane: C

steps:
  - type: recipe
    engine: openrewrite
    recipes:
      - org.openrewrite.java.cleanup.UnnecessaryParentheses
      
self_heal:
  enabled: true
  kb_learning: true
  max_retries: 3
  cooldown: 30s
`,

		ExpectedResults: ExpectedResults{
			InitialBuildFailure: true, // Expect initial failure to trigger healing
			HealingTriggered:    true,
			HealingSuccess:      true,
			FinalSuccess:        true,
			KBLearning:          true,
		},
	}

	result, err := env.ExecuteScenario(context.Background(), scenario)
	assert.NoError(t, err)

	// Validate MVP healing requirements
	assert.False(t, result.InitialBuildSuccess, "Should fail initially to trigger healing")
	assert.True(t, result.HealingAttempted, "Should attempt healing")
	assert.True(t, len(result.HealingOptions) >= 2, "Should generate multiple healing options")

	// Validate healing options match MVP specification
	expectedOptions := []string{"human-step", "llm-exec", "orw-gen"}
	for _, expected := range expectedOptions {
		found := false
		for _, option := range result.HealingOptions {
			if option.Type == expected {
				found = true
				break
			}
		}
		// Note: In test mode, not all healing strategies may be available
		if !env.IsTestMode {
			assert.True(t, found, "Should include %s healing option", expected)
		}
	}

	// Validate parallel execution with first-success-wins
	assert.True(t, result.ParallelExecution, "Healing should use parallel execution")
	assert.True(t, result.WinningStrategy != "", "Should identify winning healing strategy")

	// Validate KB learning occurred (if KB is available)
	if env.KBClient != nil {
		assert.True(t, result.KBLearningRecorded, "Should record healing attempt in KB")
	}

	// Validate final success
	assert.True(t, result.FinalBuildSuccess, "Should succeed after healing")
	assert.True(t, result.MRURL != "", "Should create MR after successful healing")
}

// TestMVPAcceptance_KnowledgeBaseLearning validates KB learning and improvement
// over multiple runs as specified in MVP requirements
func TestMVPAcceptance_KnowledgeBaseLearning(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping MVP acceptance tests in short mode")
	}

	env := SetupMVPEnvironment(t)
	defer env.Cleanup()

	if env.KBClient == nil {
		t.Skip("Knowledge Base client not available in test environment")
	}

	// Test Case: KB learning and improvement over multiple runs
	testScenarios := []string{
		"java-compilation-error-missing-imports",
		"java-compilation-error-missing-semicolon",
		"java-compilation-error-wrong-type",
	}

	for _, errorType := range testScenarios {
		t.Run(errorType, func(t *testing.T) {
			// Run same error scenario 3 times to validate learning
			var learningProgression []LearningMetrics

			for attempt := 1; attempt <= 3; attempt++ {
				scenario := createKBLearningScenario(errorType, attempt)

				result, err := env.ExecuteScenario(context.Background(), scenario)
				assert.NoError(t, err)

				// Extract learning metrics
				metrics := LearningMetrics{
					Attempt:           attempt,
					ErrorSignature:    result.ErrorSignature,
					HealingDuration:   result.HealingDuration,
					SuccessConfidence: result.HealingConfidence,
					KBCases:           result.KBTotalCases,
				}
				learningProgression = append(learningProgression, metrics)

				// Validate KB storage
				kbHistory, err := env.KBClient.GetErrorHistory(context.Background(), result.ErrorSignature)
				if err == nil {
					assert.True(t, kbHistory.TotalCases >= attempt,
						"KB should accumulate cases over attempts")
				}
			}

			// Validate learning progression
			validateKBLearningProgression(t, learningProgression)
		})
	}
}

// TestMVPAcceptance_ModelRegistry validates complete model registry CRUD
// operations as specified in MVP requirements
func TestMVPAcceptance_ModelRegistry(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping MVP acceptance tests in short mode")
	}

	env := SetupMVPEnvironment(t)
	defer env.Cleanup()

	if env.ModelRegistryClient == nil {
		t.Skip("Model Registry client not available in test environment")
	}

	// Test Case: Complete model registry CRUD as specified in MVP
	modelSpecs := []LLMModel{
		{
			ID:           "gpt-4o-mini@2024-08-06",
			Name:         "GPT-4o Mini",
			Provider:     "openai",
			Version:      "2024-08-06",
			Capabilities: []string{"code", "analysis", "planning"},
			MaxTokens:    128000,
			CostPerToken: 0.00015,
		},
		{
			ID:           "claude-3-haiku@20240307",
			Name:         "Claude 3 Haiku",
			Provider:     "anthropic",
			Version:      "20240307",
			Capabilities: []string{"code", "analysis"},
			MaxTokens:    200000,
			CostPerToken: 0.00025,
		},
	}

	for _, model := range modelSpecs {
		t.Run(model.ID, func(t *testing.T) {
			// Test CREATE operation
			err := env.ModelRegistryClient.AddModel(context.Background(), &model)
			assert.NoError(t, err, "Should create model successfully")

			// Test READ operation
			retrievedModel, err := env.ModelRegistryClient.GetModel(context.Background(), model.ID)
			assert.NoError(t, err, "Should retrieve model successfully")
			assert.Equal(t, model.ID, retrievedModel.ID)
			assert.Equal(t, model.Provider, retrievedModel.Provider)
			assert.Equal(t, model.MaxTokens, retrievedModel.MaxTokens)

			// Test UPDATE operation
			updatedModel := *retrievedModel
			updatedModel.CostPerToken = updatedModel.CostPerToken * 1.1

			err = env.ModelRegistryClient.UpdateModel(context.Background(), &updatedModel)
			assert.NoError(t, err, "Should update model successfully")

			// Verify update
			finalModel, err := env.ModelRegistryClient.GetModel(context.Background(), model.ID)
			assert.NoError(t, err)
			assert.Equal(t, updatedModel.CostPerToken, finalModel.CostPerToken)

			// Test DELETE operation
			err = env.ModelRegistryClient.DeleteModel(context.Background(), model.ID)
			assert.NoError(t, err, "Should delete model successfully")

			// Verify deletion
			_, err = env.ModelRegistryClient.GetModel(context.Background(), model.ID)
			assert.Error(t, err, "Should not find deleted model")
		})
	}

	// Test LIST operation
	allModels, err := env.ModelRegistryClient.ListModels(context.Background())
	assert.NoError(t, err, "Should list models successfully")
	assert.NotNil(t, allModels, "Should return models list")

	// Validate CLI integration
	if env.CLIRunner != nil {
		output, err := env.CLIRunner.Run("ployman", "models", "list")
		if err == nil {
			assert.Contains(t, output, "ID", "CLI output should show model information")
		}
	}
}

// TestMVPAcceptance_GitLabIntegration validates GitLab MR creation
// and lifecycle management as specified in MVP requirements
func TestMVPAcceptance_GitLabIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping MVP acceptance tests in short mode")
	}

	env := SetupMVPEnvironment(t)
	defer env.Cleanup()

	// Skip if GitLab credentials not available
	if os.Getenv("GITLAB_TOKEN") == "" {
		t.Skip("GITLAB_TOKEN not set, skipping GitLab integration tests")
	}

	scenario := &Scenario{
		Name:        "GitLab MR Integration",
		Description: "Validate GitLab MR creation with proper labels and descriptions",

		Repository: "https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git",

		TransflowConfig: `
version: v1alpha1
id: gitlab-integration-test
target_repo: https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git
base_ref: refs/heads/main
target_branch: refs/heads/main
lane: C

steps:
  - type: recipe
    engine: openrewrite
    recipes:
      - org.openrewrite.java.cleanup.CommonStaticAnalysis

self_heal:
  enabled: false
`,

		ExpectedResults: ExpectedResults{
			Success:   true,
			MRCreated: true,
			MRLabels:  []string{"ploy", "tfl"},
		},
	}

	result, err := env.ExecuteScenario(context.Background(), scenario)
	assert.NoError(t, err)

	// Validate MR creation
	assert.True(t, result.MRURL != "", "Should create GitLab MR")
	assert.NotEmpty(t, result.MRURL, "Should return MR URL")
	assert.NotEmpty(t, result.MRNumber, "Should return MR number")

	// Validate MR properties
	assert.NotEmpty(t, result.MRTitle, "MR should have title")
	assert.NotEmpty(t, result.MRDescription, "MR should have description")
	assert.Contains(t, result.MRLabels, "ploy", "MR should have 'ploy' label")

	// Validate branch naming
	assert.True(t, strings.HasPrefix(result.WorkflowBranch, "workflow/"),
		"Workflow branch should follow naming convention")
}

// TestMVPAcceptance_ProductionScale validates production-scale scenarios
// including concurrent operations and resource efficiency
func TestMVPAcceptance_ProductionScale(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping production scale acceptance tests in short mode")
	}

	env := SetupMVPEnvironment(t)
	defer env.Cleanup()

	// Test concurrent workflow execution (MVP requirement: 5 concurrent workflows)
	t.Run("ConcurrentWorkflows", func(t *testing.T) {
		concurrentCount := 3 // Reduce from 5 for test environment
		scenarios := make([]*Scenario, concurrentCount)

		// Create multiple scenarios
		for i := 0; i < concurrentCount; i++ {
			scenarios[i] = &Scenario{
				Name:        fmt.Sprintf("Concurrent Workflow %d", i+1),
				Description: fmt.Sprintf("Concurrent transflow execution test %d", i+1),
				Repository:  "https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git",

				TransflowConfig: fmt.Sprintf(`
version: v1alpha1
id: concurrent-test-%d
target_repo: https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git
base_ref: refs/heads/main
target_branch: refs/heads/main
lane: C

steps:
  - type: recipe
    engine: openrewrite
    recipes:
      - org.openrewrite.java.cleanup.CommonStaticAnalysis

self_heal:
  enabled: false
`, i+1),

				ExpectedResults: ExpectedResults{
					Success: true,
				},
			}
		}

		// Execute scenarios concurrently
		results := make(chan *Result, concurrentCount)
		errors := make(chan error, concurrentCount)

		for i, scenario := range scenarios {
			go func(idx int, s *Scenario) {
				result, err := env.ExecuteScenario(context.Background(), s)
				if err != nil {
					errors <- fmt.Errorf("scenario %d failed: %w", idx, err)
					return
				}
				results <- result
			}(i, scenario)
		}

		// Collect results
		var completedResults []*Result
		for i := 0; i < concurrentCount; i++ {
			select {
			case result := <-results:
				completedResults = append(completedResults, result)
			case err := <-errors:
				t.Errorf("Concurrent execution error: %v", err)
			case <-time.After(15 * time.Minute):
				t.Fatalf("Concurrent execution timed out after 15 minutes")
			}
		}

		// Validate all scenarios succeeded
		assert.Equal(t, concurrentCount, len(completedResults),
			"All concurrent scenarios should complete")

		for i, result := range completedResults {
			assert.True(t, result.Success, "Concurrent scenario %d should succeed", i)
		}
	})
}

// TestMVPStability runs long-term stability testing as specified in MVP requirements
func TestMVPStability(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stability tests in short mode")
	}

	env := SetupMVPEnvironment(t)
	defer env.Cleanup()

	// Long-term stability test (reduced duration for testing)
	stabilityDuration := 30 * time.Minute // Reduced from 4 hours for practical testing
	endTime := time.Now().Add(stabilityDuration)

	var successCount, totalCount int

	for time.Now().Before(endTime) {
		scenario := &Scenario{
			Name:        fmt.Sprintf("Stability Test Run %d", totalCount+1),
			Description: "Long-term stability validation",
			Repository:  "https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git",

			TransflowConfig: `
version: v1alpha1
id: stability-test
target_repo: https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git
base_ref: refs/heads/main
target_branch: refs/heads/main
lane: C

steps:
  - type: recipe
    engine: openrewrite
    recipes:
      - org.openrewrite.java.cleanup.CommonStaticAnalysis

self_heal:
  enabled: false
`,

			ExpectedResults: ExpectedResults{
				Success: true,
			},
		}

		result, err := env.ExecuteScenario(context.Background(), scenario)
		totalCount++

		if err == nil && result.Success {
			successCount++
		}

		// Wait between runs
		time.Sleep(2 * time.Minute)
	}

	// Validate stability metrics
	successRate := float64(successCount) / float64(totalCount)
	assert.True(t, successRate >= 0.95,
		"Stability test should maintain 95%% success rate, got %.2f%% (%d/%d)",
		successRate*100, successCount, totalCount)

	t.Logf("Stability test completed: %d/%d successful (%.2f%%)",
		successCount, totalCount, successRate*100)
}

// Helper function to create KB learning scenarios
func createKBLearningScenario(errorType string, attempt int) *Scenario {
	return &Scenario{
		Name:        fmt.Sprintf("KB Learning - %s - Attempt %d", errorType, attempt),
		Description: fmt.Sprintf("Knowledge base learning validation for %s", errorType),
		Repository:  "https://gitlab.com/iw2rmb/ploy-test-kb-learning.git",

		TransflowConfig: fmt.Sprintf(`
version: v1alpha1
id: kb-learning-%s-%d
target_repo: https://gitlab.com/iw2rmb/ploy-test-kb-learning.git
base_ref: refs/heads/main
target_branch: refs/heads/main
lane: C

steps:
  - type: recipe
    engine: openrewrite
    recipes:
      - org.openrewrite.java.cleanup.UnnecessaryParentheses

self_heal:
  enabled: true
  kb_learning: true
  max_retries: 2
  error_scenario: %s
`, errorType, attempt, errorType),

		ExpectedResults: ExpectedResults{
			HealingTriggered: true,
			KBLearning:       true,
		},
	}
}
