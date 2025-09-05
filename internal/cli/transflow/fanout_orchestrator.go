package transflow

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/iw2rmb/ploy/internal/orchestration"
)

// ProductionBranchRunner defines the interface for production branch job execution
type ProductionBranchRunner interface {
	RenderLLMExecAssets(optionID string) (string, error)
	RenderORWApplyAssets(optionID string) (string, error)
}

// fanoutOrchestrator implements the FanoutOrchestrator interface
type fanoutOrchestrator struct {
	submitter interface{} // MockJobSubmitter in tests, real submitter in production
	runner    ProductionBranchRunner // For accessing asset rendering methods in production
}

// NewFanoutOrchestrator creates a new fanout orchestrator
func NewFanoutOrchestrator(submitter interface{}) FanoutOrchestrator {
	return &fanoutOrchestrator{
		submitter: submitter,
		runner:    nil, // Will be nil for mock tests
	}
}

// NewFanoutOrchestratorWithRunner creates a new fanout orchestrator with runner access for production
func NewFanoutOrchestratorWithRunner(submitter interface{}, runner ProductionBranchRunner) FanoutOrchestrator {
	return &fanoutOrchestrator{
		submitter: submitter,
		runner:    runner,
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

	// Check if this is a test submitter (backward compatibility)
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

	// Production implementation using real Nomad job submission
	if o.runner != nil {
		switch branch.Type {
		case "llm-exec":
			return o.executeLLMExecBranch(ctx, branch, result)
		case "orw-gen":
			return o.executeORWGenBranch(ctx, branch, result)
		case "human-step":
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

// executeLLMExecBranch executes an LLM-based code generation branch
func (o *fanoutOrchestrator) executeLLMExecBranch(ctx context.Context, branch BranchSpec, result BranchResult) BranchResult {
	// Step 1: Render LLM exec assets
	hclPath, err := o.runner.RenderLLMExecAssets(branch.ID)
	if err != nil {
		result.Status = "failed"
		result.Notes = fmt.Sprintf("failed to render LLM exec assets: %v", err)
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		return result
	}

	// Step 2: Generate unique run ID for this branch
	runID := fmt.Sprintf("llm-exec-%s-%d", branch.ID, time.Now().Unix())

	// Step 3: Substitute environment variables in HCL template
	renderedHCLPath, err := substituteHCLTemplate(hclPath, runID)
	if err != nil {
		result.Status = "failed"
		result.Notes = fmt.Sprintf("failed to substitute HCL template: %v", err)
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		return result
	}

	// Step 4: Submit job to Nomad and wait for completion
	timeout := 30 * time.Minute
	if err := orchestration.SubmitAndWaitTerminal(renderedHCLPath, timeout); err != nil {
		result.Status = "failed"
		result.Notes = fmt.Sprintf("LLM exec job failed: %v", err)
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		return result
	}

	// Step 5: Check for generated diff.patch artifact
	diffPath := filepath.Join(filepath.Dir(renderedHCLPath), "out", "diff.patch")
	if _, err := os.Stat(diffPath); err != nil {
		result.Status = "failed"
		result.Notes = fmt.Sprintf("LLM exec job completed but no diff.patch found: %v", err)
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		return result
	}

	result.Status = "completed"
	result.Notes = fmt.Sprintf("LLM exec job completed successfully, diff.patch at: %s", diffPath)
	result.FinishedAt = time.Now()
	result.Duration = time.Since(result.StartedAt)
	return result
}

// executeORWGenBranch executes an OpenRewrite recipe generation and application branch
func (o *fanoutOrchestrator) executeORWGenBranch(ctx context.Context, branch BranchSpec, result BranchResult) BranchResult {
	// Step 1: Render ORW apply assets
	hclPath, err := o.runner.RenderORWApplyAssets(branch.ID)
	if err != nil {
		result.Status = "failed"
		result.Notes = fmt.Sprintf("failed to render ORW apply assets: %v", err)
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		return result
	}

	// Step 2: Read HCL template and substitute recipe-specific variables
	hclBytes, err := os.ReadFile(hclPath)
	if err != nil {
		result.Status = "failed"
		result.Notes = fmt.Sprintf("failed to read ORW HCL template: %v", err)
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		return result
	}

	// Get recipe configuration from branch inputs
	rclass := ""
	rcoords := ""
	rtimeout := "10m"
	
	if inputs, ok := branch.Inputs["recipe_config"].(map[string]interface{}); ok {
		if class, ok := inputs["class"].(string); ok {
			rclass = class
		}
		if coords, ok := inputs["coords"].(string); ok {
			rcoords = coords
		}
		if timeout, ok := inputs["timeout"].(string); ok {
			rtimeout = timeout
		}
	}

	// Perform recipe-specific substitution
	rendered := strings.NewReplacer(
		"${RECIPE_CLASS}", rclass,
		"${RECIPE_COORDS}", rcoords,
		"${RECIPE_TIMEOUT}", rtimeout,
	).Replace(string(hclBytes))

	// Write substituted HCL to a new file
	renderedHCLPath := strings.ReplaceAll(hclPath, ".rendered.hcl", ".rendered.submitted.hcl")
	if err := os.WriteFile(renderedHCLPath, []byte(rendered), 0644); err != nil {
		result.Status = "failed"
		result.Notes = fmt.Sprintf("failed to write substituted ORW HCL: %v", err)
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		return result
	}

	// Step 3: Submit job to Nomad and wait for completion
	timeout := 30 * time.Minute
	if err := orchestration.SubmitAndWaitTerminal(renderedHCLPath, timeout); err != nil {
		result.Status = "failed"
		result.Notes = fmt.Sprintf("ORW apply job failed: %v", err)
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		return result
	}

	// Step 4: Check for generated diff.patch artifact
	diffPath := filepath.Join(filepath.Dir(renderedHCLPath), "out", "diff.patch")
	if _, err := os.Stat(diffPath); err != nil {
		result.Status = "failed"
		result.Notes = fmt.Sprintf("ORW apply job completed but no diff.patch found: %v", err)
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		return result
	}

	result.Status = "completed"
	result.Notes = fmt.Sprintf("ORW apply job completed successfully, diff.patch at: %s", diffPath)
	result.FinishedAt = time.Now()
	result.Duration = time.Since(result.StartedAt)
	return result
}

// executeHumanStepBranch handles human intervention branches (placeholder for now)
func (o *fanoutOrchestrator) executeHumanStepBranch(ctx context.Context, branch BranchSpec, result BranchResult) BranchResult {
	// Human step branches require manual intervention and are not automated
	// This would typically involve creating an issue/MR and waiting for human input
	result.Status = "failed"
	result.Notes = "human-step branches are not yet implemented - requires manual intervention workflow"
	result.FinishedAt = time.Now()
	result.Duration = time.Since(result.StartedAt)
	return result
}
