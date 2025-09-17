package recipes

import (
	"fmt"
	"io"

	models "github.com/iw2rmb/ploy/api/recipes/models"
	"gopkg.in/yaml.v3"
)

// processRecipeFromReader processes a recipe from a generic reader
func processRecipeFromReader(reader io.Reader, fileName string, result *ImportResult, flags CommandFlags) error {
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	return processRecipeData(data, fileName, result, flags)
}

// processRecipeData processes recipe data and imports it
func processRecipeData(data []byte, fileName string, result *ImportResult, flags CommandFlags) error {
	var recipe models.Recipe
	if err := yaml.Unmarshal(data, &recipe); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}
	if err := recipe.Validate(); err != nil {
		if !flags.Force {
			return fmt.Errorf("validation failed: %w", err)
		}
		result.Warnings = append(result.Warnings, fmt.Sprintf("Recipe %s has validation warnings: %v", fileName, err))
	}
	if !flags.Force {
		if recipeExists(recipe.ID) {
			result.SkippedRecipes++
			result.Warnings = append(result.Warnings, fmt.Sprintf("Recipe %s already exists (skipped)", recipe.ID))
			return nil
		}
	}
	if flags.DryRun {
		result.ImportedRecipes++
		fmt.Printf("Would import: %s (%s)\n", recipe.Metadata.Name, recipe.ID)
		return nil
	}
	if err := importSingleRecipe(&recipe); err != nil {
		return fmt.Errorf("failed to import: %w", err)
	}
	result.ImportedRecipes++
	result.ImportedIDs = append(result.ImportedIDs, recipe.ID)
	if flags.Verbose {
		fmt.Printf("Imported: %s (%s)\n", recipe.Metadata.Name, recipe.ID)
	}
	return nil
}
