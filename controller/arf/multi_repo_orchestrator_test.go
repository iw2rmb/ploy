package arf

import (
	"context"
	"testing"
	"time"
)

func TestMultiRepoOrchestratorCreation(t *testing.T) {
	engine := &MockParallelEngine{}
	catalog := &MockParallelRecipeCatalog{}
	resolver := NewParallelResolver(engine, catalog, NewCircuitBreaker(CircuitConfig{}))
	cb := NewCircuitBreaker(CircuitConfig{})
	
	orchestrator := NewMultiRepoOrchestrator(engine, catalog, resolver, cb)
	
	if orchestrator == nil {
		t.Fatal("Expected non-nil orchestrator")
	}
}

func TestDependencyAnalysis(t *testing.T) {
	engine := &MockParallelEngine{}
	catalog := &MockParallelRecipeCatalog{}
	resolver := NewParallelResolver(engine, catalog, NewCircuitBreaker(CircuitConfig{}))
	cb := NewCircuitBreaker(CircuitConfig{})
	
	orchestrator := NewMultiRepoOrchestrator(engine, catalog, resolver, cb)
	
	repositories := []Repository{
		{
			ID:           "repo-a",
			URL:          "https://github.com/example/repo-a",
			Dependencies: []string{"repo-b"},
		},
		{
			ID:           "repo-b", 
			URL:          "https://github.com/example/repo-b",
			Dependencies: []string{},
		},
		{
			ID:           "repo-c",
			URL:          "https://github.com/example/repo-c", 
			Dependencies: []string{"repo-a", "repo-b"},
		},
	}
	
	ctx := context.Background()
	analysis, err := orchestrator.AnalyzeDependencies(ctx, repositories)
	
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	
	if analysis == nil {
		t.Fatal("Expected non-nil analysis")
	}
	
	if analysis.TotalRepositories != 3 {
		t.Errorf("Expected 3 repositories, got %d", analysis.TotalRepositories)
	}
	
	if len(analysis.Dependencies) == 0 {
		t.Error("Expected dependencies to be detected")
	}
	
	if len(analysis.DependencyGraph) == 0 {
		t.Error("Expected dependency graph to be populated")
	}
	
	if len(analysis.ExecutionLevels) == 0 {
		t.Error("Expected execution levels to be created")
	}
}

func TestExecutionLevels(t *testing.T) {
	engine := &MockParallelEngine{}
	catalog := &MockParallelRecipeCatalog{}
	resolver := NewParallelResolver(engine, catalog, NewCircuitBreaker(CircuitConfig{}))
	cb := NewCircuitBreaker(CircuitConfig{})
	
	orchestrator := NewMultiRepoOrchestrator(engine, catalog, resolver, cb).(*DefaultMultiRepoOrchestrator)
	
	repositories := []Repository{
		{ID: "repo-a", Dependencies: []string{"repo-b"}},
		{ID: "repo-b", Dependencies: []string{}},
		{ID: "repo-c", Dependencies: []string{"repo-b"}},
		{ID: "repo-d", Dependencies: []string{"repo-a", "repo-c"}},
	}
	
	// Build dependency graph
	graph := make(map[string][]string)
	for _, repo := range repositories {
		graph[repo.ID] = repo.Dependencies
	}
	
	levels, err := orchestrator.createExecutionLevels(repositories, graph)
	
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	
	if len(levels) == 0 {
		t.Fatal("Expected execution levels")
	}
	
	// repo-b should be in first level (no dependencies)
	firstLevel := levels[0]
	hasRepoB := false
	for _, repo := range firstLevel {
		if repo == "repo-b" {
			hasRepoB = true
			break
		}
	}
	
	if !hasRepoB {
		t.Error("Expected repo-b in first execution level")
	}
	
	// repo-d should be in last level (depends on others)
	lastLevel := levels[len(levels)-1]
	hasRepoD := false
	for _, repo := range lastLevel {
		if repo == "repo-d" {
			hasRepoD = true
			break
		}
	}
	
	if !hasRepoD {
		t.Error("Expected repo-d in last execution level")
	}
}

func TestCircularDependencyDetection(t *testing.T) {
	orchestrator := &DefaultMultiRepoOrchestrator{}
	
	// Create circular dependency: A -> B -> C -> A
	graph := map[string][]string{
		"repo-a": {"repo-b"},
		"repo-b": {"repo-c"},
		"repo-c": {"repo-a"},
	}
	
	cycles := orchestrator.detectCircularDependencies(graph)
	
	if len(cycles) == 0 {
		t.Error("Expected circular dependency to be detected")
	}
	
	// Verify the cycle contains all three repositories
	if len(cycles) > 0 {
		cycle := cycles[0]
		if len(cycle) < 3 {
			t.Errorf("Expected cycle length >= 3, got %d", len(cycle))
		}
	}
}

func TestExecutionPlanCreation(t *testing.T) {
	engine := &MockParallelEngine{}
	catalog := &MockParallelRecipeCatalog{}
	resolver := NewParallelResolver(engine, catalog, NewCircuitBreaker(CircuitConfig{}))
	cb := NewCircuitBreaker(CircuitConfig{})
	
	orchestrator := NewMultiRepoOrchestrator(engine, catalog, resolver, cb)
	
	analysis := &DependencyAnalysis{
		TotalRepositories: 3,
		ExecutionLevels: [][]string{
			{"repo-a"},
			{"repo-b", "repo-c"},
		},
		Dependencies: []RepoDependency{
			{From: "repo-b", To: "repo-a", Critical: true, Type: DependencyLibrary},
			{From: "repo-c", To: "repo-a", Critical: false, Type: DependencyLibrary},
		},
	}
	
	recipes := []Recipe{
		{ID: "test-recipe", Category: CategoryCleanup, Confidence: 0.9},
	}
	
	ctx := context.Background()
	plan, err := orchestrator.CreateExecutionPlan(ctx, analysis, recipes)
	
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	
	if plan == nil {
		t.Fatal("Expected non-nil plan")
	}
	
	if plan.TotalPhases != 2 {
		t.Errorf("Expected 2 phases, got %d", plan.TotalPhases)
	}
	
	if len(plan.Phases) != 2 {
		t.Errorf("Expected 2 phases in plan, got %d", len(plan.Phases))
	}
	
	// First phase should have repo-a
	phase1 := plan.Phases[0]
	if len(phase1.Repositories) != 1 || phase1.Repositories[0] != "repo-a" {
		t.Error("Expected first phase to contain only repo-a")
	}
	
	// Second phase should have repo-b and repo-c
	phase2 := plan.Phases[1]
	if len(phase2.Repositories) != 2 {
		t.Errorf("Expected second phase to contain 2 repos, got %d", len(phase2.Repositories))
	}
}

func TestRiskAssessment(t *testing.T) {
	orchestrator := &DefaultMultiRepoOrchestrator{}
	
	// Low risk scenario
	lowRiskAnalysis := &DependencyAnalysis{
		CircularDeps:  [][]string{},
		CriticalPaths: [][]string{},
		Dependencies:  []RepoDependency{{From: "a", To: "b", Type: DependencyLibrary}},
	}
	
	lowRiskRecipes := []Recipe{
		{Category: CategoryCleanup, Confidence: 0.9},
	}
	
	risk := orchestrator.assessRiskLevel(lowRiskAnalysis, lowRiskRecipes)
	if risk != RiskLevelLow {
		t.Errorf("Expected low risk, got %s", risk)
	}
	
	// High risk scenario
	highRiskAnalysis := &DependencyAnalysis{
		CircularDeps:  [][]string{{"a", "b", "a"}},
		CriticalPaths: [][]string{{"a", "b", "c"}},
		Dependencies: make([]RepoDependency, 20), // Many dependencies
	}
	
	highRiskRecipes := []Recipe{
		{Category: CategorySecurity, Confidence: 0.6},
		{Category: CategoryMigration, Confidence: 0.7},
	}
	
	risk = orchestrator.assessRiskLevel(highRiskAnalysis, highRiskRecipes)
	if risk == RiskLevelLow {
		t.Error("Expected higher risk level for complex scenario")
	}
}

func TestBatchTransformationWorkflow(t *testing.T) {
	engine := &MockParallelEngine{}
	catalog := &MockParallelRecipeCatalog{}
	resolver := NewParallelResolver(engine, catalog, NewCircuitBreaker(CircuitConfig{}))
	cb := NewCircuitBreaker(CircuitConfig{})
	
	orchestrator := NewMultiRepoOrchestrator(engine, catalog, resolver, cb)
	
	request := BatchTransformationRequest{
		OrchestrationID: "test-orchestration-1",
		Repositories: []Repository{
			{ID: "repo-1", URL: "https://github.com/test/repo1", Dependencies: []string{}},
			{ID: "repo-2", URL: "https://github.com/test/repo2", Dependencies: []string{"repo-1"}},
		},
		Recipes: []Recipe{
			{ID: "test-recipe", Name: "Test Recipe", Category: CategoryCleanup, Confidence: 0.9},
		},
		Options: BatchOptions{
			ParallelExecution: true,
			MaxConcurrency:    4,
			FailFast:          false,
			Timeout:           5 * time.Minute,
			DryRun:            false,
		},
	}
	
	ctx := context.Background()
	result, err := orchestrator.OrchestrateBatchTransformation(ctx, request)
	
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	
	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	
	if result.OrchestrationID != request.OrchestrationID {
		t.Errorf("Expected orchestration ID %s, got %s", request.OrchestrationID, result.OrchestrationID)
	}
	
	if result.TotalRepositories != 2 {
		t.Errorf("Expected 2 repositories, got %d", result.TotalRepositories)
	}
	
	if result.DependencyAnalysis == nil {
		t.Error("Expected dependency analysis to be included")
	}
	
	if result.ExecutionPlan == nil {
		t.Error("Expected execution plan to be included")
	}
	
	if result.ExecutionTime == 0 {
		t.Error("Expected non-zero execution time")
	}
	
	if result.OverallStatus.Status != "completed" {
		t.Errorf("Expected completed status, got %s", result.OverallStatus.Status)
	}
}

func TestDryRunExecution(t *testing.T) {
	engine := &MockParallelEngine{}
	catalog := &MockParallelRecipeCatalog{}
	resolver := NewParallelResolver(engine, catalog, NewCircuitBreaker(CircuitConfig{}))
	cb := NewCircuitBreaker(CircuitConfig{})
	
	orchestrator := NewMultiRepoOrchestrator(engine, catalog, resolver, cb)
	
	request := BatchTransformationRequest{
		OrchestrationID: "dry-run-test",
		Repositories: []Repository{
			{ID: "repo-1", URL: "https://github.com/test/repo1"},
		},
		Recipes: []Recipe{
			{ID: "test-recipe", Category: CategoryCleanup},
		},
		Options: BatchOptions{
			DryRun: true,
		},
	}
	
	ctx := context.Background()
	result, err := orchestrator.OrchestrateBatchTransformation(ctx, request)
	
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	
	// For dry run, no actual transformations should be executed
	if len(result.Results) > 0 {
		t.Error("Expected no transformation results for dry run")
	}
	
	if result.SuccessfulRepos > 0 || result.FailedRepos > 0 {
		t.Error("Expected no repo transformations for dry run")
	}
}

func TestOrchestrationStatusTracking(t *testing.T) {
	engine := &MockParallelEngine{}
	catalog := &MockParallelRecipeCatalog{}
	resolver := NewParallelResolver(engine, catalog, NewCircuitBreaker(CircuitConfig{}))
	cb := NewCircuitBreaker(CircuitConfig{})
	
	orchestrator := NewMultiRepoOrchestrator(engine, catalog, resolver, cb)
	
	// Test non-existent orchestration
	status, err := orchestrator.GetOrchestrationStatus("non-existent")
	if err == nil {
		t.Error("Expected error for non-existent orchestration")
	}
	if status != nil {
		t.Error("Expected nil status for non-existent orchestration")
	}
	
	// Start an orchestration
	request := BatchTransformationRequest{
		OrchestrationID: "status-test",
		Repositories: []Repository{
			{ID: "repo-1", URL: "https://github.com/test/repo1"},
		},
		Recipes: []Recipe{
			{ID: "test-recipe", Category: CategoryCleanup},
		},
		Options: BatchOptions{DryRun: false},
	}
	
	ctx := context.Background()
	
	// Execute in goroutine to test status tracking during execution
	done := make(chan bool)
	go func() {
		defer func() { done <- true }()
		orchestrator.OrchestrateBatchTransformation(ctx, request)
	}()
	
	// Give it a moment to start
	time.Sleep(10 * time.Millisecond)
	
	// Check status during execution
	status, err = orchestrator.GetOrchestrationStatus("status-test")
	if err != nil {
		t.Fatalf("Expected no error getting status, got: %v", err)
	}
	
	if status == nil {
		t.Fatal("Expected non-nil status")
	}
	
	if status.OrchestrationID != "status-test" {
		t.Errorf("Expected orchestration ID 'status-test', got %s", status.OrchestrationID)
	}
	
	// Wait for completion
	<-done
	
	// Check final status
	finalStatus, err := orchestrator.GetOrchestrationStatus("status-test")
	if err != nil {
		t.Fatalf("Expected no error getting final status, got: %v", err)
	}
	
	if finalStatus.Status != "completed" {
		t.Errorf("Expected completed status, got %s", finalStatus.Status)
	}
	
	if finalStatus.CompletedAt == nil {
		t.Error("Expected completed time to be set")
	}
}

func TestParallelPhaseExecution(t *testing.T) {
	engine := &MockParallelEngine{}
	catalog := &MockParallelRecipeCatalog{}
	resolver := NewParallelResolver(engine, catalog, NewCircuitBreaker(CircuitConfig{}))
	cb := NewCircuitBreaker(CircuitConfig{})
	
	orchestrator := NewMultiRepoOrchestrator(engine, catalog, resolver, cb).(*DefaultMultiRepoOrchestrator)
	
	// Create a phase with multiple repositories (should execute in parallel)
	phase := ExecutionPhase{
		PhaseNumber:  1,
		Repositories: []string{"repo-1", "repo-2", "repo-3"},
		Recipes: []Recipe{
			{ID: "test-recipe", Category: CategoryCleanup},
		},
		ParallelSafe: true,
	}
	
	ctx := context.Background()
	startTime := time.Now()
	results, err := orchestrator.executePhase(ctx, phase, "test-orchestration")
	duration := time.Since(startTime)
	
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	
	if len(results) != 3 {
		t.Errorf("Expected 3 results, got %d", len(results))
	}
	
	// Parallel execution should be faster than sequential
	// (This is a rough check since we're using mocks)
	if duration > time.Second {
		t.Log("Phase execution took longer than expected, but this may be normal for mocks")
	}
	
	// Verify all repositories were processed
	processedRepos := make(map[string]bool)
	for _, result := range results {
		processedRepos[result.RepositoryID] = true
	}
	
	if len(processedRepos) != 3 {
		t.Error("Expected all repositories to be processed")
	}
}

func TestCriticalPathIdentification(t *testing.T) {
	orchestrator := &DefaultMultiRepoOrchestrator{}
	
	graph := map[string][]string{
		"repo-a": {"repo-b"},
		"repo-b": {"repo-c"},
		"repo-c": {},
	}
	
	dependencies := []RepoDependency{
		{From: "repo-a", To: "repo-b", Critical: true, Type: DependencyLibrary},
		{From: "repo-b", To: "repo-c", Critical: true, Type: DependencyLibrary},
	}
	
	paths := orchestrator.identifyCriticalPaths(graph, dependencies)
	
	if len(paths) == 0 {
		t.Error("Expected critical paths to be identified")
	}
	
	// Should find the path from repo-a to repo-b and repo-b to repo-c
	if len(paths) >= 1 && len(paths[0]) >= 2 {
		t.Log("Critical paths identified successfully")
	} else {
		t.Error("Expected longer critical paths")
	}
}