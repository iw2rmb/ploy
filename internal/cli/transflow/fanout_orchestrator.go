package transflow

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/common"
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
}

// fanoutOrchestrator implements the FanoutOrchestrator interface
type fanoutOrchestrator struct {
	submitter JobSubmitter           // Mock in tests, real in production (Noop enables healing)
	runner    ProductionBranchRunner // For accessing asset rendering methods in production
	hcl       HCLSubmitter           // For HCL validate/submit in production
}

// NewFanoutOrchestrator creates a new fanout orchestrator
func NewFanoutOrchestrator(submitter JobSubmitter) FanoutOrchestrator {
	return &fanoutOrchestrator{submitter: submitter, runner: nil, hcl: DefaultHCLSubmitter{}}
}

// NewFanoutOrchestratorWithRunner creates a new fanout orchestrator with runner access for production
func NewFanoutOrchestratorWithRunner(submitter JobSubmitter, runner ProductionBranchRunner) FanoutOrchestrator {
	return &fanoutOrchestrator{submitter: submitter, runner: runner, hcl: DefaultHCLSubmitter{}}
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

	// Provide host directories for bind mounts during substitution
	// Use the directory of the HCL as context, and an 'out' subdir for outputs
	baseDir := filepath.Dir(hclPath)
	// Ensure out directory exists for bind mount target
	_ = os.MkdirAll(filepath.Join(baseDir, "out"), 0755)
	imgs := ResolveImagesFromEnv()
	infra := ResolveInfraFromEnv()
	llm := ResolveLLMDefaultsFromEnv()
	vars := map[string]string{
		"TRANSFLOW_CONTEXT_DIR":       baseDir,
		"TRANSFLOW_OUT_DIR":           filepath.Join(baseDir, "out"),
		"TRANSFLOW_REGISTRY":          imgs.Registry,
		"TRANSFLOW_PLANNER_IMAGE":     imgs.Planner,
		"TRANSFLOW_REDUCER_IMAGE":     imgs.Reducer,
		"TRANSFLOW_LLM_EXEC_IMAGE":    imgs.LLMExec,
		"PLOY_CONTROLLER":             infra.Controller,
		"PLOY_TRANSFLOW_EXECUTION_ID": os.Getenv("PLOY_TRANSFLOW_EXECUTION_ID"),
		"NOMAD_DC":                    infra.DC,
		"TRANSFLOW_MODEL":             llm.Model,
		"TRANSFLOW_TOOLS":             llm.ToolsJSON,
		"TRANSFLOW_LIMITS":            llm.LimitsJSON,
	}

	// Step 2: Generate unique run ID for this branch
	runID := LLMRunID(branch.ID)

	// Step 3: Extract MCP configuration from branch inputs
	var mcpConfig *MCPConfig = nil
	if mcpData, ok := branch.Inputs["mcp_config"]; ok {
		if mcpConfigMap, ok := mcpData.(map[string]interface{}); ok {
			// Convert map to MCPConfig struct
			if parsedMCP, err := parseMCPFromInputs(mcpConfigMap); err == nil {
				mcpConfig = parsedMCP
			}
		}
	}

	// Step 4: Substitute environment variables in HCL template with MCP support
	renderedHCLPath, err := substituteHCLTemplateWithMCPVars(hclPath, runID, vars, mcpConfig)
	if err != nil {
		result.Status = "failed"
		result.Notes = fmt.Sprintf("failed to substitute HCL template: %v", err)
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		return result
	}

	// Step 4: Report job metadata asynchronously (job name == runID)
	reportJobSubmittedAsync(ctx, o.runner.GetEventReporter(), runID, string(StepTypeLLMExec), string(StepTypeLLMExec))

	// Step 5: Preflight validate HCL, then submit job to Nomad and wait for completion
	if err := o.hcl.Validate(renderedHCLPath); err != nil {
		result.Status = "failed"
		result.Notes = fmt.Sprintf("LLM exec HCL validation failed: %v", err)
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		return result
	}
	timeout := ResolveDefaultsFromEnv().LLMExecTimeout
	if err := o.hcl.Submit(renderedHCLPath, timeout); err != nil {
		result.Status = "failed"
		result.Notes = fmt.Sprintf("LLM exec job failed: %v", err)
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		return result
	}

	// Step 6: Check for generated diff.patch artifact
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

// parseMCPFromInputs converts map[string]interface{} to MCPConfig struct
func parseMCPFromInputs(inputs map[string]interface{}) (*MCPConfig, error) {
	config := &MCPConfig{}

	// Parse tools
	if toolsData, ok := inputs["tools"]; ok {
		if toolsList, ok := toolsData.([]interface{}); ok {
			for _, toolData := range toolsList {
				if toolMap, ok := toolData.(map[string]interface{}); ok {
					tool := MCPTool{}
					if name, ok := toolMap["name"].(string); ok {
						tool.Name = name
					}
					if endpoint, ok := toolMap["endpoint"].(string); ok {
						tool.Endpoint = endpoint
					}
					if configData, ok := toolMap["config"].(map[string]interface{}); ok {
						tool.Config = make(map[string]string)
						for k, v := range configData {
							if vStr, ok := v.(string); ok {
								tool.Config[k] = vStr
							}
						}
					}
					config.Tools = append(config.Tools, tool)
				}
			}
		}
	}

	// Parse context
	if contextData, ok := inputs["context"]; ok {
		if contextList, ok := contextData.([]interface{}); ok {
			for _, ctxData := range contextList {
				if ctxStr, ok := ctxData.(string); ok {
					config.Context = append(config.Context, ctxStr)
				}
			}
		}
	}

	// Parse prompts
	if promptsData, ok := inputs["prompts"]; ok {
		if promptsList, ok := promptsData.([]interface{}); ok {
			for _, promptData := range promptsList {
				if promptStr, ok := promptData.(string); ok {
					config.Prompts = append(config.Prompts, promptStr)
				}
			}
		}
	}

	// Parse model
	if model, ok := inputs["model"].(string); ok {
		config.Model = model
	}

	// Parse budgets
	if budgetsData, ok := inputs["budgets"]; ok {
		if budgetsMap, ok := budgetsData.(map[string]interface{}); ok {
			if maxTokens, ok := budgetsMap["max_tokens"].(int); ok {
				config.Budgets.MaxTokens = maxTokens
			}
			if maxCost, ok := budgetsMap["max_cost"].(int); ok {
				config.Budgets.MaxCost = maxCost
			}
			if timeout, ok := budgetsMap["timeout"].(string); ok {
				config.Budgets.Timeout = timeout
			}
		}
	}

	return config, nil
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

	// Perform recipe-specific substitution first, then apply environment/template substitution
	prePath := strings.ReplaceAll(hclPath, ".rendered.hcl", ".pre.hcl")
	preContent := strings.NewReplacer(
		"${RECIPE_CLASS}", rclass,
		"${RECIPE_COORDS}", rcoords,
		"${RECIPE_TIMEOUT}", rtimeout,
	).Replace(string(hclBytes))
	if err := os.WriteFile(prePath, []byte(preContent), 0644); err != nil {
		result.Status = "failed"
		result.Notes = fmt.Sprintf("failed to write pre-substituted ORW HCL: %v", err)
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		return result
	}

	// Provide host directories for bind mounts (no global env)
	baseDir := filepath.Dir(hclPath)
	_ = os.MkdirAll(filepath.Join(baseDir, "out"), 0755)
	imgs := ResolveImagesFromEnv()
	infra := ResolveInfraFromEnv()
	vars := map[string]string{
		"TRANSFLOW_CONTEXT_DIR":       baseDir,
		"TRANSFLOW_OUT_DIR":           filepath.Join(baseDir, "out"),
		"PLOY_CONTROLLER":             infra.Controller,
		"PLOY_TRANSFLOW_EXECUTION_ID": os.Getenv("PLOY_TRANSFLOW_EXECUTION_ID"),
		"PLOY_SEAWEEDFS_URL":          infra.SeaweedURL,
		"TRANSFLOW_DIFF_KEY":          os.Getenv("TRANSFLOW_DIFF_KEY"),
		"TRANSFLOW_ORW_APPLY_IMAGE":   imgs.ORWApply,
		"TRANSFLOW_REGISTRY":          imgs.Registry,
		"NOMAD_DC":                    infra.DC,
	}

	// Step 2b: Substitute environment variables in HCL template
	runID := ORWRunID(branch.ID)
	renderedHCLPath, err := substituteORWTemplateVars(prePath, runID, vars)
	if err != nil {
		result.Status = "failed"
		result.Notes = fmt.Sprintf("failed to substitute ORW HCL template: %v", err)
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		return result
	}

	// Step 3: Report job metadata asynchronously (job name == runID)
	reportJobSubmittedAsync(ctx, o.runner.GetEventReporter(), runID, "apply", string(StepTypeORWApply))

	// Step 4: Preflight validate HCL, then submit job to Nomad and wait for completion
	if err := o.hcl.Validate(renderedHCLPath); err != nil {
		result.Status = "failed"
		result.Notes = fmt.Sprintf("ORW apply HCL validation failed: %v", err)
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		return result
	}
	timeout := ResolveDefaultsFromEnv().ORWApplyTimeout
	if err := o.hcl.Submit(renderedHCLPath, timeout); err != nil {
		diffPath := filepath.Join(filepath.Dir(renderedHCLPath), "out", "diff.patch")
		if ResolveDefaultsFromEnv().AllowPartialORW {
			if fi, statErr := os.Stat(diffPath); statErr == nil && fi.Size() > 0 {
				// proceed (partial allowed)
			} else {
				result.Status = "failed"
				result.Notes = fmt.Sprintf("ORW apply job failed: %v", err)
				result.FinishedAt = time.Now()
				result.Duration = time.Since(result.StartedAt)
				return result
			}
		} else {
			result.Status = "failed"
			result.Notes = fmt.Sprintf("ORW apply job failed: %v", err)
			result.FinishedAt = time.Now()
			result.Duration = time.Since(result.StartedAt)
			if o.runner != nil && o.runner.GetEventReporter() != nil {
				_ = o.runner.GetEventReporter().Report(ctx, Event{Phase: "fanout", Step: string(NormalizeStepType(branch.Type)), Level: "error", Message: fmt.Sprintf("branch %s failed: %s", branch.ID, result.Notes), Time: time.Now()})
			}
			return result
		}
	}

	// Step 5: Check for generated diff.patch artifact
	diffPath := filepath.Join(filepath.Dir(renderedHCLPath), "out", "diff.patch")
	if _, err := os.Stat(diffPath); err != nil {
		result.Status = "failed"
		result.Notes = fmt.Sprintf("ORW apply job completed but no diff.patch found: %v", err)
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		if o.runner != nil && o.runner.GetEventReporter() != nil {
			_ = o.runner.GetEventReporter().Report(ctx, Event{Phase: "fanout", Step: string(NormalizeStepType(branch.Type)), Level: "info", Message: fmt.Sprintf("branch %s completed", branch.ID), Time: time.Now()})
		}
		return result
	}

	result.Status = "completed"
	result.Notes = fmt.Sprintf("ORW apply job completed successfully, diff.patch at: %s", diffPath)
	result.FinishedAt = time.Now()
	result.Duration = time.Since(result.StartedAt)
	return result
}

// executeHumanStepBranch handles human intervention branches with Git-based manual intervention workflow
func (o *fanoutOrchestrator) executeHumanStepBranch(ctx context.Context, branch BranchSpec, result BranchResult) BranchResult {
	// Parse timeout from branch inputs
	timeout := ResolveDefaultsFromEnv().BuildApplyTimeout // default timeout
	if timeoutStr, ok := branch.Inputs["timeout"].(string); ok {
		if parsedTimeout, err := time.ParseDuration(timeoutStr); err == nil {
			timeout = parsedTimeout
		}
	}

	// Create timeout context for this branch
	branchCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Get build error context
	buildError, _ := branch.Inputs["buildError"].(string)
	if buildError == "" {
		buildError = "Build failure - human intervention required"
	}

	// Check if runner is available for production mode
	if o.runner == nil {
		result.Status = "failed"
		result.Notes = "human-step branches requires production runner (not available in test mode)"
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		return result
	}

	gitProvider := o.runner.GetGitProvider()
	buildChecker := o.runner.GetBuildChecker()
	_ = o.runner.GetWorkspaceDir() // Available if needed for future enhancements

	if gitProvider == nil || buildChecker == nil {
		result.Status = "failed"
		result.Notes = "human-step branches require GitProvider and BuildChecker"
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		return result
	}

	// Step 1: Create intervention branch name
	interventionBranch := fmt.Sprintf("human-intervention-%s", branch.ID)

	// Step 2: Create MR for human intervention
	mrConfig := provider.MRConfig{
		RepoURL:      o.runner.GetTargetRepo(),
		SourceBranch: interventionBranch,
		TargetBranch: "main",
		Title:        fmt.Sprintf("Human Intervention Required: %s", branch.ID),
		Description:  fmt.Sprintf("Build Error:\n```\n%s\n```\n\nPlease fix the build failure and commit your changes to this branch for automated validation.", buildError),
		Labels:       []string{"ploy", "human-intervention"},
	}

	mrResult, err := gitProvider.CreateOrUpdateMR(branchCtx, mrConfig)
	if err != nil {
		result.Status = "failed"
		result.Notes = fmt.Sprintf("Failed to create human intervention MR: %v", err)
		result.FinishedAt = time.Now()
		result.Duration = time.Since(result.StartedAt)
		return result
	}

	// Step 3: Poll for manual commits and validate build
	pollInterval := 30 * time.Second
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-branchCtx.Done():
			// Timeout reached
			result.Status = "timeout"
			result.Notes = fmt.Sprintf("Human intervention timed out after %v", timeout)
			result.FinishedAt = time.Now()
			result.Duration = time.Since(result.StartedAt)
			return result

		case <-ticker.C:
			// Check if human made changes by attempting build validation
			buildConfig := common.DeployConfig{
				App:           branch.ID,
				Lane:          "A", // Simple build validation
				Environment:   "dev",
				ControllerURL: "", // Will be set by build checker if needed
				Timeout:       ResolveDefaultsFromEnv().BuildApplyTimeout,
			}

			buildResult, err := buildChecker.CheckBuild(branchCtx, buildConfig)
			if err != nil {
				continue // Build check failed, keep polling
			}

			if buildResult != nil && buildResult.Success {
				// Human fixed the build!
				result.Status = "completed"
				result.Notes = fmt.Sprintf("Human intervention successful via MR %s - build now passes", mrResult.MRURL)
				result.FinishedAt = time.Now()
				result.Duration = time.Since(result.StartedAt)
				return result
			}

			// Build still fails, continue polling
		}
	}
}
