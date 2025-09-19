package mods

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/common"
	"github.com/iw2rmb/ploy/internal/git/provider"
	"github.com/stretchr/testify/assert"
)

// Test timeout and error handling scenarios
func TestTimeoutAndErrorHandling(t *testing.T) {
	t.Run("fanout orchestration handles context cancellation correctly", func(t *testing.T) {
		mockSubmitter := &MockJobSubmitter{
			JobResults:    make(map[string]JobResult),
			ArtifactPaths: make(map[string]string),
			JobDelays:     make(map[string]time.Duration),
		}

		// Configure slow jobs that would normally take longer than the timeout
		mockSubmitter.JobResults["slow-branch-1"] = JobResult{JobID: "slow-1", Status: "completed", Output: "would eventually succeed"}
		mockSubmitter.JobResults["slow-branch-2"] = JobResult{JobID: "slow-2", Status: "completed", Output: "would also eventually succeed"}
		mockSubmitter.JobDelays["slow-branch-1"] = 200 * time.Millisecond
		mockSubmitter.JobDelays["slow-branch-2"] = 300 * time.Millisecond

		orchestrator := NewFanoutOrchestrator(mockSubmitter)

		branches := []BranchSpec{{ID: "slow-branch-1", Type: "llm-exec"}, {ID: "slow-branch-2", Type: "llm-exec"}}

		// Create a context with very short timeout
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		winner, results, err := orchestrator.RunHealingFanout(ctx, nil, branches, 2)

		// Should fail due to timeout, but this test documents expected timeout behavior
		assert.Error(t, err, "should fail due to timeout")
		assert.Equal(t, BranchResult{}, winner, "no winner due to timeout")
		assert.Greater(t, len(results), 0, "should collect partial results")

		// At least some results should show cancellation
		cancelled := false
		for _, result := range results {
			if result.Status == "cancelled" {
				cancelled = true
				break
			}
		}
		assert.True(t, cancelled, "at least one branch should be cancelled")
	})

	t.Run("human-step branch handles timeout correctly", func(t *testing.T) {
		orchestrator := &fanoutOrchestrator{
			runner: &MockProductionBranchRunner{
				GitProviderMock: &MockGitProvider{
					MRError:  nil,
					MRResult: &provider.MRResult{MRURL: "https://gitlab.com/test/repo/-/merge_requests/123", MRID: 123, Created: true},
				},
				BuildCheckerMock: &MockBuildChecker{BuildResult: &common.DeployResult{Success: false}},
			},
		}

		branch := BranchSpec{ID: "human-timeout-test", Type: "human-step", Inputs: map[string]interface{}{"timeout": "100ms", "buildError": "Test build failure"}}

		result := orchestrator.executeHumanStepBranch(context.Background(), branch, BranchResult{ID: branch.ID, StartedAt: time.Now(), Status: "failed"})

		// Should timeout because build never succeeds within the short timeframe
		assert.Equal(t, "timeout", result.Status)
		assert.Contains(t, result.Notes, "Human intervention timed out")
		assert.True(t, result.Duration > 0, "should record execution duration")
		assert.False(t, result.FinishedAt.IsZero(), "should have finish time")
	})

	t.Run("llm-exec branch handles file system errors gracefully", func(t *testing.T) {
		orchestrator := &fanoutOrchestrator{
			runner: &MockProductionBranchRunner{LLMExecAssetsError: nil, LLMExecAssetsPath: "/nonexistent/path/file.hcl"},
		}

		branch := BranchSpec{ID: "llm-fs-error-test", Type: "llm-exec"}

		result := orchestrator.executeLLMExecBranch(context.Background(), branch, BranchResult{ID: branch.ID, StartedAt: time.Now(), Status: "failed"})

		// Should fail with appropriate error message about file system issues
		assert.Equal(t, "failed", result.Status)
		assert.Contains(t, result.Notes, "failed to substitute HCL template")
		assert.True(t, result.Duration > 0, "should record duration even on error")
	})

	t.Run("orw-gen branch handles malformed recipe configuration", func(t *testing.T) {
		orchestrator := &fanoutOrchestrator{runner: &MockProductionBranchRunner{ORWApplyAssetsError: nil, ORWApplyAssetsPath: "/tmp/test-malformed.hcl"}}

		testHCL := "job \"orw-malformed\" {}"
		tempFile := "/tmp/test-malformed.hcl"
		err := os.WriteFile(tempFile, []byte(testHCL), 0644)
		if err != nil {
			t.Fatalf("failed to create test HCL file: %v", err)
		}
		defer func() { _ = os.Remove(tempFile) }()

		branch := BranchSpec{ID: "orw-malformed-test", Type: "orw-gen", Inputs: map[string]interface{}{"recipe_config": "not-a-map"}}

		result := orchestrator.executeORWGenBranch(context.Background(), branch, BranchResult{ID: branch.ID, StartedAt: time.Now(), Status: "failed"})

		// Should handle malformed config gracefully by using defaults
		assert.Equal(t, "failed", result.Status)
		assert.Contains(t, result.Notes, "ORW apply job failed")
	})

	t.Run("fanout orchestration cancels other branches when first succeeds", func(t *testing.T) {
		mockSubmitter := &MockJobSubmitter{
			JobResults:    make(map[string]JobResult),
			ArtifactPaths: make(map[string]string),
			JobDelays:     make(map[string]time.Duration),
		}

		// Make first branch succeed quickly, second would succeed slowly
		mockSubmitter.JobResults["fast-winner"] = JobResult{JobID: "fast-1", Status: "completed", Output: "quick success"}
		mockSubmitter.JobResults["slow-branch"] = JobResult{JobID: "slow-1", Status: "completed", Output: "would succeed but cancelled"}
		mockSubmitter.JobDelays["slow-branch"] = 500 * time.Millisecond

		orchestrator := NewFanoutOrchestrator(mockSubmitter)

		branches := []BranchSpec{{ID: "fast-winner", Type: "llm-exec"}, {ID: "slow-branch", Type: "llm-exec"}}

		winner, results, err := orchestrator.RunHealingFanout(context.Background(), nil, branches, 2)

		// Should succeed with fast winner
		assert.NoError(t, err)
		assert.Equal(t, "fast-winner", winner.ID)
		assert.Equal(t, "completed", winner.Status)
		assert.Len(t, results, 2)

		// Verify that other branches were cancelled or didn't complete
		nonWinnerResults := make([]BranchResult, 0)
		for _, result := range results {
			if result.ID != winner.ID {
				nonWinnerResults = append(nonWinnerResults, result)
			}
		}
		assert.Greater(t, len(nonWinnerResults), 0)
		for _, result := range nonWinnerResults {
			assert.NotEqual(t, "completed", result.Status, "non-winner branches should be cancelled")
		}
	})
}
