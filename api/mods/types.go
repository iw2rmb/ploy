package mods

import "time"

// ModRunRequest represents the request body for running a mod
type ModRunRequest struct {
	Config     string                 `json:"config,omitempty"`      // YAML config as string
	ConfigData map[string]interface{} `json:"config_data,omitempty"` // Or as structured data
	Env        map[string]string      `json:"env,omitempty"`         // Per-run env overrides
	TestMode   bool                   `json:"test_mode,omitempty"`
}

// ModStatus represents the status of a mod execution
type ModStatus struct {
	ID        string                 `json:"id"`
	Status    string                 `json:"status"`
	StartTime time.Time              `json:"start_time"`
	EndTime   *time.Time             `json:"end_time,omitempty"`
	Error     string                 `json:"error,omitempty"`
	Result    map[string]interface{} `json:"result,omitempty"`
	// Enriched runtime fields
	Phase    string          `json:"phase,omitempty"`
	Overdue  bool            `json:"overdue,omitempty"`
	Duration string          `json:"duration,omitempty"`
	Steps    []ModStepStatus `json:"steps,omitempty"`
	LastJob  *ModLastJob     `json:"last_job,omitempty"`
}

// ModStepStatus represents a single step update with timestamp
type ModStepStatus struct {
	Step    string    `json:"step,omitempty"`
	Phase   string    `json:"phase,omitempty"`
	Level   string    `json:"level,omitempty"`
	Message string    `json:"message,omitempty"`
	Time    time.Time `json:"time"`
}

// ModLastJob captures metadata about the most recent submitted Nomad job
type ModLastJob struct {
	JobName     string    `json:"job_name,omitempty"`
	AllocID     string    `json:"alloc_id,omitempty"`
	SubmittedAt time.Time `json:"submitted_at"`
}

// ModEvent represents a real-time event emitted by runner/jobs
type ModEvent struct {
	ModID   string    `json:"mod_id"`
	Phase   string    `json:"phase,omitempty"`
	Step    string    `json:"step,omitempty"`
	Level   string    `json:"level,omitempty"`
	Message string    `json:"message,omitempty"`
	Time    time.Time `json:"ts,omitempty"`
	JobName string    `json:"job_name,omitempty"`
	AllocID string    `json:"alloc_id,omitempty"`
}
