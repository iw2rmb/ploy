package recipes

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	models "github.com/iw2rmb/ploy/internal/arf/models"
)

// RecipeComposition represents a composition of multiple recipes
type RecipeComposition struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	RecipeIDs   []string          `json:"recipe_ids"`
	Recipes     []*models.Recipe  `json:"recipes,omitempty"`
	Config      CompositionConfig `json:"config"`
	CreatedAt   time.Time         `json:"created_at"`
	CreatedBy   string            `json:"created_by"`
}

// CompositionConfig contains configuration for recipe composition
type CompositionConfig struct {
	StopOnError       bool              `json:"stop_on_error"`
	ContinueOnWarning bool              `json:"continue_on_warning"`
	Parallel          bool              `json:"parallel"`
	Timeout           string            `json:"timeout"`
	Environment       map[string]string `json:"environment,omitempty"`
	WorkingDir        string            `json:"working_dir,omitempty"`
}

// CompositionExecution represents an execution of a recipe composition
type CompositionExecution struct {
	ID            string                  `json:"id"`
	CompositionID string                  `json:"composition_id"`
	Repository    string                  `json:"repository"`
	Branch        string                  `json:"branch"`
	Status        string                  `json:"status"`
	StartTime     time.Time               `json:"start_time"`
	EndTime       time.Time               `json:"end_time,omitempty"`
	Duration      string                  `json:"duration,omitempty"`
	Results       []RecipeExecutionResult `json:"results"`
	Summary       CompositionSummary      `json:"summary"`
}

// RecipeExecutionResult represents the result of executing a single recipe
type RecipeExecutionResult struct {
	RecipeID     string    `json:"recipe_id"`
	RecipeName   string    `json:"recipe_name"`
	Status       string    `json:"status"` // success, failed, skipped
	StartTime    time.Time `json:"start_time"`
	EndTime      time.Time `json:"end_time"`
	Duration     string    `json:"duration"`
	Output       string    `json:"output,omitempty"`
	Error        string    `json:"error,omitempty"`
	FilesChanged int       `json:"files_changed"`
	LinesChanged int       `json:"lines_changed"`
}

// CompositionSummary provides a summary of composition execution
type CompositionSummary struct {
	TotalRecipes   int     `json:"total_recipes"`
	SuccessfulRuns int     `json:"successful_runs"`
	FailedRuns     int     `json:"failed_runs"`
	SkippedRuns    int     `json:"skipped_runs"`
	TotalFiles     int     `json:"total_files_changed"`
	TotalLines     int     `json:"total_lines_changed"`
	SuccessRate    float64 `json:"success_rate"`
}

// composeRecipes creates and executes a recipe composition
func composeRecipes(args []string) error {
	if len(args) < 2 {
		return NewCLIError("At least 2 recipe IDs are required for composition", 1).
			WithSuggestion("Usage: ploy recipe compose <recipe-id1> <recipe-id2> [recipe-id3...] [options]").
			WithUsage()
	}

	// Parse composition configuration
	config := CompositionConfig{
		StopOnError:       true, // Default behavior
		ContinueOnWarning: true,
		Parallel:          false,
		Timeout:           "30m",
		Environment:       make(map[string]string),
	}

	var recipeIDs []string
	var compositionName string
	var repository string
	var branch = "main"
	var outputDir string
	var generateReport bool

	// Parse arguments
	i := 0
	for i < len(args) {
		arg := args[i]

		// Check for flags
		if strings.HasPrefix(arg, "--") {
			switch arg {
			case "--name":
				if i+1 < len(args) {
					compositionName = args[i+1]
					i++
				}
			case "--repo", "--repository":
				if i+1 < len(args) {
					repository = args[i+1]
					i++
				}
			case "--branch":
				if i+1 < len(args) {
					branch = args[i+1]
					i++
				}
			case "--output-dir":
				if i+1 < len(args) {
					outputDir = args[i+1]
					i++
				}
			case "--timeout":
				if i+1 < len(args) {
					config.Timeout = args[i+1]
					i++
				}
			case "--continue-on-error":
				config.StopOnError = false
			case "--stop-on-error":
				config.StopOnError = true
			case "--parallel":
				config.Parallel = true
			case "--sequential":
				config.Parallel = false
			case "--report":
				generateReport = true
			case "--working-dir":
				if i+1 < len(args) {
					config.WorkingDir = args[i+1]
					i++
				}
			}
		} else {
			// Non-flag arguments are recipe IDs
			if err := ValidateRecipeID(arg); err != nil {
				PrintError(NewCLIError(fmt.Sprintf("Invalid recipe ID '%s'", arg), 1).WithCause(err))
				return err
			}
			recipeIDs = append(recipeIDs, arg)
		}
		i++
	}

	if len(recipeIDs) < 2 {
		return NewCLIError("At least 2 valid recipe IDs are required", 1).
			WithSuggestion("Provide more recipe IDs or check the IDs are correctly formatted")
	}

	// Generate composition name if not provided
	if compositionName == "" {
		compositionName = fmt.Sprintf("composition-%s", strings.Join(recipeIDs, "-"))
	}

	// Create composition payload
	composition := RecipeComposition{
		Name:        compositionName,
		Description: fmt.Sprintf("Composition of recipes: %s", strings.Join(recipeIDs, ", ")),
		RecipeIDs:   recipeIDs,
		Config:      config,
		CreatedAt:   time.Now(),
		CreatedBy:   "cli-user",
	}

	// Execute composition
	return executeComposition(composition, repository, branch, outputDir, generateReport)
}

// executeComposition executes a recipe composition
func executeComposition(composition RecipeComposition, repository, branch, outputDir string, generateReport bool) error {
	PrintInfo(fmt.Sprintf("Creating composition '%s' with %d recipes", composition.Name, len(composition.RecipeIDs)))

	// Create composition on server
	compositionJSON, err := json.Marshal(composition)
	if err != nil {
		return NewCLIError("Failed to serialize composition", 1).WithCause(err)
	}

	url := fmt.Sprintf("%s/arf/recipes/compose", controllerURL)
	response, err := makeAPIRequest("POST", url, compositionJSON)
	if err != nil {
		return NewCLIError("Failed to create recipe composition", 1).
			WithCause(err).
			WithSuggestion("Check network connectivity and controller status")
	}

	var result struct {
		CompositionID string `json:"composition_id"`
		Message       string `json:"message"`
		Status        string `json:"status"`
	}

	if err := json.Unmarshal(response, &result); err != nil {
		return NewCLIError("Failed to parse composition response", 1).WithCause(err)
	}

	PrintSuccess(fmt.Sprintf("Composition created: %s", result.CompositionID))

	// If repository is specified, execute the composition
	if repository != "" {
		PrintInfo(fmt.Sprintf("Executing composition against repository: %s", repository))
		return executeCompositionAgainstRepo(result.CompositionID, repository, branch, outputDir, generateReport)
	}

	fmt.Printf("\nComposition ID: %s\n", result.CompositionID)
	fmt.Printf("To execute this composition against a repository, run:\n")
	fmt.Printf("  ploy recipe run-composition %s --repo <repository-url>\n", result.CompositionID)

	return nil
}

// executeCompositionAgainstRepo executes a composition against a repository
func executeCompositionAgainstRepo(compositionID, repository, branch, outputDir string, generateReport bool) error {
	executionPayload := map[string]interface{}{
		"composition_id":  compositionID,
		"repository":      repository,
		"branch":          branch,
		"generate_report": generateReport,
	}

	if outputDir != "" {
		executionPayload["output_dir"] = outputDir
	}

	payloadJSON, err := json.Marshal(executionPayload)
	if err != nil {
		return NewCLIError("Failed to serialize execution request", 1).WithCause(err)
	}

	url := fmt.Sprintf("%s/arf/compositions/%s/execute", controllerURL, compositionID)
	response, err := makeAPIRequest("POST", url, payloadJSON)
	if err != nil {
		return NewCLIError("Failed to execute composition", 1).
			WithCause(err).
			WithSuggestion("Check if the composition exists and repository is accessible")
	}

	var execution CompositionExecution
	if err := json.Unmarshal(response, &execution); err != nil {
		return NewCLIError("Failed to parse execution response", 1).WithCause(err)
	}

	// Display execution results
	return displayCompositionResults(execution, generateReport)
}

// displayCompositionResults displays the results of a composition execution
func displayCompositionResults(execution CompositionExecution, verbose bool) error {
	fmt.Printf("\nComposition Execution Results\n")
	fmt.Printf("=============================\n")
	fmt.Printf("Execution ID: %s\n", execution.ID)
	fmt.Printf("Repository:   %s\n", execution.Repository)
	fmt.Printf("Branch:       %s\n", execution.Branch)
	fmt.Printf("Status:       %s\n", execution.Status)
	fmt.Printf("Duration:     %s\n", execution.Duration)

	// Summary
	summary := execution.Summary
	fmt.Printf("\nSummary:\n")
	fmt.Printf("  Total Recipes:    %d\n", summary.TotalRecipes)
	fmt.Printf("  Successful:       %d\n", summary.SuccessfulRuns)
	fmt.Printf("  Failed:           %d\n", summary.FailedRuns)
	fmt.Printf("  Skipped:          %d\n", summary.SkippedRuns)
	fmt.Printf("  Success Rate:     %.1f%%\n", summary.SuccessRate*100)
	fmt.Printf("  Files Changed:    %d\n", summary.TotalFiles)
	fmt.Printf("  Lines Changed:    %d\n", summary.TotalLines)

	// Individual recipe results
	if verbose || len(execution.Results) > 0 {
		fmt.Printf("\nRecipe Results:\n")
		for i, result := range execution.Results {
			status := "✅"
			switch result.Status {
			case "failed":
				status = "❌"
			case "skipped":
				status = "⏭️"
			}

			fmt.Printf("  %d. %s %s (%s) - %s\n", i+1, status, result.RecipeName, result.RecipeID, result.Duration)

			if result.FilesChanged > 0 || result.LinesChanged > 0 {
				fmt.Printf("     Changed: %d files, %d lines\n", result.FilesChanged, result.LinesChanged)
			}

			if verbose && result.Error != "" {
				fmt.Printf("     Error: %s\n", result.Error)
			}
		}
	}

	// Show success or failure message
	switch execution.Status {
	case "completed":
		if summary.FailedRuns == 0 {
			PrintSuccess("All recipes executed successfully!")
		} else {
			PrintWarning(fmt.Sprintf("Composition completed with %d failed recipes", summary.FailedRuns))
		}
	case "failed":
		PrintError(NewCLIError("Composition execution failed", 1))
	}

	return nil
}

// listCompositions lists available recipe compositions
func listCompositions(outputFormat string, verbose bool) error {
	url := fmt.Sprintf("%s/arf/compositions", controllerURL)
	response, err := makeAPIRequest("GET", url, nil)
	if err != nil {
		return NewCLIError("Failed to retrieve compositions", 1).
			WithCause(err).
			WithSuggestion("Check network connectivity and controller status")
	}

	var data struct {
		Compositions []RecipeComposition `json:"compositions"`
		Count        int                 `json:"count"`
	}

	if err := json.Unmarshal(response, &data); err != nil {
		return NewCLIError("Failed to parse compositions data", 1).WithCause(err)
	}

	if data.Count == 0 {
		PrintInfo("No recipe compositions found")
		return nil
	}

	// Format output
	switch strings.ToLower(outputFormat) {
	case "json":
		output, _ := json.MarshalIndent(data.Compositions, "", "  ")
		fmt.Println(string(output))
	case "yaml":
		// Convert to YAML (simplified)
		for _, comp := range data.Compositions {
			fmt.Printf("- id: %s\n", comp.ID)
			fmt.Printf("  name: %s\n", comp.Name)
			fmt.Printf("  description: %s\n", comp.Description)
			fmt.Printf("  recipes: [%s]\n", strings.Join(comp.RecipeIDs, ", "))
			fmt.Printf("  created_at: %s\n", comp.CreatedAt.Format(time.RFC3339))
			fmt.Printf("  created_by: %s\n\n", comp.CreatedBy)
		}
	default: // table
		fmt.Printf("Recipe Compositions:\n\n")
		for _, comp := range data.Compositions {
			fmt.Printf("• %s (%s)\n", comp.Name, comp.ID)
			fmt.Printf("  %s\n", comp.Description)
			fmt.Printf("  Recipes: %s\n", strings.Join(comp.RecipeIDs, ", "))
			if verbose {
				fmt.Printf("  Created: %s by %s\n", comp.CreatedAt.Format("2006-01-02 15:04"), comp.CreatedBy)
				fmt.Printf("  Config: stop_on_error=%t, parallel=%t, timeout=%s\n",
					comp.Config.StopOnError, comp.Config.Parallel, comp.Config.Timeout)
			}
			fmt.Println()
		}
		fmt.Printf("Total: %d compositions\n", data.Count)
	}

	return nil
}

// runComposition executes an existing composition by ID
func runComposition(compositionID string, flags CommandFlags) error {
	// Validate composition ID
	if err := ValidateRecipeID(compositionID); err != nil {
		PrintError(err)
		return err
	}

	// Parse execution parameters from flags
	// TODO: Extract repository, branch, etc. from flags
	repository := ""
	branch := "main"
	outputDir := ""
	generateReport := flags.Verbose

	if repository == "" {
		return NewCLIError("Repository is required for composition execution", 1).
			WithSuggestion("Use --repo <repository-url> to specify the target repository")
	}

	return executeCompositionAgainstRepo(compositionID, repository, branch, outputDir, generateReport)
}

// validateComposition validates a composition configuration
func validateComposition(composition RecipeComposition) error {
	if composition.Name == "" {
		return NewCLIError("Composition name is required", 1)
	}

	if len(composition.RecipeIDs) < 2 {
		return NewCLIError("At least 2 recipes are required for composition", 1)
	}

	// Validate each recipe ID format
	for _, recipeID := range composition.RecipeIDs {
		if err := ValidateRecipeID(recipeID); err != nil {
			return NewCLIError(fmt.Sprintf("Invalid recipe ID in composition: %s", recipeID), 1).WithCause(err)
		}
	}

	// Validate timeout format
	if composition.Config.Timeout != "" {
		if _, err := time.ParseDuration(composition.Config.Timeout); err != nil {
			return NewCLIError(fmt.Sprintf("Invalid timeout format: %s", composition.Config.Timeout), 1).
				WithSuggestion("Use format like '30m', '1h', '90s'")
		}
	}

	return nil
}

// getCompositionStatus gets the status of a composition execution
func getCompositionStatus(executionID string, outputFormat string) error {
	if err := ValidateRecipeID(executionID); err != nil {
		PrintError(err)
		return err
	}

	url := fmt.Sprintf("%s/arf/compositions/executions/%s", controllerURL, executionID)
	response, err := makeAPIRequest("GET", url, nil)
	if err != nil {
		return NewCLIError(fmt.Sprintf("Failed to get execution status for '%s'", executionID), 1).
			WithCause(err).
			WithSuggestion("Check if the execution ID is correct")
	}

	var execution CompositionExecution
	if err := json.Unmarshal(response, &execution); err != nil {
		return NewCLIError("Failed to parse execution status", 1).WithCause(err)
	}

	// Display results based on format
	switch strings.ToLower(outputFormat) {
	case "json":
		output, _ := json.MarshalIndent(execution, "", "  ")
		fmt.Println(string(output))
	default:
		return displayCompositionResults(execution, true)
	}

	return nil
}
