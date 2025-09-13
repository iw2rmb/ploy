package transflow

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/api/arf"
	"github.com/iw2rmb/ploy/internal/cli/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelfHealConfig_Parsing(t *testing.T) {
	tests := []struct {
		name        string
		yamlContent string
		expected    *SelfHealConfig
		expectError bool
	}{
		{
			name: "valid self heal config",
			yamlContent: `version: v1alpha1
id: test-workflow
target_repo: https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git
base_ref: refs/heads/main
self_heal:
  max_retries: 3
  cooldown: 30s
  enabled: true
steps:
  - type: recipe
    id: test-recipe
    engine: openrewrite
    recipes:
      - com.acme.Recipe`,
			expected: &SelfHealConfig{
				MaxRetries: 3,
				Cooldown:   "30s",
				Enabled:    true,
			},
			expectError: false,
		},
		{
			name: "self heal disabled",
			yamlContent: `version: v1alpha1
id: test-workflow
target_repo: https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git
base_ref: refs/heads/main
self_heal:
  enabled: false
steps:
  - type: recipe
    id: test-recipe
    engine: openrewrite
    recipes:
      - com.acme.Recipe`,
			expected: &SelfHealConfig{
				MaxRetries: 1,  // default
				Cooldown:   "", // default
				Enabled:    false,
			},
			expectError: false,
		},
		{
			name: "no self heal config uses defaults",
			yamlContent: `version: v1alpha1
id: test-workflow
target_repo: https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git
base_ref: refs/heads/main
steps:
  - type: recipe
    id: test-recipe
    engine: openrewrite
    recipes:
      - com.acme.Recipe`,
			expected: &SelfHealConfig{
				MaxRetries: 1,    // default
				Cooldown:   "",   // default
				Enabled:    true, // default
			},
			expectError: false,
		},
		{
			name: "invalid max_retries",
			yamlContent: `version: v1alpha1
id: test-workflow
target_repo: https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git
base_ref: refs/heads/main
self_heal:
  max_retries: -1
  enabled: true
steps:
  - type: recipe
    id: test-recipe
    engine: openrewrite
    recipes:
      - com.acme.Recipe`,
			expected:    nil,
			expectError: true,
		},
		{
			name: "invalid cooldown format",
			yamlContent: `version: v1alpha1
id: test-workflow
target_repo: https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git
base_ref: refs/heads/main
self_heal:
  max_retries: 2
  cooldown: invalid
  enabled: true
steps:
  - type: recipe
    id: test-recipe
    engine: openrewrite
    recipes:
      - com.acme.Recipe`,
			expected:    nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This will fail until we implement the parsing
			config, err := parseTestConfigYAML(tt.yamlContent)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, config)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, config)
				assert.Equal(t, tt.expected, config.SelfHeal)
			}
		})
	}
}

func TestSelfHealConfig_Validation(t *testing.T) {
	tests := []struct {
		name        string
		config      *SelfHealConfig
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid config",
			config: &SelfHealConfig{
				MaxRetries: 2,
				Cooldown:   "30s",
				Enabled:    true,
			},
			expectError: false,
		},
		{
			name: "negative max_retries",
			config: &SelfHealConfig{
				MaxRetries: -1,
				Enabled:    true,
			},
			expectError: true,
			errorMsg:    "max_retries cannot be negative",
		},
		{
			name: "excessive max_retries",
			config: &SelfHealConfig{
				MaxRetries: 10,
				Enabled:    true,
			},
			expectError: true,
			errorMsg:    "max_retries cannot exceed 5",
		},
		{
			name: "invalid cooldown format",
			config: &SelfHealConfig{
				MaxRetries: 2,
				Cooldown:   "invalid",
				Enabled:    true,
			},
			expectError: true,
			errorMsg:    "invalid cooldown format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestTransflowHealingAttempt_Creation(t *testing.T) {
	tests := []struct {
		name             string
		attemptNumber    int
		errorContext     arf.ErrorContext
		suggestedRecipes []string
		expected         *TransflowHealingAttempt
	}{
		{
			name:          "simple healing attempt",
			attemptNumber: 1,
			errorContext: arf.ErrorContext{
				ErrorMessage: "compilation failed",
				ErrorType:    "compilation",
				SourceFile:   "Main.java",
			},
			suggestedRecipes: []string{"com.acme.FixCompilation"},
			expected: &TransflowHealingAttempt{
				AttemptNumber: 1,
				ErrorContext: arf.ErrorContext{
					ErrorMessage: "compilation failed",
					ErrorType:    "compilation",
					SourceFile:   "Main.java",
				},
				SuggestedRecipes: []string{"com.acme.FixCompilation"},
				AppliedRecipes:   []string{},
				Success:          false,
				ErrorMessage:     "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attempt := NewTransflowHealingAttempt(tt.attemptNumber, tt.errorContext, tt.suggestedRecipes)

			assert.Equal(t, tt.expected.AttemptNumber, attempt.AttemptNumber)
			assert.Equal(t, tt.expected.ErrorContext.ErrorMessage, attempt.ErrorContext.ErrorMessage)
			assert.Equal(t, tt.expected.SuggestedRecipes, attempt.SuggestedRecipes)
			assert.False(t, attempt.Success)
			assert.Empty(t, attempt.AppliedRecipes)
		})
	}
}

func TestTransflowHealingSummary_Tracking(t *testing.T) {
	summary := NewTransflowHealingSummary(true, 2)

	// Test initial state
	assert.True(t, summary.Enabled)
	assert.Equal(t, 2, summary.MaxRetries)
	assert.Equal(t, 0, summary.AttemptsCount)
	assert.False(t, summary.FinalSuccess)
	assert.Equal(t, 0, summary.TotalHealed)
	assert.Empty(t, summary.Attempts)

	// Add healing attempts
	attempt1 := &TransflowHealingAttempt{
		AttemptNumber:    1,
		SuggestedRecipes: []string{"com.acme.FixImports"},
		AppliedRecipes:   []string{"com.acme.FixImports"},
		Success:          false,
		ErrorMessage:     "still failing",
	}

	attempt2 := &TransflowHealingAttempt{
		AttemptNumber:    2,
		SuggestedRecipes: []string{"com.acme.FixSyntax"},
		AppliedRecipes:   []string{"com.acme.FixSyntax"},
		Success:          true,
	}

	summary.AddAttempt(attempt1)
	assert.Equal(t, 1, summary.AttemptsCount)
	assert.Equal(t, 0, summary.TotalHealed)
	assert.False(t, summary.FinalSuccess)

	summary.AddAttempt(attempt2)
	assert.Equal(t, 2, summary.AttemptsCount)
	assert.Equal(t, 1, summary.TotalHealed)

	summary.SetFinalResult(true)
	assert.True(t, summary.FinalSuccess)
}

func TestErrorAnalysisIntegration(t *testing.T) {
	// Test error analysis integration (will fail until implemented)
	buildErrors := []string{
		"main.go:15:2: undefined: nonExistentFunction",
		"main.go:20:5: syntax error: unexpected ;",
	}

	analyzer := NewTransflowErrorAnalyzer()
	suggestions, err := analyzer.AnalyzeBuildFailure(context.Background(), buildErrors, "go")

	require.NoError(t, err)
	require.NotNil(t, suggestions)
	assert.NotEmpty(t, suggestions.SuggestedRecipes)
	assert.Greater(t, suggestions.Confidence, 0.0)
}

func TestSelfHealingRunnerFlow(t *testing.T) {
	// Test the complete self-healing flow (will fail until implemented)
	config := &TransflowConfig{
		ID:         "test-healing",
		TargetRepo: "https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git",
		BaseRef:    "refs/heads/main",
		SelfHeal: &SelfHealConfig{
			MaxRetries: 2,
			Enabled:    true,
		},
		Steps: []TransflowStep{
			{Type: "orw-apply", ID: "java-migration", Recipes: []string{"org.openrewrite.java.migrate.UpgradeToJava17"}},
		},
	}

	// Mock build failure and healing
	mockGit := &MockGitOperations{}
	mockRecipe := &MockRecipeExecutor{}
	mockBuild := NewHealingMockBuildChecker()
	mockBuild.buildResults = []*common.DeployResult{
		{Success: false, Message: "compilation failed: undefined symbol"},
		{Success: true, Message: "build successful", Version: "healed-v1.0.0"},
	}

	// Create mock job submitter for healing workflow
	mockJobSubmitter := &MockJobSubmitter{
		JobResults: map[string]JobResult{
			"planner": {
				JobID:    "planner-job-1",
				Status:   "completed",
				Duration: 10 * time.Second,
				Output:   `{"plan_id": "plan-123", "options": [{"id": "option-1", "type": "llm-exec"}]}`,
			},
			"llm-exec": {
				JobID:    "llm-exec-job-1",
				Status:   "completed",
				Duration: 30 * time.Second,
				Output:   `{"diff": "successful patch applied"}`,
			},
			"reducer": {
				JobID:    "reducer-job-1",
				Status:   "completed",
				Duration: 5 * time.Second,
				Output:   `{"action": "stop", "notes": "healing succeeded"}`,
			},
		},
	}

	runner, err := NewTransflowRunner(config, "/tmp/test")
	require.NoError(t, err)

	runner.SetGitOperations(mockGit)
	runner.SetRecipeExecutor(mockRecipe)
	runner.SetBuildChecker(mockBuild)
	runner.SetJobSubmitter(mockJobSubmitter)

	// This should trigger self-healing
	ctx := context.Background()
	result, err := runner.Run(ctx)

	// Should succeed after healing
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.Success)
	assert.NotNil(t, result.HealingSummary)
	assert.True(t, result.HealingSummary.Enabled)
	assert.Greater(t, result.HealingSummary.AttemptsCount, 0)

	// Debug: print healing summary details
	if result.HealingSummary != nil {
		t.Logf("Healing Summary: Enabled=%t, AttemptsCount=%d, FinalSuccess=%t, Winner=%v",
			result.HealingSummary.Enabled, result.HealingSummary.AttemptsCount,
			result.HealingSummary.FinalSuccess, result.HealingSummary.Winner != nil)
		if result.HealingSummary.Winner != nil {
			t.Logf("Winner: %+v", *result.HealingSummary.Winner)
		}
		t.Logf("All results count: %d", len(result.HealingSummary.AllResults))
	}

	assert.True(t, result.HealingSummary.FinalSuccess)
}

func TestSelfHealingBoundedRetries(t *testing.T) {
	// Test that healing respects max_retries limit
	config := &TransflowConfig{
		ID:         "test-bounded",
		TargetRepo: "https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git",
		BaseRef:    "refs/heads/main",
		SelfHeal: &SelfHealConfig{
			MaxRetries: 1, // Only one retry allowed
			Enabled:    true,
		},
		Steps: []TransflowStep{
			{Type: "orw-apply", ID: "java-migration", Recipes: []string{"org.openrewrite.java.migrate.UpgradeToJava17"}},
		},
	}

	mockBuild := NewHealingMockBuildChecker()
	mockBuild.BuildError = errors.New("persistent build failure")

	// Create mock job submitter that always fails
	mockJobSubmitter := &MockJobSubmitter{
		SubmitError: errors.New("healing job failed"),
	}

	runner, err := NewTransflowRunner(config, "/tmp/test")
	require.NoError(t, err)

	runner.SetGitOperations(&MockGitOperations{})
	runner.SetRecipeExecutor(&MockRecipeExecutor{})
	runner.SetBuildChecker(mockBuild)
	runner.SetJobSubmitter(mockJobSubmitter)

	ctx := context.Background()
	result, err := runner.Run(ctx)

	// Should fail after exhausting retries
	assert.Error(t, err)
	if result != nil && result.HealingSummary != nil {
		// Should have attempted exactly max_retries times
		assert.Equal(t, 1, result.HealingSummary.AttemptsCount)
		assert.False(t, result.HealingSummary.FinalSuccess)
	}
}

// Helper functions for testing

func parseTestConfigYAML(yamlContent string) (*TransflowConfig, error) {
	// Create temporary file with test content
	tmpFile, err := os.CreateTemp("", "transflow-test-*.yaml")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpFile.Name())

	if err := os.WriteFile(tmpFile.Name(), []byte(yamlContent), 0644); err != nil {
		return nil, err
	}

	return LoadConfig(tmpFile.Name())
}

// Enhanced MockBuildChecker for healing tests (extends the one from runner_test.go)
type HealingMockBuildChecker struct {
	*MockBuildChecker
	buildResults []*common.DeployResult
	callCount    int
}

func NewHealingMockBuildChecker() *HealingMockBuildChecker {
	return &HealingMockBuildChecker{
		MockBuildChecker: &MockBuildChecker{},
	}
}

func (m *HealingMockBuildChecker) CheckBuild(ctx context.Context, config common.DeployConfig) (*common.DeployResult, error) {
	m.BuildCalled = true
	m.BuildConfig = config

	if m.buildResults != nil && m.callCount < len(m.buildResults) {
		result := m.buildResults[m.callCount]
		m.callCount++
		if !result.Success {
			return result, errors.New(result.Message)
		}
		return result, nil
	}

	// Fall back to base MockBuildChecker behavior
	return m.MockBuildChecker.CheckBuild(ctx, config)
}
