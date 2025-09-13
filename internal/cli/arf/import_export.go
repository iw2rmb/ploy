package arf

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	models "github.com/iw2rmb/ploy/internal/arf/models"
	"gopkg.in/yaml.v3"
)

// ArchiveFormat represents supported archive formats
type ArchiveFormat string

const (
	FormatTarGz ArchiveFormat = "tar.gz"
	FormatZip   ArchiveFormat = "zip"
	FormatTar   ArchiveFormat = "tar"
)

// ArchiveMetadata contains metadata about an archive
type ArchiveMetadata struct {
	Version     string            `json:"version"`
	CreatedAt   time.Time         `json:"created_at"`
	CreatedBy   string            `json:"created_by"`
	Description string            `json:"description"`
	RecipeCount int               `json:"recipe_count"`
	Format      string            `json:"format"`
	Checksums   map[string]string `json:"checksums,omitempty"`
	Filters     RecipeFilter      `json:"filters,omitempty"`
}

// ImportResult represents the result of an import operation
type ImportResult struct {
	TotalRecipes    int      `json:"total_recipes"`
	ImportedRecipes int      `json:"imported_recipes"`
	SkippedRecipes  int      `json:"skipped_recipes"`
	FailedRecipes   int      `json:"failed_recipes"`
	Errors          []string `json:"errors,omitempty"`
	Warnings        []string `json:"warnings,omitempty"`
	ImportedIDs     []string `json:"imported_ids"`
}

// ExportResult represents the result of an export operation
type ExportResult struct {
	ArchiveFile string `json:"archive_file"`
	RecipeCount int    `json:"recipe_count"`
	ArchiveSize int64  `json:"archive_size"`
	Format      string `json:"format"`
	Compression string `json:"compression,omitempty"`
}

// importRecipes imports recipes from an archive file
func importRecipes(archiveFile string, flags CommandFlags) error {
	// Validate archive file
	if err := ValidateFilePath(archiveFile); err != nil {
		PrintError(err)
		return err
	}

	// Determine archive format
	format := detectArchiveFormat(archiveFile)
	if format == "" {
		return NewCLIError(fmt.Sprintf("Unsupported archive format: %s", archiveFile), 1).
			WithSuggestion("Supported formats: .tar.gz, .zip, .tar")
	}

	PrintInfo(fmt.Sprintf("Importing recipes from %s (%s format)...", archiveFile, format))

	// Extract and process archive
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
		return NewCLIError("Failed to import recipes", 1).
			WithCause(err).
			WithSuggestion("Check archive integrity and format")
	}

	// Display results
	return displayImportResult(result, flags)
}

// exportRecipes exports recipes to an archive file
func exportRecipes(flags CommandFlags) error {
	if flags.OutputFile == "" {
		return NewCLIError("Output file is required", 1).
			WithSuggestion("Use --output <file> to specify the archive destination")
	}

	// Determine export format from file extension
	format := detectArchiveFormat(flags.OutputFile)
	if format == "" {
		format = FormatTarGz // Default format
		if !strings.HasSuffix(flags.OutputFile, ".tar.gz") {
			flags.OutputFile += ".tar.gz"
		}
	}

	// Parse export filter from flags (TODO: implement proper flag parsing)
	filter := RecipeFilter{}

	PrintInfo(fmt.Sprintf("Exporting recipes to %s (%s format)...", flags.OutputFile, format))

	// Retrieve recipes to export
	recipes, err := getRecipesForExport(filter)
	if err != nil {
		return err
	}

	if len(recipes) == 0 {
		PrintWarning("No recipes found matching export criteria")
		return nil
	}

	// Create archive
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
		return NewCLIError("Failed to export recipes", 1).
			WithCause(err).
			WithSuggestion("Check write permissions and disk space")
	}

	// Display results
	return displayExportResult(result, flags)
}

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

	// Process tar entries
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return result, err
		}

		// Process recipe files
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

// importFromZip imports recipes from a zip archive
func importFromZip(archiveFile string, flags CommandFlags) (ImportResult, error) {
	result := ImportResult{}

	reader, err := zip.OpenReader(archiveFile)
	if err != nil {
		return result, err
	}
	defer func() { _ = reader.Close() }()

	// Process zip files
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

// importFromTar imports recipes from a tar archive
func importFromTar(archiveFile string, flags CommandFlags) (ImportResult, error) {
	result := ImportResult{}

	file, err := os.Open(archiveFile)
	if err != nil {
		return result, err
	}
	defer func() { _ = file.Close() }()

	tarReader := tar.NewReader(file)

	// Process tar entries
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return result, err
		}

		// Process recipe files
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
	// Read recipe data
	data, err := io.ReadAll(tarReader)
	if err != nil {
		return err
	}

	return processRecipeData(data, fileName, result, flags)
}

// processRecipeFromReader processes a recipe from a generic reader
func processRecipeFromReader(reader io.Reader, fileName string, result *ImportResult, flags CommandFlags) error {
	// Read recipe data
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}

	return processRecipeData(data, fileName, result, flags)
}

// processRecipeData processes recipe data and imports it
func processRecipeData(data []byte, fileName string, result *ImportResult, flags CommandFlags) error {
	// Parse recipe YAML
	var recipe models.Recipe
	if err := yaml.Unmarshal(data, &recipe); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Validate recipe
	if err := recipe.Validate(); err != nil {
		if !flags.Force {
			return fmt.Errorf("validation failed: %w", err)
		}
		result.Warnings = append(result.Warnings, fmt.Sprintf("Recipe %s has validation warnings: %v", fileName, err))
	}

	// Check if recipe already exists (unless overwrite is enabled)
	if !flags.Force {
		if recipeExists(recipe.ID) {
			result.SkippedRecipes++
			result.Warnings = append(result.Warnings, fmt.Sprintf("Recipe %s already exists (skipped)", recipe.ID))
			return nil
		}
	}

	// Dry run mode
	if flags.DryRun {
		result.ImportedRecipes++
		fmt.Printf("Would import: %s (%s)\n", recipe.Metadata.Name, recipe.ID)
		return nil
	}

	// Import recipe
	if err := importSingleRecipe(&recipe); err != nil {
		return fmt.Errorf("failed to import: %w", err)
	}

	result.ImportedRecipes++
	result.ImportedIDs = append(result.ImportedIDs, recipe.ID)

	if flags.Verbose {
		fmt.Printf("Imported: %s (%s)\n", recipe.Metadata.Name, recipe.ID)
	}

	return nil
}

// exportToTarGz exports recipes to a tar.gz archive
func exportToTarGz(recipes []*models.Recipe, outputFile string, filter RecipeFilter) (ExportResult, error) {
	result := ExportResult{
		ArchiveFile: outputFile,
		RecipeCount: len(recipes),
		Format:      string(FormatTarGz),
		Compression: "gzip",
	}

	file, err := os.Create(outputFile)
	if err != nil {
		return result, err
	}
	defer func() { _ = file.Close() }()

	gzipWriter := gzip.NewWriter(file)
	defer func() { _ = gzipWriter.Close() }()

	tarWriter := tar.NewWriter(gzipWriter)
	defer func() { _ = tarWriter.Close() }()

	// Add metadata file
	if err := addMetadataToTar(tarWriter, recipes, filter); err != nil {
		return result, err
	}

	// Add recipes
	for _, recipe := range recipes {
		if err := addRecipeToTar(tarWriter, recipe); err != nil {
			return result, err
		}
	}

	// Get file size
	stat, _ := file.Stat()
	result.ArchiveSize = stat.Size()

	return result, nil
}

// exportToZip exports recipes to a zip archive
func exportToZip(recipes []*models.Recipe, outputFile string, filter RecipeFilter) (ExportResult, error) {
	result := ExportResult{
		ArchiveFile: outputFile,
		RecipeCount: len(recipes),
		Format:      string(FormatZip),
	}

	file, err := os.Create(outputFile)
	if err != nil {
		return result, err
	}
	defer func() { _ = file.Close() }()

	zipWriter := zip.NewWriter(file)
	defer func() { _ = zipWriter.Close() }()

	// Add metadata file
	if err := addMetadataToZip(zipWriter, recipes, filter); err != nil {
		return result, err
	}

	// Add recipes
	for _, recipe := range recipes {
		if err := addRecipeToZip(zipWriter, recipe); err != nil {
			return result, err
		}
	}

	// Get file size
	stat, _ := file.Stat()
	result.ArchiveSize = stat.Size()

	return result, nil
}

// exportToTar exports recipes to a tar archive
func exportToTar(recipes []*models.Recipe, outputFile string, filter RecipeFilter) (ExportResult, error) {
	result := ExportResult{
		ArchiveFile: outputFile,
		RecipeCount: len(recipes),
		Format:      string(FormatTar),
	}

	file, err := os.Create(outputFile)
	if err != nil {
		return result, err
	}
	defer func() { _ = file.Close() }()

	tarWriter := tar.NewWriter(file)
	defer func() { _ = tarWriter.Close() }()

	// Add metadata file
	if err := addMetadataToTar(tarWriter, recipes, filter); err != nil {
		return result, err
	}

	// Add recipes
	for _, recipe := range recipes {
		if err := addRecipeToTar(tarWriter, recipe); err != nil {
			return result, err
		}
	}

	// Get file size
	stat, _ := file.Stat()
	result.ArchiveSize = stat.Size()

	return result, nil
}

// Helper functions

func detectArchiveFormat(filename string) ArchiveFormat {
	lower := strings.ToLower(filename)
	if strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz") {
		return FormatTarGz
	}
	if strings.HasSuffix(lower, ".zip") {
		return FormatZip
	}
	if strings.HasSuffix(lower, ".tar") {
		return FormatTar
	}
	return ""
}

func getRecipesForExport(filter RecipeFilter) ([]*models.Recipe, error) {
	// Build query
	queryString := BuildAPIQuery(filter)
	url := fmt.Sprintf("%s/arf/recipes%s", arfControllerURL, queryString)

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

	// Convert to pointers
	recipes := make([]*models.Recipe, len(data.Recipes))
	for i := range data.Recipes {
		recipes[i] = &data.Recipes[i]
	}

	return recipes, nil
}

func addMetadataToTar(tarWriter *tar.Writer, recipes []*models.Recipe, filter RecipeFilter) error {
	metadata := ArchiveMetadata{
		Version:     "1.0",
		CreatedAt:   time.Now(),
		CreatedBy:   "ploy-cli",
		Description: "Recipe archive created by Ploy CLI",
		RecipeCount: len(recipes),
		Format:      "tar.gz",
		Filters:     filter,
	}

	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}

	header := &tar.Header{
		Name: "metadata.json",
		Size: int64(len(data)),
		Mode: 0644,
	}

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
	header := &tar.Header{
		Name: filename,
		Size: int64(len(data)),
		Mode: 0644,
	}

	if err := tarWriter.WriteHeader(header); err != nil {
		return err
	}

	_, err = tarWriter.Write(data)
	return err
}

func addMetadataToZip(zipWriter *zip.Writer, recipes []*models.Recipe, filter RecipeFilter) error {
	metadata := ArchiveMetadata{
		Version:     "1.0",
		CreatedAt:   time.Now(),
		CreatedBy:   "ploy-cli",
		Description: "Recipe archive created by Ploy CLI",
		RecipeCount: len(recipes),
		Format:      "zip",
		Filters:     filter,
	}

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

func recipeExists(recipeID string) bool {
	url := fmt.Sprintf("%s/arf/recipes/%s", arfControllerURL, recipeID)
	_, err := makeAPIRequest("GET", url, nil)
	return err == nil // Recipe exists if no error
}

func importSingleRecipe(recipe *models.Recipe) error {
	// Set system fields
	recipe.SetSystemFields("cli-import")

	// Send to API
	recipeJSON, err := json.Marshal(recipe)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/arf/recipes", arfControllerURL)
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
