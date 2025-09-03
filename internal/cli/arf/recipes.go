package arf

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/iw2rmb/ploy/api/arf/models"
	"gopkg.in/yaml.v3"
)

// Recipe management commands

// RecipeFilter contains filtering options for recipe listing
type RecipeFilter struct {
	Language   string
	Category   string
	Tags       []string
	Author     string
	Limit      int
	Offset     int
	MinRating  float64
	SortBy     string
	SortOrder  string
}

// CommandFlags contains common flags for recipe commands
type CommandFlags struct {
	DryRun       bool
	Force        bool
	Verbose      bool
	Strict       bool
	OutputFormat string
	OutputFile   string
	Name         string
	Template     string
	Interactive  bool
}

func handleARFRecipesCommand(args []string) error {
	if len(args) == 0 {
		return listRecipes(RecipeFilter{}, "table", false)
	}

	action := args[0]
	switch action {
	case "list", "ls":
		return handleRecipeList(args[1:])
	case "unified":
		return handleUnifiedRecipes(args[1:])
	case "get", "show":
		if len(args) < 2 {
			fmt.Println("Usage: ploy arf recipe show <recipe-id> [--verbose] [--output json|yaml|table]")
			return nil
		}
		flags := parseCommonFlags(args[2:])
		return showRecipe(args[1], flags)
	case "search":
		if len(args) < 2 {
			fmt.Println("Usage: ploy arf recipe search <query> [--output json|yaml|table] [--limit <n>]")
			return nil
		}
		// Extract search query (everything except flags)
		query := ""
		queryArgs := []string{}
		for i := 1; i < len(args); i++ {
			if !strings.HasPrefix(args[i], "--") {
				queryArgs = append(queryArgs, args[i])
			} else {
				// Stop at first flag
				break
			}
		}
		query = strings.Join(queryArgs, " ")
		
		flags := parseCommonFlags(args[len(queryArgs)+1:])
		return searchRecipes(query, flags)
	case "upload", "u":
		if len(args) < 2 {
			fmt.Println("Usage: ploy arf recipe upload <recipe-file> [--name <name>] [--dry-run] [--force]")
			return nil
		}
		flags := parseCommonFlags(args[2:])
		return uploadRecipe(args[1], flags)
	case "update":
		if len(args) < 3 {
			fmt.Println("Usage: ploy arf recipe update <recipe-id> <recipe-file> [--force]")
			return nil
		}
		flags := parseCommonFlags(args[3:])
		return updateRecipe(args[1], args[2], flags)
	case "delete", "del", "rm":
		if len(args) < 2 {
			fmt.Println("Usage: ploy arf recipe delete <recipe-id> [--force]")
			return nil
		}
		flags := parseCommonFlags(args[2:])
		return deleteRecipe(args[1], flags)
	case "download", "dl":
		if len(args) < 2 {
			fmt.Println("Usage: ploy arf recipe download <recipe-id> [--output <file>]")
			return nil
		}
		flags := parseCommonFlags(args[2:])
		return downloadRecipe(args[1], flags)
	case "validate":
		if len(args) < 2 {
			fmt.Println("Usage: ploy arf recipe validate <recipe-file> [--strict]")
			return nil
		}
		flags := parseCommonFlags(args[2:])
		return validateRecipe(args[1], flags)
	case "stats":
		if len(args) < 2 {
			fmt.Println("Usage: ploy arf recipe stats <recipe-id> [--output json|yaml|table]")
			return nil
		}
		flags := parseCommonFlags(args[2:])
		return getRecipeStats(args[1], flags)
	case "create", "init":
		flags := parseCommonFlags(args[1:])
		return createRecipeInteractive(flags)
	case "run":
		if len(args) < 2 {
			fmt.Println("Usage: ploy arf recipe run <recipe-id> [--repo <url>] [--branch <branch>] [--output-dir <dir>] [--report]")
			return nil
		}
		flags := parseCommonFlags(args[2:])
		return runRecipe(args[1], flags)
	case "compose":
		if len(args) < 3 {
			fmt.Println("Usage: ploy arf recipe compose <recipe-id1> <recipe-id2> [...] [--name <composition-name>] [--repo <url>] [options]")
			return nil
		}
		return composeRecipes(args[1:])
	case "import":
		if len(args) < 2 {
			fmt.Println("Usage: ploy arf recipe import <archive-file> [--overwrite] [--validate-only]")
			return nil
		}
		flags := parseCommonFlags(args[2:])
		return importRecipes(args[1], flags)
	case "export":
		if len(args) < 2 {
			fmt.Println("Usage: ploy arf recipe export --output <archive-file> [--tag <tag>] [--author <author>] [--format tar.gz|zip]")
			return nil
		}
		flags := parseCommonFlags(args[1:])
		return exportRecipes(flags)
	case "--help", "-h":
		printRecipesUsage()
		return nil
	case "help":
		if len(args) > 1 {
			// Show help for specific command
			helpSys := NewHelpSystem()
			return helpSys.ShowHelp(args[1])
		}
		printRecipesUsage()
		return nil
	case "examples":
		helpSys := NewHelpSystem()
		return helpSys.ShowExamples()
	case "quickstart":
		helpSys := NewHelpSystem()
		return helpSys.ShowQuickStart()
	case "templates":
		flags := parseCommonFlags(args[1:])
		return listTemplates(flags.OutputFormat, flags.Verbose)
	case "config":
		if len(args) < 2 {
			fmt.Println("Usage: ploy arf recipe config <show|set|reset|list> [key] [value]")
			return nil
		}
		return handleConfigCommand(args[1:])
	default:
		fmt.Printf("Unknown recipes action: %s\n", action)
		printRecipesUsage()
		return nil
	}
}

func printRecipesUsage() {
	fmt.Println("Usage: ploy arf recipe <action> [options]")
	fmt.Println()
	fmt.Println("Available actions:")
	fmt.Println("  list, ls [--filter]              List available recipes with optional filtering")
	fmt.Println("  show <recipe-id>                 Display recipe details")
	fmt.Println("  search <query>                   Search recipes by name/description")
	fmt.Println("  upload, u <recipe-file>          Upload a new recipe from YAML file")
	fmt.Println("  update <id> <recipe-file>        Update existing recipe")
	fmt.Println("  delete, del, rm <recipe-id>      Delete a recipe")
	fmt.Println("  download, dl <recipe-id>         Download recipe to YAML file")
	fmt.Println("  validate <recipe-file>           Validate recipe without uploading")
	fmt.Println("  stats <recipe-id>                Get recipe usage statistics")
	fmt.Println("  create, init                     Create new recipe interactively")
	fmt.Println("  run <recipe-id>                  Execute recipe against repository")
	fmt.Println("  compose <recipe-ids...>          Chain multiple recipes in sequence")
	fmt.Println("  import <archive-file>            Import recipes from archive")
	fmt.Println("  export --output <file>           Export recipes to archive")
	fmt.Println()
	fmt.Println("Common flags:")
	fmt.Println("  --output, -o <format>            Output format: table, json, yaml")
	fmt.Println("  --verbose, -v                    Show detailed information")
	fmt.Println("  --force, -f                      Force operation (skip confirmations)")
	fmt.Println("  --dry-run, -n                    Validate without executing")
	fmt.Println("  --strict, -s                     Enable strict validation")
	fmt.Println()
	fmt.Println("List filters:")
	fmt.Println("  --language <lang>                Filter by programming language")
	fmt.Println("  --category <cat>                 Filter by category")
	fmt.Println("  --tag <tag>                      Filter by tag (can be used multiple times)")
	fmt.Println("  --author <author>                Filter by author")
	fmt.Println("  --limit <n>                      Maximum number of results (default: 20)")
	fmt.Println("  --offset <n>                     Offset for pagination (default: 0)")
	fmt.Println("  --sort-by <field>                Sort by: name, created, updated, rating")
	fmt.Println("  --sort-order <order>             Sort order: asc, desc (default: asc)")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  ploy arf recipe list --language java --output json")
	fmt.Println("  ploy arf recipe upload my-recipe.yaml --dry-run")
	fmt.Println("  ploy arf recipe search 'spring migration' --limit 5")
	fmt.Println("  ploy arf recipe run java11to17 --repo https://github.com/user/repo")
	fmt.Println("  ploy arf recipe compose recipe1 recipe2 --name 'full-migration'")
	fmt.Println("  ploy arf recipe export --output recipes-backup.tar.gz --tag migration")
}

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

func listRecipes(filter RecipeFilter, outputFormat string, verbose bool) error {
	// If catalog mode enabled, use lightweight catalog endpoints
	if os.Getenv("PLOY_RECIPES_CATALOG") == "true" {
		return listCatalogRecipes(outputFormat)
	}

	// Default: use unified registry endpoint
	// Build API query
	queryString := BuildAPIQuery(filter)
	url := fmt.Sprintf("%s/arf/recipes%s", arfControllerURL, queryString)

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

func showRecipe(recipeID string, flags CommandFlags) error {
	url := fmt.Sprintf("%s/arf/recipes/%s", arfControllerURL, recipeID)
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

	return nil
}

func searchRecipes(query string, flags CommandFlags) error {
	// Catalog mode: use query parameter with new endpoint and simple presentation
	if os.Getenv("PLOY_RECIPES_CATALOG") == "true" {
		return searchCatalogRecipes(query, flags.OutputFormat, flags.Verbose)
	}

	url := fmt.Sprintf("%s/arf/recipes/search?q=%s", arfControllerURL, query)
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

// Catalog client types and helpers (lightweight endpoints)
type catalogRecipe struct {
    ID          string   `json:"id"`
    DisplayName string   `json:"display_name"`
    Description string   `json:"description"`
    Tags        []string `json:"tags"`
    Pack        string   `json:"pack"`
    Version     string   `json:"version"`
}

func listCatalogRecipes(outputFormat string) error {
    url := fmt.Sprintf("%s/arf/recipes", arfControllerURL)
    response, err := makeAPIRequest("GET", url, nil)
    if err != nil {
        return fmt.Errorf("failed to retrieve catalog: %w", err)
    }
    items, err := parseCatalogList(response)
    if err != nil {
        return err
    }
    return printCatalog(items, outputFormat, false)
}

func searchCatalogRecipes(query, outputFormat string, verbose bool) error {
    url := fmt.Sprintf("%s/arf/recipes?query=%s", arfControllerURL, query)
    response, err := makeAPIRequest("GET", url, nil)
    if err != nil {
        return fmt.Errorf("failed to search catalog: %w", err)
    }
    items, err := parseCatalogList(response)
    if err != nil {
        return err
    }
    return printCatalog(items, outputFormat, verbose)
}

// parseCatalogList parses the catalog array payload (used in tests)
func parseCatalogList(data []byte) ([]catalogRecipe, error) {
    var items []catalogRecipe
    if err := json.Unmarshal(data, &items); err != nil {
        return nil, fmt.Errorf("failed to parse catalog list: %w", err)
    }
    return items, nil
}

func printCatalog(items []catalogRecipe, format string, verbose bool) error {
    switch format {
    case "json":
        out, _ := json.MarshalIndent(items, "", "  ")
        fmt.Println(string(out))
        return nil
    case "yaml":
        // minimal YAML via json2yaml-style is not available; fallback to json for now
        out, _ := json.MarshalIndent(items, "", "  ")
        fmt.Println(string(out))
        return nil
    default:
        if len(items) == 0 {
            fmt.Println("No recipes found")
            return nil
        }
        // simple table-like output
        fmt.Printf("ID\tPACK\tVERSION\tNAME\n")
        for _, it := range items {
            name := it.DisplayName
            if name == "" { name = it.ID }
            fmt.Printf("%s\t%s\t%s\t%s\n", it.ID, it.Pack, it.Version, name)
            if verbose && it.Description != "" {
                fmt.Printf("  %s\n", it.Description)
            }
        }
        fmt.Printf("Total: %d recipes\n", len(items))
        return nil
    }
}

func getRecipeStats(recipeID string, flags CommandFlags) error {
	url := fmt.Sprintf("%s/arf/recipes/%s/stats", arfControllerURL, recipeID)
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
	
	url := fmt.Sprintf("%s/arf/recipes/upload", arfControllerURL)
	response, err := makeAPIRequest("POST", url, recipeJSON)
	if err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}
	
	var result struct {
		ID      string `json:"id"`
		Message string `json:"message"`
	}
	json.Unmarshal(response, &result)
	
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
	
	url := fmt.Sprintf("%s/arf/recipes/%s", arfControllerURL, recipeID)
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
		fmt.Scanln(&confirm)
		if strings.ToLower(confirm) != "y" {
			fmt.Println("Deletion cancelled")
			return nil
		}
	}
	
	url := fmt.Sprintf("%s/arf/recipes/%s", arfControllerURL, recipeID)
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
	url := fmt.Sprintf("%s/arf/recipes/%s", arfControllerURL, recipeID)
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

// UploadFlags contains flags for the upload command (legacy)
type UploadFlags struct {
	DryRun bool
	Force  bool
	Name   string
}

// parseCommonFlags parses common command flags
func parseCommonFlags(args []string) CommandFlags {
	flags := CommandFlags{
		OutputFormat: "table", // Default output format
	}
	
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--dry-run", "-n":
			flags.DryRun = true
		case "--force", "-f":
			flags.Force = true
		case "--verbose", "-v":
			flags.Verbose = true
		case "--strict", "-s":
			flags.Strict = true
		case "--interactive", "-i":
			flags.Interactive = true
		case "--output", "-o":
			if i+1 < len(args) {
				flags.OutputFormat = args[i+1]
				i++
			}
		case "--name":
			if i+1 < len(args) {
				flags.Name = args[i+1]
				i++
			}
		case "--template", "-t":
			if i+1 < len(args) {
				flags.Template = args[i+1]
				i++
			}
		case "--file":
			if i+1 < len(args) {
				flags.OutputFile = args[i+1]
				i++
			}
		}
	}
	return flags
}

// Helper functions to parse command flags (legacy support)
func parseUploadFlags(args []string) UploadFlags {
	flags := parseCommonFlags(args)
	return UploadFlags{
		DryRun: flags.DryRun,
		Force:  flags.Force,
		Name:   flags.Name,
	}
}

// Legacy flag parsing functions for backward compatibility
func parseVerboseFlag(args []string) bool {
	flags := parseCommonFlags(args)
	return flags.Verbose
}

func parseForceFlag(args []string) bool {
	flags := parseCommonFlags(args)
	return flags.Force
}

func parseStrictFlag(args []string) bool {
	flags := parseCommonFlags(args)
	return flags.Strict
}

func parseOutputFile(args []string) string {
	flags := parseCommonFlags(args)
	return flags.OutputFile
}

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
	fmt.Println(`Usage: ploy arf recipes unified <command> [options]

Commands:
  list [options]           List all available recipes from unified registry
    --type <type>          Filter by recipe type (openrewrite, shell, custom)
    --source <source>      Filter by source (maven, custom)
    
  get <recipe-id>          Get details of a specific recipe
  
  search <keyword>         Search recipes by keyword

Examples:
  # List all unified recipes
  ploy arf recipes unified list
  
  # List only OpenRewrite recipes
  ploy arf recipes unified list --type openrewrite
  
  # Get details of a specific recipe
  ploy arf recipes unified get java11to17
  
  # Search for Java migration recipes
  ploy arf recipes unified search java`)
}




