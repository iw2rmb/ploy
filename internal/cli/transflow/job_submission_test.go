package transflow

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock job submission interfaces for testing - move these to top of file
// to be available to all parts of the package
type MockJobSubmitter struct {
	SubmitError   error
	WaitError     error
	CollectError  error
	SubmitCalled  bool
	WaitCalled    bool
	CollectCalled bool
	SubmittedJobs []JobSpec
	JobResults    map[string]JobResult
	ArtifactPaths map[string]string
}

// JobSpec and JobResult types are now defined in types.go

func (m *MockJobSubmitter) SubmitAndWaitTerminal(ctx context.Context, spec JobSpec) (JobResult, error) {
	m.SubmitCalled = true
	m.SubmittedJobs = append(m.SubmittedJobs, spec)

	if m.SubmitError != nil {
		return JobResult{}, m.SubmitError
	}

	// Return mock result
	result := JobResult{
		JobID:    fmt.Sprintf("job-%s-%d", spec.Name, time.Now().Unix()),
		Status:   "completed",
		Duration: 30 * time.Second,
	}

	if mockResult, exists := m.JobResults[spec.Name]; exists {
		result = mockResult
	}

	return result, m.WaitError
}

func (m *MockJobSubmitter) CollectArtifacts(ctx context.Context, jobID string, outputDir string) (map[string]string, error) {
	m.CollectCalled = true

	if m.CollectError != nil {
		return nil, m.CollectError
	}

	if artifacts, exists := m.ArtifactPaths[jobID]; exists {
		return map[string]string{"plan.json": artifacts}, nil
	}

	return map[string]string{}, nil
}

// Test job submission helpers
func TestSubmitPlannerJob(t *testing.T) {
	tests := []struct {
		name          string
		config        *TransflowConfig
		buildError    string
		expectError   bool
		expectJobType string
	}{
		{
			name: "successful planner submission",
			config: &TransflowConfig{
				ID:         "test-workflow",
				TargetRepo: "https://github.com/test/repo",
				BaseRef:    "main",
			},
			buildError:    "compilation failed: undefined symbol",
			expectError:   false,
			expectJobType: "planner",
		},
		{
			name: "planner submission with job failure",
			config: &TransflowConfig{
				ID:         "failing-workflow",
				TargetRepo: "https://github.com/test/repo",
				BaseRef:    "main",
			},
			buildError:    "build timeout",
			expectError:   true,
			expectJobType: "planner",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This test will fail until we implement SubmitPlannerJob
			ctx := context.Background()
			mockSubmitter := &MockJobSubmitter{
				JobResults:    make(map[string]JobResult),
				ArtifactPaths: make(map[string]string),
			}

			if tt.expectError {
				mockSubmitter.SubmitError = fmt.Errorf("job submission failed")
			} else {
				mockSubmitter.JobResults["planner"] = JobResult{
					JobID:  "planner-123",
					Status: "completed",
					Output: `{"plan_id": "test-plan", "options": [{"id": "opt1", "type": "llm-exec"}]}`,
				}
				mockSubmitter.ArtifactPaths["planner-123"] = "/tmp/plan.json"
			}

			submitter := NewJobSubmissionHelper(mockSubmitter)

			// This function doesn't exist yet - will cause compilation failure
			plan, err := submitter.SubmitPlannerJob(ctx, tt.config, tt.buildError, "/tmp/workspace")

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, plan)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, plan)
				assert.Equal(t, "test-plan", plan.PlanID)
				assert.Len(t, plan.Options, 1)
			}

			assert.True(t, mockSubmitter.SubmitCalled)
			require.Len(t, mockSubmitter.SubmittedJobs, 1)
			assert.Equal(t, tt.expectJobType, mockSubmitter.SubmittedJobs[0].Type)
		})
	}
}

func TestSubmitReducerJob(t *testing.T) {
	tests := []struct {
		name          string
		planID        string
		branchResults []BranchResult
		winner        *BranchResult
		expectError   bool
	}{
		{
			name:   "successful reducer submission",
			planID: "test-plan-123",
			branchResults: []BranchResult{
				{ID: "branch-1", Status: "completed", Notes: "success"},
				{ID: "branch-2", Status: "failed", Notes: "timeout"},
			},
			winner:      &BranchResult{ID: "branch-1", Status: "completed"},
			expectError: false,
		},
		{
			name:   "reducer with all failed branches",
			planID: "failed-plan-456",
			branchResults: []BranchResult{
				{ID: "branch-1", Status: "failed", Notes: "error"},
				{ID: "branch-2", Status: "failed", Notes: "timeout"},
			},
			winner:      nil,
			expectError: false, // Reducer should still process failure case
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			mockSubmitter := &MockJobSubmitter{
				JobResults:    make(map[string]JobResult),
				ArtifactPaths: make(map[string]string),
			}

			// Mock reducer result
			mockSubmitter.JobResults["reducer"] = JobResult{
				JobID:  "reducer-789",
				Status: "completed",
				Output: `{"action": "stop", "notes": "workflow completed"}`,
			}
			mockSubmitter.ArtifactPaths["reducer-789"] = "/tmp/next.json"

			submitter := NewJobSubmissionHelper(mockSubmitter)

			// This function doesn't exist yet - will cause compilation failure
			nextAction, err := submitter.SubmitReducerJob(ctx, tt.planID, tt.branchResults, tt.winner, "/tmp/workspace")

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, nextAction)
				assert.Equal(t, "stop", nextAction.Action)
			}

			assert.True(t, mockSubmitter.SubmitCalled)
			require.Len(t, mockSubmitter.SubmittedJobs, 1)
			assert.Equal(t, "reducer", mockSubmitter.SubmittedJobs[0].Type)
		})
	}
}

// Test fanout orchestration
func TestRunHealingFanout(t *testing.T) {
	tests := []struct {
		name         string
		branches     []BranchSpec
		maxParallel  int
		expectWinner bool
		expectError  bool
	}{
		{
			name: "successful fanout with winner",
			branches: []BranchSpec{
				{ID: "human-step", Type: "human", Inputs: map[string]interface{}{"timeout": "1h"}},
				{ID: "llm-fix", Type: "llm-exec", Inputs: map[string]interface{}{"model": "gpt-4"}},
				{ID: "orw-gen", Type: "orw-generated", Inputs: map[string]interface{}{"recipe": "java17"}},
			},
			maxParallel:  3,
			expectWinner: true,
			expectError:  false,
		},
		{
			name: "fanout with all branches failing",
			branches: []BranchSpec{
				{ID: "llm-fix", Type: "llm-exec", Inputs: map[string]interface{}{"model": "gpt-4"}},
				{ID: "orw-gen", Type: "orw-generated", Inputs: map[string]interface{}{"recipe": "java17"}},
			},
			maxParallel:  2,
			expectWinner: false,
			expectError:  true,
		},
		{
			name: "fanout with parallelism limit",
			branches: []BranchSpec{
				{ID: "branch-1", Type: "llm-exec"},
				{ID: "branch-2", Type: "llm-exec"},
				{ID: "branch-3", Type: "llm-exec"},
				{ID: "branch-4", Type: "llm-exec"},
			},
			maxParallel:  2, // Should limit to 2 concurrent
			expectWinner: true,
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			mockSubmitter := &MockJobSubmitter{
				JobResults:    make(map[string]JobResult),
				ArtifactPaths: make(map[string]string),
			}

			// Configure mock results based on test expectations
			if tt.expectWinner {
				// Make first branch succeed
				mockSubmitter.JobResults[tt.branches[0].ID] = JobResult{
					JobID:  "job-success-123",
					Status: "completed",
					Output: "success",
				}
			} else {
				// Make all branches fail
				for _, branch := range tt.branches {
					mockSubmitter.JobResults[branch.ID] = JobResult{
						JobID:  fmt.Sprintf("job-fail-%s", branch.ID),
						Status: "failed",
						Output: "failed",
					}
				}
			}

			orchestrator := NewFanoutOrchestrator(mockSubmitter)

			// This function doesn't exist yet - will cause compilation failure
			winner, results, err := orchestrator.RunHealingFanout(ctx, nil, tt.branches, tt.maxParallel)

			if tt.expectError {
				assert.Error(t, err)
				assert.Equal(t, BranchResult{}, winner)
			} else {
				assert.NoError(t, err)
				if tt.expectWinner {
					assert.NotEqual(t, BranchResult{}, winner)
					assert.Equal(t, "completed", winner.Status)
				}
			}

			assert.Len(t, results, len(tt.branches))

			// Verify parallelism was respected
			if len(tt.branches) > tt.maxParallel {
				// Should have submitted at most maxParallel jobs
				assert.LessOrEqual(t, len(mockSubmitter.SubmittedJobs), tt.maxParallel)
			}
		})
	}
}

// Test runner integration
func TestTransflowRunnerWithHealing(t *testing.T) {
	t.Run("healing triggered on build failure", func(t *testing.T) {
		// Setup
		config := &TransflowConfig{
			ID:         "healing-test",
			TargetRepo: "https://github.com/test/repo",
			BaseRef:    "main",
			SelfHeal: &SelfHealConfig{
				MaxRetries: 2,
				Enabled:    true,
			},
			Steps: []TransflowStep{
				{Type: "recipe", ID: "java-migration", Engine: "openrewrite", Recipes: []string{"Java11to17"}},
			},
		}

		// Mocks
		mockGit := &MockGitOperations{}
		mockRecipe := &MockRecipeExecutor{}
		mockBuild := &MockBuildChecker{
			// First build fails, triggering healing
			BuildError: fmt.Errorf("compilation failed: undefined symbol"),
		}
		mockJobSubmitter := &MockJobSubmitter{
			JobResults: map[string]JobResult{
				"planner": {JobID: "plan-123", Status: "completed", Output: `{"plan_id": "p1", "options": [{"id": "llm1", "type": "llm-exec"}]}`},
				"llm1":    {JobID: "llm-123", Status: "completed", Output: "diff applied successfully"},
				"reducer": {JobID: "red-123", Status: "completed", Output: `{"action": "stop", "notes": "healed"}`},
			},
		}

		runner, err := NewTransflowRunner(config, "/tmp/workspace")
		require.NoError(t, err)

		runner.SetGitOperations(mockGit)
		runner.SetRecipeExecutor(mockRecipe)
		runner.SetBuildChecker(mockBuild)

		// This integration doesn't exist yet - will need to add healing to runner
		runner.SetJobSubmitter(mockJobSubmitter)

		// Execute
		ctx := context.Background()
		result, err := runner.Run(ctx)

		// Verify healing was attempted
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.NotNil(t, result.HealingSummary)
		assert.True(t, result.HealingSummary.Enabled)
		assert.Greater(t, result.HealingSummary.AttemptsCount, 0)

		// Verify job submissions happened
		assert.True(t, mockJobSubmitter.SubmitCalled)
		assert.Greater(t, len(mockJobSubmitter.SubmittedJobs), 0)
	})
}

// All types and interfaces are now defined in types.go and implemented in job_submission.go and fanout_orchestrator.go
