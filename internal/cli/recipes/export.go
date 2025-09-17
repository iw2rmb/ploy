package recipes

import (
	"encoding/json"
	"fmt"
	"strings"

	models "github.com/iw2rmb/ploy/internal/arf/models"
)

// exportRecipes exports recipes to an archive file
func exportRecipes(flags CommandFlags) error {
	if flags.OutputFile == "" {
		return NewCLIError("Output file is required", 1).WithSuggestion("Use --output <file> to specify the archive destination")
	}
	format := detectArchiveFormat(flags.OutputFile)
	if format == "" {
		format = FormatTarGz
		if !strings.HasSuffix(flags.OutputFile, ".tar.gz") {
			flags.OutputFile += ".tar.gz"
		}
	}
	filter := RecipeFilter{}
	PrintInfo(fmt.Sprintf("Exporting recipes to %s (%s format)...", flags.OutputFile, format))
	recipes, err := getRecipesForExport(filter)
	if err != nil {
		return err
	}
	if len(recipes) == 0 {
		PrintWarning("No recipes found matching export criteria")
		return nil
	}
	var result ExportResult
	switch format {
	case FormatTarGz:
		result, err = exportToTarGz(recipes, flags.OutputFile, filter)
	case FormatZip:
		result, err = exportToZip(recipes, flags.OutputFile, filter)
	case FormatTar:
		result, err = exportToTar(recipes, flags.OutputFile, filter)
	default:
		return NewCLIError(fmt.Sprintf("Export handler not implemented for format: %s", format), 1)
	}
	if err != nil {
		return NewCLIError("Failed to export recipes", 1).WithCause(err).WithSuggestion("Check write permissions and disk space")
	}
	return displayExportResult(result, flags)
}

func getRecipesForExport(filter RecipeFilter) ([]*models.Recipe, error) {
	queryString := BuildAPIQuery(filter)
	url := fmt.Sprintf("%s/arf/recipes%s", controllerURL, queryString)
	response, err := makeAPIRequest("GET", url, nil)
	if err != nil {
		return nil, NewCLIError("Failed to retrieve recipes for export", 1).WithCause(err)
	}
	var data struct {
		Recipes []models.Recipe `json:"recipes"`
	}
	if err := json.Unmarshal(response, &data); err != nil {
		return nil, NewCLIError("Failed to parse recipes data", 1).WithCause(err)
	}
	recipes := make([]*models.Recipe, len(data.Recipes))
	for i := range data.Recipes {
		recipes[i] = &data.Recipes[i]
	}
	return recipes, nil
}

func displayExportResult(result ExportResult, flags CommandFlags) error {
	fmt.Printf("\nExport Results\n")
	fmt.Printf("==============\n")
	fmt.Printf("Archive file:    %s\n", result.ArchiveFile)
	fmt.Printf("Recipes exported: %d\n", result.RecipeCount)
	fmt.Printf("Archive size:    %s\n", FormatFileSize(result.ArchiveSize))
	fmt.Printf("Format:          %s\n", result.Format)
	if result.Compression != "" {
		fmt.Printf("Compression:     %s\n", result.Compression)
	}
	PrintSuccess(fmt.Sprintf("Successfully exported %d recipes to %s", result.RecipeCount, result.ArchiveFile))
	return nil
}
