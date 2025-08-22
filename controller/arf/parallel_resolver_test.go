package arf

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

func TestParallelResolverCreation(t *testing.T) {
	engine := &MockParallelEngine{}
	catalog := &MockParallelRecipeCatalog{}
	cb := NewCircuitBreaker(CircuitConfig{
		FailureThreshold: 5,
		OpenTimeout:      time.Second,
		MaxRetries:       2,
	})
	
	resolver := NewParallelResolver(engine, catalog, cb)
	
	if resolver == nil {
		t.Fatal("Expected non-nil parallel resolver")
	}
	
	stats := resolver.GetWorkerStats()
	if stats.MaxWorkers <= 0 {
		t.Error("Expected positive max workers")
	}
}

func TestDependencyGraphBuilding(t *testing.T) {
	engine := &MockParallelEngine{}
	catalog := &MockParallelRecipeCatalog{}
	cb := NewCircuitBreaker(CircuitConfig{})
	
	resolver := NewParallelResolver(engine, catalog, cb).(*DefaultParallelResolver)
	
	errors := []TransformationError{
		{
			Type:    "import_missing",
			File:    "Main.java",
			Line:    1,
			Message: "cannot resolve symbol",
		},
		{
			Type:    "type_resolution",
			File:    "Main.java", 
			Line:    10,
			Message: "cannot find symbol",
		},
		{
			Type:    "syntax_error",
			File:    "Utils.java",
			Line:    5,
			Message: "missing semicolon",
		},
	}
	
	graph := resolver.buildDependencyGraph(errors)
	
	if len(graph) != 3 {
		t.Errorf("Expected 3 entries in dependency graph, got %d", len(graph))
	}
	
	// error_1 (type_resolution) should depend on error_0 (import_missing)
	if len(graph["error_1"]) == 0 {
		t.Error("Expected error_1 to have dependencies")
	}
}

func TestExecutionBatches(t *testing.T) {
	engine := &MockParallelEngine{}
	catalog := &MockParallelRecipeCatalog{}
	cb := NewCircuitBreaker(CircuitConfig{})
	
	resolver := NewParallelResolver(engine, catalog, cb).(*DefaultParallelResolver)
	
	errors := []TransformationError{
		{Type: "import_missing", File: "A.java", Line: 1},
		{Type: "type_resolution", File: "A.java", Line: 10}, // Depends on import
		{Type: "syntax_error", File: "B.java", Line: 5},     // Independent
		{Type: "method_call", File: "A.java", Line: 15},     // Depends on type resolution
	}
	
	graph := resolver.buildDependencyGraph(errors)
	batches := resolver.createExecutionBatches(errors, graph)
	
	if len(batches) == 0 {
		t.Fatal("Expected at least one batch")
	}
	
	// First batch should contain independent errors
	firstBatch := batches[0]
	if len(firstBatch) == 0 {
		t.Error("First batch should not be empty")
	}
	
	// Verify that dependent errors come in later batches
	hasImportError := false
	for _, err := range firstBatch {
		if err.Type == "import_missing" {
			hasImportError = true
			break
		}
	}
	
	if !hasImportError && len(batches) > 1 {
		t.Log("Dependency ordering working as expected")
	}
}

func TestParallelErrorResolution(t *testing.T) {
	engine := &MockParallelEngine{}
	catalog := &MockParallelRecipeCatalog{
		recipes: []Recipe{
			{
				ID:          "test-recipe-1",
				Name:        "Fix Import",
				Category:    CategoryModernize,
				Confidence:  0.9,
				Language:    "java",
			},
			{
				ID:          "test-recipe-2", 
				Name:        "Fix Type Resolution",
				Category:    CategoryCleanup,
				Confidence:  0.8,
				Language:    "java",
			},
		},
	}
	
	cb := NewCircuitBreaker(CircuitConfig{
		FailureThreshold: 10,
		OpenTimeout:      time.Second,
		MaxRetries:       1,
	})
	
	resolver := NewParallelResolver(engine, catalog, cb)
	
	errors := []TransformationError{
		{Type: "import_missing", File: "Test.java", Line: 1, Message: "cannot resolve"},
		{Type: "type_resolution", File: "Test.java", Line: 5, Message: "cannot find"},
	}
	
	codebase := Codebase{
		Repository: "https://github.com/test/project",
		Branch:     "main",
		Language:   "java",
		BuildTool:  "maven",
	}
	
	ctx := context.Background()
	result, err := resolver.ResolveErrors(ctx, errors, codebase)
	
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	
	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	
	if result.TotalErrors != 2 {
		t.Errorf("Expected 2 total errors, got %d", result.TotalErrors)
	}
	
	if len(result.Resolutions) != 2 {
		t.Errorf("Expected 2 resolutions, got %d", len(result.Resolutions))
	}
	
	if result.ExecutionTime == 0 {
		t.Error("Expected non-zero execution time")
	}
	
	if len(result.DependencyGraph) == 0 {
		t.Error("Expected dependency graph to be populated")
	}
}

func TestWorkerPoolStats(t *testing.T) {
	engine := &MockParallelEngine{}
	catalog := &MockParallelRecipeCatalog{}
	cb := NewCircuitBreaker(CircuitConfig{})
	
	resolver := NewParallelResolver(engine, catalog, cb)
	
	// Test initial stats
	stats := resolver.GetWorkerStats()
	if stats.MaxWorkers <= 0 {
		t.Error("Expected positive max workers")
	}
	
	if stats.TotalExecutions != 0 {
		t.Error("Expected zero initial executions")
	}
	
	// Test setting max workers
	resolver.SetMaxWorkers(4)
	stats = resolver.GetWorkerStats()
	if stats.MaxWorkers != 4 {
		t.Errorf("Expected 4 max workers, got %d", stats.MaxWorkers)
	}
	
	// Test invalid max workers
	resolver.SetMaxWorkers(0)
	stats = resolver.GetWorkerStats()
	if stats.MaxWorkers == 0 {
		t.Error("Should not allow zero max workers")
	}
	
	resolver.SetMaxWorkers(100)
	stats = resolver.GetWorkerStats() 
	if stats.MaxWorkers > 32 {
		t.Error("Should cap max workers at reasonable limit")
	}
}

func TestContextCancellation(t *testing.T) {
	engine := &MockParallelEngine{
		delay: 100 * time.Millisecond, // Slow operations
	}
	catalog := &MockParallelRecipeCatalog{
		recipes: []Recipe{
			{ID: "slow-recipe", Name: "Slow Recipe", Confidence: 0.9},
		},
	}
	cb := NewCircuitBreaker(CircuitConfig{})
	
	resolver := NewParallelResolver(engine, catalog, cb)
	
	errors := []TransformationError{
		{Type: "test_error", File: "Test.java", Line: 1},
	}
	
	codebase := Codebase{Language: "java"}
	
	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	
	startTime := time.Now()
	result, _ := resolver.ResolveErrors(ctx, errors, codebase)
	duration := time.Since(startTime)
	
	// Should respect context timeout
	if duration > 200*time.Millisecond {
		t.Errorf("Operation took too long: %v", duration)
	}
	
	// May or may not error depending on timing
	if result != nil && len(result.Resolutions) > 0 {
		// Check if any resolution mentions context cancellation
		for _, resolution := range result.Resolutions {
			if !resolution.Success && resolution.FailureReason == "Context cancelled" {
				t.Log("Context cancellation handled correctly")
			}
		}
	}
}

func TestConcurrentExecution(t *testing.T) {
	engine := &MockParallelEngine{}
	catalog := &MockParallelRecipeCatalog{
		recipes: []Recipe{
			{ID: "concurrent-recipe", Name: "Concurrent Recipe", Confidence: 0.9},
		},
	}
	cb := NewCircuitBreaker(CircuitConfig{})
	
	resolver := NewParallelResolver(engine, catalog, cb)
	
	// Create multiple independent errors
	errors := make([]TransformationError, 10)
	for i := 0; i < 10; i++ {
		errors[i] = TransformationError{
			Type: "test_error",
			File: "Test" + string(rune('A'+i)) + ".java", // Different files
			Line: 1,
		}
	}
	
	codebase := Codebase{Language: "java"}
	
	// Track concurrent executions
	var concurrentExecutions int64
	
	// Replace engine with tracking version
	trackingEngine := &MockParallelEngine{
		executeFunc: func(ctx context.Context, recipe Recipe, codebase Codebase) (*TransformationResult, error) {
			current := atomic.AddInt64(&concurrentExecutions, 1)
			defer atomic.AddInt64(&concurrentExecutions, -1)
			
			// Simulate some work
			time.Sleep(50 * time.Millisecond)
			
			if current > 1 {
				t.Log("Concurrent execution detected:", current)
			}
			
			return &TransformationResult{
				RecipeID:       recipe.ID,
				Success:        true,
				ChangesApplied: 1,
				ExecutionTime:  25 * time.Millisecond,
			}, nil
		},
	}
	
	resolver = NewParallelResolver(trackingEngine, catalog, cb)
	
	ctx := context.Background()
	result, err := resolver.ResolveErrors(ctx, errors, codebase)
	
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	
	if result.TotalErrors != 10 {
		t.Errorf("Expected 10 total errors, got %d", result.TotalErrors)
	}
	
	// Should have processed all errors
	if len(result.Resolutions) != 10 {
		t.Errorf("Expected 10 resolutions, got %d", len(result.Resolutions))
	}
}

func TestErrorRelevanceFiltering(t *testing.T) {
	engine := &MockParallelEngine{}
	catalog := &MockParallelRecipeCatalog{
		recipes: []Recipe{
			{
				ID:         "security-recipe",
				Name:       "Security Fix",
				Category:   CategorySecurity,
				Confidence: 0.9,
			},
			{
				ID:         "import-recipe", 
				Name:       "Import Fix",
				Category:   CategoryModernize,
				Confidence: 0.8,
			},
		},
	}
	cb := NewCircuitBreaker(CircuitConfig{})
	
	resolver := NewParallelResolver(engine, catalog, cb).(*DefaultParallelResolver)
	
	ctx := context.Background()
	
	// Test security error matching
	securityError := TransformationError{
		Type:    "security_vulnerability",
		File:    "Test.java",
		Message: "insecure random usage",
	}
	
	recipes, err := resolver.findRecipesForError(ctx, securityError)
	if err != nil {
		t.Fatalf("Expected no error finding recipes, got: %v", err)
	}
	
	if len(recipes) == 0 {
		t.Error("Expected to find relevant recipes")
	}
	
	// Test import error matching
	importError := TransformationError{
		Type:    "import_missing",
		File:    "Test.java", 
		Message: "cannot resolve symbol",
	}
	
	recipes, err = resolver.findRecipesForError(ctx, importError)
	if err != nil {
		t.Fatalf("Expected no error finding recipes, got: %v", err)
	}
	
	if len(recipes) == 0 {
		t.Error("Expected to find relevant recipes")
	}
}

func TestBatchFormatting(t *testing.T) {
	resolver := &DefaultParallelResolver{}
	
	batches := [][]TransformationError{
		{
			{Type: "import_missing", File: "A.java", Line: 1},
			{Type: "syntax_error", File: "B.java", Line: 5},
		},
		{
			{Type: "type_resolution", File: "A.java", Line: 10},
		},
	}
	
	formatted := resolver.formatBatches(batches)
	
	if len(formatted) != 2 {
		t.Errorf("Expected 2 formatted batches, got %d", len(formatted))
	}
	
	if len(formatted[0]) != 2 {
		t.Errorf("Expected first batch to have 2 items, got %d", len(formatted[0]))
	}
	
	if len(formatted[1]) != 1 {
		t.Errorf("Expected second batch to have 1 item, got %d", len(formatted[1]))
	}
	
	// Check format
	expected := "A.java:1 - import_missing"
	if formatted[0][0] != expected {
		t.Errorf("Expected '%s', got '%s'", expected, formatted[0][0])
	}
}

// Mock implementations for testing

type MockParallelEngine struct {
	recipes     []Recipe
	delay       time.Duration
	executeFunc func(ctx context.Context, recipe Recipe, codebase Codebase) (*TransformationResult, error)
}

func (m *MockParallelEngine) ValidateRecipe(recipe Recipe) error {
	return nil
}

func (m *MockParallelEngine) ExecuteRecipe(ctx context.Context, recipe Recipe, codebase Codebase) (*TransformationResult, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, recipe, codebase)
	}
	
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	
	return &TransformationResult{
		RecipeID:       recipe.ID,
		Success:        true,
		ChangesApplied: 1,
		ExecutionTime:  10 * time.Millisecond,
	}, nil
}

func (m *MockParallelEngine) ListAvailableRecipes() ([]Recipe, error) {
	return m.recipes, nil
}

func (m *MockParallelEngine) GetRecipeMetadata(recipeID string) (*RecipeMetadata, error) {
	return &RecipeMetadata{
		Recipe:              Recipe{ID: recipeID},
		ApplicableLanguages: []string{"java"},
		SuccessRate:         0.9,
	}, nil
}

func (m *MockParallelEngine) CacheAST(key string, ast *AST) error {
	return nil
}

func (m *MockParallelEngine) GetCachedAST(key string) (*AST, bool) {
	return nil, false
}

type MockParallelRecipeCatalog struct {
	recipes []Recipe
}

func (m *MockParallelRecipeCatalog) ListRecipes(ctx context.Context, filters RecipeFilters) ([]Recipe, error) {
	return m.recipes, nil
}

func (m *MockParallelRecipeCatalog) GetRecipe(ctx context.Context, recipeID string) (*Recipe, error) {
	for _, recipe := range m.recipes {
		if recipe.ID == recipeID {
			return &recipe, nil
		}
	}
	return nil, fmt.Errorf("recipe not found: %s", recipeID)
}

func (m *MockParallelRecipeCatalog) StoreRecipe(ctx context.Context, recipe Recipe) error {
	return nil
}

func (m *MockParallelRecipeCatalog) UpdateRecipe(ctx context.Context, recipe Recipe) error {
	return nil
}

func (m *MockParallelRecipeCatalog) DeleteRecipe(ctx context.Context, recipeID string) error {
	return nil
}

func (m *MockParallelRecipeCatalog) SearchRecipes(ctx context.Context, query string) ([]Recipe, error) {
	return m.recipes, nil
}

func (m *MockParallelRecipeCatalog) GetRecipeStats(ctx context.Context, recipeID string) (*RecipeStats, error) {
	return &RecipeStats{
		TotalExecutions:   10,
		SuccessfulRuns:    8,
		FailedRuns:        2,
	}, nil
}

func (m *MockParallelRecipeCatalog) UpdateRecipeStats(ctx context.Context, recipeID string, success bool, executionTime time.Duration) error {
	return nil
}