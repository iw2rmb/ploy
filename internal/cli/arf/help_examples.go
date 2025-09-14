package arf

import (
	"fmt"
	"strings"
)

// ShowExamples displays curated examples for common use cases
func (hs *HelpSystem) ShowExamples() error {
	fmt.Printf("ARF Recipe Examples\n")
	fmt.Printf("===================\n\n")

	examples := []struct {
		Title       string
		Description string
		Commands    []string
	}{
		{
			Title:       "Basic Recipe Management",
			Description: "Common operations for managing recipes",
			Commands: []string{
				"# List all available recipes",
				"ploy arf recipe list",
				"",
				"# Search for Spring Boot recipes",
				"ploy arf recipe search 'spring boot'",
				"",
				"# Show details of a specific recipe",
				"ploy arf recipe show spring-boot-2-to-3",
				"",
				"# Upload a new recipe from file",
				"ploy arf recipe upload my-recipe.yaml",
			},
		},
		// Execution workflows via ARF have been removed; use Mods.
		{
			Title:       "Advanced Filtering and Search",
			Description: "Find specific recipes using filters",
			Commands: []string{
				"# Filter by language and category",
				"ploy arf recipe list --language java --category migration",
				"",
				"# Multiple tag filtering with JSON output",
				"ploy arf recipe list --tag spring --tag boot --output json",
				"",
				"# Sort by creation date, show recent first",
				"ploy arf recipe list --sort-by created --sort-order desc --limit 5",
				"",
				"# Find recipes by specific author",
				"ploy arf recipe list --author openrewrite --verbose",
			},
		},
		{
			Title:       "Recipe Creation and Templates",
			Description: "Create new recipes using templates",
			Commands: []string{
				"# Interactive recipe creation",
				"ploy arf recipe create",
				"",
				"# Create using specific template",
				"ploy arf recipe create --template openrewrite",
				"",
				"# Preview without creating",
				"ploy arf recipe create --template shell --dry-run",
				"",
				"# Validate existing recipe file",
				"ploy arf recipe validate my-recipe.yaml --strict",
			},
		},
		{
			Title:       "Bulk Operations and Backup",
			Description: "Import, export, and manage recipe collections",
			Commands: []string{
				"# Export all recipes to archive",
				"ploy arf recipe export --output backup.tar.gz",
				"",
				"# Export specific category as ZIP",
				"ploy arf recipe export --output migrations.zip \\",
				"  --category migration --format zip",
				"",
				"# Import recipes from archive",
				"ploy arf recipe import backup.tar.gz --verbose",
				"",
				"# Validate archive without importing",
				"ploy arf recipe import recipes.tar.gz --validate-only",
			},
		},
		{
			Title:       "Integration with Development Workflow",
			Description: "Use recipes in CI/CD and development processes",
			Commands: []string{
				"# Parallel execution for independent changes",
				"ploy arf recipe compose style-fix license-update \\",
				"  --parallel \\",
				"  --continue-on-error",
			},
		},
	}

	for i, example := range examples {
		if i > 0 {
			fmt.Println()
		}

		fmt.Printf("%d. %s\n", i+1, example.Title)
		fmt.Printf("   %s\n\n", example.Description)

		for _, cmd := range example.Commands {
			if cmd == "" {
				fmt.Println()
			} else if strings.HasPrefix(cmd, "#") {
				fmt.Printf("   \033[32m%s\033[0m\n", cmd) // Green for comments
			} else {
				fmt.Printf("   %s\n", cmd)
			}
		}
	}

	fmt.Printf("\nFor more examples and tutorials:\n")
	fmt.Printf("  https://docs.ployd.app/arf/examples\n")

	return nil
}

// ShowQuickStart displays a quick start guide
func (hs *HelpSystem) ShowQuickStart() error {
	fmt.Printf("ARF Recipe Quick Start\n")
	fmt.Printf("======================\n\n")

	steps := []struct {
		Step        string
		Description string
		Command     string
	}{
		{
			Step:        "1. List Available Recipes",
			Description: "See what transformation recipes are available",
			Command:     "ploy arf recipe list",
		},
		{
			Step:        "2. Find Relevant Recipes",
			Description: "Search for recipes matching your technology stack",
			Command:     "ploy arf recipe search 'java spring'",
		},
		{
			Step:        "3. Examine Recipe Details",
			Description: "Review what a recipe does before running it",
			Command:     "ploy arf recipe show java11to17-migration",
		},
		// Execution via ARF removed; use Mods for running transformations.
		{
			Step:        "6. Create Custom Recipe",
			Description: "Build your own transformation recipe",
			Command:     "ploy arf recipe create --template openrewrite",
		},
	}

	for _, step := range steps {
		fmt.Printf("%s\n", step.Step)
		fmt.Printf("   %s\n", step.Description)
		fmt.Printf("   \033[36m$ %s\033[0m\n\n", step.Command) // Cyan for commands
	}

	fmt.Printf("Next Steps:\n")
	fmt.Printf("  • Read the recipe format guide: https://docs.ployd.app/arf/recipes\n")
	fmt.Printf("  • Browse example recipes: https://docs.ployd.app/arf/examples\n")
	fmt.Printf("  • Join the community: https://github.com/iw2rmb/recipes\n")

	return nil
}
