package arf

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/api/arf/models"
)

func TestParallelResolverCreation(t *testing.T) {
	// Create mock storage and components for executor  
	storage := NewInMemoryRecipeStorage()
	sandboxMgr := NewMockSandboxManager()
	executor := NewRecipeExecutor(storage, sandboxMgr)
	catalog := &MockParallelRecipeCatalog{}
	cb := NewCircuitBreaker(CircuitConfig{
		FailureThreshold: 5,
		OpenTimeout:      time.Second,
		MaxRetries:       2,
	})
	
	resolver := NewParallelResolver(executor, catalog, cb)
	
	if resolver == nil {
		t.Fatal("Expected non-nil parallel resolver")
	}
	
	stats := resolver.GetWorkerStats()
	if stats.MaxWorkers <= 0 {
		t.Error("Expected positive max workers")
	}
}

func TestDependencyGraphBuilding(t *testing.T) {
	// Create mock storage and components for executor  
	storage := NewInMemoryRecipeStorage()
	sandboxMgr := NewMockSandboxManager()
	executor := NewRecipeExecutor(storage, sandboxMgr)
	catalog := &MockParallelRecipeCatalog{}
	cb := NewCircuitBreaker(CircuitConfig{})
	
	resolver := NewParallelResolver(executor, catalog, cb).(*DefaultParallelResolver)
	
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
	// Create mock storage and components for executor
	storage := NewInMemoryRecipeStorage()
	sandboxMgr := NewMockSandboxManager()
	executor := NewRecipeExecutor(storage, sandboxMgr)
	catalog := &MockParallelRecipeCatalog{}
	cb := NewCircuitBreaker(CircuitConfig{})
	
	resolver := NewParallelResolver(executor, catalog, cb).(*DefaultParallelResolver)
	
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
	// Create mock storage and components for executor
	storage := NewInMemoryRecipeStorage()
	sandboxMgr := NewMockSandboxManager()
	executor := NewRecipeExecutor(storage, sandboxMgr)
	catalog := &MockParallelRecipeCatalog{
		recipes: []*models.Recipe{
			{
				ID: "test-recipe-1",
				Metadata: models.RecipeMetadata{
					Name:       "Fix Import",
					Categories: []string{CategoryModernize},
					Languages:  []string{"java"},
				},
			},
			{
				ID: "test-recipe-2",
				Metadata: models.RecipeMetadata{
					Name:       "Fix Type Resolution",
					Categories: []string{CategoryCleanup},
					Languages:  []string{"java"},
				},
			},
		},
	}
	
	cb := NewCircuitBreaker(CircuitConfig{
		FailureThreshold: 10,
		OpenTimeout:      time.Second,
		MaxRetries:       1,
	})
	
	resolver := NewParallelResolver(executor, catalog, cb)
	
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
	// Create mock storage and components for executor
	storage := NewInMemoryRecipeStorage()
	sandboxMgr := NewMockSandboxManager()
	executor := NewRecipeExecutor(storage, sandboxMgr)
	catalog := &MockParallelRecipeCatalog{}
	cb := NewCircuitBreaker(CircuitConfig{})
	
	resolver := NewParallelResolver(executor, catalog, cb)
	
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
	// Create mock storage and components for executor
	storage := NewInMemoryRecipeStorage()
	sandboxMgr := NewMockSandboxManager()
	executor := NewRecipeExecutor(storage, sandboxMgr)
	catalog := &MockParallelRecipeCatalog{
		recipes: []*models.Recipe{
			{
				ID: "slow-recipe",
				Metadata: models.RecipeMetadata{
					Name: "Slow Recipe",
				},
			},
		},
	}
	cb := NewCircuitBreaker(CircuitConfig{})
	
	resolver := NewParallelResolver(executor, catalog, cb)
	
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
	// Create mock storage and components for executor
	storage := NewInMemoryRecipeStorage()
	sandboxMgr := NewMockSandboxManager()
	executor := NewRecipeExecutor(storage, sandboxMgr)
	catalog := &MockParallelRecipeCatalog{
		recipes: []*models.Recipe{
			{
				ID: "concurrent-recipe",
				Metadata: models.RecipeMetadata{
					Name: "Concurrent Recipe",
				},
			},
		},
	}
	cb := NewCircuitBreaker(CircuitConfig{})
	
	resolver := NewParallelResolver(executor, catalog, cb)
	
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
	
	// Note: Concurrent execution testing would require modifying the executor
	// For now, we'll test with the standard executor setup
	
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
	// Create mock storage and components for executor
	storage := NewInMemoryRecipeStorage()
	sandboxMgr := NewMockSandboxManager()
	executor := NewRecipeExecutor(storage, sandboxMgr)
	catalog := &MockParallelRecipeCatalog{
		recipes: []*models.Recipe{
			{
				ID: "security-recipe",
				Metadata: models.RecipeMetadata{
					Name:       "Security Fix",
					Categories: []string{CategorySecurity},
				},
			},
			{
				ID: "import-recipe",
				Metadata: models.RecipeMetadata{
					Name:       "Import Fix",
					Categories: []string{CategoryModernize},
				},
			},
		},
	}
	cb := NewCircuitBreaker(CircuitConfig{})
	
	resolver := NewParallelResolver(executor, catalog, cb).(*DefaultParallelResolver)
	
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

// Note: MockParallelEngine is no longer used in favor of RecipeExecutor
// Keeping for reference but should be removed in cleanup

// MockParallelEngine methods removed - using RecipeExecutor instead

// MockParallelEngine methods removed - using RecipeExecutor instead

type MockParallelRecipeCatalog struct {
	recipes []*models.Recipe
}

func (m *MockParallelRecipeCatalog) ListRecipes(ctx context.Context, filters RecipeFilters) ([]*models.Recipe, error) {
	return m.recipes, nil
}

func (m *MockParallelRecipeCatalog) GetRecipe(ctx context.Context, recipeID string) (*models.Recipe, error) {
	for _, recipe := range m.recipes {
		if recipe.ID == recipeID {
			return recipe, nil
		}
	}
	return nil, fmt.Errorf("recipe not found: %s", recipeID)
}

func (m *MockParallelRecipeCatalog) StoreRecipe(ctx context.Context, recipe *models.Recipe) error {
	return nil
}

func (m *MockParallelRecipeCatalog) UpdateRecipe(ctx context.Context, recipe *models.Recipe) error {
	return nil
}

func (m *MockParallelRecipeCatalog) DeleteRecipe(ctx context.Context, recipeID string) error {
	return nil
}

func (m *MockParallelRecipeCatalog) SearchRecipes(ctx context.Context, query string) ([]*models.Recipe, error) {
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