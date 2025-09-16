package mods

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
