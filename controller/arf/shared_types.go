package arf

import "time"

// Shared types for ARF components to avoid duplication

// ValidationTest represents a validation test that can be executed
type ValidationTest struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	Command     string                 `json:"command,omitempty"`
	Expected    interface{}            `json:"expected"`
	Description string                 `json:"description"`
	Critical    bool                   `json:"critical"`
	Parameters  map[string]interface{} `json:"parameters"`
	Timeout     time.Duration          `json:"timeout,omitempty"`
}

// ValidationResult represents the result of a validation test
type ValidationResult struct {
	Test        ValidationTest `json:"test"`
	Success     bool           `json:"success"`
	Output      string         `json:"output"`
	Error       string         `json:"error,omitempty"`
	Duration    time.Duration  `json:"duration"`
	Critical    bool           `json:"critical"`
	Timestamp   time.Time      `json:"timestamp"`
}

// Dependency represents a software dependency with comprehensive metadata
type Dependency struct {
	Name         string                 `json:"name"`
	Version      string                 `json:"version"`
	Ecosystem    string                 `json:"ecosystem"`
	Type         string                 `json:"type"`
	Path         string                 `json:"path,omitempty"`
	Metadata     map[string]interface{} `json:"metadata"`
	// Additional fields for multi-repo orchestrator
	Repository   string                 `json:"repository,omitempty"`
	Branch       string                 `json:"branch,omitempty"`
	LastUpdated  time.Time              `json:"last_updated,omitempty"`
}

// RiskAssessment represents a comprehensive risk assessment
type RiskAssessment struct {
	OverallRisk      string                    `json:"overall_risk"`
	RiskScore        float64                   `json:"risk_score"`
	CriticalCount    int                       `json:"critical_count"`
	HighCount        int                       `json:"high_count"`
	MediumCount      int                       `json:"medium_count"`
	LowCount         int                       `json:"low_count"`
	Recommendations  []SecurityRecommendation  `json:"recommendations"`
	Timeline         RemediationTimeline       `json:"timeline"`
	ComplianceStatus ComplianceStatus          `json:"compliance"`
	// Additional fields for strategy selector
	Confidence       float64                   `json:"confidence,omitempty"`
	Factors          []string                  `json:"risk_factors,omitempty"`
	Mitigation       []string                  `json:"mitigation_steps,omitempty"`
}

// SecurityRecommendation represents an actionable security recommendation
type SecurityRecommendation struct {
	Priority    int             `json:"priority"`
	Category    string          `json:"category"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Action      string          `json:"action"`
	Impact      string          `json:"impact"`
	Effort      EstimatedEffort `json:"effort"`
	Urgency     string          `json:"urgency"`
}

// RemediationTimeline provides timeline for addressing vulnerabilities
type RemediationTimeline struct {
	Immediate []string `json:"immediate"` // < 24 hours
	Short     []string `json:"short_term"` // < 1 week
	Medium    []string `json:"medium_term"` // < 1 month
	Long      []string `json:"long_term"` // > 1 month
}

// ComplianceStatus tracks compliance with security frameworks
type ComplianceStatus struct {
	Frameworks map[string]FrameworkCompliance `json:"frameworks"`
	Overall    string                         `json:"overall_status"`
	Issues     []ComplianceIssue              `json:"issues"`
}

// FrameworkCompliance represents compliance with a specific framework
type FrameworkCompliance struct {
	Name         string             `json:"name"`
	Version      string             `json:"version"`
	Score        float64            `json:"score"`
	Status       string             `json:"status"`
	Requirements map[string]bool    `json:"requirements"`
}

// ComplianceIssue represents a compliance-related issue
type ComplianceIssue struct {
	Framework   string   `json:"framework"`
	Requirement string   `json:"requirement"`
	Description string   `json:"description"`
	Severity    string   `json:"severity"`
	Actions     []string `json:"required_actions"`
}

// EstimatedEffort represents the effort required for remediation
type EstimatedEffort struct {
	Level       string        `json:"level"` // low, medium, high, critical
	TimeMinutes int           `json:"estimated_minutes"`
	Complexity  int           `json:"complexity_score"` // 1-10
	Risk        string        `json:"risk_level"`
	Resources   []string      `json:"required_resources"`
}