package arf

import (
	"context"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/controller/arf/models"
)

// MockRecipeCatalog for testing without Consul dependency
type MockRecipeCatalog struct {
	recipes map[string]*models.Recipe
	stats   map[string]*RecipeStats
}

func NewMockRecipeCatalog() *MockRecipeCatalog {
	return &MockRecipeCatalog{
		recipes: make(map[string]*models.Recipe),
		stats:   make(map[string]*RecipeStats),
	}
}

func (m *MockRecipeCatalog) StoreRecipe(ctx context.Context, recipe *models.Recipe) error {
	m.recipes[recipe.ID] = recipe
	if _, exists := m.stats[recipe.ID]; !exists {
		m.stats[recipe.ID] = &RecipeStats{
			RecipeID:      recipe.ID,
			FirstExecuted: time.Now(),
		}
	}
	return nil
}

func (m *MockRecipeCatalog) GetRecipe(ctx context.Context, recipeID string) (*models.Recipe, error) {
	recipe, exists := m.recipes[recipeID]
	if !exists {
		return nil, &RecipeNotFoundError{RecipeID: recipeID}
	}
	return recipe, nil
}

func (m *MockRecipeCatalog) ListRecipes(ctx context.Context, filters RecipeFilters) ([]*models.Recipe, error) {
	var result []*models.Recipe
	for _, recipe := range m.recipes {
		if m.matchesFilters(recipe, filters) {
			result = append(result, recipe)
		}
	}
	return result, nil
}

func (m *MockRecipeCatalog) UpdateRecipe(ctx context.Context, recipe *models.Recipe) error {
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

func (m *MockRecipeCatalog) SearchRecipes(ctx context.Context, query string) ([]*models.Recipe, error) {
	var result []*models.Recipe
	for _, recipe := range m.recipes {
		if m.containsQuery(recipe.Metadata.Name, query) ||
		   m.containsQuery(recipe.Metadata.Description, query) ||
		   m.containsQueryInTags(recipe.Metadata.Tags, query) {
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

func (m *MockRecipeCatalog) matchesFilters(recipe *models.Recipe, filters RecipeFilters) bool {
	// Language filter - check if recipe supports the language
	if filters.Language != "" {
		hasLanguage := false
		for _, lang := range recipe.Metadata.Languages {
			if lang == filters.Language {
				hasLanguage = true
				break
			}
		}
		if !hasLanguage {
			return false
		}
	}
	
	// Category filter - check if recipe has the category
	if filters.Category != "" {
		hasCategory := false
		for _, cat := range recipe.Metadata.Categories {
			if cat == filters.Category {
				hasCategory = true
				break
			}
		}
		if !hasCategory {
			return false
		}
	}
	
	// Author filter
	if filters.Author != "" && recipe.Metadata.Author != filters.Author {
		return false
	}
	
	// Tags filter - recipe must have all specified tags
	if len(filters.Tags) > 0 {
		for _, filterTag := range filters.Tags {
			found := false
			for _, recipeTag := range recipe.Metadata.Tags {
				if recipeTag == filterTag {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
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
		recipe := &models.Recipe{
			ID: "test-recipe-1",
			Metadata: models.RecipeMetadata{
				Name:        "test-recipe-1",
				Description: "A test recipe for unit testing",
				Author:      "test-author",
				Version:     "1.0.0",
				Languages:   []string{"java"},
				Categories:  []string{"code-cleanup"},
				Tags:        []string{"test", "cleanup"},
			},
			Steps: []models.RecipeStep{
				{
					Name: "cleanup-step",
					Type: models.StepTypeOpenRewrite,
					Config: map[string]interface{}{
						"recipe": "org.openrewrite.java.cleanup.TestRecipe1",
					},
				},
			},
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
		if retrieved.Metadata.Name != recipe.Metadata.Name {
			t.Errorf("Expected name %s, got %s", recipe.Metadata.Name, retrieved.Metadata.Name)
		}
		if retrieved.Metadata.Description != recipe.Metadata.Description {
			t.Errorf("Expected description %s, got %s", recipe.Metadata.Description, retrieved.Metadata.Description)
		}
	})

	t.Run("Update recipe", func(t *testing.T) {
		recipe := &models.Recipe{
			ID: "update-recipe",
			Metadata: models.RecipeMetadata{
				Name:        "update-recipe",
				Description: "Original description",
				Author:      "test-author",
				Version:     "1.0.0",
				Languages:   []string{"java"},
				Categories:  []string{"code-cleanup"},
			},
			Steps: []models.RecipeStep{
				{
					Name: "update-step",
					Type: models.StepTypeOpenRewrite,
					Config: map[string]interface{}{"recipe": "test"},
				},
			},
		}

		// Store original
		catalog.StoreRecipe(ctx, recipe)

		// Update recipe
		recipe.Metadata.Description = "Updated description"
		err := catalog.UpdateRecipe(ctx, recipe)
		if err != nil {
			t.Fatalf("Failed to update recipe: %v", err)
		}

		// Verify update
		retrieved, err := catalog.GetRecipe(ctx, recipe.ID)
		if err != nil {
			t.Fatalf("Failed to retrieve updated recipe: %v", err)
		}

		if retrieved.Metadata.Description != "Updated description" {
			t.Errorf("Expected updated description, got %s", retrieved.Metadata.Description)
		}
	})

	t.Run("Delete recipe", func(t *testing.T) {
		recipe := &models.Recipe{
			ID: "delete-recipe",
			Metadata: models.RecipeMetadata{
				Name:      "delete-recipe",
				Author:    "test-author",
				Languages: []string{"java"},
			},
			Steps: []models.RecipeStep{
				{
					Name:   "delete-step",
					Type:   models.StepTypeOpenRewrite,
					Config: map[string]interface{}{"recipe": "test"},
				},
			},
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
		recipes := []*models.Recipe{
			{
				ID: "java-recipe-1",
				Metadata: models.RecipeMetadata{
					Name:       "java-cleanup-1",
					Description: "Java cleanup recipe",
					Author:     "test-author",
					Languages:  []string{"java"},
					Categories: []string{"code-cleanup"},
				},
				Steps: []models.RecipeStep{{
					Name:   "cleanup",
					Type:   models.StepTypeOpenRewrite,
					Config: map[string]interface{}{"recipe": "cleanup"},
				}},
			},
			{
				ID: "java-recipe-2",
				Metadata: models.RecipeMetadata{
					Name:       "java-modernize-1",
					Description: "Java modernization recipe",
					Author:     "test-author",
					Languages:  []string{"java"},
					Categories: []string{"modernization"},
				},
				Steps: []models.RecipeStep{{
					Name:   "modernize",
					Type:   models.StepTypeOpenRewrite,
					Config: map[string]interface{}{"recipe": "modernize"},
				}},
			},
			{
				ID: "python-recipe-1",
				Metadata: models.RecipeMetadata{
					Name:       "python-cleanup-1",
					Description: "Python cleanup recipe",
					Author:     "test-author",
					Languages:  []string{"python"},
					Categories: []string{"code-cleanup"},
				},
				Steps: []models.RecipeStep{{
					Name:   "cleanup",
					Type:   models.StepTypeShellScript,
					Config: map[string]interface{}{"script": "cleanup.py"},
				}},
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
		cleanupRecipes, err := catalog.ListRecipes(ctx, RecipeFilters{Category: "code-cleanup"})
		if err != nil {
			t.Fatalf("Failed to list cleanup recipes: %v", err)
		}
		if len(cleanupRecipes) != 2 {
			t.Errorf("Expected 2 cleanup recipes, got %d", len(cleanupRecipes))
		}

		// Test author filter
		authorRecipes, err := catalog.ListRecipes(ctx, RecipeFilters{Author: "test-author"})
		if err != nil {
			t.Fatalf("Failed to list author recipes: %v", err)
		}
		if len(authorRecipes) != 3 {
			t.Errorf("Expected 3 author recipes, got %d", len(authorRecipes))
		}

		// Test combined filters
		javaCleanupRecipes, err := catalog.ListRecipes(ctx, RecipeFilters{
			Language: "java",
			Category: "code-cleanup",
		})
		if err != nil {
			t.Fatalf("Failed to list Java cleanup recipes: %v", err)
		}
		if len(javaCleanupRecipes) != 1 {
			t.Errorf("Expected 1 Java cleanup recipe, got %d", len(javaCleanupRecipes))
		}
	})

	t.Run("Search recipes", func(t *testing.T) {
		recipe := &models.Recipe{
			ID: "search-recipe",
			Metadata: models.RecipeMetadata{
				Name:        "searchable-recipe",
				Description: "This recipe can be found by search",
				Author:      "test-author",
				Languages:   []string{"java"},
				Tags:        []string{"searchable", "test"},
			},
			Steps: []models.RecipeStep{{
				Name:   "search-step",
				Type:   models.StepTypeOpenRewrite,
				Config: map[string]interface{}{"recipe": "search"},
			}},
		}

		catalog.StoreRecipe(ctx, recipe)

		// Search by name (simplified mock implementation)
		results, err := catalog.SearchRecipes(ctx, "searchable-recipe")
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

	recipe := &models.Recipe{
		ID: "stats-recipe",
		Metadata: models.RecipeMetadata{
			Name:      "stats-recipe",
			Author:    "test-author",
			Languages: []string{"java"},
		},
		Steps: []models.RecipeStep{{
			Name:   "stats-step",
			Type:   models.StepTypeOpenRewrite,
			Config: map[string]interface{}{"recipe": "stats"},
		}},
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
		recipe  *models.Recipe
		filters RecipeFilters
		matches bool
	}{
		{
			name: "language filter match",
			recipe: &models.Recipe{
				Metadata: models.RecipeMetadata{
					Name:      "test-recipe",
					Author:    "test-author",
					Languages: []string{"java"},
				},
			},
			filters: RecipeFilters{Language: "java"},
			matches: true,
		},
		{
			name: "language filter no match",
			recipe: &models.Recipe{
				Metadata: models.RecipeMetadata{
					Name:      "test-recipe",
					Author:    "test-author",
					Languages: []string{"python"},
				},
			},
			filters: RecipeFilters{Language: "java"},
			matches: false,
		},
		{
			name: "category filter match",
			recipe: &models.Recipe{
				Metadata: models.RecipeMetadata{
					Name:       "test-recipe",
					Author:     "test-author",
					Languages:  []string{"java"},
					Categories: []string{"code-cleanup"},
				},
			},
			filters: RecipeFilters{Category: "code-cleanup"},
			matches: true,
		},
		{
			name: "author filter match",
			recipe: &models.Recipe{
				Metadata: models.RecipeMetadata{
					Name:      "test-recipe",
					Author:    "test-author",
					Languages: []string{"java"},
				},
			},
			filters: RecipeFilters{Author: "test-author"},
			matches: true,
		},
		{
			name: "multiple filters match",
			recipe: &models.Recipe{
				Metadata: models.RecipeMetadata{
					Name:       "test-recipe",
					Author:     "test-author",
					Languages:  []string{"java"},
					Categories: []string{"code-cleanup"},
				},
			},
			filters: RecipeFilters{
				Language: "java",
				Category: "code-cleanup",
				Author:   "test-author",
			},
			matches: true,
		},
		{
			name: "multiple filters no match",
			recipe: &models.Recipe{
				Metadata: models.RecipeMetadata{
					Name:       "test-recipe",
					Author:     "test-author",
					Languages:  []string{"java"},
					Categories: []string{"modernization"},
				},
			},
			filters: RecipeFilters{
				Language: "java",
				Category: "code-cleanup",
				Author:   "test-author",
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