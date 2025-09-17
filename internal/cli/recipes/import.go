package recipes

import (
	"encoding/json"
	"fmt"

	models "github.com/iw2rmb/ploy/api/recipes/models"
)

// importRecipes imports recipes from an archive file
func importRecipes(archiveFile string, flags CommandFlags) error {
	if err := ValidateFilePath(archiveFile); err != nil {
		PrintError(err)
		return err
	}
	format := detectArchiveFormat(archiveFile)
	if format == "" {
		return NewCLIError(fmt.Sprintf("Unsupported archive format: %s", archiveFile), 1).WithSuggestion("Supported formats: .tar.gz, .zip, .tar")
	}
	PrintInfo(fmt.Sprintf("Importing recipes from %s (%s format)...", archiveFile, format))
	var result ImportResult
	var err error
	switch format {
	case FormatTarGz:
		result, err = importFromTarGz(archiveFile, flags)
	case FormatZip:
		result, err = importFromZip(archiveFile, flags)
	case FormatTar:
		result, err = importFromTar(archiveFile, flags)
	default:
		return NewCLIError(fmt.Sprintf("Import handler not implemented for format: %s", format), 1)
	}
	if err != nil {
		return NewCLIError("Failed to import recipes", 1).WithCause(err).WithSuggestion("Check archive integrity and format")
	}
	return displayImportResult(result, flags)
}

func recipeExists(recipeID string) bool {
	url := fmt.Sprintf("%s/arf/recipes/%s", controllerURL, recipeID)
	_, err := makeAPIRequest("GET", url, nil)
	return err == nil
}

func importSingleRecipe(recipe *models.Recipe) error {
	recipe.SetSystemFields("cli-import")
	recipeJSON, err := json.Marshal(recipe)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/arf/recipes", controllerURL)
	_, err = makeAPIRequest("POST", url, recipeJSON)
	return err
}

func displayImportResult(result ImportResult, flags CommandFlags) error {
	fmt.Printf("\nImport Results\n")
	fmt.Printf("==============\n")
	fmt.Printf("Total recipes processed: %d\n", result.TotalRecipes)
	fmt.Printf("Successfully imported:   %d\n", result.ImportedRecipes)
	fmt.Printf("Skipped (existing):      %d\n", result.SkippedRecipes)
	fmt.Printf("Failed to import:        %d\n", result.FailedRecipes)
	if len(result.Warnings) > 0 {
		fmt.Printf("\nWarnings (%d):\n", len(result.Warnings))
		for _, warning := range result.Warnings {
			PrintWarning(warning)
		}
	}
	if len(result.Errors) > 0 {
		fmt.Printf("\nErrors (%d):\n", len(result.Errors))
		for _, err := range result.Errors {
			fmt.Printf("  ❌ %s\n", err)
		}
	}
	if flags.Verbose && len(result.ImportedIDs) > 0 {
		fmt.Printf("\nImported Recipe IDs:\n")
		for _, id := range result.ImportedIDs {
			fmt.Printf("  • %s\n", id)
		}
	}
	if result.ImportedRecipes > 0 {
		PrintSuccess(fmt.Sprintf("Successfully imported %d recipes", result.ImportedRecipes))
	} else if result.TotalRecipes > 0 {
		PrintWarning("No recipes were imported")
	}
	return nil
}
