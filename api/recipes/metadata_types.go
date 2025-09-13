package recipes

import (
	"time"

	"github.com/iw2rmb/ploy/api/arf/models"
)

// RecipeFilters defines search criteria for recipes
type RecipeFilters struct {
	Language      string   `json:"language,omitempty"`
	Category      string   `json:"category,omitempty"`
	Tags          []string `json:"tags,omitempty"`
	Author        string   `json:"author,omitempty"`
	MinConfidence float64  `json:"min_confidence,omitempty"`
	MaxConfidence float64  `json:"max_confidence,omitempty"`
}

// RecipeStats tracks usage and performance metrics for recipes
type RecipeStats struct {
	RecipeID         string        `json:"recipe_id"`
	TotalExecutions  int64         `json:"total_executions"`
	SuccessfulRuns   int64         `json:"successful_runs"`
	FailedRuns       int64         `json:"failed_runs"`
	SuccessRate      float64       `json:"success_rate"`
	AvgExecutionTime time.Duration `json:"avg_execution_time"`
	LastExecuted     time.Time     `json:"last_executed"`
	FirstExecuted    time.Time     `json:"first_executed"`
}

// UnifiedRecipeMetadata represents the unified format for all recipes
type UnifiedRecipeMetadata struct {
	Metadata RecipeInfo          `json:"metadata" yaml:"metadata"`
	Maven    *MavenInfo          `json:"maven,omitempty" yaml:"maven,omitempty"`
	Steps    []models.RecipeStep `json:"steps,omitempty" yaml:"steps,omitempty"`
	Cache    *CacheInfo          `json:"cache,omitempty" yaml:"cache,omitempty"`
}

// RecipeInfo contains basic recipe metadata
type RecipeInfo struct {
	ID         string   `json:"id" yaml:"id"`
	Name       string   `json:"name" yaml:"name"`
	Version    string   `json:"version" yaml:"version"`
	Type       string   `json:"type" yaml:"type"`
	Source     string   `json:"source" yaml:"source"`
	Author     string   `json:"author" yaml:"author"`
	Tags       []string `json:"tags" yaml:"tags"`
	Categories []string `json:"categories" yaml:"categories"`
}

// MavenInfo contains Maven-specific recipe information
type MavenInfo struct {
	Group    string `json:"group" yaml:"group"`
	Artifact string `json:"artifact" yaml:"artifact"`
	Version  string `json:"version" yaml:"version"`
	Class    string `json:"class" yaml:"class"`
}

// CacheInfo contains cache-related metadata
type CacheInfo struct {
	StoredAt  time.Time `json:"stored_at" yaml:"stored_at"`
	JarPath   string    `json:"jar_path,omitempty" yaml:"jar_path,omitempty"`
	SizeBytes int64     `json:"size_bytes" yaml:"size_bytes"`
	Hash      string    `json:"hash" yaml:"hash"`
}
