package arf

import (
	"encoding/json"
	"fmt"

	"github.com/iw2rmb/ploy/controller/arf"
)

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