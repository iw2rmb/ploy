package transflow

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Implementation of job submission helpers for the transflow healing workflow.
// This provides the GREEN phase implementation for the failing tests.

// jobSubmissionHelper implements the JobSubmissionHelper interface
type jobSubmissionHelper struct {
	submitter interface{} // MockJobSubmitter in tests, real submitter in production
}

// NewJobSubmissionHelper creates a new job submission helper
func NewJobSubmissionHelper(submitter interface{}) JobSubmissionHelper {
	return &jobSubmissionHelper{
		submitter: submitter,
	}
}

// SubmitPlannerJob submits a planner job after a build failure
func (h *jobSubmissionHelper) SubmitPlannerJob(ctx context.Context, config *TransflowConfig, buildError string, workspace string) (*PlanResult, error) {
	// Use type assertion to check if this is a test submitter
	// In production, would use a different interface/implementation
	if testSubmitter, ok := h.submitter.(interface {
		SubmitAndWaitTerminal(ctx context.Context, spec JobSpec) (JobResult, error)
	}); ok {
		spec := JobSpec{
			Name:    "planner",
			Type:    "planner",
			HCLPath: "", // Would be set from rendered assets
			EnvVars: map[string]string{
				"BUILD_ERROR": buildError,
				"TARGET_REPO": config.TargetRepo,
				"BASE_REF":    config.BaseRef,
			},
			Timeout: 15 * time.Minute,
			Inputs: map[string]interface{}{
				"workspace": workspace,
			},
		}

		result, err := testSubmitter.SubmitAndWaitTerminal(ctx, spec)
		if err != nil {
			return nil, fmt.Errorf("planner job failed: %w", err)
		}

		// Parse the planner output
		var planResult PlanResult
		if err := json.Unmarshal([]byte(result.Output), &planResult); err != nil {
			return nil, fmt.Errorf("failed to parse planner output: %w", err)
		}

		return &planResult, nil
	}

	// For production implementation, would use actual job submission
	return nil, fmt.Errorf("production job submission not implemented yet")
}

// SubmitReducerJob submits a reducer job to determine the next action
func (h *jobSubmissionHelper) SubmitReducerJob(ctx context.Context, planID string, results []BranchResult, winner *BranchResult, workspace string) (*NextAction, error) {
	// Use type assertion to check if this is a test submitter
	if testSubmitter, ok := h.submitter.(interface {
		SubmitAndWaitTerminal(ctx context.Context, spec JobSpec) (JobResult, error)
	}); ok {
		spec := JobSpec{
			Name:    "reducer",
			Type:    "reducer",
			HCLPath: "", // Would be set from rendered assets
			EnvVars: map[string]string{
				"PLAN_ID": planID,
			},
			Timeout: 10 * time.Minute,
			Inputs: map[string]interface{}{
				"workspace": workspace,
				"results":   results,
				"winner":    winner,
			},
		}

		result, err := testSubmitter.SubmitAndWaitTerminal(ctx, spec)
		if err != nil {
			return nil, fmt.Errorf("reducer job failed: %w", err)
		}

		// Parse the reducer output
		var nextAction NextAction
		if err := json.Unmarshal([]byte(result.Output), &nextAction); err != nil {
			return nil, fmt.Errorf("failed to parse reducer output: %w", err)
		}

		return &nextAction, nil
	}

	// For production implementation, would use actual job submission
	return nil, fmt.Errorf("production job submission not implemented yet")
}
