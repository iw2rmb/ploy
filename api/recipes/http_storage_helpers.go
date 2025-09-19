package recipes

import (
	"context"
	"fmt"

	"github.com/iw2rmb/ploy/api/recipes/models"
)

func (h *HTTPHandler) getRecipeWithStorage(ctx context.Context, recipeID string) (*models.Recipe, error) {
	if h.storage != nil {
		if recipe, err := h.storage.GetRecipe(ctx, recipeID); err == nil {
			return recipe, nil
		}
	}

	if h.recipeRegistry != nil {
		return h.recipeRegistry.GetRecipeAsModelsRecipe(ctx, recipeID)
	}

	return nil, fmt.Errorf("no storage backend available")
}

func (h *HTTPHandler) createRecipeWithStorage(ctx context.Context, recipe *models.Recipe) error {
	if h.validator != nil {
		if err := h.validator.ValidateRecipe(ctx, recipe); err != nil {
			return fmt.Errorf("recipe validation failed: %w", err)
		}
	}

	if h.storage != nil {
		return h.storage.CreateRecipe(ctx, recipe)
	}

	if h.recipeRegistry != nil {
		return h.recipeRegistry.StoreRecipe(ctx, recipe)
	}

	return fmt.Errorf("no storage backend available")
}

func (h *HTTPHandler) updateRecipeWithStorage(ctx context.Context, recipeID string, recipe *models.Recipe) error {
	if h.validator != nil {
		if err := h.validator.ValidateRecipe(ctx, recipe); err != nil {
			return fmt.Errorf("recipe validation failed: %w", err)
		}
	}

	if h.storage != nil {
		return h.storage.UpdateRecipe(ctx, recipeID, recipe)
	}

	if h.recipeRegistry != nil {
		return h.recipeRegistry.UpdateRecipe(ctx, recipe)
	}

	return fmt.Errorf("no storage backend available")
}

func (h *HTTPHandler) deleteRecipeWithStorage(ctx context.Context, recipeID string) error {
	if h.storage != nil {
		return h.storage.DeleteRecipe(ctx, recipeID)
	}

	if h.recipeRegistry != nil {
		return h.recipeRegistry.DeleteRecipe(ctx, recipeID)
	}

	return fmt.Errorf("no storage backend available")
}

func (h *HTTPHandler) searchRecipesWithStorage(ctx context.Context, query string) ([]*models.Recipe, error) {
	if h.storage != nil {
		results, err := h.storage.SearchRecipes(ctx, query)
		if err != nil {
			return nil, err
		}
		recipes := make([]*models.Recipe, len(results))
		for i, result := range results {
			recipes[i] = result.Recipe
		}
		return recipes, nil
	}

	if h.recipeRegistry != nil {
		return h.recipeRegistry.SearchRecipes(ctx, query)
	}

	return nil, fmt.Errorf("no storage backend available")
}

func (h *HTTPHandler) getRecipeStatsWithStorage(ctx context.Context, recipeID string) (interface{}, error) {
	if h.recipeRegistry != nil {
		return h.recipeRegistry.GetRecipeStats(ctx, recipeID)
	}

	return map[string]interface{}{
		"recipe_id":        recipeID,
		"execution_count":  0,
		"success_rate":     0.0,
		"average_duration": "0s",
		"last_executed":    nil,
		"error_patterns":   []string{},
		"resource_usage": map[string]string{
			"cpu_average":    "0m",
			"memory_average": "0Mi",
		},
	}, nil
}
