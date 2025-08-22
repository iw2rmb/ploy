package arf

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// ARFCmd handles ARF (Automated Recipe Framework) commands
func ARFCmd(args []string, controllerURL string) {
	if len(args) == 0 {
		printARFUsage()
		return
	}

	switch args[0] {
	case "recipe":
		recipeCmd(args[1:], controllerURL)
	case "transform":
		transformCmd(args[1:], controllerURL)
	case "validate":
		validateCmd(args[1:], controllerURL)
	case "patterns":
		patternsCmd(args[1:], controllerURL)
	case "test":
		testCmd(args[1:], controllerURL)
	case "status":
		statusCmd(controllerURL)
	default:
		fmt.Printf("Unknown ARF command: %s\n", args[0])
		printARFUsage()
	}
}

func printARFUsage() {
	fmt.Println("ARF (Automated Recipe Framework) Commands:")
	fmt.Println()
	fmt.Println("  ploy arf recipe <command>    - Recipe management")
	fmt.Println("  ploy arf transform <path>    - Transform repository")
	fmt.Println("  ploy arf validate <recipe>   - Validate recipe")
	fmt.Println("  ploy arf patterns            - Learning patterns")
	fmt.Println("  ploy arf test <type>         - Test components")
	fmt.Println("  ploy arf status              - System status")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  ploy arf recipe generate --repo ./myapp --type cleanup")
	fmt.Println("  ploy arf transform ./myapp --recipe spring-boot-3")
	fmt.Println("  ploy arf validate custom-recipe.yaml")
	fmt.Println("  ploy arf patterns list --category security")
	fmt.Println("  ploy arf test ab --recipe1 current --recipe2 optimized")
}

func recipeCmd(args []string, controllerURL string) {
	if len(args) == 0 {
		fmt.Println("Recipe commands:")
		fmt.Println("  generate - Generate recipe using LLM")
		fmt.Println("  list     - List available recipes")
		fmt.Println("  search   - Search recipes")
		fmt.Println("  show     - Show recipe details")
		return
	}

	switch args[0] {
	case "generate":
		generateRecipe(args[1:], controllerURL)
	case "list":
		listRecipes(controllerURL)
	case "search":
		searchRecipes(args[1:], controllerURL)
	case "show":
		if len(args) < 2 {
			fmt.Println("Usage: ploy arf recipe show <recipe-id>")
			return
		}
		showRecipe(args[1], controllerURL)
	default:
		fmt.Printf("Unknown recipe command: %s\n", args[0])
	}
}

func generateRecipe(args []string, controllerURL string) {
	var repoPath, recipeType, language string
	
	// Parse arguments
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--repo", "-r":
			if i+1 < len(args) {
				repoPath = args[i+1]
				i++
			}
		case "--type", "-t":
			if i+1 < len(args) {
				recipeType = args[i+1]
				i++
			}
		case "--lang", "-l":
			if i+1 < len(args) {
				language = args[i+1]
				i++
			}
		}
	}

	if repoPath == "" {
		fmt.Println("Error: Repository path required (--repo)")
		return
	}

	// Read repository context
	context, err := getRepositoryContext(repoPath)
	if err != nil {
		fmt.Printf("Error reading repository: %v\n", err)
		return
	}

	// Prepare request
	request := map[string]interface{}{
		"repository_context": context,
		"transformation_type": recipeType,
		"language": language,
	}

	body, _ := json.Marshal(request)
	resp, err := http.Post(
		fmt.Sprintf("%s/arf/llm/generate", controllerURL),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		fmt.Printf("Error generating recipe: %v\n", err)
		return
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	
	if recipe, ok := result["recipe"].(map[string]interface{}); ok {
		fmt.Printf("Generated Recipe: %s\n", recipe["name"])
		fmt.Printf("Confidence: %.2f\n", result["confidence"])
		fmt.Printf("ID: %s\n", recipe["id"])
		fmt.Println("\nUse 'ploy arf transform' to apply this recipe")
	}
}

func listRecipes(controllerURL string) {
	resp, err := http.Get(fmt.Sprintf("%s/arf/recipes", controllerURL))
	if err != nil {
		fmt.Printf("Error listing recipes: %v\n", err)
		return
	}
	defer resp.Body.Close()

	var recipes []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&recipes)

	fmt.Println("Available Recipes:")
	fmt.Println("==================")
	for _, recipe := range recipes {
		fmt.Printf("- %s: %s (%s)\n", 
			recipe["id"], 
			recipe["name"],
			recipe["language"])
	}
}

func searchRecipes(args []string, controllerURL string) {
	if len(args) == 0 {
		fmt.Println("Usage: ploy arf recipe search <query>")
		return
	}

	query := strings.Join(args, " ")
	resp, err := http.Get(fmt.Sprintf("%s/arf/recipes/search?q=%s", controllerURL, query))
	if err != nil {
		fmt.Printf("Error searching recipes: %v\n", err)
		return
	}
	defer resp.Body.Close()

	var results []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&results)

	fmt.Printf("Search Results for '%s':\n", query)
	fmt.Println("========================")
	for _, recipe := range results {
		fmt.Printf("- %s: %s\n", recipe["id"], recipe["name"])
	}
}

func showRecipe(recipeID string, controllerURL string) {
	resp, err := http.Get(fmt.Sprintf("%s/arf/recipes/%s", controllerURL, recipeID))
	if err != nil {
		fmt.Printf("Error fetching recipe: %v\n", err)
		return
	}
	defer resp.Body.Close()

	var recipe map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&recipe)

	fmt.Printf("Recipe: %s\n", recipe["name"])
	fmt.Printf("ID: %s\n", recipe["id"])
	fmt.Printf("Language: %s\n", recipe["language"])
	fmt.Printf("Description: %s\n", recipe["description"])
	fmt.Printf("Category: %s\n", recipe["category"])
	fmt.Printf("Version: %s\n", recipe["version"])
}

func transformCmd(args []string, controllerURL string) {
	if len(args) == 0 {
		fmt.Println("Usage: ploy arf transform <path> [options]")
		fmt.Println("Options:")
		fmt.Println("  --recipe <id>    - Recipe to apply")
		fmt.Println("  --dry-run        - Preview changes")
		fmt.Println("  --validate       - Validate after transform")
		return
	}

	path := args[0]
	var recipeID string
	var dryRun, validate bool

	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--recipe", "-r":
			if i+1 < len(args) {
				recipeID = args[i+1]
				i++
			}
		case "--dry-run":
			dryRun = true
		case "--validate":
			validate = true
		}
	}

	fmt.Printf("Transforming %s", path)
	if recipeID != "" {
		fmt.Printf(" with recipe %s", recipeID)
	}
	if dryRun {
		fmt.Printf(" (dry run)")
	}
	fmt.Println()

	// Prepare transformation request
	request := map[string]interface{}{
		"repository_path": path,
		"recipe_id": recipeID,
		"dry_run": dryRun,
		"validate": validate,
	}

	body, _ := json.Marshal(request)
	resp, err := http.Post(
		fmt.Sprintf("%s/arf/transform", controllerURL),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	if success, ok := result["success"].(bool); ok && success {
		fmt.Println("✓ Transformation completed successfully")
		if changes, ok := result["changes"].([]interface{}); ok {
			fmt.Printf("  Modified %d files\n", len(changes))
		}
	} else {
		fmt.Println("✗ Transformation failed")
		if msg, ok := result["error"].(string); ok {
			fmt.Printf("  Error: %s\n", msg)
		}
	}
}

func validateCmd(args []string, controllerURL string) {
	if len(args) == 0 {
		fmt.Println("Usage: ploy arf validate <recipe-file>")
		return
	}

	recipePath := args[0]
	
	// Read recipe file
	content, err := os.ReadFile(recipePath)
	if err != nil {
		fmt.Printf("Error reading recipe: %v\n", err)
		return
	}

	// Send for validation
	resp, err := http.Post(
		fmt.Sprintf("%s/arf/validate", controllerURL),
		"application/yaml",
		bytes.NewReader(content),
	)
	if err != nil {
		fmt.Printf("Error validating: %v\n", err)
		return
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

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
}

func patternsCmd(args []string, controllerURL string) {
	if len(args) == 0 {
		fmt.Println("Pattern commands:")
		fmt.Println("  list     - List learned patterns")
		fmt.Println("  extract  - Extract new patterns")
		fmt.Println("  stats    - Pattern statistics")
		return
	}

	switch args[0] {
	case "list":
		listPatterns(args[1:], controllerURL)
	case "extract":
		extractPatterns(controllerURL)
	case "stats":
		patternStats(controllerURL)
	default:
		fmt.Printf("Unknown patterns command: %s\n", args[0])
	}
}

func listPatterns(args []string, controllerURL string) {
	var category string
	for i := 0; i < len(args); i++ {
		if args[i] == "--category" && i+1 < len(args) {
			category = args[i+1]
			break
		}
	}

	url := fmt.Sprintf("%s/arf/learning/patterns", controllerURL)
	if category != "" {
		url += "?category=" + category
	}

	resp, err := http.Get(url)
	if err != nil {
		fmt.Printf("Error fetching patterns: %v\n", err)
		return
	}
	defer resp.Body.Close()

	var patterns []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&patterns)

	fmt.Println("Learned Patterns:")
	fmt.Println("=================")
	for _, pattern := range patterns {
		fmt.Printf("- %s (confidence: %.2f)\n", 
			pattern["name"],
			pattern["confidence"])
	}
}

func extractPatterns(controllerURL string) {
	fmt.Println("Extracting patterns from historical data...")
	
	resp, err := http.Post(
		fmt.Sprintf("%s/arf/learning/extract", controllerURL),
		"application/json",
		nil,
	)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	if count, ok := result["patterns_extracted"].(float64); ok {
		fmt.Printf("✓ Extracted %d new patterns\n", int(count))
	}
}

func patternStats(controllerURL string) {
	resp, err := http.Get(fmt.Sprintf("%s/arf/learning/stats", controllerURL))
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	var stats map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&stats)

	fmt.Println("Pattern Statistics:")
	fmt.Println("==================")
	fmt.Printf("Total Patterns: %v\n", stats["total_patterns"])
	fmt.Printf("Success Rate: %.2f%%\n", stats["success_rate"])
	fmt.Printf("Last Update: %v\n", stats["last_update"])
}

func testCmd(args []string, controllerURL string) {
	if len(args) == 0 {
		fmt.Println("Test commands:")
		fmt.Println("  ab       - A/B test recipes")
		fmt.Println("  sandbox  - Test in sandbox")
		fmt.Println("  pipeline - Test pipeline")
		return
	}

	switch args[0] {
	case "ab":
		abTest(args[1:], controllerURL)
	case "sandbox":
		sandboxTest(args[1:], controllerURL)
	case "pipeline":
		pipelineTest(args[1:], controllerURL)
	default:
		fmt.Printf("Unknown test command: %s\n", args[0])
	}
}

func abTest(args []string, controllerURL string) {
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
		return
	}

	fmt.Printf("Starting A/B test: %s vs %s\n", recipe1, recipe2)
	
	request := map[string]interface{}{
		"recipe_a": recipe1,
		"recipe_b": recipe2,
		"sample_size": samples,
	}

	body, _ := json.Marshal(request)
	resp, err := http.Post(
		fmt.Sprintf("%s/arf/test/ab", controllerURL),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	fmt.Printf("Test ID: %s\n", result["test_id"])
	fmt.Println("Test started. Check status with 'ploy arf status'")
}

func sandboxTest(args []string, controllerURL string) {
	if len(args) == 0 {
		fmt.Println("Usage: ploy arf test sandbox <recipe-id>")
		return
	}

	recipeID := args[0]
	fmt.Printf("Testing recipe %s in sandbox...\n", recipeID)

	resp, err := http.Post(
		fmt.Sprintf("%s/arf/test/sandbox/%s", controllerURL, recipeID),
		"application/json",
		nil,
	)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	if success, ok := result["success"].(bool); ok && success {
		fmt.Println("✓ Sandbox test passed")
	} else {
		fmt.Println("✗ Sandbox test failed")
		if msg, ok := result["error"].(string); ok {
			fmt.Printf("  Error: %s\n", msg)
		}
	}
}

func pipelineTest(args []string, controllerURL string) {
	fmt.Println("Testing transformation pipeline...")

	resp, err := http.Post(
		fmt.Sprintf("%s/arf/test/pipeline", controllerURL),
		"application/json",
		nil,
	)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Println(string(body))
}

func statusCmd(controllerURL string) {
	resp, err := http.Get(fmt.Sprintf("%s/arf/status", controllerURL))
	if err != nil {
		fmt.Printf("Error fetching status: %v\n", err)
		return
	}
	defer resp.Body.Close()

	var status map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&status)

	fmt.Println("ARF System Status")
	fmt.Println("=================")
	fmt.Printf("LLM Integration: %v\n", status["llm_enabled"])
	fmt.Printf("Learning System: %v\n", status["learning_enabled"])
	fmt.Printf("Multi-Language: %v\n", status["multi_lang_enabled"])
	fmt.Printf("A/B Testing: %v\n", status["ab_testing_enabled"])
	
	if tests, ok := status["active_tests"].([]interface{}); ok && len(tests) > 0 {
		fmt.Printf("\nActive A/B Tests: %d\n", len(tests))
	}
}

func getRepositoryContext(path string) (map[string]interface{}, error) {
	// Get basic repository information
	context := make(map[string]interface{})
	
	// Check for common project files
	files := []string{"pom.xml", "build.gradle", "package.json", "go.mod", "Cargo.toml"}
	for _, file := range files {
		if _, err := os.Stat(filepath.Join(path, file)); err == nil {
			context["project_file"] = file
			
			// Read file content for context
			content, _ := os.ReadFile(filepath.Join(path, file))
			if len(content) > 1000 {
				content = content[:1000] // Limit context size
			}
			context["project_content"] = string(content)
			break
		}
	}

	// Count files
	var fileCount int
	filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			fileCount++
		}
		return nil
	})
	context["file_count"] = fileCount

	return context, nil
}