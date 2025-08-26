package openrewrite

import (
	"time"
)

// TransformRequest represents an HTTP request for code transformation
type TransformRequest struct {
	// JobID is a unique identifier for this transformation job
	JobID string `json:"job_id" validate:"required,min=1,max=100"`
	
	// TarArchive is the base64-encoded tar.gz archive of the source code
	TarArchive string `json:"tar_archive" validate:"required"`
	
	// RecipeConfig contains the OpenRewrite recipe configuration
	RecipeConfig RecipeConfig `json:"recipe_config" validate:"required"`
	
	// Timeout is the maximum duration for the transformation (optional)
	// If not provided, defaults to 5 minutes
	Timeout string `json:"timeout,omitempty"`
}

// RecipeConfig defines the OpenRewrite recipe configuration for HTTP requests
type RecipeConfig struct {
	// Recipe is the fully qualified recipe name
	Recipe string `json:"recipe" validate:"required"`
	
	// Artifacts are the Maven coordinates for recipe artifacts (optional)
	Artifacts string `json:"artifacts,omitempty"`
	
	// Options are additional recipe-specific options
	Options map[string]string `json:"options,omitempty"`
}

// TransformResponse represents the HTTP response for a transformation request
type TransformResponse struct {
	// Success indicates whether the transformation completed successfully
	Success bool `json:"success"`
	
	// JobID echoes back the job ID from the request
	JobID string `json:"job_id"`
	
	// Diff is the base64-encoded unified diff showing the changes
	Diff string `json:"diff,omitempty"`
	
	// Error contains any error message if the transformation failed
	Error string `json:"error,omitempty"`
	
	// Duration is how long the transformation took (in seconds)
	Duration float64 `json:"duration_seconds"`
	
	// BuildSystem indicates which build system was detected
	BuildSystem string `json:"build_system,omitempty"`
	
	// JavaVersion indicates which Java version was detected
	JavaVersion string `json:"java_version,omitempty"`
	
	// Stats contains additional statistics about the transformation
	Stats *TransformStats `json:"stats,omitempty"`
}

// TransformStats contains statistics about a transformation
type TransformStats struct {
	// FilesChanged is the number of files that were modified
	FilesChanged int `json:"files_changed"`
	
	// LinesAdded is the number of lines added
	LinesAdded int `json:"lines_added"`
	
	// LinesRemoved is the number of lines removed
	LinesRemoved int `json:"lines_removed"`
	
	// TarSize is the size of the input tar archive in bytes
	TarSize int `json:"tar_size_bytes"`
	
	// DiffSize is the size of the generated diff in bytes
	DiffSize int `json:"diff_size_bytes"`
}

// HealthResponse represents the health check response
type HealthResponse struct {
	// Status indicates the health status
	Status string `json:"status"`
	
	// Version is the service version
	Version string `json:"version"`
	
	// JavaVersion is the detected Java version on the system
	JavaVersion string `json:"java_version,omitempty"`
	
	// MavenVersion is the detected Maven version
	MavenVersion string `json:"maven_version,omitempty"`
	
	// GradleVersion is the detected Gradle version
	GradleVersion string `json:"gradle_version,omitempty"`
	
	// GitVersion is the detected Git version
	GitVersion string `json:"git_version,omitempty"`
	
	// Timestamp is the current server time
	Timestamp time.Time `json:"timestamp"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	// Error is the error message
	Error string `json:"error"`
	
	// Code is an optional error code
	Code string `json:"code,omitempty"`
	
	// Details provides additional error details
	Details map[string]interface{} `json:"details,omitempty"`
}

// ValidationError represents a request validation error
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidationErrorResponse represents a validation error response
type ValidationErrorResponse struct {
	Error  string            `json:"error"`
	Errors []ValidationError `json:"validation_errors"`
}