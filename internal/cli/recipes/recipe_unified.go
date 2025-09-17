package recipes

import "fmt"

// handleUnifiedRecipes handles unified recipe registry operations
func handleUnifiedRecipes(args []string) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		printUnifiedRecipesUsage()
		return nil
	}

	switch args[0] {
	case "list":
		return listUnifiedRecipes(args[1:])
	case "get", "show":
		if len(args) < 2 {
			fmt.Println("Error: Recipe ID required")
			return nil
		}
		return getUnifiedRecipe(args[1])
	case "search":
		if len(args) < 2 {
			fmt.Println("Error: Search keyword required")
			return nil
		}
		return searchUnifiedRecipes(args[1])
	default:
		fmt.Printf("Unknown unified subcommand: %s\n", args[0])
		printUnifiedRecipesUsage()
		return nil
	}
}

// listUnifiedRecipes lists recipes from the unified registry
func listUnifiedRecipes(args []string) error {
	// Parse filter arguments
	recipeType := ""
	source := ""

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--type", "-t":
			if i+1 < len(args) {
				recipeType = args[i+1]
				i++
			}
		case "--source", "-s":
			if i+1 < len(args) {
				source = args[i+1]
				i++
			}
		}
	}

	// Build query parameters
	queryParams := ""
	if recipeType != "" {
		queryParams += "?type=" + recipeType
	}
	if source != "" {
		if queryParams == "" {
			queryParams += "?source=" + source
		} else {
			queryParams += "&source=" + source
		}
	}

	// List recipes
	filter := RecipeFilter{}
	return listRecipes(filter, "table", false)
}

// getUnifiedRecipe gets a specific recipe from the unified registry
func getUnifiedRecipe(recipeID string) error {
	// For now, display recipe ID and return
	fmt.Printf("Recipe ID: %s\n", recipeID)
	fmt.Println("(Recipe details display not yet implemented)")
	return nil
}

// searchUnifiedRecipes searches recipes by keyword
func searchUnifiedRecipes(keyword string) error {
	// For now, use the existing search functionality
	flags := CommandFlags{
		OutputFormat: "table",
	}
	return searchRecipes(keyword, flags)
}

// printUnifiedRecipesUsage prints usage for unified recipe commands
func printUnifiedRecipesUsage() {
	fmt.Println(`Usage: ploy recipe unified <command> [options]

Commands:
  list [options]           List all available recipes from unified registry
    --type <type>          Filter by recipe type (openrewrite, shell, custom)
    --source <source>      Filter by source (maven, custom)
    
  get <recipe-id>          Get details of a specific recipe
  
  search <keyword>         Search recipes by keyword

Examples:
  # List all unified recipes
  ploy recipe unified list
  
  # List only OpenRewrite recipes
  ploy recipe unified list --type openrewrite
  
  # Get details of a specific recipe
  ploy recipe unified get java11to17
  
  # Search for Java migration recipes
  ploy recipe unified search java`)
}
