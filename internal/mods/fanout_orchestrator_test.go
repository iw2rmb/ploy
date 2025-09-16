package mods

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

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
						JobID:  "job-fail-" + branch.ID,
						Status: "failed",
						Output: "failed",
					}
				}
			}

			orchestrator := NewFanoutOrchestrator(mockSubmitter)

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
