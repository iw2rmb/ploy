package mods

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/iw2rmb/ploy/internal/git/provider"
)

// ProductionBranchRunner defines the interface for production branch job execution
type ProductionBranchRunner interface {
	RenderLLMExecAssets(optionID string) (string, error)
	RenderORWApplyAssets(optionID string) (string, error)

	// Human-step branch support
	GetGitProvider() provider.GitProvider
	GetBuildChecker() BuildCheckerInterface
	GetWorkspaceDir() string
	GetTargetRepo() string
	GetEventReporter() EventReporter
	GetArtifactUploader() ArtifactUploader
}

// fanoutOrchestrator implements the FanoutOrchestrator interface
type fanoutOrchestrator struct {
	submitter JobSubmitter           // Mock in tests, real in production (Noop enables healing)
	runner    ProductionBranchRunner // For accessing asset rendering methods in production
	hcl       HCLSubmitter           // For HCL validate/submit in production
}

// NewFanoutOrchestrator creates a new fanout orchestrator
func NewFanoutOrchestrator(submitter JobSubmitter) FanoutOrchestrator {
	return &fanoutOrchestrator{submitter: submitter, runner: nil, hcl: NewDefaultHCLSubmitter(nil)}
}

// NewFanoutOrchestratorWithRunner creates a new fanout orchestrator with runner access for production
func NewFanoutOrchestratorWithRunner(submitter JobSubmitter, runner ProductionBranchRunner) FanoutOrchestrator {
	return &fanoutOrchestrator{submitter: submitter, runner: runner, hcl: NewDefaultHCLSubmitter(nil)}
}

// SetHCLSubmitterForFanout attempts to inject a custom HCLSubmitter into the
// provided FanoutOrchestrator. Returns true if the underlying implementation
// is the built-in fanout orchestrator and the submitter was set; false otherwise.
func SetHCLSubmitterForFanout(fo FanoutOrchestrator, h HCLSubmitter) bool {
	if f, ok := fo.(*fanoutOrchestrator); ok {
		f.hcl = h
		return true
	}
	return false
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
		Type:      branch.Type,
		Status:    "failed", // Default to failed
		StartedAt: startTime,
	}

	// Emit branch start event if reporter available
	if o.runner != nil && o.runner.GetEventReporter() != nil {
		_ = o.runner.GetEventReporter().Report(ctx, Event{Phase: "fanout", Step: string(NormalizeStepType(branch.Type)), Level: "info", Message: "branch started: " + branch.ID, Time: time.Now()})
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

	// Check if this is a test submitter (backward compatibility)
	if o.submitter != nil {
		// Choose sensible default timeouts based on branch type
		var tmo time.Duration
		switch branch.Type {
		case string(StepTypeLLMExec):
			tmo = ResolveDefaultsFromEnv().LLMExecTimeout
		case string(StepTypeORWGen):
			tmo = ResolveDefaultsFromEnv().ORWApplyTimeout
		default:
			tmo = ResolveDefaultsFromEnv().BuildApplyTimeout
		}
		spec := JobSpec{
			Name:    branch.ID,
			Type:    branch.Type,
			Inputs:  branch.Inputs,
			Timeout: tmo,
		}
		jobResult, err := o.submitter.SubmitAndWaitTerminal(ctx, spec)
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
		if o.runner != nil && o.runner.GetEventReporter() != nil {
			lvl := "info"
			if result.Status != "completed" {
				lvl = "error"
			}
			_ = o.runner.GetEventReporter().Report(ctx, Event{Phase: "fanout", Step: string(NormalizeStepType(branch.Type)), Level: lvl, Message: fmt.Sprintf("branch %s finished: %s", branch.ID, result.Status), Time: time.Now()})
		}
		return result
	}

	// Production implementation using real Nomad job submission
	if o.runner != nil {
		switch branch.Type {
		case string(StepTypeLLMExec):
			return o.executeLLMExecBranch(ctx, branch, result)
		case string(StepTypeORWGen):
			return o.executeORWGenBranch(ctx, branch, result)
		case string(StepTypeHumanStep):
			return o.executeHumanStepBranch(ctx, branch, result)
		default:
			result.Status = "failed"
			result.Notes = fmt.Sprintf("unsupported branch type: %s", branch.Type)
			result.FinishedAt = time.Now()
			result.Duration = time.Since(startTime)
			return result
		}
	}

	// No runner provided and not a test submitter
	result.Status = "failed"
	result.Notes = "no production runner or test submitter available for job submission"
	result.FinishedAt = time.Now()
	result.Duration = time.Since(startTime)
	return result
}
