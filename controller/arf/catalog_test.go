package arf

import (
	"context"
	"testing"
	"time"
)

// MockRecipeCatalog for testing without Consul dependency
type MockRecipeCatalog struct {
	recipes map[string]Recipe
	stats   map[string]*RecipeStats
}

func NewMockRecipeCatalog() *MockRecipeCatalog {
	return &MockRecipeCatalog{
		recipes: make(map[string]Recipe),
		stats:   make(map[string]*RecipeStats),
	}
}

func (m *MockRecipeCatalog) StoreRecipe(ctx context.Context, recipe Recipe) error {
	m.recipes[recipe.ID] = recipe
	if _, exists := m.stats[recipe.ID]; !exists {
		m.stats[recipe.ID] = &RecipeStats{
			RecipeID:      recipe.ID,
			FirstExecuted: time.Now(),
		}
	}
	return nil
}

func (m *MockRecipeCatalog) GetRecipe(ctx context.Context, recipeID string) (*Recipe, error) {
	recipe, exists := m.recipes[recipeID]
	if !exists {
		return nil, &RecipeNotFoundError{RecipeID: recipeID}
	}
	return &recipe, nil
}

func (m *MockRecipeCatalog) ListRecipes(ctx context.Context, filters RecipeFilters) ([]Recipe, error) {
	var result []Recipe
	for _, recipe := range m.recipes {
		if m.matchesFilters(recipe, filters) {
			result = append(result, recipe)
		}
	}
	return result, nil
}

func (m *MockRecipeCatalog) UpdateRecipe(ctx context.Context, recipe Recipe) error {
	if _, exists := m.recipes[recipe.ID]; !exists {
		return &RecipeNotFoundError{RecipeID: recipe.ID}
	}
	m.recipes[recipe.ID] = recipe
	return nil
}

func (m *MockRecipeCatalog) DeleteRecipe(ctx context.Context, recipeID string) error {
	if _, exists := m.recipes[recipeID]; !exists {
		return &RecipeNotFoundError{RecipeID: recipeID}
	}
	delete(m.recipes, recipeID)
	delete(m.stats, recipeID)
	return nil
}

func (m *MockRecipeCatalog) SearchRecipes(ctx context.Context, query string) ([]Recipe, error) {
	var result []Recipe
	for _, recipe := range m.recipes {
		if m.containsQuery(recipe.Name, query) ||
		   m.containsQuery(recipe.Description, query) ||
		   m.containsQueryInTags(recipe.Tags, query) {
			result = append(result, recipe)
		}
	}
	return result, nil
}

func (m *MockRecipeCatalog) GetRecipeStats(ctx context.Context, recipeID string) (*RecipeStats, error) {
	stats, exists := m.stats[recipeID]
	if !exists {
		return &RecipeStats{RecipeID: recipeID}, nil
	}
	return stats, nil
}

func (m *MockRecipeCatalog) UpdateRecipeStats(ctx context.Context, recipeID string, success bool, executionTime time.Duration) error {
	stats, exists := m.stats[recipeID]
	if !exists {
		stats = &RecipeStats{
			RecipeID:      recipeID,
			FirstExecuted: time.Now(),
		}
		m.stats[recipeID] = stats
	}

	stats.TotalExecutions++
	if success {
		stats.SuccessfulRuns++
	} else {
		stats.FailedRuns++
	}

	if stats.TotalExecutions > 0 {
		stats.SuccessRate = float64(stats.SuccessfulRuns) / float64(stats.TotalExecutions)
	}

	if stats.TotalExecutions == 1 {
		stats.AvgExecutionTime = executionTime
	} else {
		totalTime := stats.AvgExecutionTime * time.Duration(stats.TotalExecutions-1)
		stats.AvgExecutionTime = (totalTime + executionTime) / time.Duration(stats.TotalExecutions)
	}

	stats.LastExecuted = time.Now()
	return nil
}

func (m *MockRecipeCatalog) matchesFilters(recipe Recipe, filters RecipeFilters) bool {
	if filters.Language != "" && recipe.Language != filters.Language {
		return false
	}
	if filters.Category != "" && recipe.Category != filters.Category {
		return false
	}
	if filters.MinConfidence > 0 && recipe.Confidence < filters.MinConfidence {
		return false
	}
	if filters.MaxConfidence > 0 && recipe.Confidence > filters.MaxConfidence {
		return false
	}
	return true
}

func (m *MockRecipeCatalog) containsQuery(text, query string) bool {
	return len(text) > 0 && len(query) > 0 && text == query // Simplified for testing
}

func (m *MockRecipeCatalog) containsQueryInTags(tags []string, query string) bool {
	for _, tag := range tags {
		if m.containsQuery(tag, query) {
			return true
		}
	}
	return false
}

// Custom error types for testing
type RecipeNotFoundError struct {
	RecipeID string
}

func (e *RecipeNotFoundError) Error() string {
	return "recipe " + e.RecipeID + " not found"
}

func TestRecipeCatalogOperations(t *testing.T) {
	catalog := NewMockRecipeCatalog()
	ctx := context.Background()

	t.Run("Store and retrieve recipe", func(t *testing.T) {
		recipe := Recipe{
			ID:          "test-recipe-1",
			Name:        "Test Recipe 1",
			Description: "A test recipe for unit testing",
			Language:    "java",
			Category:    CategoryCleanup,
			Confidence:  0.9,
			Source:      "org.openrewrite.java.cleanup.TestRecipe1",
			Version:     "1.0.0",
			Tags:        []string{"test", "cleanup"},
		}

		// Store recipe
		err := catalog.StoreRecipe(ctx, recipe)
		if err != nil {
			t.Fatalf("Failed to store recipe: %v", err)
		}

		// Retrieve recipe
		retrieved, err := catalog.GetRecipe(ctx, recipe.ID)
		if err != nil {
			t.Fatalf("Failed to retrieve recipe: %v", err)
		}

		if retrieved.ID != recipe.ID {
			t.Errorf("Expected ID %s, got %s", recipe.ID, retrieved.ID)
		}
		if retrieved.Name != recipe.Name {
			t.Errorf("Expected name %s, got %s", recipe.Name, retrieved.Name)
		}
		if retrieved.Confidence != recipe.Confidence {
			t.Errorf("Expected confidence %f, got %f", recipe.Confidence, retrieved.Confidence)
		}
	})

	t.Run("Update recipe", func(t *testing.T) {
		recipe := Recipe{
			ID:          "update-recipe",
			Name:        "Update Recipe",
			Description: "Original description",
			Language:    "java",
			Category:    CategoryCleanup,
			Confidence:  0.8,
		}

		// Store original
		catalog.StoreRecipe(ctx, recipe)

		// Update recipe
		recipe.Description = "Updated description"
		recipe.Confidence = 0.95
		err := catalog.UpdateRecipe(ctx, recipe)
		if err != nil {
			t.Fatalf("Failed to update recipe: %v", err)
		}

		// Verify update
		retrieved, err := catalog.GetRecipe(ctx, recipe.ID)
		if err != nil {
			t.Fatalf("Failed to retrieve updated recipe: %v", err)
		}

		if retrieved.Description != "Updated description" {
			t.Errorf("Expected updated description, got %s", retrieved.Description)
		}
		if retrieved.Confidence != 0.95 {
			t.Errorf("Expected updated confidence 0.95, got %f", retrieved.Confidence)
		}
	})

	t.Run("Delete recipe", func(t *testing.T) {
		recipe := Recipe{
			ID:       "delete-recipe",
			Name:     "Delete Recipe",
			Language: "java",
		}

		// Store and verify
		catalog.StoreRecipe(ctx, recipe)
		_, err := catalog.GetRecipe(ctx, recipe.ID)
		if err != nil {
			t.Fatalf("Recipe should exist before deletion: %v", err)
		}

		// Delete recipe
		err = catalog.DeleteRecipe(ctx, recipe.ID)
		if err != nil {
			t.Fatalf("Failed to delete recipe: %v", err)
		}

		// Verify deletion
		_, err = catalog.GetRecipe(ctx, recipe.ID)
		if err == nil {
			t.Error("Recipe should not exist after deletion")
		}
	})

	t.Run("List recipes with filters", func(t *testing.T) {
		// Add test recipes
		recipes := []Recipe{
			{
				ID:         "java-recipe-1",
				Name:       "Java Cleanup 1",
				Language:   "java",
				Category:   CategoryCleanup,
				Confidence: 0.9,
			},
			{
				ID:         "java-recipe-2",
				Name:       "Java Modernize 1",
				Language:   "java",
				Category:   CategoryModernize,
				Confidence: 0.8,
			},
			{
				ID:         "python-recipe-1",
				Name:       "Python Cleanup 1",
				Language:   "python",
				Category:   CategoryCleanup,
				Confidence: 0.85,
			},
		}

		for _, recipe := range recipes {
			catalog.StoreRecipe(ctx, recipe)
		}

		// Test language filter
		javaRecipes, err := catalog.ListRecipes(ctx, RecipeFilters{Language: "java"})
		if err != nil {
			t.Fatalf("Failed to list Java recipes: %v", err)
		}
		if len(javaRecipes) != 2 {
			t.Errorf("Expected 2 Java recipes, got %d", len(javaRecipes))
		}

		// Test category filter
		cleanupRecipes, err := catalog.ListRecipes(ctx, RecipeFilters{Category: CategoryCleanup})
		if err != nil {
			t.Fatalf("Failed to list cleanup recipes: %v", err)
		}
		if len(cleanupRecipes) != 2 {
			t.Errorf("Expected 2 cleanup recipes, got %d", len(cleanupRecipes))
		}

		// Test confidence filter
		highConfidenceRecipes, err := catalog.ListRecipes(ctx, RecipeFilters{MinConfidence: 0.85})
		if err != nil {
			t.Fatalf("Failed to list high confidence recipes: %v", err)
		}
		if len(highConfidenceRecipes) != 2 {
			t.Errorf("Expected 2 high confidence recipes, got %d", len(highConfidenceRecipes))
		}

		// Test combined filters
		javaCleanupRecipes, err := catalog.ListRecipes(ctx, RecipeFilters{
			Language: "java",
			Category: CategoryCleanup,
		})
		if err != nil {
			t.Fatalf("Failed to list Java cleanup recipes: %v", err)
		}
		if len(javaCleanupRecipes) != 1 {
			t.Errorf("Expected 1 Java cleanup recipe, got %d", len(javaCleanupRecipes))
		}
	})

	t.Run("Search recipes", func(t *testing.T) {
		recipe := Recipe{
			ID:          "search-recipe",
			Name:        "Searchable Recipe",
			Description: "This recipe can be found by search",
			Language:    "java",
			Tags:        []string{"searchable", "test"},
		}

		catalog.StoreRecipe(ctx, recipe)

		// Search by name (simplified mock implementation)
		results, err := catalog.SearchRecipes(ctx, "Searchable Recipe")
		if err != nil {
			t.Fatalf("Failed to search recipes: %v", err)
		}
		if len(results) != 1 {
			t.Errorf("Expected 1 search result, got %d", len(results))
		}
	})
}

func TestRecipeStats(t *testing.T) {
	catalog := NewMockRecipeCatalog()
	ctx := context.Background()

	recipe := Recipe{
		ID:       "stats-recipe",
		Name:     "Stats Recipe",
		Language: "java",
	}

	catalog.StoreRecipe(ctx, recipe)

	t.Run("Initial stats", func(t *testing.T) {
		stats, err := catalog.GetRecipeStats(ctx, recipe.ID)
		if err != nil {
			t.Fatalf("Failed to get recipe stats: %v", err)
		}

		if stats.TotalExecutions != 0 {
			t.Errorf("Expected 0 total executions, got %d", stats.TotalExecutions)
		}
		if stats.SuccessRate != 0 {
			t.Errorf("Expected 0 success rate, got %f", stats.SuccessRate)
		}
	})

	t.Run("Update stats", func(t *testing.T) {
		// Record successful execution
		err := catalog.UpdateRecipeStats(ctx, recipe.ID, true, 2*time.Second)
		if err != nil {
			t.Fatalf("Failed to update recipe stats: %v", err)
		}

		stats, err := catalog.GetRecipeStats(ctx, recipe.ID)
		if err != nil {
			t.Fatalf("Failed to get updated stats: %v", err)
		}

		if stats.TotalExecutions != 1 {
			t.Errorf("Expected 1 total execution, got %d", stats.TotalExecutions)
		}
		if stats.SuccessfulRuns != 1 {
			t.Errorf("Expected 1 successful run, got %d", stats.SuccessfulRuns)
		}
		if stats.SuccessRate != 1.0 {
			t.Errorf("Expected 100%% success rate, got %f", stats.SuccessRate)
		}
		if stats.AvgExecutionTime != 2*time.Second {
			t.Errorf("Expected 2s average execution time, got %v", stats.AvgExecutionTime)
		}

		// Record failed execution
		err = catalog.UpdateRecipeStats(ctx, recipe.ID, false, 1*time.Second)
		if err != nil {
			t.Fatalf("Failed to update recipe stats: %v", err)
		}

		stats, err = catalog.GetRecipeStats(ctx, recipe.ID)
		if err != nil {
			t.Fatalf("Failed to get updated stats: %v", err)
		}

		if stats.TotalExecutions != 2 {
			t.Errorf("Expected 2 total executions, got %d", stats.TotalExecutions)
		}
		if stats.FailedRuns != 1 {
			t.Errorf("Expected 1 failed run, got %d", stats.FailedRuns)
		}
		if stats.SuccessRate != 0.5 {
			t.Errorf("Expected 50%% success rate, got %f", stats.SuccessRate)
		}

		expectedAvg := (2*time.Second + 1*time.Second) / 2
		if stats.AvgExecutionTime != expectedAvg {
			t.Errorf("Expected %v average execution time, got %v", expectedAvg, stats.AvgExecutionTime)
		}
	})
}

func TestRecipeFilters(t *testing.T) {
	tests := []struct {
		name    string
		recipe  Recipe
		filters RecipeFilters
		matches bool
	}{
		{
			name: "language filter match",
			recipe: Recipe{
				Language:   "java",
				Confidence: 0.9,
			},
			filters: RecipeFilters{Language: "java"},
			matches: true,
		},
		{
			name: "language filter no match",
			recipe: Recipe{
				Language:   "python",
				Confidence: 0.9,
			},
			filters: RecipeFilters{Language: "java"},
			matches: false,
		},
		{
			name: "confidence filter match",
			recipe: Recipe{
				Language:   "java",
				Confidence: 0.9,
			},
			filters: RecipeFilters{MinConfidence: 0.8},
			matches: true,
		},
		{
			name: "confidence filter no match",
			recipe: Recipe{
				Language:   "java",
				Confidence: 0.7,
			},
			filters: RecipeFilters{MinConfidence: 0.8},
			matches: false,
		},
		{
			name: "category filter match",
			recipe: Recipe{
				Language: "java",
				Category: CategoryCleanup,
			},
			filters: RecipeFilters{Category: CategoryCleanup},
			matches: true,
		},
		{
			name: "multiple filters match",
			recipe: Recipe{
				Language:   "java",
				Category:   CategoryCleanup,
				Confidence: 0.9,
			},
			filters: RecipeFilters{
				Language:      "java",
				Category:      CategoryCleanup,
				MinConfidence: 0.8,
			},
			matches: true,
		},
		{
			name: "multiple filters no match",
			recipe: Recipe{
				Language:   "java",
				Category:   CategoryModernize,
				Confidence: 0.9,
			},
			filters: RecipeFilters{
				Language:      "java",
				Category:      CategoryCleanup,
				MinConfidence: 0.8,
			},
			matches: false,
		},
	}

	catalog := NewMockRecipeCatalog()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := catalog.matchesFilters(tt.recipe, tt.filters)
			if matches != tt.matches {
				t.Errorf("Expected matches %v, got %v", tt.matches, matches)
			}
		})
	}
}