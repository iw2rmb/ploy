package security

import "time"

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

// AST represents an abstract syntax tree for caching
type AST struct {
	FilePath string                 `json:"file_path"`
	Language string                 `json:"language"`
	Checksum string                 `json:"checksum"`
	Nodes    map[string]interface{} `json:"nodes"`
	ParsedAt time.Time              `json:"parsed_at"`
	Size     int64                  `json:"size"`
}

// ASTCacheStats provides cache performance metrics
type ASTCacheStats struct {
	Hits        int64   `json:"hits"`
	Misses      int64   `json:"misses"`
	HitRate     float64 `json:"hit_rate"`
	Size        int64   `json:"size"`
	MemoryUsage int64   `json:"memory_usage"`
}
