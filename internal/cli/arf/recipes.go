package arf

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/iw2rmb/ploy/controller/arf"
)

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