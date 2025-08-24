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

	"github.com/iw2rmb/ploy/controller/arf"
)

// ARF command handlers

var arfControllerURL string

// ARFCmd handles ARF (Automated Recipe Framework) commands
func ARFCmd(args []string, controllerURL string) {
	// Set the global controller URL
	arfControllerURL = controllerURL
	
	if len(args) == 0 {
		printARFUsage()
		return
	}

	subcommand := args[0]
	switch subcommand {
	case "recipes", "recipe":
		handleARFRecipesCommand(args[1:])
	case "transform":
		handleARFTransformCommand(args[1:])
	case "sandbox":
		handleARFSandboxCommand(args[1:])
	case "health":
		handleARFHealthCommand()
	case "cache":
		handleARFCacheCommand(args[1:])
	case "benchmark":
		handleARFBenchmarkCommand(args[1:])
	case "validate":
		handleARFValidateCommand(args[1:])
	case "patterns":
		handleARFPatternsCommand(args[1:])
	case "test":
		handleARFTestCommand(args[1:])
	case "status":
		handleARFStatusCommand()
	default:
		fmt.Printf("Unknown ARF subcommand: %s\n", subcommand)
		printARFUsage()
	}
}

func printARFUsage() {
	fmt.Println("Usage: ploy arf <subcommand> [options]")
	fmt.Println()
	fmt.Println("Available subcommands:")
	fmt.Println("  recipes    Manage transformation recipes")
	fmt.Println("  transform  Execute code transformations")
	fmt.Println("  validate   Validate recipe files")
	fmt.Println("  patterns   Manage learning patterns")
	fmt.Println("  test       Test components (A/B, sandbox, pipeline)")
	fmt.Println("  benchmark  Run benchmark tests")
	fmt.Println("  sandbox    Manage sandboxes")
	fmt.Println("  health     Check ARF system health")
	fmt.Println("  cache      Manage AST cache")
	fmt.Println("  status     Check ARF system status")
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
	url := fmt.Sprintf("%s/arf/recipes", arfControllerURL)
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
	url := fmt.Sprintf("%s/arf/recipes/%s", arfControllerURL, recipeID)
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
	url := fmt.Sprintf("%s/arf/recipes/search?q=%s", arfControllerURL, query)
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
	url := fmt.Sprintf("%s/arf/recipes/%s/stats", arfControllerURL, recipeID)
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

	url := fmt.Sprintf("%s/arf/recipes", arfControllerURL)
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
	url := fmt.Sprintf("%s/arf/transform", arfControllerURL)
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
	url := fmt.Sprintf("%s/arf/sandboxes", arfControllerURL)
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

	url := fmt.Sprintf("%s/arf/sandboxes", arfControllerURL)
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
	url := fmt.Sprintf("%s/arf/sandboxes/%s", arfControllerURL, sandboxID)
	_, err := makeAPIRequest("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to destroy sandbox: %w", err)
	}

	fmt.Printf("Sandbox %s destroyed successfully\n", sandboxID)
	return nil
}

// Health and cache commands

func handleARFHealthCommand() error {
	url := fmt.Sprintf("%s/arf/health", arfControllerURL)
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
	url := fmt.Sprintf("%s/arf/cache/stats", arfControllerURL)
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
	url := fmt.Sprintf("%s/arf/cache/clear", arfControllerURL)
	_, err := makeAPIRequest("POST", url, nil)
	if err != nil {
		return fmt.Errorf("failed to clear cache: %w", err)
	}

	fmt.Println("Cache cleared successfully")
	return nil
}

// Helper functions

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

// Additional command handlers for complete ARF functionality

func handleARFValidateCommand(args []string) error {
	if len(args) == 0 {
		fmt.Println("Usage: ploy arf validate <recipe-file>")
		return nil
	}

	recipePath := args[0]
	
	// Read recipe file
	content, err := os.ReadFile(recipePath)
	if err != nil {
		return fmt.Errorf("error reading recipe: %w", err)
	}

	// Send for validation
	url := fmt.Sprintf("%s/arf/validate", arfControllerURL)
	response, err := makeAPIRequest("POST", url, content)
	if err != nil {
		return fmt.Errorf("error validating: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(response, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if valid, ok := result["valid"].(bool); ok && valid {
		fmt.Println("✓ Recipe is valid")
	} else {
		fmt.Println("✗ Recipe validation failed")
		if errors, ok := result["errors"].([]interface{}); ok {
			for _, err := range errors {
				fmt.Printf("  - %v\n", err)
			}
		}
	}

	return nil
}

func handleARFPatternsCommand(args []string) error {
	if len(args) == 0 {
		fmt.Println("Pattern commands:")
		fmt.Println("  list     - List learned patterns")
		fmt.Println("  extract  - Extract new patterns")
		fmt.Println("  stats    - Pattern statistics")
		return nil
	}

	switch args[0] {
	case "list":
		return listPatterns(args[1:])
	case "extract":
		return extractPatterns()
	case "stats":
		return patternStats()
	default:
		fmt.Printf("Unknown patterns command: %s\n", args[0])
	}
	return nil
}

func listPatterns(args []string) error {
	var category string
	for i := 0; i < len(args); i++ {
		if args[i] == "--category" && i+1 < len(args) {
			category = args[i+1]
			break
		}
	}

	url := fmt.Sprintf("%s/arf/learning/patterns", arfControllerURL)
	if category != "" {
		url += "?category=" + category
	}

	response, err := makeAPIRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("error fetching patterns: %w", err)
	}

	var patterns []map[string]interface{}
	if err := json.Unmarshal(response, &patterns); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	fmt.Println("Learned Patterns:")
	fmt.Println("=================")
	for _, pattern := range patterns {
		fmt.Printf("- %s (confidence: %.2f)\n", 
			pattern["name"],
			pattern["confidence"])
	}
	
	return nil
}

func extractPatterns() error {
	fmt.Println("Extracting patterns from historical data...")
	
	url := fmt.Sprintf("%s/arf/learning/extract", arfControllerURL)
	response, err := makeAPIRequest("POST", url, nil)
	if err != nil {
		return fmt.Errorf("error: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(response, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if count, ok := result["patterns_extracted"].(float64); ok {
		fmt.Printf("✓ Extracted %d new patterns\n", int(count))
	}
	
	return nil
}

func patternStats() error {
	url := fmt.Sprintf("%s/arf/learning/stats", arfControllerURL)
	response, err := makeAPIRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("error: %w", err)
	}

	var stats map[string]interface{}
	if err := json.Unmarshal(response, &stats); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	fmt.Println("Pattern Statistics:")
	fmt.Println("==================")
	fmt.Printf("Total Patterns: %v\n", stats["total_patterns"])
	fmt.Printf("Success Rate: %.2f%%\n", stats["success_rate"])
	fmt.Printf("Last Update: %v\n", stats["last_update"])
	
	return nil
}

func handleARFTestCommand(args []string) error {
	if len(args) == 0 {
		fmt.Println("Test commands:")
		fmt.Println("  ab       - A/B test recipes")
		fmt.Println("  sandbox  - Test in sandbox")
		fmt.Println("  pipeline - Test pipeline")
		return nil
	}

	switch args[0] {
	case "ab":
		return abTest(args[1:])
	case "sandbox":
		return sandboxTest(args[1:])
	case "pipeline":
		return pipelineTest()
	default:
		fmt.Printf("Unknown test command: %s\n", args[0])
	}
	return nil
}

func abTest(args []string) error {
	var recipe1, recipe2 string
	var samples int = 100

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--recipe1":
			if i+1 < len(args) {
				recipe1 = args[i+1]
				i++
			}
		case "--recipe2":
			if i+1 < len(args) {
				recipe2 = args[i+1]
				i++
			}
		case "--samples":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &samples)
				i++
			}
		}
	}

	if recipe1 == "" || recipe2 == "" {
		fmt.Println("Error: Both --recipe1 and --recipe2 required")
		return nil
	}

	fmt.Printf("Starting A/B test: %s vs %s\n", recipe1, recipe2)
	
	request := map[string]interface{}{
		"recipe_a": recipe1,
		"recipe_b": recipe2,
		"sample_size": samples,
	}

	body, _ := json.Marshal(request)
	url := fmt.Sprintf("%s/arf/test/ab", arfControllerURL)
	response, err := makeAPIRequest("POST", url, body)
	if err != nil {
		return fmt.Errorf("error: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(response, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	fmt.Printf("Test ID: %s\n", result["test_id"])
	fmt.Println("Test started. Check status with 'ploy arf status'")
	
	return nil
}

func sandboxTest(args []string) error {
	if len(args) == 0 {
		fmt.Println("Usage: ploy arf test sandbox <recipe-id>")
		return nil
	}

	recipeID := args[0]
	fmt.Printf("Testing recipe %s in sandbox...\n", recipeID)

	url := fmt.Sprintf("%s/arf/test/sandbox/%s", arfControllerURL, recipeID)
	response, err := makeAPIRequest("POST", url, nil)
	if err != nil {
		return fmt.Errorf("error: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(response, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if success, ok := result["success"].(bool); ok && success {
		fmt.Println("✓ Sandbox test passed")
	} else {
		fmt.Println("✗ Sandbox test failed")
		if msg, ok := result["error"].(string); ok {
			fmt.Printf("  Error: %s\n", msg)
		}
	}
	
	return nil
}

func pipelineTest() error {
	fmt.Println("Testing transformation pipeline...")

	url := fmt.Sprintf("%s/arf/test/pipeline", arfControllerURL)
	response, err := makeAPIRequest("POST", url, nil)
	if err != nil {
		return fmt.Errorf("error: %w", err)
	}

	fmt.Println(string(response))
	return nil
}

func handleARFStatusCommand() error {
	url := fmt.Sprintf("%s/arf/status", arfControllerURL)
	response, err := makeAPIRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("error fetching status: %w", err)
	}

	var status map[string]interface{}
	if err := json.Unmarshal(response, &status); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	fmt.Println("ARF System Status")
	fmt.Println("=================")
	fmt.Printf("LLM Integration: %v\n", status["llm_enabled"])
	fmt.Printf("Learning System: %v\n", status["learning_enabled"])
	fmt.Printf("Multi-Language: %v\n", status["multi_lang_enabled"])
	fmt.Printf("A/B Testing: %v\n", status["ab_testing_enabled"])
	
	if tests, ok := status["active_tests"].([]interface{}); ok && len(tests) > 0 {
		fmt.Printf("\nActive A/B Tests: %d\n", len(tests))
	}
	
	return nil
}

// Benchmark command handlers

func handleARFBenchmarkCommand(args []string) error {
	if len(args) == 0 {
		printBenchmarkUsage()
		return nil
	}

	switch args[0] {
	case "run":
		return handleBenchmarkRun(args[1:])
	case "list":
		return handleBenchmarkList(args[1:])
	case "status":
		if len(args) < 2 {
			fmt.Println("Usage: ploy arf benchmark status <benchmark-id>")
			return nil
		}
		return handleBenchmarkStatus(args[1])
	case "results":
		if len(args) < 2 {
			fmt.Println("Usage: ploy arf benchmark results <benchmark-id>")
			return nil
		}
		return handleBenchmarkResults(args[1])
	case "errors":
		if len(args) < 2 {
			fmt.Println("Usage: ploy arf benchmark errors <benchmark-id>")
			return nil
		}
		return handleBenchmarkErrors(args[1])
	case "logs":
		if len(args) < 2 {
			fmt.Println("Usage: ploy arf benchmark logs <benchmark-id> [--stage <stage>]")
			return nil
		}
		return handleBenchmarkLogs(args[1:])
	case "compare":
		return handleBenchmarkCompare(args[1:])
	case "report":
		if len(args) < 2 {
			fmt.Println("Usage: ploy arf benchmark report <benchmark-id> [--format <html|pdf|markdown>]")
			return nil
		}
		return handleBenchmarkReport(args[1:])
	case "cancel":
		if len(args) < 2 {
			fmt.Println("Usage: ploy arf benchmark cancel <benchmark-id>")
			return nil
		}
		return handleBenchmarkCancel(args[1])
	default:
		fmt.Printf("Unknown benchmark command: %s\n", args[0])
		printBenchmarkUsage()
	}
	return nil
}

func printBenchmarkUsage() {
	fmt.Println("Benchmark Commands:")
	fmt.Println()
	fmt.Println("  ploy arf benchmark run <name> --repository <url> [options]")
	fmt.Println("  ploy arf benchmark list [--active|--completed]")
	fmt.Println("  ploy arf benchmark status <benchmark-id>")
	fmt.Println("  ploy arf benchmark results <benchmark-id>")
	fmt.Println("  ploy arf benchmark errors <benchmark-id>")
	fmt.Println("  ploy arf benchmark logs <benchmark-id> [--stage <stage>]")
	fmt.Println("  ploy arf benchmark compare <id1> <id2> [<id3>...]")
	fmt.Println("  ploy arf benchmark report <benchmark-id> [--format <html|pdf|markdown>]")
	fmt.Println("  ploy arf benchmark cancel <benchmark-id>")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  ploy arf benchmark run test-java --repository https://github.com/spring-projects/spring-petclinic.git")
	fmt.Println("  ploy arf benchmark list --active")
	fmt.Println("  ploy arf benchmark status bench-1234567890")
}

func handleBenchmarkRun(args []string) error {
	if len(args) == 0 {
		fmt.Println("Usage: ploy arf benchmark run <name> --repository <url> [options]")
		return nil
	}
	
	benchmarkName := args[0]
	repository := ""
	transformations := ""
	appName := ""
	lane := "auto"
	
	// Parse arguments
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--repository":
			if i+1 < len(args) {
				repository = args[i+1]
				i++
			}
		case "--transformations":
			if i+1 < len(args) {
				transformations = args[i+1]
				i++
			}
		case "--app-name":
			if i+1 < len(args) {
				appName = args[i+1]
				i++
			}
		case "--lane":
			if i+1 < len(args) {
				lane = args[i+1]
				i++
			}
		}
	}
	
	if repository == "" {
		fmt.Println("Error: --repository is required")
		return nil
	}
	
	if appName == "" {
		appName = fmt.Sprintf("bench-%s-%d", benchmarkName, time.Now().Unix())
	}

	// Create benchmark config matching BenchmarkConfig struct fields
	benchmarkConfig := map[string]interface{}{
		"name":     benchmarkName,
		"repo_url": repository,  // Fixed: was "repository", should be "repo_url"
		"deployment_config": map[string]interface{}{
			"app_name": appName,
			"lane":     lane,
		},
	}

	// Add LLM provider configuration (required for benchmark suite)
	llmProvider := os.Getenv("ARF_LLM_PROVIDER")
	llmModel := os.Getenv("ARF_LLM_MODEL")
	
	// Set defaults if not configured
	if llmProvider == "" {
		llmProvider = "ollama"
	}
	if llmModel == "" {
		llmModel = "codellama:7b"
	}
	
	benchmarkConfig["llm_provider"] = llmProvider
	benchmarkConfig["llm_model"] = llmModel

	if transformations != "" {
		recipeList := strings.Split(transformations, ",")
		var transformationList []map[string]interface{}
		for _, recipe := range recipeList {
			transformationList = append(transformationList, map[string]interface{}{
				"type":   "openrewrite",
				"recipe": strings.TrimSpace(recipe),
			})
		}
		benchmarkConfig["transformations"] = transformationList
	}

	// Wrap config in the format controller expects
	request := map[string]interface{}{
		"config": benchmarkConfig,
	}

	// Submit benchmark
	data, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to serialize benchmark request: %w", err)
	}

	fmt.Printf("Starting benchmark '%s'...\n", benchmarkName)
	fmt.Printf("Repository: %s\n", repository)
	fmt.Printf("App Name: %s\n", appName)
	fmt.Printf("Lane: %s\n", lane)
	if transformations != "" {
		fmt.Printf("Transformations: %s\n", transformations)
	}
	fmt.Println()

	url := fmt.Sprintf("%s/arf/benchmark/run", arfControllerURL)
	response, err := makeAPIRequest("POST", url, data)
	if err != nil {
		return fmt.Errorf("failed to start benchmark: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(response, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if benchmarkID, ok := result["benchmark_id"].(string); ok {
		fmt.Printf("✅ Benchmark started successfully!\n")
		fmt.Printf("Benchmark ID: %s\n", benchmarkID)
		fmt.Println()
		fmt.Printf("Use 'ploy arf benchmark status %s' to check progress\n", benchmarkID)
		fmt.Printf("Use 'ploy arf benchmark logs %s' to view logs\n", benchmarkID)
	} else {
		fmt.Println("✅ Benchmark started, but no ID returned")
	}

	return nil
}

func handleBenchmarkList(args []string) error {
	activeOnly := false
	completedOnly := false

	// Parse filters
	for _, arg := range args {
		switch arg {
		case "--active":
			activeOnly = true
		case "--completed":
			completedOnly = true
		}
	}

	url := fmt.Sprintf("%s/arf/benchmark/list", arfControllerURL)
	if activeOnly {
		url += "?status=running"
	} else if completedOnly {
		url += "?status=completed"
	}

	response, err := makeAPIRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to list benchmarks: %w", err)
	}

	var data struct {
		Benchmarks []struct {
			ID          string    `json:"id"`
			Name        string    `json:"name"`
			Status      string    `json:"status"`
			Repository  string    `json:"repository"`
			StartedAt   time.Time `json:"started_at"`
			CompletedAt *time.Time `json:"completed_at"`
			AppName     string    `json:"app_name"`
		} `json:"benchmarks"`
		Count int `json:"count"`
	}

	if err := json.Unmarshal(response, &data); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if data.Count == 0 {
		fmt.Println("No benchmarks found")
		return nil
	}

	// Display benchmarks in table format
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tSTATUS\tREPOSITORY\tAPP NAME\tSTARTED\tDURATION")
	fmt.Fprintln(w, "--\t----\t------\t----------\t--------\t-------\t--------")

	for _, benchmark := range data.Benchmarks {
		started := benchmark.StartedAt.Format("15:04:05")
		duration := ""
		
		if benchmark.CompletedAt != nil {
			duration = benchmark.CompletedAt.Sub(benchmark.StartedAt).Round(time.Second).String()
		} else if benchmark.Status == "running" {
			duration = time.Since(benchmark.StartedAt).Round(time.Second).String()
		}
		
		// Truncate repository URL for display
		repo := benchmark.Repository
		if len(repo) > 40 {
			repo = repo[:37] + "..."
		}
		
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			benchmark.ID, benchmark.Name, benchmark.Status, repo, benchmark.AppName, started, duration)
	}

	w.Flush()
	fmt.Printf("\nTotal: %d benchmarks\n", data.Count)
	return nil
}

func handleBenchmarkStatus(benchmarkID string) error {
	url := fmt.Sprintf("%s/arf/benchmark/status/%s", arfControllerURL, benchmarkID)
	response, err := makeAPIRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to get benchmark status: %w", err)
	}

	var status struct {
		ID               string     `json:"id"`
		Name             string     `json:"name"`
		Status           string     `json:"status"`
		CurrentStage     string     `json:"current_stage"`
		Progress         float64    `json:"progress"`
		StartedAt        time.Time  `json:"started_at"`
		CompletedAt      *time.Time `json:"completed_at"`
		Repository       string     `json:"repository"`
		AppName          string     `json:"app_name"`
		Lane             string     `json:"lane"`
		TransformationsCount int    `json:"transformations_count"`
		SuccessfulStages []string   `json:"successful_stages"`
		FailedStages     []string   `json:"failed_stages"`
	}

	if err := json.Unmarshal(response, &status); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	fmt.Printf("Benchmark Status: %s\n", status.ID)
	fmt.Printf("Name: %s\n", status.Name)
	fmt.Printf("Status: %s\n", status.Status)
	
	if status.CurrentStage != "" {
		fmt.Printf("Current Stage: %s\n", status.CurrentStage)
	}
	
	fmt.Printf("Progress: %.1f%%\n", status.Progress*100)
	fmt.Printf("Repository: %s\n", status.Repository)
	fmt.Printf("App Name: %s\n", status.AppName)
	fmt.Printf("Lane: %s\n", status.Lane)
	fmt.Printf("Started: %s\n", status.StartedAt.Format("2006-01-02 15:04:05"))
	
	if status.CompletedAt != nil {
		fmt.Printf("Completed: %s\n", status.CompletedAt.Format("2006-01-02 15:04:05"))
		fmt.Printf("Duration: %s\n", status.CompletedAt.Sub(status.StartedAt).Round(time.Second))
	} else if status.Status == "running" {
		fmt.Printf("Running for: %s\n", time.Since(status.StartedAt).Round(time.Second))
	}
	
	if len(status.SuccessfulStages) > 0 {
		fmt.Printf("Completed Stages: %s\n", strings.Join(status.SuccessfulStages, ", "))
	}
	
	if len(status.FailedStages) > 0 {
		fmt.Printf("Failed Stages: %s\n", strings.Join(status.FailedStages, ", "))
	}

	return nil
}

func handleBenchmarkResults(benchmarkID string) error {
	url := fmt.Sprintf("%s/arf/benchmark/results/%s", arfControllerURL, benchmarkID)
	response, err := makeAPIRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to get benchmark results: %w", err)
	}

	var results map[string]interface{}
	if err := json.Unmarshal(response, &results); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	// Pretty print results as JSON
	output, _ := json.MarshalIndent(results, "", "  ")
	fmt.Println(string(output))

	return nil
}

func handleBenchmarkErrors(benchmarkID string) error {
	url := fmt.Sprintf("%s/arf/benchmark/errors/%s", arfControllerURL, benchmarkID)
	response, err := makeAPIRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to get benchmark errors: %w", err)
	}

	var data struct {
		BenchmarkID string `json:"benchmark_id"`
		Status      string `json:"status"`
		Errors      []struct {
			Stage      string    `json:"stage"`
			Type       string    `json:"type"`
			Message    string    `json:"message"`
			Details    string    `json:"details"`
			Timestamp  time.Time `json:"timestamp"`
		} `json:"errors"`
		ErrorCount  int `json:"error_count"`
	}

	if err := json.Unmarshal(response, &data); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	fmt.Printf("Benchmark Errors: %s\n", data.BenchmarkID)
	fmt.Printf("Status: %s\n", data.Status)
	fmt.Printf("Total Errors: %d\n", data.ErrorCount)
	fmt.Println()

	if data.ErrorCount == 0 {
		fmt.Println("No errors found")
		return nil
	}

	for i, err := range data.Errors {
		fmt.Printf("Error %d:\n", i+1)
		fmt.Printf("  Stage: %s\n", err.Stage)
		fmt.Printf("  Type: %s\n", err.Type)
		fmt.Printf("  Message: %s\n", err.Message)
		if err.Details != "" {
			fmt.Printf("  Details: %s\n", err.Details)
		}
		fmt.Printf("  Time: %s\n", err.Timestamp.Format("15:04:05"))
		fmt.Println()
	}

	return nil
}

func handleBenchmarkLogs(args []string) error {
	benchmarkID := args[0]
	stage := "all"

	// Parse stage filter
	for i := 1; i < len(args); i++ {
		if args[i] == "--stage" && i+1 < len(args) {
			stage = args[i+1]
			break
		}
	}

	url := fmt.Sprintf("%s/arf/benchmark/logs/%s", arfControllerURL, benchmarkID)
	if stage != "all" {
		url += "?stage=" + stage
	}

	response, err := makeAPIRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to get benchmark logs: %w", err)
	}

	var logs struct {
		BenchmarkID string `json:"benchmark_id"`
		Stage       string `json:"stage"`
		Logs        []struct {
			Timestamp time.Time `json:"timestamp"`
			Level     string    `json:"level"`
			Stage     string    `json:"stage"`
			Message   string    `json:"message"`
		} `json:"logs"`
	}

	if err := json.Unmarshal(response, &logs); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	fmt.Printf("Benchmark Logs: %s\n", logs.BenchmarkID)
	if logs.Stage != "" {
		fmt.Printf("Stage: %s\n", logs.Stage)
	}
	fmt.Println(strings.Repeat("=", 60))

	for _, log := range logs.Logs {
		fmt.Printf("[%s] [%s] [%s] %s\n",
			log.Timestamp.Format("15:04:05"),
			log.Level,
			log.Stage,
			log.Message)
	}

	return nil
}

func handleBenchmarkCompare(args []string) error {
	if len(args) < 2 {
		fmt.Println("Usage: ploy arf benchmark compare <id1> <id2> [<id3>...]")
		return nil
	}

	request := map[string]interface{}{
		"results": args,
	}

	data, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to serialize request: %w", err)
	}

	url := fmt.Sprintf("%s/arf/benchmark/compare", arfControllerURL)
	response, err := makeAPIRequest("POST", url, data)
	if err != nil {
		return fmt.Errorf("failed to compare benchmarks: %w", err)
	}

	var comparison map[string]interface{}
	if err := json.Unmarshal(response, &comparison); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	// Pretty print comparison as JSON
	output, _ := json.MarshalIndent(comparison, "", "  ")
	fmt.Println(string(output))

	return nil
}

func handleBenchmarkReport(args []string) error {
	benchmarkID := args[0]
	format := "html"
	includeDiffs := true

	// Parse options
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--format":
			if i+1 < len(args) {
				format = args[i+1]
				i++
			}
		case "--no-diffs":
			includeDiffs = false
		}
	}

	request := map[string]interface{}{
		"format":        format,
		"include_diffs": includeDiffs,
	}

	data, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to serialize request: %w", err)
	}

	url := fmt.Sprintf("%s/arf/benchmark/report/%s", arfControllerURL, benchmarkID)
	response, err := makeAPIRequest("POST", url, data)
	if err != nil {
		return fmt.Errorf("failed to generate report: %w", err)
	}

	var result struct {
		BenchmarkID string    `json:"benchmark_id"`
		ReportURL   string    `json:"report_url"`
		Format      string    `json:"format"`
		GeneratedAt time.Time `json:"generated_at"`
	}

	if err := json.Unmarshal(response, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	fmt.Printf("Report generated successfully!\n")
	fmt.Printf("Benchmark ID: %s\n", result.BenchmarkID)
	fmt.Printf("Format: %s\n", result.Format)
	fmt.Printf("URL: %s\n", result.ReportURL)
	fmt.Printf("Generated: %s\n", result.GeneratedAt.Format("2006-01-02 15:04:05"))

	return nil
}

func handleBenchmarkCancel(benchmarkID string) error {
	url := fmt.Sprintf("%s/arf/benchmark/%s", arfControllerURL, benchmarkID)
	response, err := makeAPIRequest("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to cancel benchmark: %w", err)
	}

	var result struct {
		BenchmarkID string `json:"benchmark_id"`
		Status      string `json:"status"`
		Message     string `json:"message"`
	}

	if err := json.Unmarshal(response, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	fmt.Printf("✅ %s\n", result.Message)
	fmt.Printf("Benchmark ID: %s\n", result.BenchmarkID)
	fmt.Printf("Status: %s\n", result.Status)

	return nil
}