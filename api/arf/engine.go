package arf

import (
	"time"
)

// Core ARF types for transformation engine

// Codebase represents the source code to be transformed
type Codebase struct {
	Repository string            `json:"repository"`
	Branch     string            `json:"branch"`
	Path       string            `json:"path"`
	RootPath   string            `json:"root_path"` // Root directory of the codebase
	Language   string            `json:"language"`
	BuildTool  string            `json:"build_tool"` // maven, gradle, etc.
	Metadata   map[string]string `json:"metadata"`
}

// TransformationResult contains the results of a recipe execution
type TransformationResult struct {
	RecipeID        string                 `json:"recipe_id"`
	Success         bool                   `json:"success"`
	ChangesApplied  int                    `json:"changes_applied"`
	TotalFiles      int                    `json:"total_files"`
	FilesModified   []string               `json:"files_modified"`
	Diff            string                 `json:"diff"`
	ValidationScore float64                `json:"validation_score"`
	ExecutionTime   time.Duration          `json:"execution_time"`
	Errors          []TransformationError  `json:"errors,omitempty"`
	Warnings        []TransformationError  `json:"warnings,omitempty"`
	Metadata        map[string]interface{} `json:"metadata"`
}

// TransformationError represents errors and warnings during transformation
type TransformationError struct {
	Type        string `json:"type"`
	Message     string `json:"message"`
	File        string `json:"file,omitempty"`
	Line        int    `json:"line,omitempty"`
	Column      int    `json:"column,omitempty"`
	Recoverable bool   `json:"recoverable"`
}


// AST represents an abstract syntax tree for caching
type AST struct {
	FilePath    string                 `json:"file_path"`
	Language    string                 `json:"language"`
	Checksum    string                 `json:"checksum"`
	Nodes       map[string]interface{} `json:"nodes"`
	ParsedAt    time.Time              `json:"parsed_at"`
	Size        int64                  `json:"size"`
}

// ASTCacheStats provides cache performance metrics
type ASTCacheStats struct {
	Hits        int64   `json:"hits"`
	Misses      int64   `json:"misses"`
	HitRate     float64 `json:"hit_rate"`
	Size        int64   `json:"size"`
	MemoryUsage int64   `json:"memory_usage"`
}