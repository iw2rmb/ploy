package recipes

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	models "github.com/iw2rmb/ploy/internal/arf/models"
	"gopkg.in/yaml.v3"
)

// uploadRecipe uploads a new recipe from a YAML file
func uploadRecipe(recipePath string, flags CommandFlags) error {
	// Read recipe file
	data, err := os.ReadFile(recipePath)
	if err != nil {
		return fmt.Errorf("failed to read recipe file: %w", err)
	}

	// Parse YAML
	var recipe models.Recipe
	if err := yaml.Unmarshal(data, &recipe); err != nil {
		return fmt.Errorf("failed to parse recipe YAML: %w", err)
	}

	// Override name if specified
	if flags.Name != "" {
		recipe.Metadata.Name = flags.Name
	}

	// Validate recipe
	if err := recipe.Validate(); err != nil {
		if !flags.Force {
			return fmt.Errorf("recipe validation failed: %w", err)
		}
		fmt.Printf("Warning: %v (continuing due to --force)\n", err)
	}

	// Dry run mode
	if flags.DryRun {
		fmt.Printf("Recipe '%s' is valid and ready for upload\n", recipe.Metadata.Name)
		return nil
	}

	// Send to API
	recipeJSON, err := json.Marshal(recipe)
	if err != nil {
		return fmt.Errorf("failed to serialize recipe: %w", err)
	}

	url := fmt.Sprintf("%s/arf/recipes/upload", controllerURL)
	response, err := makeAPIRequest("POST", url, recipeJSON)
	if err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}

	var result struct {
		ID      string `json:"id"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(response, &result)

	fmt.Printf("Recipe '%s' uploaded successfully (ID: %s)\n", recipe.Metadata.Name, result.ID)
	return nil
}

// updateRecipe updates an existing recipe from a YAML file
func updateRecipe(recipeID, recipePath string, flags CommandFlags) error {
	// Read recipe file
	data, err := os.ReadFile(recipePath)
	if err != nil {
		return fmt.Errorf("failed to read recipe file: %w", err)
	}

	// Parse YAML
	var recipe models.Recipe
	if err := yaml.Unmarshal(data, &recipe); err != nil {
		return fmt.Errorf("failed to parse recipe YAML: %w", err)
	}

	// Validate recipe
	if err := recipe.Validate(); err != nil {
		return fmt.Errorf("recipe validation failed: %w", err)
	}

	// Send to API
	recipeJSON, err := json.Marshal(recipe)
	if err != nil {
		return fmt.Errorf("failed to serialize recipe: %w", err)
	}

	url := fmt.Sprintf("%s/arf/recipes/%s", controllerURL, recipeID)
	_, err = makeAPIRequest("PUT", url, recipeJSON)
	if err != nil {
		return fmt.Errorf("update failed: %w", err)
	}

	fmt.Printf("Recipe '%s' updated successfully\n", recipeID)
	return nil
}

// deleteRecipe deletes a recipe by ID
func deleteRecipe(recipeID string, flags CommandFlags) error {
	// Confirm deletion unless force flag is set
	if !flags.Force {
		fmt.Printf("Are you sure you want to delete recipe '%s'? (y/N): ", recipeID)
		var confirm string
		_, _ = fmt.Scanln(&confirm)
		if strings.ToLower(confirm) != "y" {
			fmt.Println("Deletion cancelled")
			return nil
		}
	}

	url := fmt.Sprintf("%s/arf/recipes/%s", controllerURL, recipeID)
	_, err := makeAPIRequest("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("deletion failed: %w", err)
	}

	fmt.Printf("Recipe '%s' deleted successfully\n", recipeID)
	return nil
}

// downloadRecipe downloads a recipe to a YAML file
func downloadRecipe(recipeID string, flags CommandFlags) error {
	// Fetch recipe from API
	url := fmt.Sprintf("%s/arf/recipes/%s", controllerURL, recipeID)
	response, err := makeAPIRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to fetch recipe: %w", err)
	}

	var recipe models.Recipe
	if err := json.Unmarshal(response, &recipe); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	// Convert to YAML
	yamlData, err := yaml.Marshal(recipe)
	if err != nil {
		return fmt.Errorf("failed to convert to YAML: %w", err)
	}

	// Determine output file name
	outputFile := flags.OutputFile
	if outputFile == "" {
		outputFile = fmt.Sprintf("%s.yaml", recipeID)
	}

	// Write to file
	if err := os.WriteFile(outputFile, yamlData, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	fmt.Printf("Recipe downloaded to %s\n", outputFile)
	return nil
}
