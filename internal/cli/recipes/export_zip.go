package recipes

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"os"
	"time"

	models "github.com/iw2rmb/ploy/internal/arf/models"
	"gopkg.in/yaml.v3"
)

func exportToZip(recipes []*models.Recipe, outputFile string, filter RecipeFilter) (ExportResult, error) {
	result := ExportResult{ArchiveFile: outputFile, RecipeCount: len(recipes), Format: string(FormatZip)}
	file, err := os.Create(outputFile)
	if err != nil {
		return result, err
	}
	defer func() { _ = file.Close() }()
	zipWriter := zip.NewWriter(file)
	defer func() { _ = zipWriter.Close() }()
	if err := addMetadataToZip(zipWriter, recipes, filter); err != nil {
		return result, err
	}
	for _, recipe := range recipes {
		if err := addRecipeToZip(zipWriter, recipe); err != nil {
			return result, err
		}
	}
	stat, _ := file.Stat()
	result.ArchiveSize = stat.Size()
	return result, nil
}

func addMetadataToZip(zipWriter *zip.Writer, recipes []*models.Recipe, filter RecipeFilter) error {
	metadata := ArchiveMetadata{Version: "1.0", CreatedAt: time.Now(), CreatedBy: "ploy-cli", Description: "Recipe archive created by Ploy CLI", RecipeCount: len(recipes), Format: "zip", Filters: filter}
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	writer, err := zipWriter.Create("metadata.json")
	if err != nil {
		return err
	}
	_, err = writer.Write(data)
	return err
}

func addRecipeToZip(zipWriter *zip.Writer, recipe *models.Recipe) error {
	data, err := yaml.Marshal(recipe)
	if err != nil {
		return err
	}
	filename := fmt.Sprintf("recipes/%s.yaml", recipe.ID)
	writer, err := zipWriter.Create(filename)
	if err != nil {
		return err
	}
	_, err = writer.Write(data)
	return err
}
