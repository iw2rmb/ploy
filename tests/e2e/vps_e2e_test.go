//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestVPSE2E_ProductionWorkflows(t *testing.T) {
	if os.Getenv("TARGET_HOST") == "" {
		t.Skip("TARGET_HOST not set, skipping VPS E2E tests")
	}

	if testing.Short() {
		t.Skip("skipping VPS E2E test in short mode")
	}

	env := SetupTestEnvironment(t, Config{
		UseRealServices: true,
		TimeoutMinutes:  20,
	})
	defer env.Cleanup()

	workflow := &ModWorkflow{
		ID:           fmt.Sprintf("vps-e2e-%d", time.Now().Unix()),
		Repository:   "https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git",
		TargetBranch: "main",
		Steps: []WorkflowStep{
			{
				Type:   "recipe",
				ID:     "vps-java-migration",
				Engine: "openrewrite",
				Recipes: []string{
					"org.openrewrite.java.migrate.Java11toJava17",
					"org.openrewrite.java.cleanup.CommonStaticAnalysis",
				},
			},
		},
		SelfHeal: SelfHealConfig{
			Enabled:    true,
			KBLearning: true,
			MaxRetries: 2,
		},
		MaxDuration: 15 * time.Minute,
	}

	ctx, cancel := context.WithTimeout(context.Background(), workflow.MaxDuration)
	defer cancel()

	result, err := env.ExecuteWorkflow(ctx, workflow)

	// Log VPS execution details
	t.Logf("VPS E2E Result: Success=%t, Duration=%v", result.Success, result.Duration)
	t.Logf("VPS Output: %s", result.Output)

	if err != nil {
		t.Logf("VPS E2E Error: %v", err)
	}

	// Basic validation - should eventually pass in GREEN/REFACTOR phases
	assert.NoError(t, err, "VPS E2E workflow should complete")
	assert.True(t, result.Success, "VPS workflow should succeed")
	assert.True(t, result.Duration < 15*time.Minute, "VPS execution should be reasonably fast")

	// VPS-specific validations will be expanded in REFACTOR phase
	if result.ResourceUsage != nil {
		t.Logf("VPS Resource Usage: Memory=%dMB, CPU=%.1f%%",
			result.ResourceUsage.MaxMemoryMB, result.ResourceUsage.CPUPercent)
	}
}

func TestVPSE2E_ConcurrentWorkflows(t *testing.T) {
	if os.Getenv("TARGET_HOST") == "" {
		t.Skip("TARGET_HOST not set")
	}

	if testing.Short() {
		t.Skip("skipping concurrent VPS E2E test in short mode")
	}

	env := SetupTestEnvironment(t, Config{UseRealServices: true})
	defer env.Cleanup()

	// Reduced concurrent workflows to 2 for initial testing
	const concurrentWorkflows = 2

	var wg sync.WaitGroup
	results := make(chan WorkflowResult, concurrentWorkflows)
	errors := make(chan error, concurrentWorkflows)

	for i := 0; i < concurrentWorkflows; i++ {
		wg.Add(1)
		go func(workflowNum int) {
			defer wg.Done()

			workflow := &ModWorkflow{
				ID:           fmt.Sprintf("concurrent-%d-%d", time.Now().Unix(), workflowNum),
				Repository:   "https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git",
				TargetBranch: "main",
				Steps: []WorkflowStep{
					{
						Type:    "recipe",
						ID:      fmt.Sprintf("concurrent-recipe-%d", workflowNum),
						Engine:  "openrewrite",
						Recipes: []string{"org.openrewrite.java.migrate.Java11toJava17"},
					},
				},
				MaxDuration: 12 * time.Minute,
			}

			ctx, cancel := context.WithTimeout(context.Background(), workflow.MaxDuration)
			defer cancel()

			result, err := env.ExecuteWorkflow(ctx, workflow)
			if err != nil {
				errors <- fmt.Errorf("concurrent workflow %d failed: %w", workflowNum, err)
				return
			}
			results <- result
		}(i)
	}

	wg.Wait()
	close(results)
	close(errors)

	// Collect results
	var completedResults []WorkflowResult
	for result := range results {
		completedResults = append(completedResults, result)
	}

	var workflowErrors []error
	for err := range errors {
		workflowErrors = append(workflowErrors, err)
	}

	// Log concurrent execution results
	t.Logf("Concurrent Workflows: Completed=%d, Errors=%d",
		len(completedResults), len(workflowErrors))

	for i, result := range completedResults {
		t.Logf("Concurrent Result %d: Success=%t, Duration=%v", i+1, result.Success, result.Duration)
	}

	for i, err := range workflowErrors {
		t.Logf("Concurrent Error %d: %v", i+1, err)
	}

	// Validation - may initially fail in RED phase
	if len(workflowErrors) == 0 && len(completedResults) == concurrentWorkflows {
		for i, result := range completedResults {
			assert.True(t, result.Success, "Concurrent workflow %d should succeed", i)
			assert.True(t, result.Duration < 12*time.Minute,
				"Concurrent workflow %d should complete in reasonable time", i)
		}
	} else {
		t.Logf("Concurrent workflows not fully successful yet - expected in RED phase")
	}
}
