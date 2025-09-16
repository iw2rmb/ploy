package mods

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test job submission helpers (planner)
func TestSubmitPlannerJob(t *testing.T) {
	tests := []struct {
		name          string
		config        *ModConfig
		buildError    string
		expectError   bool
		expectJobType string
	}{
		{
			name: "successful planner submission",
			config: &ModConfig{
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
			config: &ModConfig{
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
