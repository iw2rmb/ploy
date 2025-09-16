package mods

import (
	"context"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/common"
	"github.com/iw2rmb/ploy/internal/git/provider"
	"github.com/stretchr/testify/assert"
)

// Test human-step branch implementation (current behavior)
func TestHumanStepBranchCurrentBehavior(t *testing.T) {
	// This test documents the current behavior - human-step immediately fails
	// Once we implement proper human-step functionality, this test should be updated

	orchestrator := &fanoutOrchestrator{}

	branchSpec := BranchSpec{
		ID:   "human-intervention-test",
		Type: "human-step",
		Inputs: map[string]interface{}{
			"timeout":    "5m",
			"buildError": "Build failed: test error",
		},
	}

	result := BranchResult{
		ID:        branchSpec.ID,
		StartedAt: time.Now(),
		Status:    "pending", // Will be changed by executeHumanStepBranch
	}

	ctx := context.Background()
	actualResult := orchestrator.executeHumanStepBranch(ctx, branchSpec, result)

	// Current behavior - should fail with "requires production runner" in test mode
	assert.Equal(t, "failed", actualResult.Status, "human-step should fail in test mode")
	assert.Contains(t, actualResult.Notes, "requires production runner", "should indicate production runner needed")
	assert.True(t, actualResult.Duration > 0, "should have recorded duration")
	assert.False(t, actualResult.FinishedAt.IsZero(), "should have finished time")
}

// Test human-step branch full workflow (expected behavior)
func TestHumanStepBranchFullWorkflow(t *testing.T) {
	t.Run("human-step creates MR with correct configuration", func(t *testing.T) {
		mockGitProvider := &MockGitProvider{
			MRResult: &provider.MRResult{
				MRURL:   "https://gitlab.com/test/repo/-/merge_requests/42",
				MRID:    42,
				Created: true,
			},
		}

		orchestrator := &fanoutOrchestrator{
			runner: &MockProductionBranchRunner{
				GitProviderMock:  mockGitProvider,
				BuildCheckerMock: &MockBuildChecker{},
				TargetRepo:       "https://gitlab.com/test/repo.git",
			},
		}

		branchSpec := BranchSpec{
			ID:   "human-fix-001",
			Type: "human-step",
			Inputs: map[string]interface{}{
				"timeout":    "15m",
				"buildError": "Compilation failed: undefined method 'getFoo()'",
			},
		}

		result := BranchResult{
			ID:        branchSpec.ID,
			StartedAt: time.Now(),
			Status:    "pending",
		}

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		actualResult := orchestrator.executeHumanStepBranch(ctx, branchSpec, result)

		// Expected behavior - MR should be created
		assert.True(t, mockGitProvider.MRCalled, "MR should be created")
		assert.Equal(t, "https://gitlab.com/test/repo.git", mockGitProvider.MRConfig.RepoURL, "should use correct repo URL")
		assert.Equal(t, "human-intervention-human-fix-001", mockGitProvider.MRConfig.SourceBranch, "should create intervention branch")
		assert.Equal(t, "main", mockGitProvider.MRConfig.TargetBranch, "should target main branch")
		assert.Contains(t, mockGitProvider.MRConfig.Title, "Human Intervention Required", "MR title should indicate intervention needed")
		assert.Contains(t, mockGitProvider.MRConfig.Description, "getFoo()", "MR description should contain build error")
		assert.Contains(t, mockGitProvider.MRConfig.Labels, "human-intervention", "should have human-intervention label")

		// Will timeout since no human fixes the build in test mode
		assert.Equal(t, "timeout", actualResult.Status, "should timeout waiting for human fix")
		assert.Contains(t, actualResult.Notes, "timed out", "should indicate timeout occurred")
	})

	t.Run("human-step detects successful manual fix", func(t *testing.T) {
		mockBuildChecker := &MockBuildChecker{
			BuildResult: &common.DeployResult{Success: true}, // Simulate human fixing the build
		}

		mockGitProvider := &MockGitProvider{
			MRResult: &provider.MRResult{
				MRURL:   "https://gitlab.com/test/repo/-/merge_requests/43",
				MRID:    43,
				Created: true,
			},
		}

		orchestrator := &fanoutOrchestrator{
			runner: &MockProductionBranchRunner{
				GitProviderMock:  mockGitProvider,
				BuildCheckerMock: mockBuildChecker,
				TargetRepo:       "https://gitlab.com/test/repo.git",
			},
		}

		branchSpec := BranchSpec{
			ID:   "human-fix-002",
			Type: "human-step",
			Inputs: map[string]interface{}{
				"timeout":    "30s", // Short timeout for test
				"buildError": "Build failed: missing dependency",
			},
		}

		result := BranchResult{
			ID:        branchSpec.ID,
			StartedAt: time.Now(),
			Status:    "pending",
		}

		ctx := context.Background()
		actualResult := orchestrator.executeHumanStepBranch(ctx, branchSpec, result)

		// Expected behavior - should detect successful fix
		assert.Equal(t, "completed", actualResult.Status, "should complete when build succeeds")
		assert.Contains(t, actualResult.Notes, "Human intervention successful", "should indicate success")
		assert.Contains(t, actualResult.Notes, mockGitProvider.MRResult.MRURL, "should reference MR URL")
		assert.True(t, actualResult.Duration > 0, "should record duration")
		assert.False(t, actualResult.FinishedAt.IsZero(), "should have finish time")
	})
}
