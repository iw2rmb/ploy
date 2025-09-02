package arf

import "time"

// TransformationStatus represents the current status of a transformation with healing support
type TransformationStatus struct {
	TransformationID     string                  `json:"transformation_id"`
	WorkflowStage        string                  `json:"workflow_stage"` // openrewrite, build, deploy, test, heal
	Status               string                  `json:"status"`         // pending, in_progress, completed, failed
	StartTime            time.Time               `json:"start_time"`
	EndTime              time.Time               `json:"end_time,omitempty"`
	Children             []HealingAttempt        `json:"children"`
	ActiveHealingCount   int                     `json:"active_healing_count"`
	TotalHealingAttempts int                     `json:"total_healing_attempts"`
	Progress             *TransformationProgress `json:"progress,omitempty"`
	Error                string                  `json:"error,omitempty"`
}

// TransformationProgress represents the progress of a transformation
type TransformationProgress struct {
	Stage           string `json:"stage"`
	PercentComplete int    `json:"percent_complete"`
	Message         string `json:"message,omitempty"`
}

// HealingAttempt represents a single healing attempt in the transformation workflow
type HealingAttempt struct {
	TransformationID    string             `json:"transformation_id"`
	AttemptPath         string             `json:"attempt_path"`   // "1.1.2" for nested attempts
	TriggerReason       string             `json:"trigger_reason"` // build_failure, test_failure, etc.
	TargetErrors        []string           `json:"target_errors"`  // Specific errors this attempt targets
	LLMAnalysis         *LLMAnalysisResult `json:"llm_analysis,omitempty"`
	Status              string             `json:"status"`           // pending, in_progress, completed, failed
	Result              string             `json:"result,omitempty"` // success, partial_success, failed
	StartTime           time.Time          `json:"start_time"`
	EndTime             time.Time          `json:"end_time,omitempty"`
	NewIssuesDiscovered []string           `json:"new_issues_discovered,omitempty"`
	Children            []HealingAttempt   `json:"children"`
	ParentAttempt       string             `json:"parent_attempt,omitempty"` // "1.1" for parent path
}

// LLMAnalysisResult represents the result of LLM analysis for error resolution
type LLMAnalysisResult struct {
	ErrorType        string   `json:"error_type"`
	Confidence       float64  `json:"confidence"`
	SuggestedFix     string   `json:"suggested_fix"`
	AlternativeFixes []string `json:"alternative_fixes"`
	RiskAssessment   string   `json:"risk_assessment"`
}

// HealingTree represents the complete healing attempt hierarchy
type HealingTree struct {
	RootTransformID string           `json:"root_transform_id"`
	Attempts        []HealingAttempt `json:"attempts"`        // Array of attempts
	ActiveAttempts  []string         `json:"active_attempts"` // Currently running
	TotalAttempts   int              `json:"total_attempts"`
	SuccessfulHeals int              `json:"successful_heals"`
	FailedHeals     int              `json:"failed_heals"`
	MaxDepth        int              `json:"max_depth"`
}

// DeploymentMetrics contains deployment-related metrics
type DeploymentMetrics struct {
	DeploymentID     string        `json:"deployment_id"`
	DeploymentURL    string        `json:"deployment_url"`
	DeploymentStatus string        `json:"deployment_status"`
	DeploymentTime   time.Duration `json:"deployment_time"`
	SandboxID        string        `json:"sandbox_id"`
}
