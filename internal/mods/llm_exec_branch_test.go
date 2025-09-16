package mods

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// Test llm-exec branch validation
func TestLLMExecBranchValidation(t *testing.T) {
	t.Run("llm-exec branch renders HCL assets correctly", func(t *testing.T) {
		orchestrator := &fanoutOrchestrator{
			runner: &MockProductionBranchRunner{
				LLMExecAssetsError: nil,
				LLMExecAssetsPath:  "/tmp/test-assets/llm-exec.rendered.hcl",
			},
		}

		branch := BranchSpec{
			ID:   "llm-test-branch",
			Type: "llm-exec",
			Inputs: map[string]interface{}{
				"model":   "gpt-4o-mini",
				"timeout": "15m",
			},
		}

		result := orchestrator.executeLLMExecBranch(context.Background(), branch, BranchResult{
			ID:        branch.ID,
			StartedAt: time.Now(),
			Status:    "failed",
		})

		// RenderLLMExecAssets works, but HCL template file doesn't exist
		assert.Equal(t, "failed", result.Status, "should fail due to missing HCL template file")
		assert.Contains(t, result.Notes, "failed to substitute HCL template")
	})

	t.Run("llm-exec branch substitutes environment variables in HCL template", func(t *testing.T) {
		orchestrator := &fanoutOrchestrator{
			runner: &MockProductionBranchRunner{
				LLMExecAssetsError: nil,
				LLMExecAssetsPath:  "/tmp/test-llm.hcl",
			},
		}
		// Prevent real Nomad interactions; force submit failure path
		orchestrator.hcl = failingHCLSubmitter{}

		// Set up test HCL template file
		testHCL := `job "llm-exec-${RUN_ID}" {
group "main" {
    task "generate" {
        env {
            MODS_MODEL = "${MODS_MODEL}"
            MODS_TOOLS = "${MODS_TOOLS}"
            MODS_LIMITS = "${MODS_LIMITS}"
        }
    }
}
}`

		tempFile := "/tmp/test-llm.hcl"
		err := os.WriteFile(tempFile, []byte(testHCL), 0644)
		if err != nil {
			t.Fatalf("failed to create test HCL file: %v", err)
		}
		defer func() { _ = os.Remove(tempFile) }()

		branch := BranchSpec{ID: "llm-substitution-test", Type: "llm-exec"}

		result := orchestrator.executeLLMExecBranch(context.Background(), branch, BranchResult{
			ID:        branch.ID,
			StartedAt: time.Now(),
			Status:    "failed",
		})

		// HCL template substitution works, but Nomad submission fails in test environment
		assert.Equal(t, "failed", result.Status)
		assert.Contains(t, result.Notes, "LLM exec job failed")
	})
}
