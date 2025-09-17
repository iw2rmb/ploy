package recipes

import (
	"fmt"
	"os"
	"strings"

	models "github.com/iw2rmb/ploy/api/recipes/models"
	"gopkg.in/yaml.v3"
)

// validateRecipe validates a recipe file without uploading
func validateRecipe(recipePath string, flags CommandFlags) error {
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

	// Basic validation
	if err := recipe.Validate(); err != nil {
		fmt.Printf("❌ Recipe validation failed: %v\n", err)
		return nil
	}

	// Additional strict validation
	if flags.Strict {
		warnings := []string{}

		// Check for missing optional but recommended fields
		if recipe.Metadata.MinPlatform == "" {
			warnings = append(warnings, "Missing minimum platform version")
		}
		if len(recipe.Metadata.Tags) == 0 {
			warnings = append(warnings, "No tags specified")
		}
		if recipe.Metadata.License == "" {
			warnings = append(warnings, "No license specified")
		}

		// Check step configurations
		for i, step := range recipe.Steps {
			if step.Timeout.Duration == 0 {
				warnings = append(warnings, fmt.Sprintf("Step %d (%s) has no timeout specified", i+1, step.Name))
			}
		}

		if len(warnings) > 0 {
			fmt.Println("⚠️  Warnings (strict mode):")
			for _, warning := range warnings {
				fmt.Printf("  - %s\n", warning)
			}
		}
	}

	fmt.Printf("✅ Recipe '%s' is valid\n", recipe.Metadata.Name)

	// Display recipe summary
	fmt.Printf("\nRecipe Summary:\n")
	fmt.Printf("  Name: %s\n", recipe.Metadata.Name)
	fmt.Printf("  Version: %s\n", recipe.Metadata.Version)
	fmt.Printf("  Steps: %d\n", len(recipe.Steps))
	fmt.Printf("  Languages: %s\n", strings.Join(recipe.Metadata.Languages, ", "))
	fmt.Printf("  Categories: %s\n", strings.Join(recipe.Metadata.Categories, ", "))

	return nil
}
