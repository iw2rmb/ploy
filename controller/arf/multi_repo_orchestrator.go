package arf

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// MultiRepoOrchestrator manages coordinated transformations across multiple repositories
type MultiRepoOrchestrator interface {
	OrchestrateBatchTransformation(ctx context.Context, request BatchTransformationRequest) (*BatchTransformationResult, error)
	AnalyzeDependencies(ctx context.Context, repositories []Repository) (*DependencyAnalysis, error)
	CreateExecutionPlan(ctx context.Context, analysis *DependencyAnalysis, recipes []Recipe) (*ExecutionPlan, error)
	ExecutePlan(ctx context.Context, plan *ExecutionPlan) (*BatchTransformationResult, error)
	GetOrchestrationStatus(orchestrationID string) (*OrchestrationStatus, error)
}

// BatchTransformationRequest defines a request for multi-repository transformation
type BatchTransformationRequest struct {
	OrchestrationID string       `json:"orchestration_id"`
	Repositories    []Repository `json:"repositories"`
	Recipes         []Recipe     `json:"recipes"`
	Options         BatchOptions `json:"options"`
	Dependencies    []RepoDependency `json:"dependencies,omitempty"`
}

// Repository represents a source code repository for transformation
type Repository struct {
	ID           string            `json:"id"`
	URL          string            `json:"url"`
	Branch       string            `json:"branch"`
	Language     string            `json:"language"`
	BuildTool    string            `json:"build_tool"`
	Dependencies []string          `json:"dependencies"`
	Metadata     map[string]string `json:"metadata"`
	Priority     int               `json:"priority"`
}

// BatchOptions configures how the batch transformation should be executed
type BatchOptions struct {
	ParallelExecution bool          `json:"parallel_execution"`
	MaxConcurrency    int           `json:"max_concurrency"`
	FailFast          bool          `json:"fail_fast"`
	Timeout           time.Duration `json:"timeout"`
	DryRun            bool          `json:"dry_run"`
	CreatePullRequest bool          `json:"create_pull_request"`
}

// RepoDependency represents a relationship between repositories
type RepoDependency struct {
	From         string       `json:"from"`          // Source repository ID
	To           string       `json:"to"`            // Target repository ID
	Type         DependencyType `json:"type"`        // Type of dependency
	Relationship string       `json:"relationship"`  // Description of relationship
	Critical     bool         `json:"critical"`      // Whether this is a critical dependency
}

// DependencyType categorizes different types of repository dependencies
type DependencyType string

const (
	DependencyLibrary    DependencyType = "library"     // Library dependency
	DependencyService    DependencyType = "service"     // Service dependency
	DependencyData       DependencyType = "data"        // Data schema dependency
	DependencyBuild      DependencyType = "build"       // Build-time dependency
	DependencyAPI        DependencyType = "api"         // API contract dependency
	DependencyConfig     DependencyType = "config"      // Configuration dependency
)

// BatchTransformationResult contains the results of multi-repository transformation
type BatchTransformationResult struct {
	OrchestrationID   string                  `json:"orchestration_id"`
	TotalRepositories int                     `json:"total_repositories"`
	SuccessfulRepos   int                     `json:"successful_repos"`
	FailedRepos       int                     `json:"failed_repos"`
	ExecutionTime     time.Duration           `json:"execution_time"`
	Results           []RepoTransformationResult `json:"results"`
	DependencyAnalysis *DependencyAnalysis    `json:"dependency_analysis"`
	ExecutionPlan     *ExecutionPlan          `json:"execution_plan"`
	OverallStatus     OrchestrationStatus     `json:"overall_status"`
}

// RepoTransformationResult contains the result for a single repository transformation
type RepoTransformationResult struct {
	RepositoryID    string                  `json:"repository_id"`
	Success         bool                    `json:"success"`
	ExecutionTime   time.Duration           `json:"execution_time"`
	ChangesApplied  int                     `json:"changes_applied"`
	FilesModified   []string                `json:"files_modified"`
	RecipesApplied  []string                `json:"recipes_applied"`
	ErrorMessage    string                  `json:"error_message,omitempty"`
	TransformationResults []TransformationResult `json:"transformation_results"`
	PullRequestURL  string                  `json:"pull_request_url,omitempty"`
	ValidationScore float64                 `json:"validation_score"`
}

// DependencyAnalysis contains the analysis of dependencies between repositories
type DependencyAnalysis struct {
	TotalRepositories int                    `json:"total_repositories"`
	Dependencies      []RepoDependency           `json:"dependencies"`
	DependencyGraph   map[string][]string    `json:"dependency_graph"`
	ExecutionLevels   [][]string             `json:"execution_levels"`
	CriticalPaths     [][]string             `json:"critical_paths"`
	CircularDeps      [][]string             `json:"circular_dependencies"`
	AnalysisTime      time.Duration          `json:"analysis_time"`
}

// ExecutionPlan defines the order and strategy for executing transformations
type ExecutionPlan struct {
	OrchestrationID string            `json:"orchestration_id"`
	CreatedAt       time.Time         `json:"created_at"`
	Phases          []ExecutionPhase  `json:"phases"`
	TotalPhases     int               `json:"total_phases"`
	EstimatedTime   time.Duration     `json:"estimated_time"`
	RiskLevel       RiskLevel         `json:"risk_level"`
}

// ExecutionPhase represents a single phase in the execution plan
type ExecutionPhase struct {
	PhaseNumber   int                       `json:"phase_number"`
	Repositories  []string                  `json:"repositories"`
	Recipes       []Recipe                  `json:"recipes"`
	ParallelSafe  bool                      `json:"parallel_safe"`
	Dependencies  []string                  `json:"dependencies"`
	EstimatedTime time.Duration             `json:"estimated_time"`
	Status        ExecutionPhaseStatus      `json:"status"`
}

// ExecutionPhaseStatus represents the current status of an execution phase
type ExecutionPhaseStatus string

const (
	PhaseStatusPending    ExecutionPhaseStatus = "pending"
	PhaseStatusRunning    ExecutionPhaseStatus = "running"
	PhaseStatusCompleted  ExecutionPhaseStatus = "completed"
	PhaseStatusFailed     ExecutionPhaseStatus = "failed"
	PhaseStatusSkipped    ExecutionPhaseStatus = "skipped"
)

// RiskLevel represents the risk level of an execution plan
type RiskLevel string

const (
	RiskLevelLow      RiskLevel = "low"
	RiskLevelModerate RiskLevel = "moderate"
	RiskLevelHigh     RiskLevel = "high"
	RiskLevelCritical RiskLevel = "critical"
)

// OrchestrationStatus represents the current status of an orchestration
type OrchestrationStatus struct {
	OrchestrationID string                `json:"orchestration_id"`
	Status          string                `json:"status"`
	StartedAt       time.Time             `json:"started_at"`
	CompletedAt     *time.Time            `json:"completed_at,omitempty"`
	CurrentPhase    int                   `json:"current_phase"`
	TotalPhases     int                   `json:"total_phases"`
	Progress        float64               `json:"progress"`
	LastUpdate      time.Time             `json:"last_update"`
}

// DefaultMultiRepoOrchestrator implements the MultiRepoOrchestrator interface
type DefaultMultiRepoOrchestrator struct {
	engine           ARFEngine
	catalog          RecipeCatalog
	parallelResolver ParallelResolver
	circuitBreaker   CircuitBreaker
	
	// Orchestration state management
	activeOrchestrations map[string]*OrchestrationStatus
	mutex                sync.RWMutex
}

// NewMultiRepoOrchestrator creates a new multi-repository orchestrator
func NewMultiRepoOrchestrator(engine ARFEngine, catalog RecipeCatalog, resolver ParallelResolver, cb CircuitBreaker) MultiRepoOrchestrator {
	return &DefaultMultiRepoOrchestrator{
		engine:               engine,
		catalog:              catalog,
		parallelResolver:     resolver,
		circuitBreaker:       cb,
		activeOrchestrations: make(map[string]*OrchestrationStatus),
	}
}

// OrchestrateBatchTransformation executes a complete batch transformation workflow
func (mro *DefaultMultiRepoOrchestrator) OrchestrateBatchTransformation(ctx context.Context, request BatchTransformationRequest) (*BatchTransformationResult, error) {
	startTime := time.Now()
	
	// Initialize orchestration status
	status := &OrchestrationStatus{
		OrchestrationID: request.OrchestrationID,
		Status:          "initializing",
		StartedAt:       startTime,
		LastUpdate:      startTime,
	}
	mro.setOrchestrationStatus(status)
	
	result := &BatchTransformationResult{
		OrchestrationID:   request.OrchestrationID,
		TotalRepositories: len(request.Repositories),
		OverallStatus:     *status,
	}
	
	// Step 1: Analyze dependencies
	status.Status = "analyzing_dependencies"
	mro.setOrchestrationStatus(status)
	
	analysis, err := mro.AnalyzeDependencies(ctx, request.Repositories)
	if err != nil {
		return result, fmt.Errorf("dependency analysis failed: %w", err)
	}
	result.DependencyAnalysis = analysis
	
	// Step 2: Create execution plan
	status.Status = "creating_plan"
	mro.setOrchestrationStatus(status)
	
	plan, err := mro.CreateExecutionPlan(ctx, analysis, request.Recipes)
	if err != nil {
		return result, fmt.Errorf("execution plan creation failed: %w", err)
	}
	result.ExecutionPlan = plan
	status.TotalPhases = plan.TotalPhases
	
	// Step 3: Execute the plan
	status.Status = "executing"
	mro.setOrchestrationStatus(status)
	
	if !request.Options.DryRun {
		executionResult, err := mro.ExecutePlan(ctx, plan)
		if err != nil {
			status.Status = "failed"
			completedAt := time.Now()
			status.CompletedAt = &completedAt
			mro.setOrchestrationStatus(status)
			return result, fmt.Errorf("plan execution failed: %w", err)
		}
		
		// Merge execution results
		result.Results = executionResult.Results
		result.SuccessfulRepos = executionResult.SuccessfulRepos
		result.FailedRepos = executionResult.FailedRepos
	}
	
	// Complete orchestration
	result.ExecutionTime = time.Since(startTime)
	status.Status = "completed"
	status.Progress = 100.0
	completedAt := time.Now()
	status.CompletedAt = &completedAt
	status.LastUpdate = completedAt
	result.OverallStatus = *status
	mro.setOrchestrationStatus(status)
	
	return result, nil
}

// AnalyzeDependencies analyzes the dependencies between repositories
func (mro *DefaultMultiRepoOrchestrator) AnalyzeDependencies(ctx context.Context, repositories []Repository) (*DependencyAnalysis, error) {
	startTime := time.Now()
	
	analysis := &DependencyAnalysis{
		TotalRepositories: len(repositories),
		DependencyGraph:   make(map[string][]string),
	}
	
	// Extract explicit dependencies from repository metadata
	dependencies := make([]RepoDependency, 0)
	for _, repo := range repositories {
		for _, depID := range repo.Dependencies {
			dep := RepoDependency{
				From:         repo.ID,
				To:           depID,
				Type:         DependencyLibrary, // Default type
				Relationship: "depends on",
				Critical:     true, // Conservative approach
			}
			dependencies = append(dependencies, dep)
		}
	}
	
	// Build dependency graph
	for _, dep := range dependencies {
		if _, exists := analysis.DependencyGraph[dep.From]; !exists {
			analysis.DependencyGraph[dep.From] = make([]string, 0)
		}
		analysis.DependencyGraph[dep.From] = append(analysis.DependencyGraph[dep.From], dep.To)
	}
	
	// Detect circular dependencies
	circularDeps := mro.detectCircularDependencies(analysis.DependencyGraph)
	analysis.CircularDeps = circularDeps
	
	// Create execution levels using topological sort
	executionLevels, err := mro.createExecutionLevels(repositories, analysis.DependencyGraph)
	if err != nil {
		return nil, fmt.Errorf("failed to create execution levels: %w", err)
	}
	analysis.ExecutionLevels = executionLevels
	
	// Identify critical paths
	criticalPaths := mro.identifyCriticalPaths(analysis.DependencyGraph, dependencies)
	analysis.CriticalPaths = criticalPaths
	
	analysis.Dependencies = dependencies
	analysis.AnalysisTime = time.Since(startTime)
	
	return analysis, nil
}

// detectCircularDependencies detects cycles in the dependency graph
func (mro *DefaultMultiRepoOrchestrator) detectCircularDependencies(graph map[string][]string) [][]string {
	visited := make(map[string]bool)
	recStack := make(map[string]bool)
	cycles := make([][]string, 0)
	
	var dfs func(node string, path []string) []string
	dfs = func(node string, path []string) []string {
		visited[node] = true
		recStack[node] = true
		path = append(path, node)
		
		for _, neighbor := range graph[node] {
			if !visited[neighbor] {
				if cycle := dfs(neighbor, path); cycle != nil {
					return cycle
				}
			} else if recStack[neighbor] {
				// Found a cycle
				cycleStart := -1
				for i, n := range path {
					if n == neighbor {
						cycleStart = i
						break
					}
				}
				if cycleStart >= 0 {
					return path[cycleStart:]
				}
			}
		}
		
		recStack[node] = false
		return nil
	}
	
	for node := range graph {
		if !visited[node] {
			if cycle := dfs(node, []string{}); cycle != nil {
				cycles = append(cycles, cycle)
			}
		}
	}
	
	return cycles
}

// createExecutionLevels creates levels of repositories that can be executed in parallel
func (mro *DefaultMultiRepoOrchestrator) createExecutionLevels(repositories []Repository, graph map[string][]string) ([][]string, error) {
	// Create reverse graph (who depends on whom)
	reverseGraph := make(map[string][]string)
	inDegree := make(map[string]int)
	
	// Initialize
	for _, repo := range repositories {
		inDegree[repo.ID] = 0
		reverseGraph[repo.ID] = make([]string, 0)
	}
	
	// Build reverse graph and calculate in-degrees
	for from, toList := range graph {
		for _, to := range toList {
			reverseGraph[to] = append(reverseGraph[to], from)
			inDegree[from]++
		}
	}
	
	levels := make([][]string, 0)
	queue := make([]string, 0)
	
	// Find nodes with no dependencies (in-degree 0)
	for repo, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, repo)
		}
	}
	
	for len(queue) > 0 {
		currentLevel := make([]string, len(queue))
		copy(currentLevel, queue)
		levels = append(levels, currentLevel)
		
		nextQueue := make([]string, 0)
		
		for _, node := range queue {
			// Remove this node and reduce in-degree of dependent nodes
			for _, dependent := range reverseGraph[node] {
				inDegree[dependent]--
				if inDegree[dependent] == 0 {
					nextQueue = append(nextQueue, dependent)
				}
			}
		}
		
		queue = nextQueue
	}
	
	// Check if all repositories are included (no circular dependencies)
	totalProcessed := 0
	for _, level := range levels {
		totalProcessed += len(level)
	}
	
	if totalProcessed != len(repositories) {
		return nil, fmt.Errorf("circular dependencies detected, cannot create execution levels")
	}
	
	return levels, nil
}

// identifyCriticalPaths identifies critical dependency paths
func (mro *DefaultMultiRepoOrchestrator) identifyCriticalPaths(graph map[string][]string, dependencies []RepoDependency) [][]string {
	criticalPaths := make([][]string, 0)
	
	// Find critical dependencies
	criticalDeps := make([]RepoDependency, 0)
	for _, dep := range dependencies {
		if dep.Critical {
			criticalDeps = append(criticalDeps, dep)
		}
	}
	
	// For each critical dependency, trace the path
	for _, dep := range criticalDeps {
		path := mro.traceDependencyPath(graph, dep.From, dep.To)
		if len(path) > 0 {
			criticalPaths = append(criticalPaths, path)
		}
	}
	
	return criticalPaths
}

// traceDependencyPath traces a path from source to target in the dependency graph
func (mro *DefaultMultiRepoOrchestrator) traceDependencyPath(graph map[string][]string, from, to string) []string {
	visited := make(map[string]bool)
	
	var dfs func(current string, target string, path []string) []string
	dfs = func(current string, target string, path []string) []string {
		if current == target {
			return append(path, current)
		}
		
		if visited[current] {
			return nil
		}
		
		visited[current] = true
		newPath := append(path, current)
		
		for _, neighbor := range graph[current] {
			if result := dfs(neighbor, target, newPath); result != nil {
				return result
			}
		}
		
		return nil
	}
	
	return dfs(from, to, []string{})
}

// CreateExecutionPlan creates an execution plan based on dependency analysis
func (mro *DefaultMultiRepoOrchestrator) CreateExecutionPlan(ctx context.Context, analysis *DependencyAnalysis, recipes []Recipe) (*ExecutionPlan, error) {
	plan := &ExecutionPlan{
		CreatedAt:   time.Now(),
		TotalPhases: len(analysis.ExecutionLevels),
		Phases:      make([]ExecutionPhase, 0, len(analysis.ExecutionLevels)),
	}
	
	totalEstimatedTime := time.Duration(0)
	
	// Create phases based on execution levels
	for i, level := range analysis.ExecutionLevels {
		phase := ExecutionPhase{
			PhaseNumber:   i + 1,
			Repositories:  level,
			Recipes:       recipes, // All recipes apply to all repositories
			ParallelSafe:  true,    // Repositories in the same level can run in parallel
			Dependencies:  mro.getPhaseDependencies(level, analysis.DependencyGraph),
			EstimatedTime: mro.estimatePhaseTime(len(level), len(recipes)),
			Status:        PhaseStatusPending,
		}
		
		plan.Phases = append(plan.Phases, phase)
		totalEstimatedTime += phase.EstimatedTime
	}
	
	plan.EstimatedTime = totalEstimatedTime
	plan.RiskLevel = mro.assessRiskLevel(analysis, recipes)
	
	return plan, nil
}

// getPhaseDependencies gets the dependencies for a phase
func (mro *DefaultMultiRepoOrchestrator) getPhaseDependencies(repositories []string, graph map[string][]string) []string {
	dependencies := make(map[string]bool)
	
	for _, repo := range repositories {
		for _, dep := range graph[repo] {
			dependencies[dep] = true
		}
	}
	
	result := make([]string, 0, len(dependencies))
	for dep := range dependencies {
		result = append(result, dep)
	}
	
	sort.Strings(result)
	return result
}

// estimatePhaseTime estimates the time required for a phase
func (mro *DefaultMultiRepoOrchestrator) estimatePhaseTime(repoCount, recipeCount int) time.Duration {
	// Base time per repository per recipe
	baseTime := 30 * time.Second
	
	// Factor in parallelization (diminishing returns)
	parallelFactor := 1.0
	if repoCount > 1 {
		parallelFactor = 1.0 / float64(repoCount) * 1.5 // Not perfectly parallel
	}
	
	estimatedSeconds := float64(baseTime.Seconds()) * float64(repoCount) * float64(recipeCount) * parallelFactor
	return time.Duration(estimatedSeconds) * time.Second
}

// assessRiskLevel assesses the risk level of the execution plan
func (mro *DefaultMultiRepoOrchestrator) assessRiskLevel(analysis *DependencyAnalysis, recipes []Recipe) RiskLevel {
	riskScore := 0
	
	// Factor in circular dependencies
	riskScore += len(analysis.CircularDeps) * 10
	
	// Factor in critical paths
	riskScore += len(analysis.CriticalPaths) * 5
	
	// Factor in number of dependencies
	riskScore += len(analysis.Dependencies) * 2
	
	// Factor in recipe risk (security recipes are higher risk)
	for _, recipe := range recipes {
		if recipe.Category == CategorySecurity {
			riskScore += 5
		} else if recipe.Category == CategoryMigration {
			riskScore += 3
		}
	}
	
	// Determine risk level based on score
	switch {
	case riskScore >= 50:
		return RiskLevelCritical
	case riskScore >= 25:
		return RiskLevelHigh
	case riskScore >= 10:
		return RiskLevelModerate
	default:
		return RiskLevelLow
	}
}

// ExecutePlan executes the given execution plan
func (mro *DefaultMultiRepoOrchestrator) ExecutePlan(ctx context.Context, plan *ExecutionPlan) (*BatchTransformationResult, error) {
	result := &BatchTransformationResult{
		OrchestrationID: plan.OrchestrationID,
		Results:         make([]RepoTransformationResult, 0),
	}
	
	// Update orchestration status
	if status := mro.getOrchestrationStatus(plan.OrchestrationID); status != nil {
		status.TotalPhases = plan.TotalPhases
	}
	
	// Execute phases in sequence
	for i, phase := range plan.Phases {
		phaseResults, err := mro.executePhase(ctx, phase, plan.OrchestrationID)
		if err != nil {
			return result, fmt.Errorf("phase %d execution failed: %w", i+1, err)
		}
		
		result.Results = append(result.Results, phaseResults...)
		
		// Update progress
		if status := mro.getOrchestrationStatus(plan.OrchestrationID); status != nil {
			status.CurrentPhase = i + 1
			status.Progress = float64(i+1) / float64(plan.TotalPhases) * 100.0
			status.LastUpdate = time.Now()
			mro.setOrchestrationStatus(status)
		}
	}
	
	// Calculate final statistics
	for _, res := range result.Results {
		if res.Success {
			result.SuccessfulRepos++
		} else {
			result.FailedRepos++
		}
	}
	
	result.TotalRepositories = len(result.Results)
	
	return result, nil
}

// executePhase executes a single phase of the execution plan
func (mro *DefaultMultiRepoOrchestrator) executePhase(ctx context.Context, phase ExecutionPhase, orchestrationID string) ([]RepoTransformationResult, error) {
	results := make([]RepoTransformationResult, 0, len(phase.Repositories))
	
	if phase.ParallelSafe && len(phase.Repositories) > 1 {
		// Execute repositories in parallel
		resultsChan := make(chan RepoTransformationResult, len(phase.Repositories))
		var wg sync.WaitGroup
		
		for _, repoID := range phase.Repositories {
			wg.Add(1)
			go func(repositoryID string) {
				defer wg.Done()
				
				result := mro.executeRepositoryTransformation(ctx, repositoryID, phase.Recipes)
				resultsChan <- result
			}(repoID)
		}
		
		// Wait for all to complete
		go func() {
			wg.Wait()
			close(resultsChan)
		}()
		
		// Collect results
		for result := range resultsChan {
			results = append(results, result)
		}
	} else {
		// Execute repositories sequentially
		for _, repoID := range phase.Repositories {
			result := mro.executeRepositoryTransformation(ctx, repoID, phase.Recipes)
			results = append(results, result)
		}
	}
	
	return results, nil
}

// executeRepositoryTransformation executes transformation on a single repository
func (mro *DefaultMultiRepoOrchestrator) executeRepositoryTransformation(ctx context.Context, repositoryID string, recipes []Recipe) RepoTransformationResult {
	startTime := time.Now()
	
	result := RepoTransformationResult{
		RepositoryID:          repositoryID,
		TransformationResults: make([]TransformationResult, 0),
		RecipesApplied:        make([]string, 0),
		FilesModified:         make([]string, 0),
	}
	
	// Create a mock codebase for this repository
	// In a real implementation, this would clone/checkout the repository
	codebase := Codebase{
		Repository: repositoryID,
		Branch:     "main", // Default branch
		Language:   "java", // Default language
		BuildTool:  "maven", // Default build tool
	}
	
	totalChanges := 0
	allModifiedFiles := make(map[string]bool)
	
	// Execute each recipe on the repository
	for _, recipe := range recipes {
		if ctx.Err() != nil {
			result.ErrorMessage = "Context cancelled"
			break
		}
		
		// Execute recipe with circuit breaker protection
		err := mro.circuitBreaker.Execute(ctx, func() error {
			transformResult, err := mro.engine.ExecuteRecipe(ctx, recipe, codebase)
			if err != nil {
				return err
			}
			
			result.TransformationResults = append(result.TransformationResults, *transformResult)
			
			if transformResult.Success {
				result.RecipesApplied = append(result.RecipesApplied, recipe.ID)
				totalChanges += transformResult.ChangesApplied
				
				// Track modified files
				for _, file := range transformResult.FilesModified {
					allModifiedFiles[file] = true
				}
			}
			
			return nil
		})
		
		if err != nil {
			result.ErrorMessage = err.Error()
			break
		}
	}
	
	// Finalize result
	result.ExecutionTime = time.Since(startTime)
	result.Success = result.ErrorMessage == ""
	result.ChangesApplied = totalChanges
	
	// Convert modified files map to slice
	for file := range allModifiedFiles {
		result.FilesModified = append(result.FilesModified, file)
	}
	sort.Strings(result.FilesModified)
	
	// Calculate validation score based on success and changes
	if result.Success && totalChanges > 0 {
		result.ValidationScore = 0.95 // High score for successful transformations
	} else if result.Success {
		result.ValidationScore = 0.8 // Medium score for successful but no-change transformations
	} else {
		result.ValidationScore = 0.1 // Low score for failed transformations
	}
	
	return result
}

// GetOrchestrationStatus returns the current status of an orchestration
func (mro *DefaultMultiRepoOrchestrator) GetOrchestrationStatus(orchestrationID string) (*OrchestrationStatus, error) {
	status := mro.getOrchestrationStatus(orchestrationID)
	if status == nil {
		return nil, fmt.Errorf("orchestration not found: %s", orchestrationID)
	}
	return status, nil
}

// setOrchestrationStatus updates the orchestration status
func (mro *DefaultMultiRepoOrchestrator) setOrchestrationStatus(status *OrchestrationStatus) {
	mro.mutex.Lock()
	defer mro.mutex.Unlock()
	mro.activeOrchestrations[status.OrchestrationID] = status
}

// getOrchestrationStatus retrieves the orchestration status
func (mro *DefaultMultiRepoOrchestrator) getOrchestrationStatus(orchestrationID string) *OrchestrationStatus {
	mro.mutex.RLock()
	defer mro.mutex.RUnlock()
	return mro.activeOrchestrations[orchestrationID]
}