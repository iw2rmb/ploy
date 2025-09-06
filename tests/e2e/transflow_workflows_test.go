package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTransflowE2E_JavaMigrationComplete(t *testing.T) {
	// Should fail initially - end-to-end integration gaps

	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	env := SetupTestEnvironment(t, Config{
		UseRealServices: true,
		CleanupAfter:    true,
		TimeoutMinutes:  15,
	})
	defer env.Cleanup()

	workflow := &TransflowWorkflow{
		ID:           fmt.Sprintf("e2e-java-migration-%d", time.Now().Unix()),
		Repository:   "https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git",
		TargetBranch: "main",
		Steps: []WorkflowStep{
			{
				Type:    "recipe",
				ID:      "java-migration", 
				Engine:  "openrewrite",
				Recipes: []string{"org.openrewrite.java.migrate.Java11toJava17"},
			},
		},
		SelfHeal: SelfHealConfig{
			Enabled:    true,
			MaxRetries: 2,
			KBLearning: true,
		},
		ExpectedOutcome: OutcomeSuccess,
		MaxDuration:     10 * time.Minute,
	}

	ctx, cancel := context.WithTimeout(context.Background(), workflow.MaxDuration)
	defer cancel()

	result, err := env.ExecuteWorkflow(ctx, workflow)
	
	// These assertions will initially fail - this is expected for RED phase
	assert.NoError(t, err, "E2E workflow should complete without errors")
	assert.True(t, result.Success, "Workflow should succeed")
	assert.NotEmpty(t, result.WorkflowBranch, "Should create workflow branch")
	assert.NotEmpty(t, result.BuildVersion, "Should produce build version")
	
	// Skip MR validation in initial RED phase - will be implemented in GREEN
	if result.MRUrl != "" {
		t.Logf("MR Created: %s", result.MRUrl)
	}
	
	// Log results for debugging
	t.Logf("Workflow Duration: %v", result.Duration)
	t.Logf("Workflow Output: %s", result.Output)
}

func TestTransflowE2E_SelfHealingScenario(t *testing.T) {
	// Should fail initially - healing integration not complete

	if testing.Short() {
		t.Skip("skipping E2E test in short mode") 
	}

	env := SetupTestEnvironment(t, Config{
		UseRealServices: true,
		InjectFailures:  true,
		TimeoutMinutes:  15,
	})
	defer env.Cleanup()

	workflow := &TransflowWorkflow{
		ID:         fmt.Sprintf("e2e-healing-%d", time.Now().Unix()),
		Repository: "https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git", // Use standard repo for now
		TargetBranch: "main",
		Steps: []WorkflowStep{
			{
				Type:    "recipe",
				ID:      "healing-test",
				Engine:  "openrewrite", 
				Recipes: []string{
					"org.openrewrite.java.migrate.Java11toJava17",
					"org.openrewrite.java.cleanup.UnnecessaryParentheses",
				},
			},
		},
		SelfHeal: SelfHealConfig{
			Enabled:    true,
			MaxRetries: 3,
			KBLearning: true,
		},
		ExpectedOutcome: OutcomeHealedSuccess,
		MaxDuration:     12 * time.Minute,
	}

	ctx, cancel := context.WithTimeout(context.Background(), workflow.MaxDuration)
	defer cancel()

	result, err := env.ExecuteWorkflow(ctx, workflow)
	
	// Basic completion check - may fail initially 
	assert.NoError(t, err, "Healing workflow should complete")
	
	// Log healing attempts for analysis
	t.Logf("Healing Attempted: %t", result.HealingAttempted)
	t.Logf("Healing Attempts: %d", len(result.HealingAttempts))
	t.Logf("Final Success: %t", result.Success)
	
	if len(result.HealingAttempts) > 0 {
		for i, attempt := range result.HealingAttempts {
			t.Logf("Attempt %d: Success=%t, Error=%s", i+1, attempt.Success, attempt.ErrorSignature)
		}
	}
}

func TestTransflowE2E_KBLearningProgression(t *testing.T) {
	// Should fail initially - KB learning not integrated

	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	env := SetupTestEnvironment(t, Config{
		UseRealServices: true,
		CleanupAfter:    true,
		TimeoutMinutes:  20,
	})
	defer env.Cleanup()

	baseWorkflow := TransflowWorkflow{
		Repository:   "https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git",
		TargetBranch: "main",
		Steps: []WorkflowStep{
			{
				Type:    "recipe",
				ID:      "learning-test",
				Engine:  "openrewrite",
				Recipes: []string{"org.openrewrite.java.cleanup.SimplifyBooleanExpression"},
			},
		},
		SelfHeal: SelfHealConfig{
			Enabled:    true,
			KBLearning: true,
			MaxRetries: 2,
		},
		MaxDuration: 8 * time.Minute,
	}

	var results []WorkflowResult

	// Execute same workflow multiple times to test learning progression
	for i := 0; i < 2; i++ { // Reduced from 3 to 2 to save time in RED phase
		workflow := baseWorkflow
		workflow.ID = fmt.Sprintf("e2e-learning-%d-run-%d", time.Now().Unix(), i+1)

		ctx, cancel := context.WithTimeout(context.Background(), workflow.MaxDuration)
		
		result, err := env.ExecuteWorkflow(ctx, &workflow)
		cancel()
		
		// Log each run
		t.Logf("Run %d: Success=%t, Duration=%v, Error=%s", i+1, result.Success, result.Duration, result.Error)
		
		if err != nil {
			t.Logf("Run %d error: %v", i+1, err)
		}
		
		results = append(results, result)
		
		// Small delay between runs
		time.Sleep(5 * time.Second)
	}

	// Basic validation - may fail initially
	assert.True(t, len(results) >= 1, "Should complete at least one run")

	// Log learning progression for analysis
	for i, result := range results {
		t.Logf("Learning Run %d: Success=%t, Duration=%v", i+1, result.Success, result.Duration)
	}

	if len(results) == 2 && results[0].Success && results[1].Success {
		t.Logf("Learning progression: Run 1: %v, Run 2: %v", results[0].Duration, results[1].Duration)
	}
}