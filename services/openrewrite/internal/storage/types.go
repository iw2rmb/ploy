package openrewrite

import (
	"time"
)

// JobStatus represents the status of an OpenRewrite transformation job
type JobStatus struct {
	JobID       string     `json:"job_id"`
	Status      string     `json:"status"` // queued, processing, completed, failed
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	DiffURL     string     `json:"diff_url,omitempty"`
	Error       string     `json:"error,omitempty"`
	Progress    int        `json:"progress"` // 0-100
	Message     string     `json:"message"`
}

// JobStatusConstants defines valid status values
const (
	StatusQueued     = "queued"
	StatusProcessing = "processing"
	StatusCompleted  = "completed"
	StatusFailed     = "failed"
)

// RecipeConfig represents the configuration for an OpenRewrite recipe
type RecipeConfig struct {
	Name        string                 `json:"name"`
	Type        string                 `json:"type"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
	Description string                 `json:"description,omitempty"`
}

// Job represents a transformation job in the queue
type Job struct {
	ID        string       `json:"id"`
	Priority  int          `json:"priority"`
	TarData   []byte       `json:"-"` // Excluded from JSON
	Recipe    RecipeConfig `json:"recipe"`
	CreatedAt time.Time    `json:"created_at"`
	Retries   int          `json:"retries"`
}

// TransformationResult represents the result of an OpenRewrite transformation
type TransformationResult struct {
	JobID       string    `json:"job_id"`
	Diff        []byte    `json:"-"` // Excluded from JSON, stored separately
	DiffSize    int64     `json:"diff_size"`
	FilesChanged int      `json:"files_changed"`
	Success     bool      `json:"success"`
	Message     string    `json:"message"`
	Duration    time.Duration `json:"duration"`
}

// Metrics represents metrics for the OpenRewrite service
type Metrics struct {
	QueueDepth    int       `json:"queue_depth"`
	ActiveWorkers int       `json:"active_workers"`
	LastActivity  time.Time `json:"last_activity"`
	JobsProcessed int64     `json:"jobs_processed"`
	JobsFailed    int64     `json:"jobs_failed"`
}