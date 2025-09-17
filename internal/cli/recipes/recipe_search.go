package recipes

import (
	"encoding/json"
	"fmt"
	"os"

	models "github.com/iw2rmb/ploy/api/recipes/models"
	"gopkg.in/yaml.v3"
)

// handleRecipeList handles the recipe list command with filtering
func handleRecipeList(args []string) error {
	// Parse filter and common flags
	filter, remainingArgs := ParseFilterFlags(args)
	flags := parseCommonFlags(remainingArgs)

	// Validate filter values
	if err := ValidateFilterValues(filter); err != nil {
		PrintError(err)
		return err
	}

	// Validate output format
	if err := ValidateOutputFormat(flags.OutputFormat); err != nil {
		PrintError(err)
		return err
	}

	return listRecipes(filter, flags.OutputFormat, flags.Verbose)
}

// listRecipes lists recipes with filtering and pagination support
func listRecipes(filter RecipeFilter, outputFormat string, verbose bool) error {
	// If catalog mode enabled, use lightweight catalog endpoints
	if os.Getenv("PLOY_RECIPES_CATALOG") == "true" {
		return listCatalogRecipes(outputFormat)
	}

	// Default: use unified registry endpoint
	// Build API query
	queryString := BuildAPIQuery(filter)
	url := fmt.Sprintf("%s/arf/recipes%s", controllerURL, queryString)

	// Make API request
	response, err := makeAPIRequest("GET", url, nil)
	if err != nil {
		cliErr := NewCLIError("Failed to retrieve recipes", 1).
			WithCause(err).
			WithSuggestion("Check network connectivity and controller status")
		PrintError(cliErr)
		return cliErr
	}

	// Parse response
	var data struct {
		Recipes []models.Recipe `json:"recipes"`
		Count   int             `json:"count"`
		Total   int             `json:"total,omitempty"` // Total count for pagination
	}

	if err := json.Unmarshal(response, &data); err != nil {
		cliErr := NewCLIError("Failed to parse recipe data", 1).WithCause(err)
		PrintError(cliErr)
		return cliErr
	}

	// Convert to Recipe pointers for consistency
	recipes := make([]*models.Recipe, len(data.Recipes))
	for i := range data.Recipes {
		recipes[i] = &data.Recipes[i]
	}

	// Sort recipes client-side if requested (in case API doesn't support sorting)
	SortRecipes(recipes, filter.SortBy, filter.SortOrder)

	// Create paginated result
	totalCount := data.Total
	if totalCount == 0 {
		totalCount = data.Count
	}

	page := (filter.Offset / filter.Limit) + 1
	if page < 1 {
		page = 1
	}
	paginationInfo := NewPaginationInfo(page, filter.Limit, totalCount)

	result := PaginatedResult{
		Recipes:    recipes,
		Pagination: paginationInfo,
		Filter:     filter,
	}

	// Display results with pagination
	return DisplayAdvancedPaginatedResult(result, outputFormat, verbose)
}

// showRecipe displays detailed information about a specific recipe
func showRecipe(recipeID string, flags CommandFlags) error {
	url := fmt.Sprintf("%s/arf/recipes/%s", controllerURL, recipeID)
	response, err := makeAPIRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to get recipe: %w", err)
	}

	var recipe models.Recipe
	if err := json.Unmarshal(response, &recipe); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	// Use formatting utility
	return FormatRecipeDetails(&recipe, flags.OutputFormat, flags.Verbose)
}

// searchRecipes searches for recipes using a query string
func searchRecipes(query string, flags CommandFlags) error {
	// Catalog mode: use query parameter with new endpoint and simple presentation
	if os.Getenv("PLOY_RECIPES_CATALOG") == "true" {
		return searchCatalogRecipes(query, flags.OutputFormat, flags.Verbose)
	}

	url := fmt.Sprintf("%s/arf/recipes/search?q=%s", controllerURL, query)
	response, err := makeAPIRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to search recipes: %w", err)
	}

	var data struct {
		Recipes []models.Recipe `json:"recipes"`
		Count   int             `json:"count"`
		Query   string          `json:"query"`
	}

	if err := json.Unmarshal(response, &data); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	// Convert to Recipe pointers for consistency
	recipes := make([]*models.Recipe, len(data.Recipes))
	for i := range data.Recipes {
		recipes[i] = &data.Recipes[i]
	}

	// Use formatting utility
	return FormatSearchResults(recipes, data.Query, flags.OutputFormat, flags.Verbose)
}

// getRecipeStats retrieves and displays usage statistics for a recipe
func getRecipeStats(recipeID string, flags CommandFlags) error {
	url := fmt.Sprintf("%s/arf/recipes/%s/stats", controllerURL, recipeID)
	response, err := makeAPIRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to get recipe stats: %w", err)
	}

	var stats map[string]interface{}
	if err := json.Unmarshal(response, &stats); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	// Handle different output formats
	switch flags.OutputFormat {
	case "json":
		output, _ := json.MarshalIndent(stats, "", "  ")
		fmt.Println(string(output))
	case "yaml":
		output, _ := yaml.Marshal(stats)
		fmt.Println(string(output))
	default: // table format
		fmt.Printf("Recipe Statistics: %s\n", stats["recipe_id"])
		if totalExec, ok := stats["total_executions"].(float64); ok {
			fmt.Printf("Total Executions: %d\n", int(totalExec))
		}
		if successfulRuns, ok := stats["successful_runs"].(float64); ok {
			fmt.Printf("Successful Runs: %d\n", int(successfulRuns))
		}
		if failedRuns, ok := stats["failed_runs"].(float64); ok {
			fmt.Printf("Failed Runs: %d\n", int(failedRuns))
		}
		if successRate, ok := stats["success_rate"].(float64); ok {
			fmt.Printf("Success Rate: %.2f%%\n", successRate*100)
		}

		if totalExec, ok := stats["total_executions"].(float64); ok && totalExec > 0 {
			if avgTime, ok := stats["avg_execution_time"].(string); ok {
				fmt.Printf("Average Execution Time: %s\n", avgTime)
			}
			if lastExec, ok := stats["last_executed"].(string); ok {
				fmt.Printf("Last Executed: %s\n", lastExec)
			}
			if firstExec, ok := stats["first_executed"].(string); ok {
				fmt.Printf("First Executed: %s\n", firstExec)
			}
		}
	}

	return nil
}
