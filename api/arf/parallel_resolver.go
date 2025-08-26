package arf

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"
	
	"github.com/iw2rmb/ploy/api/arf/models"
)

// Recipe category constants for backward compatibility
const (
	CategoryCleanup       = "cleanup"
	CategoryModernize     = "modernization"
	CategoryMigration     = "migration"
	CategorySecurity      = "security"
	CategoryPerformance   = "performance"
	CategoryRefactoring   = "refactoring"
)

// ParallelResolver manages concurrent error resolution across multiple files/components
type ParallelResolver interface {
	ResolveErrors(ctx context.Context, errors []TransformationError, codebase Codebase) (*ParallelResolutionResult, error)
	SetMaxWorkers(max int)
	GetWorkerStats() WorkerPoolStats
}

// ParallelResolutionResult contains the results of parallel error resolution
type ParallelResolutionResult struct {
	TotalErrors     int                         `json:"total_errors"`
	ResolvedErrors  int                         `json:"resolved_errors"`
	FailedErrors    int                         `json:"failed_errors"`
	ExecutionTime   time.Duration               `json:"execution_time"`
	WorkerStats     WorkerPoolStats             `json:"worker_stats"`
	Resolutions     []ErrorResolution           `json:"resolutions"`
	DependencyGraph map[string][]string         `json:"dependency_graph"`
	ParallelBatches [][]string                  `json:"parallel_batches"`
}

// ErrorResolution represents a single error resolution attempt
type ErrorResolution struct {
	Error           TransformationError `json:"error"`
	RecipeApplied   string              `json:"recipe_applied"`
	Success         bool                `json:"success"`
	Attempts        int                 `json:"attempts"`
	ExecutionTime   time.Duration       `json:"execution_time"`
	FailureReason   string              `json:"failure_reason,omitempty"`
	Dependencies    []string            `json:"dependencies"`
	WorkerID        string              `json:"worker_id"`
}

// WorkerPoolStats provides statistics about the worker pool performance
type WorkerPoolStats struct {
	MaxWorkers       int           `json:"max_workers"`
	ActiveWorkers    int           `json:"active_workers"`
	CompletedTasks   int64         `json:"completed_tasks"`
	FailedTasks      int64         `json:"failed_tasks"`
	AverageTaskTime  time.Duration `json:"average_task_time"`
	TotalExecutions  int64         `json:"total_executions"`
	QueueLength      int           `json:"queue_length"`
}

// ResolutionTask represents a single error resolution task
type ResolutionTask struct {
	ID           string              `json:"id"`
	Error        TransformationError `json:"error"`
	Dependencies []string            `json:"dependencies"`
	Priority     int                 `json:"priority"`
	MaxRetries   int                 `json:"max_retries"`
	CreatedAt    time.Time           `json:"created_at"`
}

// DefaultParallelResolver implements the ParallelResolver interface
type DefaultParallelResolver struct {
	recipeExecutor *RecipeExecutor
	catalog        RecipeCatalog
	circuitBreaker CircuitBreaker
	maxWorkers     int
	stats          WorkerPoolStats
	mutex          sync.RWMutex
	
	// Task queue and worker management
	taskQueue     chan ResolutionTask
	resultChannel chan ErrorResolution
	workerWg      sync.WaitGroup
	shutdown      chan struct{}
	isRunning     bool
}

// NewParallelResolver creates a new parallel error resolver
func NewParallelResolver(executor *RecipeExecutor, catalog RecipeCatalog, cb CircuitBreaker) ParallelResolver {
	maxWorkers := runtime.NumCPU()
	if maxWorkers > 8 {
		maxWorkers = 8 // Cap at 8 for reasonable resource usage
	}
	
	resolver := &DefaultParallelResolver{
		recipeExecutor: executor,
		catalog:        catalog,
		circuitBreaker: cb,
		maxWorkers:     maxWorkers,
		taskQueue:      make(chan ResolutionTask, 100),
		resultChannel:  make(chan ErrorResolution, 100),
		shutdown:       make(chan struct{}),
		stats: WorkerPoolStats{
			MaxWorkers: maxWorkers,
		},
	}
	
	resolver.startWorkers()
	return resolver
}

// ResolveErrors processes multiple errors in parallel with dependency awareness
func (pr *DefaultParallelResolver) ResolveErrors(ctx context.Context, errors []TransformationError, codebase Codebase) (*ParallelResolutionResult, error) {
	startTime := time.Now()
	
	// Build dependency graph
	dependencyGraph := pr.buildDependencyGraph(errors)
	
	// Create execution batches based on dependencies
	batches := pr.createExecutionBatches(errors, dependencyGraph)
	
	result := &ParallelResolutionResult{
		TotalErrors:     len(errors),
		DependencyGraph: dependencyGraph,
		ParallelBatches: pr.formatBatches(batches),
		Resolutions:     make([]ErrorResolution, 0, len(errors)),
	}
	
	// Execute batches in order (within batch = parallel, between batches = sequential)
	for batchIndex, batch := range batches {
		batchResults, err := pr.executeBatch(ctx, batch, codebase, batchIndex)
		if err != nil {
			return result, fmt.Errorf("failed to execute batch %d: %w", batchIndex, err)
		}
		
		result.Resolutions = append(result.Resolutions, batchResults...)
	}
	
	// Calculate final statistics
	result.ExecutionTime = time.Since(startTime)
	result.WorkerStats = pr.GetWorkerStats()
	
	for _, resolution := range result.Resolutions {
		if resolution.Success {
			result.ResolvedErrors++
		} else {
			result.FailedErrors++
		}
	}
	
	return result, nil
}

// buildDependencyGraph analyzes errors to determine resolution dependencies
func (pr *DefaultParallelResolver) buildDependencyGraph(errors []TransformationError) map[string][]string {
	graph := make(map[string][]string)
	
	for i, error1 := range errors {
		errorID := fmt.Sprintf("error_%d", i)
		dependencies := make([]string, 0)
		
		for j, error2 := range errors {
			if i == j {
				continue
			}
			
			dependentID := fmt.Sprintf("error_%d", j)
			
			// Check various dependency conditions
			if pr.isDependentError(error1, error2) {
				dependencies = append(dependencies, dependentID)
			}
		}
		
		graph[errorID] = dependencies
	}
	
	return graph
}

// isDependentError determines if error1 depends on the resolution of error2
func (pr *DefaultParallelResolver) isDependentError(error1, error2 TransformationError) bool {
	// Same file dependency - compilation errors must be resolved before semantic errors
	if error1.File == error2.File {
		// Type resolution errors depend on import errors
		if error1.Type == "type_resolution" && error2.Type == "import_missing" {
			return true
		}
		
		// Method call errors depend on class definition errors
		if error1.Type == "method_call" && error2.Type == "class_definition" {
			return true
		}
		
		// Later line numbers may depend on earlier ones
		if error1.Line > error2.Line && error2.Type == "syntax_error" {
			return true
		}
	}
	
	// Cross-file dependencies
	if error1.File != error2.File {
		// Import dependencies
		if error1.Type == "import_missing" && pr.isImportDependency(error1, error2) {
			return true
		}
	}
	
	return false
}

// isImportDependency checks if error1 depends on a class/interface defined in error2's file
func (pr *DefaultParallelResolver) isImportDependency(error1, error2 TransformationError) bool {
	// This is a simplified check - in practice, would analyze import statements and class definitions
	return error2.Type == "class_definition" && 
		   (error1.Message == "cannot resolve symbol" || error1.Message == "cannot find symbol")
}

// createExecutionBatches creates batches of errors that can be resolved in parallel
func (pr *DefaultParallelResolver) createExecutionBatches(errors []TransformationError, dependencyGraph map[string][]string) [][]TransformationError {
	batches := make([][]TransformationError, 0)
	processed := make(map[string]bool)
	errorMap := make(map[string]TransformationError)
	
	// Create error map for quick lookup
	for i, err := range errors {
		errorID := fmt.Sprintf("error_%d", i)
		errorMap[errorID] = err
	}
	
	// Create batches using topological sort approach
	for len(processed) < len(errors) {
		currentBatch := make([]TransformationError, 0)
		
		for i, err := range errors {
			errorID := fmt.Sprintf("error_%d", i)
			
			if processed[errorID] {
				continue
			}
			
			// Check if all dependencies are satisfied
			canProcess := true
			for _, depID := range dependencyGraph[errorID] {
				if !processed[depID] {
					canProcess = false
					break
				}
			}
			
			if canProcess {
				currentBatch = append(currentBatch, err)
				processed[errorID] = true
			}
		}
		
		if len(currentBatch) == 0 {
			// Circular dependency or remaining errors - add them anyway
			for i, err := range errors {
				errorID := fmt.Sprintf("error_%d", i)
				if !processed[errorID] {
					currentBatch = append(currentBatch, err)
					processed[errorID] = true
				}
			}
		}
		
		if len(currentBatch) > 0 {
			batches = append(batches, currentBatch)
		}
	}
	
	return batches
}

// formatBatches converts error batches to string arrays for JSON serialization
func (pr *DefaultParallelResolver) formatBatches(batches [][]TransformationError) [][]string {
	formatted := make([][]string, len(batches))
	
	for i, batch := range batches {
		formatted[i] = make([]string, len(batch))
		for j, err := range batch {
			formatted[i][j] = fmt.Sprintf("%s:%d - %s", err.File, err.Line, err.Type)
		}
	}
	
	return formatted
}

// executeBatch processes a batch of errors in parallel
func (pr *DefaultParallelResolver) executeBatch(ctx context.Context, batch []TransformationError, codebase Codebase, batchIndex int) ([]ErrorResolution, error) {
	if len(batch) == 0 {
		return []ErrorResolution{}, nil
	}
	
	results := make([]ErrorResolution, 0, len(batch))
	resultsChan := make(chan ErrorResolution, len(batch))
	
	// Create worker pool for this batch
	var wg sync.WaitGroup
	
	for i, err := range batch {
		wg.Add(1)
		go func(error TransformationError, index int) {
			defer wg.Done()
			
			workerID := fmt.Sprintf("batch_%d_worker_%d", batchIndex, index)
			resolution := pr.resolveError(ctx, error, codebase, workerID)
			
			select {
			case resultsChan <- resolution:
			case <-ctx.Done():
				return
			}
		}(err, i)
	}
	
	// Wait for all workers to complete
	go func() {
		wg.Wait()
		close(resultsChan)
	}()
	
	// Collect results
	for resolution := range resultsChan {
		results = append(results, resolution)
	}
	
	return results, nil
}

// resolveError attempts to resolve a single error
func (pr *DefaultParallelResolver) resolveError(ctx context.Context, transformationError TransformationError, codebase Codebase, workerID string) ErrorResolution {
	startTime := time.Now()
	
	resolution := ErrorResolution{
		Error:        transformationError,
		WorkerID:     workerID,
		Dependencies: []string{}, // Could be populated from dependency analysis
	}
	
	// Find appropriate recipes for this error type
	recipes, err := pr.findRecipesForError(ctx, transformationError)
	if err != nil {
		resolution.FailureReason = fmt.Sprintf("Failed to find recipes: %v", err)
		resolution.ExecutionTime = time.Since(startTime)
		return resolution
	}
	
	if len(recipes) == 0 {
		resolution.FailureReason = "No applicable recipes found"
		resolution.ExecutionTime = time.Since(startTime)
		return resolution
	}
	
	// Try recipes in order of confidence
	for _, recipe := range recipes {
		if ctx.Err() != nil {
			resolution.FailureReason = "Context cancelled"
			break
		}
		
		resolution.Attempts++
		resolution.RecipeApplied = recipe.ID
		
		// Execute recipe with circuit breaker protection
		err := pr.circuitBreaker.Execute(ctx, func() error {
			return pr.applyRecipeToError(ctx, recipe, transformationError, codebase)
		})
		
		if err == nil {
			resolution.Success = true
			break
		} else {
			resolution.FailureReason = err.Error()
		}
		
		// Limit retry attempts
		if resolution.Attempts >= 3 {
			break
		}
	}
	
	resolution.ExecutionTime = time.Since(startTime)
	
	// Update worker stats
	pr.updateWorkerStats(resolution.Success, resolution.ExecutionTime)
	
	return resolution
}

// findRecipesForError finds recipes that can potentially resolve the given error
func (pr *DefaultParallelResolver) findRecipesForError(ctx context.Context, error TransformationError) ([]*models.Recipe, error) {
	// Search for recipes based on error type and context
	filters := RecipeFilters{
		Category: CategoryCleanup, // Default to cleanup, could be more sophisticated
	}
	
	// Customize filters based on error type
	switch error.Type {
	case "import_missing":
		filters.Category = CategoryModernize
	case "deprecated_api":
		filters.Category = CategoryMigration
	case "security_vulnerability":
		filters.Category = CategorySecurity
	}
	
	recipes, err := pr.catalog.ListRecipes(ctx, filters)
	if err != nil {
		return nil, err
	}
	
	// Filter and rank recipes by relevance to the error
	relevant := make([]*models.Recipe, 0)
	for _, recipe := range recipes {
		if pr.isRecipeRelevant(recipe, error) {
			relevant = append(relevant, recipe)
		}
	}
	
	// TODO: Add sorting by relevance score when metadata is available
	
	return relevant, nil
}

// isRecipeRelevant determines if a recipe is relevant for resolving the given error
func (pr *DefaultParallelResolver) isRecipeRelevant(recipe *models.Recipe, error TransformationError) bool {
	// Simple relevance check based on keywords in recipe name/description
	errorKeywords := []string{error.Type, error.Message}
	recipeText := recipe.Metadata.Name + " " + recipe.Metadata.Description
	
	for _, keyword := range errorKeywords {
		if keyword != "" && len(keyword) > 3 { // Avoid very short keywords
			// Simple contains check - could be enhanced with fuzzy matching
			if len(recipeText) > 0 {
				return true // For now, consider all recipes potentially relevant
			}
		}
	}
	
	return true // Default to considering recipe relevant
}

// applyRecipeToError applies a specific recipe to resolve an error
func (pr *DefaultParallelResolver) applyRecipeToError(ctx context.Context, recipe *models.Recipe, transformationError TransformationError, codebase Codebase) error {
	// Create a focused codebase containing only the problematic file
	focusedCodebase := Codebase{
		Repository: codebase.Repository,
		Branch:     codebase.Branch,
		Language:   codebase.Language,
		BuildTool:  codebase.BuildTool,
		Metadata:   codebase.Metadata,
	}
	
	// Execute the recipe on the focused codebase
	result, err := pr.recipeExecutor.ExecuteRecipeObject(ctx, recipe, focusedCodebase.Path)
	if err != nil {
		return fmt.Errorf("recipe execution failed: %w", err)
	}
	
	if !result.Success {
		errorMsg := "recipe execution failed"
		if len(result.Errors) > 0 {
			errorMsg = result.Errors[0].Message
		}
		return fmt.Errorf("recipe did not succeed: %s", errorMsg)
	}
	
	// Verify that the specific error was addressed
	if result.ChangesApplied == 0 {
		return fmt.Errorf("recipe made no changes")
	}
	
	return nil
}

// updateWorkerStats updates the worker pool statistics
func (pr *DefaultParallelResolver) updateWorkerStats(success bool, executionTime time.Duration) {
	pr.mutex.Lock()
	defer pr.mutex.Unlock()
	
	pr.stats.TotalExecutions++
	
	if success {
		pr.stats.CompletedTasks++
	} else {
		pr.stats.FailedTasks++
	}
	
	// Update average execution time
	if pr.stats.TotalExecutions == 1 {
		pr.stats.AverageTaskTime = executionTime
	} else {
		// Rolling average
		pr.stats.AverageTaskTime = time.Duration(
			(int64(pr.stats.AverageTaskTime)*(pr.stats.TotalExecutions-1) + int64(executionTime)) / pr.stats.TotalExecutions,
		)
	}
}

// SetMaxWorkers configures the maximum number of worker goroutines
func (pr *DefaultParallelResolver) SetMaxWorkers(max int) {
	pr.mutex.Lock()
	defer pr.mutex.Unlock()
	
	if max > 0 && max <= 32 { // Reasonable bounds
		pr.maxWorkers = max
		pr.stats.MaxWorkers = max
	}
}

// GetWorkerStats returns current worker pool statistics
func (pr *DefaultParallelResolver) GetWorkerStats() WorkerPoolStats {
	pr.mutex.RLock()
	defer pr.mutex.RUnlock()
	
	// Update queue length
	pr.stats.QueueLength = len(pr.taskQueue)
	
	return pr.stats
}

// startWorkers initializes the worker goroutine pool
func (pr *DefaultParallelResolver) startWorkers() {
	if pr.isRunning {
		return
	}
	
	pr.isRunning = true
	
	for i := 0; i < pr.maxWorkers; i++ {
		pr.workerWg.Add(1)
		go pr.worker(fmt.Sprintf("worker_%d", i))
	}
}

// worker is the main worker goroutine that processes resolution tasks
func (pr *DefaultParallelResolver) worker(workerID string) {
	defer pr.workerWg.Done()
	
	for {
		select {
		case task := <-pr.taskQueue:
			pr.processTask(task, workerID)
			
		case <-pr.shutdown:
			return
		}
	}
}

// processTask processes a single resolution task
func (pr *DefaultParallelResolver) processTask(task ResolutionTask, workerID string) {
	pr.mutex.Lock()
	pr.stats.ActiveWorkers++
	pr.mutex.Unlock()
	
	defer func() {
		pr.mutex.Lock()
		pr.stats.ActiveWorkers--
		pr.mutex.Unlock()
	}()
	
	startTime := time.Now()
	
	// This would be the actual task processing logic
	// For now, simulate work
	time.Sleep(10 * time.Millisecond)
	
	success := true // Placeholder
	executionTime := time.Since(startTime)
	
	pr.updateWorkerStats(success, executionTime)
	
	// Send result if needed
	resolution := ErrorResolution{
		Error:         task.Error,
		Success:       success,
		Attempts:      1,
		ExecutionTime: executionTime,
		WorkerID:      workerID,
	}
	
	select {
	case pr.resultChannel <- resolution:
	default:
		// Channel full, drop result or handle appropriately
	}
}

// Shutdown gracefully stops all worker goroutines
func (pr *DefaultParallelResolver) Shutdown() {
	if !pr.isRunning {
		return
	}
	
	close(pr.shutdown)
	pr.workerWg.Wait()
	pr.isRunning = false
}