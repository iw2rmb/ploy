package recipes

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"strings"
)

// importFromTarGz imports recipes from a tar.gz archive
func importFromTarGz(archiveFile string, flags CommandFlags) (ImportResult, error) {
	result := ImportResult{}
	file, err := os.Open(archiveFile)
	if err != nil {
		return result, err
	}
	defer func() { _ = file.Close() }()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return result, err
	}
	defer func() { _ = gzipReader.Close() }()
	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return result, err
		}
		if strings.HasSuffix(header.Name, ".yaml") || strings.HasSuffix(header.Name, ".yml") {
			if err := processRecipeFromTar(tarReader, header.Name, &result, flags); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("Failed to process %s: %v", header.Name, err))
				result.FailedRecipes++
			}
		}
	}
	result.TotalRecipes = result.ImportedRecipes + result.SkippedRecipes + result.FailedRecipes
	return result, nil
}

// importFromTar imports recipes from a tar archive
func importFromTar(archiveFile string, flags CommandFlags) (ImportResult, error) {
	result := ImportResult{}
	file, err := os.Open(archiveFile)
	if err != nil {
		return result, err
	}
	defer func() { _ = file.Close() }()
	tarReader := tar.NewReader(file)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return result, err
		}
		if strings.HasSuffix(header.Name, ".yaml") || strings.HasSuffix(header.Name, ".yml") {
			if err := processRecipeFromTar(tarReader, header.Name, &result, flags); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("Failed to process %s: %v", header.Name, err))
				result.FailedRecipes++
			}
		}
	}
	result.TotalRecipes = result.ImportedRecipes + result.SkippedRecipes + result.FailedRecipes
	return result, nil
}

// processRecipeFromTar processes a recipe from tar reader
func processRecipeFromTar(tarReader *tar.Reader, fileName string, result *ImportResult, flags CommandFlags) error {
	data, err := io.ReadAll(tarReader)
	if err != nil {
		return err
	}
	return processRecipeData(data, fileName, result, flags)
}
