package arf

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/api/arf/models"
)

// Recipe Model Compatibility Methods
// These methods provide models.Recipe-compatible operations backed by RecipeRegistry

// StoreRecipe stores a models.Recipe in the registry
func (r *RecipeRegistry) StoreRecipe(ctx context.Context, recipe *models.Recipe) error {
	if recipe == nil {
		return fmt.Errorf("recipe cannot be nil")
	}

	// Convert models.Recipe to UnifiedRecipeMetadata
	metadata := &UnifiedRecipeMetadata{
		Metadata: RecipeInfo{
			ID:         recipe.ID,
			Name:       recipe.Metadata.Name,
			Version:    recipe.Metadata.Version,
			Type:       "custom",
			Source:     "user",
			Author:     recipe.Metadata.Author,
			Tags:       recipe.Metadata.Tags,
			Categories: recipe.Metadata.Categories,
		},
		Steps: recipe.Steps,
		Cache: &CacheInfo{
			StoredAt:  time.Now(),
			SizeBytes: int64(len(recipe.Hash)),
			Hash:      recipe.Hash,
		},
	}

	return r.storeRecipe(ctx, metadata)
}

// GetRecipeAsModelsRecipe retrieves a recipe by ID
func (r *RecipeRegistry) GetRecipeAsModelsRecipe(ctx context.Context, recipeID string) (*models.Recipe, error) {
	unified, err := r.GetRecipe(ctx, recipeID)
	if err != nil {
		return nil, err
	}

	// Convert UnifiedRecipeMetadata to models.Recipe
	recipe := &models.Recipe{
		Metadata: models.RecipeMetadata{
			Name:        unified.Metadata.Name,
			Version:     unified.Metadata.Version,
			Description: "Converted from RecipeRegistry",
			Author:      unified.Metadata.Author,
			Tags:        unified.Metadata.Tags,
			Categories:  unified.Metadata.Categories,
		},
		Steps:     unified.Steps,
		ID:        unified.Metadata.ID,
		CreatedAt: unified.Cache.StoredAt,
		UpdatedAt: unified.Cache.StoredAt,
	}

	if unified.Cache != nil {
		recipe.Hash = unified.Cache.Hash
	}

	return recipe, nil
}

// ListRecipes lists recipes with filters
func (r *RecipeRegistry) ListRecipes(ctx context.Context, filters RecipeFilters) ([]*models.Recipe, error) {
	// Get all recipes from registry
	allUnified, err := r.ListAllRecipes(ctx)
	if err != nil {
		return nil, err
	}

	var recipes []*models.Recipe
	for _, unified := range allUnified {
		// Apply filters
		if !matchesFilters(unified, filters) {
			continue
		}

		// Convert to models.Recipe
		recipe := &models.Recipe{
			Metadata: models.RecipeMetadata{
				Name:        unified.Metadata.Name,
				Version:     unified.Metadata.Version,
				Description: "Converted from RecipeRegistry",
				Author:      unified.Metadata.Author,
				Tags:        unified.Metadata.Tags,
				Categories:  unified.Metadata.Categories,
			},
			Steps:     unified.Steps,
			ID:        unified.Metadata.ID,
			CreatedAt: unified.Cache.StoredAt,
			UpdatedAt: unified.Cache.StoredAt,
		}

		if unified.Cache != nil {
			recipe.Hash = unified.Cache.Hash
		}

		recipes = append(recipes, recipe)
	}

	return recipes, nil
}

// UpdateRecipe updates an existing recipe
func (r *RecipeRegistry) UpdateRecipe(ctx context.Context, recipe *models.Recipe) error {
	// For now, treat update as store (replace)
	return r.StoreRecipe(ctx, recipe)
}

// DeleteRecipe deletes a recipe by ID
func (r *RecipeRegistry) DeleteRecipe(ctx context.Context, recipeID string) error {
	// Check if storage provider supports Delete method
	if storageDeleter, ok := r.storage.(interface {
		Delete(ctx context.Context, key string) error
	}); ok {
		// Delete from registry
		key := fmt.Sprintf("registry/%s.yaml", recipeID)
		err := storageDeleter.Delete(ctx, key)
		if err != nil {
			return fmt.Errorf("failed to delete recipe from registry: %w", err)
		}

		// Also delete custom implementation if it exists
		customKey := fmt.Sprintf("custom/%s/recipe.yaml", recipeID)
		_ = storageDeleter.Delete(ctx, customKey) // Ignore error if doesn't exist
	} else {
		// Fallback: StorageProvider doesn't have Delete method
		return fmt.Errorf("delete operation not supported by storage provider")
	}

	return nil
}

// SearchRecipes searches recipes by query
func (r *RecipeRegistry) SearchRecipes(ctx context.Context, query string) ([]*models.Recipe, error) {
	// Get all recipes and filter by query
	allUnified, err := r.ListAllRecipes(ctx)
	if err != nil {
		return nil, err
	}

	var recipes []*models.Recipe
	query = strings.ToLower(query)

	for _, unified := range allUnified {
		// Search in name, tags, and categories
		matches := strings.Contains(strings.ToLower(unified.Metadata.Name), query) ||
			containsInSlice(unified.Metadata.Tags, query) ||
			containsInSlice(unified.Metadata.Categories, query)

		if matches {
			recipe := &models.Recipe{
				Metadata: models.RecipeMetadata{
					Name:        unified.Metadata.Name,
					Version:     unified.Metadata.Version,
					Description: "Converted from RecipeRegistry",
					Author:      unified.Metadata.Author,
					Tags:        unified.Metadata.Tags,
					Categories:  unified.Metadata.Categories,
				},
				Steps:     unified.Steps,
				ID:        unified.Metadata.ID,
				CreatedAt: unified.Cache.StoredAt,
				UpdatedAt: unified.Cache.StoredAt,
			}

			if unified.Cache != nil {
				recipe.Hash = unified.Cache.Hash
			}

			recipes = append(recipes, recipe)
		}
	}

	return recipes, nil
}

// GetRecipeStats returns stats for a recipe
func (r *RecipeRegistry) GetRecipeStats(ctx context.Context, recipeID string) (*RecipeStats, error) {
	// For now, return default stats
	// In the future, this could be implemented with proper tracking
	return &RecipeStats{
		RecipeID:         recipeID,
		TotalExecutions:  0,
		SuccessfulRuns:   0,
		FailedRuns:       0,
		SuccessRate:      0.0,
		AvgExecutionTime: 0,
		LastExecuted:     time.Time{},
		FirstExecuted:    time.Time{},
	}, nil
}

// UpdateRecipeStats updates stats for a recipe
func (r *RecipeRegistry) UpdateRecipeStats(ctx context.Context, recipeID string, success bool, executionTime time.Duration) error {
	// For now, this is a no-op
	// In the future, implement proper stats tracking
	return nil
}
