# ARF Robust Transformation System Roadmap

## Overview
Transform the ARF (Automated Refactoring Framework) into a robust, self-healing transformation system that combines recipe-based transformations with LLM-powered problem-solving and automatic error recovery.

## Goals
1. Simplify the CLI interface by removing redundant commands (workflow, sandbox, benchmark)
2. Create a single, powerful `transform` command that handles all transformation scenarios
3. Implement self-healing capabilities with LLM-powered error resolution
4. Support multiple output formats (archive, diff, merge request)
5. Provide comprehensive reporting with timing and change tracking

## Architecture

### Core Transform Function

```go
type TransformRequest struct {
    // Input sources
    Codebase       CodebaseSource  // Repository URL or archive path
    
    // Transformation specifications
    Recipes        []string        // OpenRewrite or other recipe IDs (optional)
    LLMPrompts     []string        // Custom LLM prompts to apply (optional)
    
    // LLM configuration
    PlanningModel  LLMConfig       // Model for planning solutions (default: local ollama)
    ExecutionModel LLMConfig       // Model for execution (default: local ollama)
    
    // Execution parameters
    MaxIterations  int             // Max retries per stuck point (default: 3)
    ParallelTries  int             // Parallel solution attempts (default: 3)
    
    // Output configuration
    OutputFormat   OutputType      // archive, diff, merge_request
    OutputPath     string          // Where to save results
    ReportLevel    string          // minimal, standard, detailed
}

type TransformResult struct {
    Success        bool
    Report         TransformationReport
    Output         interface{} // Archive, Diff, or MergeRequest based on format
    Errors         []ErrorWithResolution
}
```

## Implementation Plan

### Phase 1: Command Consolidation

#### 1.1 Remove Redundant Commands
- **Remove from CLI (`internal/cli/arf/`):**
  - `workflow.go` - Human approval workflows (not needed for automated system)
  - `sandbox.go` - FreeBSD jail management (replaced by deployment sandbox)
  - `benchmark.go` - Multi-iteration testing (integrated into transform)

- **Remove from API (`api/arf/`):**
  - `human_workflow_*.go` - All human workflow files
  - `sandbox.go` - FreeBSD jail sandbox (keep deployment sandbox)
  - Keep benchmark infrastructure but refactor for single use

#### 1.2 Refactor Transform Command
Location: `internal/cli/arf/transform.go`

```go
func handleARFTransformCommand(args []string) error {
    req := parseTransformArgs(args)
    
    // New flags to add:
    // --recipes <ids>       Comma-separated recipe IDs
    // --prompts <prompts>   Comma-separated LLM prompts
    // --repo <url>          Repository URL
    // --archive <path>      Archive path (alternative to repo)
    // --plan-model <model>  LLM model for planning (default: ollama/codellama:7b)
    // --exec-model <model>  LLM model for execution (default: ollama/codellama:7b)
    // --max-iterations <n>  Max retries per error (default: 3)
    // --output <format>     archive|diff|mr (default: diff)
    // --output-path <path>  Where to save output
    // --report <level>      minimal|standard|detailed (default: standard)
    
    result := executeRobustTransformation(req)
    displayResult(result)
    return result.Error
}
```

### Phase 2: Core Transformation Engine

#### 2.1 Reusable Components from Existing Code

**From `benchmark_suite.go`:**
- LLM provider initialization (Ollama, OpenAI, etc.)
- Git operations (`gitOps`)
- Build operations (`buildOps`)
- Recipe executor
- Deployment sandbox manager
- Iteration tracking and metrics

**From `deployment_sandbox.go`:**
- Application deployment for testing
- Tar creation from directory
- HTTP endpoint testing
- Cleanup mechanisms

**From `transformation_workflow.go`:**
- Transformation execution pipeline
- Error capture and reporting
- Stage tracking

#### 2.2 New Robust Transformation Function

Location: `api/arf/robust_transform.go`

```go
func ExecuteRobustTransformation(ctx context.Context, req TransformRequest) (*TransformResult, error) {
    // Step 1: Prepare workspace
    workspace := prepareWorkspace(req.Codebase)
    defer workspace.Cleanup()
    
    // Step 2: Apply recipes sequentially
    if len(req.Recipes) > 0 {
        for _, recipeID := range req.Recipes {
            result := applyRecipeWithRetry(ctx, workspace, recipeID, req)
            if !result.Success {
                return result, nil
            }
        }
    }
    
    // Step 3: Build and deploy after recipes
    if len(req.Recipes) > 0 {
        buildResult := buildAndDeploy(ctx, workspace, "after-recipes")
        if !buildResult.Success {
            // Enter self-healing mode
            healResult := selfHeal(ctx, workspace, buildResult.Error, req)
            if !healResult.Success {
                return buildResult, nil
            }
        }
    }
    
    // Step 4: Apply LLM prompts sequentially
    if len(req.LLMPrompts) > 0 {
        for _, prompt := range req.LLMPrompts {
            result := applyLLMPromptWithRetry(ctx, workspace, prompt, req)
            if !result.Success {
                return result, nil
            }
        }
    }
    
    // Step 5: Final build and deploy
    finalResult := buildAndDeploy(ctx, workspace, "final")
    
    // Step 6: Generate output based on format
    output := generateOutput(workspace, req.OutputFormat)
    
    // Step 7: Generate comprehensive report
    report := generateReport(workspace, req.ReportLevel)
    
    return &TransformResult{
        Success: finalResult.Success,
        Report:  report,
        Output:  output,
    }, nil
}
```

### Phase 3: Self-Healing Capabilities

#### 3.1 Error Resolution Pipeline

```go
func selfHeal(ctx context.Context, workspace *Workspace, err error, req TransformRequest) *HealingResult {
    iterations := 0
    currentError := err
    
    for iterations < req.MaxIterations {
        // Step 1: Ask LLM to analyze error and plan solutions
        solutions := planSolutions(ctx, currentError, workspace, req.PlanningModel)
        
        // Step 2: Try solutions in parallel
        results := make(chan *TransformResult, len(solutions))
        for _, solution := range solutions {
            go func(sol Solution) {
                // Create branch for this solution attempt
                branch := workspace.CreateBranch(sol.ID)
                
                // Apply solution
                result := applySolution(ctx, branch, sol, req.ExecutionModel)
                
                // Test with build and deploy
                if result.Success {
                    buildResult := buildAndDeploy(ctx, branch, sol.ID)
                    result.Success = buildResult.Success
                    result.Error = buildResult.Error
                }
                
                results <- result
            }(solution)
        }
        
        // Step 3: Wait for first success or all failures
        for i := 0; i < len(solutions); i++ {
            result := <-results
            if result.Success {
                // Merge successful branch back
                workspace.MergeBranch(result.BranchID)
                return &HealingResult{Success: true, Solution: result.Solution}
            }
        }
        
        // Step 4: If all failed, check if error changed
        if currentError.Error() != err.Error() {
            // Different error, reset iteration count
            currentError = err
            iterations = 0
        } else {
            iterations++
        }
    }
    
    return &HealingResult{Success: false, Error: currentError}
}
```

#### 3.2 LLM Solution Planning

```go
func planSolutions(ctx context.Context, err error, workspace *Workspace, model LLMConfig) []Solution {
    prompt := fmt.Sprintf(`
    I have encountered an error while transforming code:
    
    Error: %s
    
    Context:
    - Language: %s
    - Framework: %s
    - Recent changes: %s
    
    Please provide 3 different solutions to fix this error.
    For each solution, provide:
    1. Description of the fix
    2. Specific code changes or commands to run
    3. Why this might work
    
    Format as JSON array of solutions.
    `, err.Error(), workspace.Language, workspace.Framework, workspace.RecentChanges())
    
    // Query LLM (with web search if available)
    response := queryLLMWithWebSearch(ctx, prompt, model)
    
    // Parse solutions
    var solutions []Solution
    json.Unmarshal(response, &solutions)
    
    return solutions
}
```

### Phase 4: Output Generation

#### 4.1 Multiple Output Formats

```go
func generateOutput(workspace *Workspace, format OutputType) interface{} {
    switch format {
    case OutputArchive:
        // Create tar.gz of final codebase
        return createArchive(workspace.Path)
        
    case OutputDiff:
        // Generate unified diff from original to final
        return generateUnifiedDiff(workspace.OriginalPath, workspace.Path)
        
    case OutputMergeRequest:
        // Create merge request to original repo
        return createMergeRequest(workspace)
    }
}
```

#### 4.2 Comprehensive Reporting

```go
type TransformationReport struct {
    Summary    Summary
    Timeline   []StageExecution
    Changes    []FileChange
    Errors     []ErrorWithResolution
    Metrics    PerformanceMetrics
    
    // Detailed sections
    RecipeExecutions []RecipeExecution
    LLMExecutions    []LLMExecution
    BuildAttempts    []BuildAttempt
    DeploymentTests  []DeploymentTest
}

func generateReport(workspace *Workspace, level string) TransformationReport {
    report := TransformationReport{
        Summary: generateSummary(workspace),
        Timeline: workspace.GetTimeline(),
        Changes: workspace.GetAllChanges(),
    }
    
    if level == "standard" || level == "detailed" {
        report.RecipeExecutions = workspace.GetRecipeExecutions()
        report.LLMExecutions = workspace.GetLLMExecutions()
    }
    
    if level == "detailed" {
        report.BuildAttempts = workspace.GetBuildAttempts()
        report.DeploymentTests = workspace.GetDeploymentTests()
        report.Errors = workspace.GetAllErrors()
    }
    
    return report
}
```

## Code Removal and Reuse Strategy

### Files to Remove:
1. **CLI Layer (`internal/cli/arf/`):**
   - `workflow.go` - Completely remove
   - `sandbox.go` - Completely remove
   - `benchmark.go` - Completely remove
   - `execution.go` - Merge useful parts into `transform.go`

2. **API Layer (`api/arf/`):**
   - `human_workflow_*.go` (all 4 files) - Completely remove
   - `sandbox.go` - Remove (keep `deployment_sandbox.go`)
   - `workflow_*.go` files - Remove if human-workflow related

### Files to Refactor:
1. **Keep and Refactor:**
   - `benchmark_suite.go` → `robust_executor.go` (extract reusable logic)
   - `benchmark_manager.go` → Integrate into transform reporting
   - `deployment_sandbox.go` → Keep as-is for deployment testing
   - `transformation_workflow.go` → Extract stage tracking

2. **New Files to Create:**
   - `api/arf/robust_transform.go` - Main transformation engine
   - `api/arf/self_healing.go` - Self-healing capabilities
   - `api/arf/solution_planner.go` - LLM solution planning
   - `api/arf/output_generator.go` - Output format handlers

### Reusable Components:

**From Benchmark:**
- LLM provider initialization and configuration
- Git operations (clone, diff, commit)
- Build operations (detect build tool, run build)
- Deployment and testing infrastructure
- Metrics and timing collection
- Parallel execution framework

**From Sandbox:**
- Deployment sandbox creation (from `deployment_sandbox.go`)
- Application lifecycle management
- HTTP endpoint testing
- Resource cleanup

**From Workflow:**
- Stage tracking and reporting
- Error capture and classification
- Audit logging

## Testing Strategy

1. **Unit Tests:**
   - Test each component in isolation
   - Mock LLM responses for predictable testing
   - Test error recovery scenarios

2. **Integration Tests:**
   - Test full transformation pipeline
   - Test self-healing with real build errors
   - Test different output formats

3. **End-to-End Tests:**
   - Test with real repositories
   - Test with actual LLM providers
   - Verify deployment and testing

## Success Metrics

1. **Simplicity:**
   - Single command for all transformations
   - Clear, predictable behavior
   - Minimal configuration required

2. **Robustness:**
   - 90% success rate with self-healing
   - Automatic recovery from common errors
   - Parallel solution attempts

3. **Performance:**
   - < 5 minutes for typical transformation
   - Efficient parallel processing
   - Minimal LLM API calls

4. **Visibility:**
   - Clear progress reporting
   - Comprehensive final reports
   - Detailed error explanations

## Timeline

- **Week 1:** Remove redundant commands, refactor CLI
- **Week 2:** Implement core transformation engine
- **Week 3:** Add self-healing capabilities
- **Week 4:** Implement output formats and reporting
- **Week 5:** Testing and documentation
- **Week 6:** Performance optimization and polish

## Future Enhancements

1. **Caching:**
   - Cache successful solutions for similar errors
   - Cache LLM responses for common queries
   - Cache build artifacts

2. **Learning:**
   - Learn from successful resolutions
   - Improve solution planning over time
   - Pattern recognition for common issues

3. **Collaboration:**
   - Share successful solutions across teams
   - Community recipe contributions
   - Collective learning database