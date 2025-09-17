package recipes

import "time"

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
