package recipes

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"time"

	models "github.com/iw2rmb/ploy/api/recipes/models"
	"gopkg.in/yaml.v3"
)

func exportToTarGz(recipes []*models.Recipe, outputFile string, filter RecipeFilter) (ExportResult, error) {
	result := ExportResult{ArchiveFile: outputFile, RecipeCount: len(recipes), Format: string(FormatTarGz), Compression: "gzip"}
	file, err := os.Create(outputFile)
	if err != nil {
		return result, err
	}
	defer func() { _ = file.Close() }()
	gzipWriter := gzip.NewWriter(file)
	defer func() { _ = gzipWriter.Close() }()
	tarWriter := tar.NewWriter(gzipWriter)
	defer func() { _ = tarWriter.Close() }()
	if err := addMetadataToTar(tarWriter, recipes, filter); err != nil {
		return result, err
	}
	for _, recipe := range recipes {
		if err := addRecipeToTar(tarWriter, recipe); err != nil {
			return result, err
		}
	}
	stat, _ := file.Stat()
	result.ArchiveSize = stat.Size()
	return result, nil
}

func exportToTar(recipes []*models.Recipe, outputFile string, filter RecipeFilter) (ExportResult, error) {
	result := ExportResult{ArchiveFile: outputFile, RecipeCount: len(recipes), Format: string(FormatTar)}
	file, err := os.Create(outputFile)
	if err != nil {
		return result, err
	}
	defer func() { _ = file.Close() }()
	tarWriter := tar.NewWriter(file)
	defer func() { _ = tarWriter.Close() }()
	if err := addMetadataToTar(tarWriter, recipes, filter); err != nil {
		return result, err
	}
	for _, recipe := range recipes {
		if err := addRecipeToTar(tarWriter, recipe); err != nil {
			return result, err
		}
	}
	stat, _ := file.Stat()
	result.ArchiveSize = stat.Size()
	return result, nil
}

func addMetadataToTar(tarWriter *tar.Writer, recipes []*models.Recipe, filter RecipeFilter) error {
	metadata := ArchiveMetadata{Version: "1.0", CreatedAt: time.Now(), CreatedBy: "ploy-cli", Description: "Recipe archive created by Ploy CLI", RecipeCount: len(recipes), Format: "tar.gz", Filters: filter}
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	header := &tar.Header{Name: "metadata.json", Size: int64(len(data)), Mode: 0644}
	if err := tarWriter.WriteHeader(header); err != nil {
		return err
	}
	_, err = tarWriter.Write(data)
	return err
}

func addRecipeToTar(tarWriter *tar.Writer, recipe *models.Recipe) error {
	data, err := yaml.Marshal(recipe)
	if err != nil {
		return err
	}
	filename := fmt.Sprintf("recipes/%s.yaml", recipe.ID)
	header := &tar.Header{Name: filename, Size: int64(len(data)), Mode: 0644}
	if err := tarWriter.WriteHeader(header); err != nil {
		return err
	}
	_, err = tarWriter.Write(data)
	return err
}
