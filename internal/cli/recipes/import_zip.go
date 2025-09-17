package recipes

import (
	"archive/zip"
	"fmt"
	"strings"
)

// importFromZip imports recipes from a zip archive
func importFromZip(archiveFile string, flags CommandFlags) (ImportResult, error) {
	result := ImportResult{}
	reader, err := zip.OpenReader(archiveFile)
	if err != nil {
		return result, err
	}
	defer func() { _ = reader.Close() }()
	for _, file := range reader.File {
		if strings.HasSuffix(file.Name, ".yaml") || strings.HasSuffix(file.Name, ".yml") {
			rc, err := file.Open()
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("Failed to open %s: %v", file.Name, err))
				result.FailedRecipes++
				continue
			}
			if err := processRecipeFromReader(rc, file.Name, &result, flags); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("Failed to process %s: %v", file.Name, err))
				result.FailedRecipes++
			}
			_ = rc.Close()
		}
	}
	result.TotalRecipes = result.ImportedRecipes + result.SkippedRecipes + result.FailedRecipes
	return result, nil
}
