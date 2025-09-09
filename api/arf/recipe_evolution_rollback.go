package arf

import (
	"context"
	"fmt"
	"time"
)

// RollbackRecipe rolls back a recipe to a previous version
func (re *DefaultRecipeEvolution) RollbackRecipe(ctx context.Context, recipeID string, version int) error {
	if re.versioning == nil {
		return fmt.Errorf("recipe versioning not available")
	}

	// Get the specific version
	recipeVersion, err := re.versioning.GetVersion(ctx, recipeID, version)
	if err != nil {
		return fmt.Errorf("failed to get recipe version %d: %w", version, err)
	}

	if !recipeVersion.Rollbackable {
		return fmt.Errorf("recipe version %d is not rollbackable", version)
	}

	// Store the current version as a rollback point
	currentRecipe, err := re.registry.GetRecipeAsModelsRecipe(ctx, recipeID)
	if err != nil {
		return fmt.Errorf("failed to get current recipe: %w", err)
	}

	rollbackVersion := RecipeVersion{
		Version:      re.versioning.GetNextVersion(ctx, recipeID),
		Recipe:       currentRecipe,
		Changes:      []RecipeModification{},
		Reason:       fmt.Sprintf("Rollback to version %d", version),
		CreatedAt:    time.Now(),
		CreatedBy:    "system",
		Rollbackable: true,
	}

	// Store the rollback version
	if err := re.versioning.StoreVersion(ctx, rollbackVersion); err != nil {
		return fmt.Errorf("failed to store rollback version: %w", err)
	}

	// Update the registry with the rolled-back recipe
	return re.registry.UpdateRecipe(ctx, recipeVersion.Recipe)
}
