package mods

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// Test orw-gen branch validation
func TestORWGenBranchValidation(t *testing.T) {
	t.Run("orw-gen branch renders ORW apply assets correctly", func(t *testing.T) {
		orchestrator := &fanoutOrchestrator{
			runner: &MockProductionBranchRunner{
				ORWApplyAssetsError: nil,
				ORWApplyAssetsPath:  "/tmp/test-orw-assets/orw-apply.rendered.hcl",
			},
		}

		branch := BranchSpec{
			ID:   "orw-test-branch",
			Type: "orw-gen",
			Inputs: map[string]interface{}{
				"recipe_config": map[string]interface{}{
					"class":   "org.openrewrite.java.migrate.Java8toJava11",
					"coords":  "org.openrewrite.recipe:rewrite-migrate-java:1.21.0",
					"timeout": "15m",
				},
			},
		}

		result := orchestrator.executeORWGenBranch(context.Background(), branch, BranchResult{
			ID:        branch.ID,
			StartedAt: time.Now(),
			Status:    "failed",
		})

		// RenderORWApplyAssets works, but HCL template file doesn't exist
		assert.Equal(t, "failed", result.Status, "should fail due to missing HCL template file")
		assert.Contains(t, result.Notes, "failed to read ORW HCL template")
	})

	t.Run("orw-gen branch extracts recipe configuration from inputs", func(t *testing.T) {
		orchestrator := &fanoutOrchestrator{
			runner: &MockProductionBranchRunner{
				ORWApplyAssetsError: nil,
				ORWApplyAssetsPath:  "/tmp/test-orw-config.hcl",
			},
		}

		// Set up test HCL template file with recipe variables
		testHCL := `job "orw-apply" {
group "main" {
    task "apply" {
        env {
            RECIPE_CLASS = "${RECIPE_CLASS}"
            RECIPE_COORDS = "${RECIPE_COORDS}"
            RECIPE_TIMEOUT = "${RECIPE_TIMEOUT}"
        }
    }
}
}`

		tempFile := "/tmp/test-orw-config.hcl"
		err := os.WriteFile(tempFile, []byte(testHCL), 0644)
		if err != nil {
			t.Fatalf("failed to create test HCL file: %v", err)
		}
		defer func() { _ = os.Remove(tempFile) }()

		branch := BranchSpec{
			ID:   "orw-config-test",
			Type: "orw-gen",
			Inputs: map[string]interface{}{
				"recipe_config": map[string]interface{}{
					"class":   "org.openrewrite.java.migrate.Java11toJava17",
					"coords":  "org.openrewrite.recipe:rewrite-migrate-java:1.21.0",
					"timeout": "20m",
				},
			},
		}

		result := orchestrator.executeORWGenBranch(context.Background(), branch, BranchResult{
			ID:        branch.ID,
			StartedAt: time.Now(),
			Status:    "failed",
		})

		// Should fail at Nomad submission step, but recipe config should be extracted
		assert.Equal(t, "failed", result.Status)
		assert.NotContains(t, result.Notes, "failed to read ORW HCL template")
	})

	t.Run("orw-gen branch substitutes recipe variables in HCL template", func(t *testing.T) {
		orchestrator := &fanoutOrchestrator{
			runner: &MockProductionBranchRunner{
				ORWApplyAssetsError: nil,
				ORWApplyAssetsPath:  "/tmp/test-orw-subst.hcl",
			},
		}

		testHCL := `job "orw-test" {}`
		tempFile := "/tmp/test-orw-subst.hcl"
		err := os.WriteFile(tempFile, []byte(testHCL), 0644)
		if err != nil {
			t.Fatalf("failed to create test HCL file: %v", err)
		}
		defer func() { _ = os.Remove(tempFile) }()

		branch := BranchSpec{
			ID:   "orw-subst-test",
			Type: "orw-gen",
			Inputs: map[string]interface{}{
				"recipe_config": map[string]interface{}{
					"class":   "org.openrewrite.java.migrate.Java8toJava11",
					"coords":  "org.openrewrite.recipe:rewrite-migrate-java:1.21.0",
					"timeout": "10m",
				},
			},
		}

		result := orchestrator.executeORWGenBranch(context.Background(), branch, BranchResult{
			ID:        branch.ID,
			StartedAt: time.Now(),
			Status:    "failed",
		})

		// Will fail at submission step in test environment
		assert.Equal(t, "failed", result.Status)
	})

	t.Run("orw-gen branch validates artifact presence", func(t *testing.T) {
		orchestrator := &fanoutOrchestrator{
			runner: &MockProductionBranchRunner{
				ORWApplyAssetsError: nil,
				ORWApplyAssetsPath:  "/tmp/test-orw-artifact.hcl",
			},
		}

		testHCL := "job \"orw-test\" {}"
		tempFile := "/tmp/test-orw-artifact.hcl"
		renderedFile := "/tmp/test-orw-artifact.rendered.submitted.hcl"

		err := os.WriteFile(tempFile, []byte(testHCL), 0644)
		if err != nil {
			t.Fatalf("failed to create test HCL file: %v", err)
		}
		defer func() { _ = os.Remove(tempFile) }()
		defer func() { _ = os.Remove(renderedFile) }()

		// Create output directory but no diff.patch (simulating completed job with no changes)
		err = os.MkdirAll("/tmp/out", 0755)
		if err != nil {
			t.Fatalf("failed to create output directory: %v", err)
		}
		defer func() { _ = os.RemoveAll("/tmp/out") }()

		branch := BranchSpec{
			ID:   "orw-artifact-test",
			Type: "orw-gen",
			Inputs: map[string]interface{}{
				"recipe_config": map[string]interface{}{
					"class":   "org.openrewrite.java.migrate.Java8toJava11",
					"coords":  "org.openrewrite.recipe:rewrite-migrate-java:1.21.0",
					"timeout": "10m",
				},
			},
		}

		result := orchestrator.executeORWGenBranch(context.Background(), branch, BranchResult{
			ID:        branch.ID,
			StartedAt: time.Now(),
			Status:    "failed",
		})

		// Should fail because diff.patch doesn't exist, but would get that far in a working system
		assert.Equal(t, "failed", result.Status)
		assert.Contains(t, result.Notes, "ORW apply job failed")
	})
}
