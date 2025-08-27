package executor

import (
	"time"
)

// TransformRequest represents a transformation request
type TransformRequest struct {
	JobID        string       `json:"job_id"`
	TarArchive   string       `json:"tar_archive"`   // base64 encoded
	RecipeConfig RecipeConfig `json:"recipe_config"`
}

// RecipeConfig represents OpenRewrite recipe configuration
type RecipeConfig struct {
	Recipe    string `json:"recipe"`
	Artifacts string `json:"artifacts,omitempty"`
}

// TransformationResult represents the result of an OpenRewrite transformation
type TransformationResult struct {
	RecipeID        string                 `json:"recipe_id"`
	Success         bool                   `json:"success"`
	ChangesApplied  int                    `json:"changes_applied"`
	FilesModified   []string               `json:"files_modified"`
	Diff            string                 `json:"diff"`
	ValidationScore float64                `json:"validation_score"`
	ExecutionTime   time.Duration          `json:"execution_time"`
	Errors          []TransformationError  `json:"errors,omitempty"`
	Warnings        []TransformationError  `json:"warnings,omitempty"`
	Metadata        map[string]interface{} `json:"metadata"`
}

// TransformationError represents an error during transformation
type TransformationError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	File    string `json:"file,omitempty"`
	Line    int    `json:"line,omitempty"`
}