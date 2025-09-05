package arf

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	models "github.com/iw2rmb/ploy/internal/arf/models"
	"gopkg.in/yaml.v3"
)

// TableColumn represents a column in a table output
type TableColumn struct {
	Header    string
	Accessor  func(recipe *models.Recipe) string
	MinWidth  int
	MaxWidth  int
	Alignment string // "left", "right", "center"
}

// TableFormatter handles table output formatting
type TableFormatter struct {
	columns []TableColumn
	writer  *tabwriter.Writer
}

// NewTableFormatter creates a new table formatter
func NewTableFormatter() *TableFormatter {
	return &TableFormatter{
		writer: tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0),
	}
}

// WithColumns sets the table columns
func (tf *TableFormatter) WithColumns(columns []TableColumn) *TableFormatter {
	tf.columns = columns
	return tf
}

// FormatRecipes formats a list of recipes based on the specified format
func FormatRecipes(recipes []*models.Recipe, format string, verbose bool) error {
	switch strings.ToLower(format) {
	case "json":
		return formatRecipesJSON(recipes, verbose)
	case "yaml":
		return formatRecipesYAML(recipes, verbose)
	case "table":
		return formatRecipesTable(recipes, verbose)
	default:
		return NewCLIError(fmt.Sprintf("Unsupported output format: %s", format), 1).
			WithSuggestion("Use 'table', 'json', or 'yaml'")
	}
}

// formatRecipesJSON formats recipes as JSON
func formatRecipesJSON(recipes []*models.Recipe, verbose bool) error {
	var output interface{}

	if verbose {
		output = recipes
	} else {
		// Create summary view
		summaries := make([]map[string]interface{}, len(recipes))
		for i, recipe := range recipes {
			summaries[i] = map[string]interface{}{
				"id":          recipe.ID,
				"name":        recipe.Metadata.Name,
				"version":     recipe.Metadata.Version,
				"description": recipe.Metadata.Description,
				"author":      recipe.Metadata.Author,
				"languages":   recipe.Metadata.Languages,
				"categories":  recipe.Metadata.Categories,
				"tags":        recipe.Metadata.Tags,
				"created_at":  recipe.CreatedAt,
				"steps_count": len(recipe.Steps),
			}
		}
		output = summaries
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return NewCLIError("Failed to format recipes as JSON", 1).WithCause(err)
	}

	fmt.Println(string(data))
	return nil
}

// formatRecipesYAML formats recipes as YAML
func formatRecipesYAML(recipes []*models.Recipe, verbose bool) error {
	var output interface{}

	if verbose {
		output = recipes
	} else {
		// Create summary view
		summaries := make([]map[string]interface{}, len(recipes))
		for i, recipe := range recipes {
			summaries[i] = map[string]interface{}{
				"id":          recipe.ID,
				"name":        recipe.Metadata.Name,
				"version":     recipe.Metadata.Version,
				"description": recipe.Metadata.Description,
				"author":      recipe.Metadata.Author,
				"languages":   recipe.Metadata.Languages,
				"categories":  recipe.Metadata.Categories,
				"tags":        recipe.Metadata.Tags,
				"created_at":  recipe.CreatedAt.Format(time.RFC3339),
				"steps_count": len(recipe.Steps),
			}
		}
		output = summaries
	}

	data, err := yaml.Marshal(output)
	if err != nil {
		return NewCLIError("Failed to format recipes as YAML", 1).WithCause(err)
	}

	fmt.Println(string(data))
	return nil
}

// formatRecipesTable formats recipes as a table
func formatRecipesTable(recipes []*models.Recipe, verbose bool) error {
	if len(recipes) == 0 {
		PrintInfo("No recipes found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	if verbose {
		// Verbose table format
		fmt.Fprintln(w, "ID\tNAME\tVERSION\tAUTHOR\tLANGUAGES\tCATEGORIES\tTAGS\tSTEPS\tCREATED\tUPDATED")
		fmt.Fprintln(w, "--\t----\t-------\t------\t---------\t----------\t----\t-----\t-------\t-------")

		for _, recipe := range recipes {
			languages := formatStringSlice(recipe.Metadata.Languages, 15)
			categories := formatStringSlice(recipe.Metadata.Categories, 15)
			tags := formatStringSlice(recipe.Metadata.Tags, 10)

			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%d\t%s\t%s\n",
				TruncateString(recipe.ID, 20),
				TruncateString(recipe.Metadata.Name, 25),
				TruncateString(recipe.Metadata.Version, 10),
				TruncateString(recipe.Metadata.Author, 15),
				languages,
				categories,
				tags,
				len(recipe.Steps),
				recipe.CreatedAt.Format("2006-01-02"),
				recipe.UpdatedAt.Format("2006-01-02"),
			)
		}
	} else {
		// Compact table format
		fmt.Fprintln(w, "ID\tNAME\tVERSION\tLANGUAGES\tCATEGORIES\tAUTHOR\tSTEPS")
		fmt.Fprintln(w, "--\t----\t-------\t---------\t----------\t------\t-----")

		for _, recipe := range recipes {
			languages := formatStringSlice(recipe.Metadata.Languages, 20)
			categories := formatStringSlice(recipe.Metadata.Categories, 20)

			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%d\n",
				TruncateString(recipe.ID, 25),
				TruncateString(recipe.Metadata.Name, 30),
				TruncateString(recipe.Metadata.Version, 10),
				languages,
				categories,
				TruncateString(recipe.Metadata.Author, 15),
				len(recipe.Steps),
			)
		}
	}

	w.Flush()
	fmt.Printf("\nTotal: %d recipes\n", len(recipes))
	return nil
}

// formatStringSlice formats a string slice for table display
func formatStringSlice(slice []string, maxLength int) string {
	if len(slice) == 0 {
		return "-"
	}

	joined := strings.Join(slice, ",")
	if len(joined) <= maxLength {
		return joined
	}

	if maxLength <= 3 {
		return joined[:maxLength]
	}

	return joined[:maxLength-3] + "..."
}

// FormatRecipeDetails formats a single recipe's details
func FormatRecipeDetails(recipe *models.Recipe, format string, verbose bool) error {
	switch strings.ToLower(format) {
	case "json":
		data, err := json.MarshalIndent(recipe, "", "  ")
		if err != nil {
			return NewCLIError("Failed to format recipe as JSON", 1).WithCause(err)
		}
		fmt.Println(string(data))

	case "yaml":
		data, err := yaml.Marshal(recipe)
		if err != nil {
			return NewCLIError("Failed to format recipe as YAML", 1).WithCause(err)
		}
		fmt.Println(string(data))

	case "table":
		formatRecipeDetailsTable(recipe, verbose)

	default:
		return NewCLIError(fmt.Sprintf("Unsupported output format: %s", format), 1).
			WithSuggestion("Use 'table', 'json', or 'yaml'")
	}

	return nil
}

// formatRecipeDetailsTable formats a single recipe as a detailed table
func formatRecipeDetailsTable(recipe *models.Recipe, verbose bool) {
	fmt.Printf("Recipe Details\n")
	fmt.Printf("==============\n")
	fmt.Printf("ID:          %s\n", recipe.ID)
	fmt.Printf("Name:        %s\n", recipe.Metadata.Name)
	fmt.Printf("Description: %s\n", recipe.Metadata.Description)
	fmt.Printf("Version:     %s\n", recipe.Metadata.Version)
	fmt.Printf("Author:      %s\n", recipe.Metadata.Author)

	if recipe.Metadata.License != "" {
		fmt.Printf("License:     %s\n", recipe.Metadata.License)
	}

	if len(recipe.Metadata.Languages) > 0 {
		fmt.Printf("Languages:   %s\n", strings.Join(recipe.Metadata.Languages, ", "))
	}

	if len(recipe.Metadata.Categories) > 0 {
		fmt.Printf("Categories:  %s\n", strings.Join(recipe.Metadata.Categories, ", "))
	}

	if len(recipe.Metadata.Tags) > 0 {
		fmt.Printf("Tags:        %s\n", strings.Join(recipe.Metadata.Tags, ", "))
	}

	if recipe.Metadata.MinPlatform != "" {
		fmt.Printf("Min Platform: %s\n", recipe.Metadata.MinPlatform)
	}

	if recipe.Metadata.MaxPlatform != "" {
		fmt.Printf("Max Platform: %s\n", recipe.Metadata.MaxPlatform)
	}

	if len(recipe.Steps) > 0 {
		fmt.Printf("\nSteps (%d):\n", len(recipe.Steps))
		for i, step := range recipe.Steps {
			fmt.Printf("  %d. %s (%s)\n", i+1, step.Name, step.Type)
			if verbose {
				fmt.Printf("     Config: %+v\n", step.Config)
			}
		}
	}

	if verbose {
		fmt.Printf("\nSystem Information:\n")
		fmt.Printf("  Created:     %s\n", recipe.CreatedAt.Format(time.RFC3339))
		fmt.Printf("  Updated:     %s\n", recipe.UpdatedAt.Format(time.RFC3339))
		fmt.Printf("  Uploaded By: %s\n", recipe.UploadedBy)
		fmt.Printf("  Hash:        %s\n", recipe.Hash)
	}
}

// FormatSearchResults formats search results
func FormatSearchResults(recipes []*models.Recipe, query string, format string, verbose bool) error {
	switch strings.ToLower(format) {
	case "json":
		result := map[string]interface{}{
			"query":   query,
			"count":   len(recipes),
			"recipes": recipes,
		}
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return NewCLIError("Failed to format search results as JSON", 1).WithCause(err)
		}
		fmt.Println(string(data))

	case "yaml":
		result := map[string]interface{}{
			"query":   query,
			"count":   len(recipes),
			"recipes": recipes,
		}
		data, err := yaml.Marshal(result)
		if err != nil {
			return NewCLIError("Failed to format search results as YAML", 1).WithCause(err)
		}
		fmt.Println(string(data))

	case "table":
		formatSearchResultsTable(recipes, query, verbose)

	default:
		return NewCLIError(fmt.Sprintf("Unsupported output format: %s", format), 1).
			WithSuggestion("Use 'table', 'json', or 'yaml'")
	}

	return nil
}

// formatSearchResultsTable formats search results as a table
func formatSearchResultsTable(recipes []*models.Recipe, query string, verbose bool) {
	fmt.Printf("Search results for \"%s\":\n\n", query)

	if len(recipes) == 0 {
		PrintInfo("No recipes found")
		return
	}

	for _, recipe := range recipes {
		fmt.Printf("• %s (%s)\n", recipe.Metadata.Name, recipe.ID)
		fmt.Printf("  %s\n", recipe.Metadata.Description)
		fmt.Printf("  Languages: %s\n", strings.Join(recipe.Metadata.Languages, ", "))
		fmt.Printf("  Categories: %s\n", strings.Join(recipe.Metadata.Categories, ", "))

		if verbose {
			fmt.Printf("  Author: %s\n", recipe.Metadata.Author)
			fmt.Printf("  Version: %s\n", recipe.Metadata.Version)
			fmt.Printf("  Created: %s\n", recipe.CreatedAt.Format("2006-01-02"))
		}
		fmt.Println()
	}

	fmt.Printf("Total: %d recipes\n", len(recipes))
}

// SortRecipes sorts recipes based on the specified field and order
func SortRecipes(recipes []*models.Recipe, sortBy, sortOrder string) {
	if sortBy == "" {
		return
	}

	ascending := strings.ToLower(sortOrder) != "desc"

	sort.Slice(recipes, func(i, j int) bool {
		var result bool

		switch strings.ToLower(sortBy) {
		case "name":
			result = recipes[i].Metadata.Name < recipes[j].Metadata.Name
		case "created":
			result = recipes[i].CreatedAt.Before(recipes[j].CreatedAt)
		case "updated":
			result = recipes[i].UpdatedAt.Before(recipes[j].UpdatedAt)
		case "author":
			result = recipes[i].Metadata.Author < recipes[j].Metadata.Author
		case "version":
			result = recipes[i].Metadata.Version < recipes[j].Metadata.Version
		default:
			// Default to name sorting
			result = recipes[i].Metadata.Name < recipes[j].Metadata.Name
		}

		if !ascending {
			result = !result
		}

		return result
	})
}
