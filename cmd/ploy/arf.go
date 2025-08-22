package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/ploy/ploy/controller/arf"
)

// ARF command handlers

func handleARFCommand(args []string) error {
	if len(args) < 2 {
		printARFUsage()
		return nil
	}

	subcommand := args[1]
	switch subcommand {
	case "recipes":
		return handleARFRecipesCommand(args[2:])
	case "transform":
		return handleARFTransformCommand(args[2:])
	case "sandbox":
		return handleARFSandboxCommand(args[2:])
	case "health":
		return handleARFHealthCommand()
	case "cache":
		return handleARFCacheCommand(args[2:])
	default:
		fmt.Printf("Unknown ARF subcommand: %s\n", subcommand)
		printARFUsage()
		return nil
	}
}

func printARFUsage() {
	fmt.Println("Usage: ploy arf <subcommand> [options]")
	fmt.Println()
	fmt.Println("Available subcommands:")
	fmt.Println("  recipes    Manage transformation recipes")
	fmt.Println("  transform  Execute code transformations")
	fmt.Println("  sandbox    Manage sandboxes")
	fmt.Println("  health     Check ARF system health")
	fmt.Println("  cache      Manage AST cache")
	fmt.Println()
	fmt.Println("Use 'ploy arf <subcommand> --help' for more information")
}

// Recipe management commands

func handleARFRecipesCommand(args []string) error {
	if len(args) == 0 {
		return listRecipes("")
	}

	action := args[0]
	switch action {
	case "list":
		language := ""
		if len(args) > 1 && args[1] == "--language" && len(args) > 2 {
			language = args[2]
		}
		return listRecipes(language)
	case "get":
		if len(args) < 2 {
			fmt.Println("Usage: ploy arf recipes get <recipe-id>")
			return nil
		}
		return getRecipe(args[1])
	case "search":
		if len(args) < 2 {
			fmt.Println("Usage: ploy arf recipes search <query>")
			return nil
		}
		return searchRecipes(strings.Join(args[1:], " "))
	case "stats":
		if len(args) < 2 {
			fmt.Println("Usage: ploy arf recipes stats <recipe-id>")
			return nil
		}
		return getRecipeStats(args[1])
	case "create":
		return createRecipeInteractive()
	case "--help":
		printRecipesUsage()
		return nil
	default:
		fmt.Printf("Unknown recipes action: %s\n", action)
		printRecipesUsage()
		return nil
	}
}

func printRecipesUsage() {
	fmt.Println("Usage: ploy arf recipes <action> [options]")
	fmt.Println()
	fmt.Println("Available actions:")
	fmt.Println("  list                    List all available recipes")
	fmt.Println("  list --language <lang>  List recipes for specific language")
	fmt.Println("  get <recipe-id>         Get recipe details")
	fmt.Println("  search <query>          Search recipes by name/description")
	fmt.Println("  stats <recipe-id>       Get recipe usage statistics")
	fmt.Println("  create                  Create new recipe interactively")
}

func listRecipes(language string) error {
	url := fmt.Sprintf("%s/v1/arf/recipes", getControllerURL())
	if language != "" {
		url += "?language=" + language
	}

	response, err := makeAPIRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to list recipes: %w", err)
	}

	var data struct {
		Recipes []arf.Recipe `json:"recipes"`
		Count   int          `json:"count"`
	}

	if err := json.Unmarshal(response, &data); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if data.Count == 0 {
		fmt.Println("No recipes found")
		return nil
	}

	// Display recipes in table format
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tLANGUAGE\tCATEGORY\tCONFIDENCE\tTAGS")
	fmt.Fprintln(w, "--\t----\t--------\t--------\t----------\t----")

	for _, recipe := range data.Recipes {
		tags := strings.Join(recipe.Tags, ",")
		if len(tags) > 30 {
			tags = tags[:27] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%.2f\t%s\n",
			recipe.ID, recipe.Name, recipe.Language, recipe.Category, recipe.Confidence, tags)
	}

	w.Flush()
	fmt.Printf("\nTotal: %d recipes\n", data.Count)
	return nil
}

func getRecipe(recipeID string) error {
	url := fmt.Sprintf("%s/v1/arf/recipes/%s", getControllerURL(), recipeID)
	response, err := makeAPIRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to get recipe: %w", err)
	}

	var recipe arf.Recipe
	if err := json.Unmarshal(response, &recipe); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	fmt.Printf("Recipe: %s\n", recipe.ID)
	fmt.Printf("Name: %s\n", recipe.Name)
	fmt.Printf("Description: %s\n", recipe.Description)
	fmt.Printf("Language: %s\n", recipe.Language)
	fmt.Printf("Category: %s\n", recipe.Category)
	fmt.Printf("Confidence: %.2f\n", recipe.Confidence)
	fmt.Printf("Source: %s\n", recipe.Source)
	fmt.Printf("Version: %s\n", recipe.Version)
	fmt.Printf("Tags: %s\n", strings.Join(recipe.Tags, ", "))

	if len(recipe.Options) > 0 {
		fmt.Println("\nOptions:")
		for key, value := range recipe.Options {
			fmt.Printf("  %s: %s\n", key, value)
		}
	}

	return nil
}

func searchRecipes(query string) error {
	url := fmt.Sprintf("%s/v1/arf/recipes/search?q=%s", getControllerURL(), query)
	response, err := makeAPIRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to search recipes: %w", err)
	}

	var data struct {
		Recipes []arf.Recipe `json:"recipes"`
		Count   int          `json:"count"`
		Query   string       `json:"query"`
	}

	if err := json.Unmarshal(response, &data); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	fmt.Printf("Search results for \"%s\":\n\n", data.Query)

	if data.Count == 0 {
		fmt.Println("No recipes found")
		return nil
	}

	for _, recipe := range data.Recipes {
		fmt.Printf("• %s (%s)\n", recipe.Name, recipe.ID)
		fmt.Printf("  %s\n", recipe.Description)
		fmt.Printf("  Language: %s | Category: %s | Confidence: %.2f\n", 
			recipe.Language, recipe.Category, recipe.Confidence)
		fmt.Println()
	}

	fmt.Printf("Total: %d recipes\n", data.Count)
	return nil
}

func getRecipeStats(recipeID string) error {
	url := fmt.Sprintf("%s/v1/arf/recipes/%s/stats", getControllerURL(), recipeID)
	response, err := makeAPIRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to get recipe stats: %w", err)
	}

	var stats arf.RecipeStats
	if err := json.Unmarshal(response, &stats); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	fmt.Printf("Recipe Statistics: %s\n", stats.RecipeID)
	fmt.Printf("Total Executions: %d\n", stats.TotalExecutions)
	fmt.Printf("Successful Runs: %d\n", stats.SuccessfulRuns)
	fmt.Printf("Failed Runs: %d\n", stats.FailedRuns)
	fmt.Printf("Success Rate: %.2f%%\n", stats.SuccessRate*100)
	
	if stats.TotalExecutions > 0 {
		fmt.Printf("Average Execution Time: %s\n", stats.AvgExecutionTime.String())
		fmt.Printf("Last Executed: %s\n", stats.LastExecuted.Format(time.RFC3339))
		fmt.Printf("First Executed: %s\n", stats.FirstExecuted.Format(time.RFC3339))
	}

	return nil
}

func createRecipeInteractive() error {
	fmt.Println("Creating new recipe (interactive mode)")
	fmt.Println("Press Ctrl+C to cancel at any time")
	fmt.Println()

	recipe := arf.Recipe{
		Options: make(map[string]string),
	}

	// Get basic recipe information
	recipe.ID = promptUser("Recipe ID: ")
	recipe.Name = promptUser("Name: ")
	recipe.Description = promptUser("Description: ")
	recipe.Language = promptUser("Language (java/go/python/etc): ")
	
	categoryStr := promptUser("Category (cleanup/modernize/security/etc): ")
	recipe.Category = arf.RecipeCategory(categoryStr)
	
	confidenceStr := promptUser("Confidence (0.0-1.0): ")
	if conf, err := strconv.ParseFloat(confidenceStr, 64); err == nil {
		recipe.Confidence = conf
	} else {
		recipe.Confidence = 0.8 // Default
	}
	
	recipe.Source = promptUser("OpenRewrite class name: ")
	recipe.Version = promptUser("Version (optional): ")
	
	tagsStr := promptUser("Tags (comma-separated): ")
	if tagsStr != "" {
		recipe.Tags = strings.Split(tagsStr, ",")
		for i, tag := range recipe.Tags {
			recipe.Tags[i] = strings.TrimSpace(tag)
		}
	}

	// Serialize and send
	data, err := json.Marshal(recipe)
	if err != nil {
		return fmt.Errorf("failed to serialize recipe: %w", err)
	}

	url := fmt.Sprintf("%s/v1/arf/recipes", getControllerURL())
	_, err = makeAPIRequest("POST", url, data)
	if err != nil {
		return fmt.Errorf("failed to create recipe: %w", err)
	}

	fmt.Printf("\nRecipe '%s' created successfully!\n", recipe.ID)
	return nil
}

// Transform command

func handleARFTransformCommand(args []string) error {
	if len(args) == 0 {
		printTransformUsage()
		return nil
	}

	if args[0] == "--help" {
		printTransformUsage()
		return nil
	}

	// Parse arguments
	recipeID := ""
	repository := ""
	branch := "main"
	language := ""

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--recipe":
			if i+1 < len(args) {
				recipeID = args[i+1]
				i++
			}
		case "--repo":
			if i+1 < len(args) {
				repository = args[i+1]
				i++
			}
		case "--branch":
			if i+1 < len(args) {
				branch = args[i+1]
				i++
			}
		case "--language":
			if i+1 < len(args) {
				language = args[i+1]
				i++
			}
		}
	}

	if recipeID == "" {
		fmt.Println("Error: --recipe is required")
		printTransformUsage()
		return nil
	}

	if repository == "" {
		fmt.Println("Error: --repo is required")
		printTransformUsage()
		return nil
	}

	return executeTransformation(recipeID, repository, branch, language)
}

func printTransformUsage() {
	fmt.Println("Usage: ploy arf transform --recipe <recipe-id> --repo <repository> [options]")
	fmt.Println()
	fmt.Println("Required options:")
	fmt.Println("  --recipe <id>    Recipe ID to execute")
	fmt.Println("  --repo <url>     Repository URL to transform")
	fmt.Println()
	fmt.Println("Optional options:")
	fmt.Println("  --branch <name>  Git branch (default: main)")
	fmt.Println("  --language <lang> Programming language")
}

func executeTransformation(recipeID, repository, branch, language string) error {
	fmt.Printf("Executing transformation...\n")
	fmt.Printf("Recipe: %s\n", recipeID)
	fmt.Printf("Repository: %s\n", repository)
	fmt.Printf("Branch: %s\n", branch)
	
	if language != "" {
		fmt.Printf("Language: %s\n", language)
	}
	fmt.Println()

	// Prepare transformation request
	request := map[string]interface{}{
		"recipe_id": recipeID,
		"codebase": map[string]interface{}{
			"repository": repository,
			"branch":     branch,
			"language":   language,
		},
	}

	data, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to serialize request: %w", err)
	}

	// Execute transformation
	url := fmt.Sprintf("%s/v1/arf/transform", getControllerURL())
	response, err := makeAPIRequest("POST", url, data)
	if err != nil {
		return fmt.Errorf("transformation failed: %w", err)
	}

	var result arf.TransformationResult
	if err := json.Unmarshal(response, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	// Display results
	fmt.Printf("Transformation completed!\n\n")
	fmt.Printf("Status: ")
	if result.Success {
		fmt.Printf("✅ Success\n")
	} else {
		fmt.Printf("❌ Failed\n")
	}
	
	fmt.Printf("Changes Applied: %d\n", result.ChangesApplied)
	fmt.Printf("Files Modified: %d\n", len(result.FilesModified))
	fmt.Printf("Execution Time: %s\n", result.ExecutionTime.String())
	fmt.Printf("Validation Score: %.2f\n", result.ValidationScore)

	if len(result.FilesModified) > 0 {
		fmt.Println("\nModified Files:")
		for _, file := range result.FilesModified {
			fmt.Printf("  • %s\n", file)
		}
	}

	if len(result.Errors) > 0 {
		fmt.Println("\nErrors:")
		for _, err := range result.Errors {
			fmt.Printf("  ❌ %s\n", err.Message)
			if err.File != "" {
				fmt.Printf("     File: %s:%d:%d\n", err.File, err.Line, err.Column)
			}
		}
	}

	if len(result.Warnings) > 0 {
		fmt.Println("\nWarnings:")
		for _, warn := range result.Warnings {
			fmt.Printf("  ⚠️  %s\n", warn.Message)
		}
	}

	return nil
}

// Sandbox commands

func handleARFSandboxCommand(args []string) error {
	if len(args) == 0 {
		return listSandboxes()
	}

	action := args[0]
	switch action {
	case "list":
		return listSandboxes()
	case "create":
		return createSandboxInteractive()
	case "destroy":
		if len(args) < 2 {
			fmt.Println("Usage: ploy arf sandbox destroy <sandbox-id>")
			return nil
		}
		return destroySandbox(args[1])
	case "--help":
		printSandboxUsage()
		return nil
	default:
		fmt.Printf("Unknown sandbox action: %s\n", action)
		printSandboxUsage()
		return nil
	}
}

func printSandboxUsage() {
	fmt.Println("Usage: ploy arf sandbox <action> [options]")
	fmt.Println()
	fmt.Println("Available actions:")
	fmt.Println("  list              List active sandboxes")
	fmt.Println("  create            Create new sandbox interactively")
	fmt.Println("  destroy <id>      Destroy sandbox")
}

func listSandboxes() error {
	url := fmt.Sprintf("%s/v1/arf/sandboxes", getControllerURL())
	response, err := makeAPIRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to list sandboxes: %w", err)
	}

	var data struct {
		Sandboxes []arf.SandboxInfo `json:"sandboxes"`
		Count     int               `json:"count"`
	}

	if err := json.Unmarshal(response, &data); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if data.Count == 0 {
		fmt.Println("No active sandboxes")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tJAIL NAME\tSTATUS\tCREATED\tEXPIRES")
	fmt.Fprintln(w, "--\t---------\t------\t-------\t-------")

	for _, sandbox := range data.Sandboxes {
		created := sandbox.CreatedAt.Format("15:04:05")
		expires := sandbox.ExpiresAt.Format("15:04:05")
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			sandbox.ID, sandbox.JailName, sandbox.Status, created, expires)
	}

	w.Flush()
	fmt.Printf("\nTotal: %d sandboxes\n", data.Count)
	return nil
}

func createSandboxInteractive() error {
	fmt.Println("Creating new sandbox (interactive mode)")
	
	config := arf.SandboxConfig{}
	config.Repository = promptUser("Repository URL (optional): ")
	config.Branch = promptUser("Branch (optional): ")
	config.Language = promptUser("Language (optional): ")
	config.MemoryLimit = promptUser("Memory limit (e.g., 2G, default: 1G): ")
	if config.MemoryLimit == "" {
		config.MemoryLimit = "1G"
	}
	
	ttlStr := promptUser("TTL in minutes (default: 60): ")
	ttlMinutes := 60
	if ttlStr != "" {
		if minutes, err := strconv.Atoi(ttlStr); err == nil {
			ttlMinutes = minutes
		}
	}
	config.TTL = time.Duration(ttlMinutes) * time.Minute

	data, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to serialize config: %w", err)
	}

	url := fmt.Sprintf("%s/v1/arf/sandboxes", getControllerURL())
	response, err := makeAPIRequest("POST", url, data)
	if err != nil {
		return fmt.Errorf("failed to create sandbox: %w", err)
	}

	var sandbox arf.Sandbox
	if err := json.Unmarshal(response, &sandbox); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	fmt.Printf("\nSandbox created successfully!\n")
	fmt.Printf("ID: %s\n", sandbox.ID)
	fmt.Printf("Jail Name: %s\n", sandbox.JailName)
	fmt.Printf("Status: %s\n", sandbox.Status)
	fmt.Printf("Expires: %s\n", sandbox.ExpiresAt.Format(time.RFC3339))

	return nil
}

func destroySandbox(sandboxID string) error {
	url := fmt.Sprintf("%s/v1/arf/sandboxes/%s", getControllerURL(), sandboxID)
	_, err := makeAPIRequest("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to destroy sandbox: %w", err)
	}

	fmt.Printf("Sandbox %s destroyed successfully\n", sandboxID)
	return nil
}

// Health and cache commands

func handleARFHealthCommand() error {
	url := fmt.Sprintf("%s/v1/arf/health", getControllerURL())
	response, err := makeAPIRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}

	var health map[string]interface{}
	if err := json.Unmarshal(response, &health); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	fmt.Printf("ARF System Health: %s\n", health["status"])
	if components, ok := health["components"].(map[string]interface{}); ok {
		if engine, ok := components["engine"].(map[string]interface{}); ok {
			fmt.Printf("Available Recipes: %.0f\n", engine["available_recipes"])
		}
		if cache, ok := components["cache"].(map[string]interface{}); ok {
			fmt.Printf("Cache Hit Rate: %.2f%%\n", cache["hit_rate"].(float64)*100)
			fmt.Printf("Cache Size: %.0f entries\n", cache["size"])
		}
	}

	return nil
}

func handleARFCacheCommand(args []string) error {
	if len(args) == 0 {
		return getCacheStats()
	}

	action := args[0]
	switch action {
	case "stats":
		return getCacheStats()
	case "clear":
		return clearCache()
	default:
		fmt.Printf("Unknown cache action: %s\n", action)
		fmt.Println("Available actions: stats, clear")
		return nil
	}
}

func getCacheStats() error {
	url := fmt.Sprintf("%s/v1/arf/cache/stats", getControllerURL())
	response, err := makeAPIRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to get cache stats: %w", err)
	}

	var stats arf.ASTCacheStats
	if err := json.Unmarshal(response, &stats); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	fmt.Printf("AST Cache Statistics:\n")
	fmt.Printf("Hits: %d\n", stats.Hits)
	fmt.Printf("Misses: %d\n", stats.Misses)
	fmt.Printf("Hit Rate: %.2f%%\n", stats.HitRate*100)
	fmt.Printf("Size: %d entries\n", stats.Size)
	fmt.Printf("Memory Usage: %d bytes\n", stats.MemoryUsage)

	return nil
}

func clearCache() error {
	url := fmt.Sprintf("%s/v1/arf/cache/clear", getControllerURL())
	_, err := makeAPIRequest("POST", url, nil)
	if err != nil {
		return fmt.Errorf("failed to clear cache: %w", err)
	}

	fmt.Println("Cache cleared successfully")
	return nil
}

// Helper functions

func getControllerURL() string {
	if url := os.Getenv("PLOY_CONTROLLER"); url != "" {
		return url
	}
	return "http://localhost:8081"
}

func makeAPIRequest(method, url string, body []byte) ([]byte, error) {
	var req *http.Request
	var err error

	if body != nil {
		req, err = http.NewRequest(method, url, bytes.NewBuffer(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
	} else {
		req, err = http.NewRequest(method, url, nil)
		if err != nil {
			return nil, err
		}
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errorResponse map[string]interface{}
		if json.Unmarshal(responseBody, &errorResponse) == nil {
			if errMsg, ok := errorResponse["error"].(string); ok {
				return nil, fmt.Errorf("API error: %s", errMsg)
			}
		}
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(responseBody))
	}

	return responseBody, nil
}

func promptUser(prompt string) string {
	fmt.Print(prompt)
	var input string
	fmt.Scanln(&input)
	return input
}