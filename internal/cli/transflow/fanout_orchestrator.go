package transflow

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// fanoutOrchestrator implements the FanoutOrchestrator interface
type fanoutOrchestrator struct {
	submitter interface{} // MockJobSubmitter in tests, real submitter in production
}

// NewFanoutOrchestrator creates a new fanout orchestrator
func NewFanoutOrchestrator(submitter interface{}) FanoutOrchestrator {
	return &fanoutOrchestrator{
		submitter: submitter,
	}
}

// RunHealingFanout executes parallel healing branches with first-success-wins semantics
func (o *fanoutOrchestrator) RunHealingFanout(ctx context.Context, runCtx interface{}, branches []BranchSpec, maxParallel int) (BranchResult, []BranchResult, error) {
	if len(branches) == 0 {
		return BranchResult{}, nil, fmt.Errorf("no branches to execute")
	}

	// Create context for cancellation when first branch succeeds
	fanoutCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Channel to receive results
	resultCh := make(chan BranchResult, len(branches))
	var wg sync.WaitGroup

	// Semaphore to limit parallelism
	sem := make(chan struct{}, maxParallel)

	// Launch all branches
	for _, branch := range branches {
		wg.Add(1)
		go func(b BranchSpec) {
			defer wg.Done()

			// Acquire semaphore slot
			select {
			case sem <- struct{}{}:
			case <-fanoutCtx.Done():
				// Context cancelled, branch doesn't execute
				resultCh <- BranchResult{
					ID:     b.ID,
					Status: "cancelled",
					Notes:  "cancelled before execution",
				}
				return
			}
			defer func() { <-sem }() // Release semaphore slot

			// Execute the branch
			result := o.executeBranch(fanoutCtx, b)
			resultCh <- result

			// If this branch succeeded and context isn't cancelled, cancel others
			if result.Status == "completed" {
				cancel()
			}
		}(branch)
	}

	// Collect results
	var allResults []BranchResult
	var winner BranchResult
	foundWinner := false

	// Wait for all branches to complete or be cancelled
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	for result := range resultCh {
		allResults = append(allResults, result)

		// First successful result becomes the winner
		if result.Status == "completed" && !foundWinner {
			winner = result
			foundWinner = true
		}
	}

	if !foundWinner {
		return BranchResult{}, allResults, fmt.Errorf("all branches failed, no winner")
	}

	return winner, allResults, nil
}

// executeBranch executes a single branch and returns the result
func (o *fanoutOrchestrator) executeBranch(ctx context.Context, branch BranchSpec) BranchResult {
	startTime := time.Now()

	result := BranchResult{
		ID:        branch.ID,
		Status:    "failed", // Default to failed
		StartedAt: startTime,
	}

	// Check if context is already cancelled
	select {
	case <-ctx.Done():
		result.Status = "cancelled"
		result.Notes = "context cancelled"
		result.FinishedAt = time.Now()
		result.Duration = time.Since(startTime)
		return result
	default:
	}

	// Use type assertion to check if this is a test submitter
	if testSubmitter, ok := o.submitter.(interface {
		SubmitAndWaitTerminal(ctx context.Context, spec JobSpec) (JobResult, error)
	}); ok {
		spec := JobSpec{
			Name:    branch.ID,
			Type:    branch.Type,
			Inputs:  branch.Inputs,
			Timeout: 30 * time.Minute, // Default timeout
		}

		jobResult, err := testSubmitter.SubmitAndWaitTerminal(ctx, spec)
		result.FinishedAt = time.Now()
		result.Duration = time.Since(startTime)

		if err != nil {
			result.Status = "failed"
			result.Notes = fmt.Sprintf("job execution failed: %v", err)
		} else {
			result.JobID = jobResult.JobID
			result.Status = jobResult.Status
			result.Notes = jobResult.Output
		}

		return result
	}

	// For production implementation, would use actual job submission
	result.Status = "failed"
	result.Notes = "production job submission not implemented yet"
	result.FinishedAt = time.Now()
	result.Duration = time.Since(startTime)
	return result
}
