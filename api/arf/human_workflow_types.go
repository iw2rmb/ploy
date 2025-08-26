package arf

import (
	"time"
)

// HumanWorkflow represents a human-in-the-loop workflow
type HumanWorkflow struct {
	ID               string                 `json:"id"`
	Type             string                 `json:"type"` // approval, review, validation
	Status           WorkflowStatus         `json:"status"`
	Priority         Priority               `json:"priority"`
	Context          WorkflowContext        `json:"context"`
	Stakeholders     []Stakeholder          `json:"stakeholders"`
	Steps            []WorkflowStep         `json:"steps"`
	CurrentStep      int                    `json:"current_step"`
	Decisions        []Decision             `json:"decisions"`
	Metadata         map[string]interface{} `json:"metadata"`
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
	CompletedAt      *time.Time             `json:"completed_at,omitempty"`
	ExpiresAt        time.Time              `json:"expires_at"`
	EscalationLevel  int                    `json:"escalation_level"`
	ParentWorkflowID string                 `json:"parent_workflow_id,omitempty"`
}

// WorkflowStatus represents workflow state
type WorkflowStatus string

const (
	WorkflowPending     WorkflowStatus = "pending"
	WorkflowInProgress  WorkflowStatus = "in_progress"
	WorkflowApproved    WorkflowStatus = "approved"
	WorkflowRejected    WorkflowStatus = "rejected"
	WorkflowCompleted   WorkflowStatus = "completed"
	WorkflowExpired     WorkflowStatus = "expired"
	WorkflowEscalated   WorkflowStatus = "escalated"
	WorkflowCancelled   WorkflowStatus = "cancelled"
)

// Priority represents workflow priority levels
type Priority string

const (
	PriorityLow      Priority = "low"
	PriorityMedium   Priority = "medium"
	PriorityHigh     Priority = "high"
	PriorityCritical Priority = "critical"
)

// WorkflowContext provides context about the workflow
type WorkflowContext struct {
	RecipeID         string                 `json:"recipe_id,omitempty"`
	VulnerabilityIDs []string               `json:"vulnerability_ids,omitempty"`
	CodebaseID       string                 `json:"codebase_id,omitempty"`
	AffectedFiles    []string               `json:"affected_files,omitempty"`
	RiskLevel        string                 `json:"risk_level"`
	ImpactAssessment ImpactAssessment       `json:"impact_assessment"`
	BusinessContext  BusinessContext        `json:"business_context"`
	TechnicalContext TechnicalContext       `json:"technical_context"`
	Metadata         map[string]interface{} `json:"metadata"`
}

// ImpactAssessment represents the impact of a proposed change
type ImpactAssessment struct {
	SecurityImpact     ImpactLevel            `json:"security_impact"`
	PerformanceImpact  ImpactLevel            `json:"performance_impact"`
	StabilityImpact    ImpactLevel            `json:"stability_impact"`
	BusinessImpact     ImpactLevel            `json:"business_impact"`
	UserImpact         ImpactLevel            `json:"user_impact"`
	ComplianceImpact   ImpactLevel            `json:"compliance_impact"`
	Details            map[string]interface{} `json:"details"`
	Mitigation         string                 `json:"mitigation_strategy,omitempty"`
}

// ImpactLevel represents the level of impact
type ImpactLevel string

const (
	ImpactNone     ImpactLevel = "none"
	ImpactLow      ImpactLevel = "low"
	ImpactMedium   ImpactLevel = "medium"
	ImpactHigh     ImpactLevel = "high"
	ImpactCritical ImpactLevel = "critical"
)

// BusinessContext provides business context for the workflow
type BusinessContext struct {
	ApplicationName    string   `json:"application_name"`
	BusinessUnit       string   `json:"business_unit"`
	Environment        string   `json:"environment"`
	ServiceTier        string   `json:"service_tier"` // tier1, tier2, tier3
	ComplianceReqs     []string `json:"compliance_requirements"`
	MaintenanceWindow  string   `json:"maintenance_window,omitempty"`
	ContactPerson      string   `json:"contact_person"`
	SLARequirements    string   `json:"sla_requirements"`
}

// TechnicalContext provides technical context for the workflow
type TechnicalContext struct {
	Language           string   `json:"language"`
	Framework          string   `json:"framework"`
	Dependencies       []string `json:"dependencies"`
	Architecture       string   `json:"architecture"`
	DeploymentStrategy string   `json:"deployment_strategy"`
	TestCoverage       float64  `json:"test_coverage"`
	BuildSystem        string   `json:"build_system"`
	CISystem           string   `json:"ci_system"`
}

// Stakeholder represents a person involved in the workflow
type Stakeholder struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	Email        string         `json:"email"`
	Role         StakeholderRole `json:"role"`
	Permissions  []Permission   `json:"permissions"`
	NotifyMethod []string       `json:"notify_methods"` // email, slack, teams
	Required     bool           `json:"required"`
}

// StakeholderRole represents roles in the workflow
type StakeholderRole string

const (
	RoleApprover        StakeholderRole = "approver"
	RoleReviewer        StakeholderRole = "reviewer"
	RoleSecurityOfficer StakeholderRole = "security_officer"
	RoleArchitect       StakeholderRole = "architect"
	RoleProductOwner    StakeholderRole = "product_owner"
	RoleDevLead         StakeholderRole = "dev_lead"
	RoleTechLead        StakeholderRole = "tech_lead"
	RoleObserver        StakeholderRole = "observer"
)

// Permission represents a permission level
type Permission string

const (
	PermissionApprove Permission = "approve"
	PermissionReject  Permission = "reject"
	PermissionReview  Permission = "review"
	PermissionComment Permission = "comment"
	PermissionView    Permission = "view"
	PermissionEdit    Permission = "edit"
)

// WorkflowStep represents a step in the workflow
type WorkflowStep struct {
	ID           string                 `json:"id"`
	Type         StepType               `json:"type"`
	Name         string                 `json:"name"`
	Description  string                 `json:"description"`
	AssignedTo   []string               `json:"assigned_to"`
	Status       StepStatus             `json:"status"`
	RequiredApprovals int               `json:"required_approvals"`
	ReceivedApprovals int               `json:"received_approvals"`
	Timeout      time.Duration          `json:"timeout"`
	StartedAt    *time.Time             `json:"started_at,omitempty"`
	CompletedAt  *time.Time             `json:"completed_at,omitempty"`
	Metadata     map[string]interface{} `json:"metadata"`
	Dependencies []string               `json:"dependencies"`
}

// StepType represents the type of workflow step
type StepType string

const (
	StepApproval       StepType = "approval"
	StepReview         StepType = "review"
	StepValidation     StepType = "validation"
	StepNotification   StepType = "notification"
	StepEscalation     StepType = "escalation"
	StepAutomation     StepType = "automation"
	StepManualAction   StepType = "manual_action"
	StepCheckpoint     StepType = "checkpoint"
)

// StepStatus represents the status of a workflow step
type StepStatus string

const (
	StepPending     StepStatus = "pending"
	StepInProgress  StepStatus = "in_progress"
	StepCompleted   StepStatus = "completed"
	StepSkipped     StepStatus = "skipped"
	StepFailed      StepStatus = "failed"
	StepTimedOut    StepStatus = "timed_out"
)

// Decision represents a decision made in the workflow
type Decision struct {
	ID           string                 `json:"id"`
	StepID       string                 `json:"step_id"`
	DecisionType DecisionType           `json:"type"`
	Value        interface{}            `json:"value"`
	Reasoning    string                 `json:"reasoning"`
	MadeBy       string                 `json:"made_by"`
	MadeAt       time.Time              `json:"made_at"`
	Confidence   float64                `json:"confidence"`
	Evidence     []string               `json:"evidence"`
	Metadata     map[string]interface{} `json:"metadata"`
}

// DecisionType represents types of decisions
type DecisionType string

const (
	DecisionApprove      DecisionType = "approve"
	DecisionReject       DecisionType = "reject"
	DecisionRequestChanges DecisionType = "request_changes"
	DecisionEscalate     DecisionType = "escalate"
	DecisionDefer        DecisionType = "defer"
	DecisionConditional  DecisionType = "conditional"
)

// ApprovalRequest represents a request for approval
type ApprovalRequest struct {
	WorkflowID       string                 `json:"workflow_id"`
	RequesterID      string                 `json:"requester_id"`
	ApproverIDs      []string               `json:"approver_ids"`
	Title            string                 `json:"title"`
	Description      string                 `json:"description"`
	Priority         Priority               `json:"priority"`
	RequiredApprovals int                   `json:"required_approvals"`
	Timeout          time.Duration          `json:"timeout"`
	Context          WorkflowContext        `json:"context"`
	Attachments      []Attachment           `json:"attachments"`
	Metadata         map[string]interface{} `json:"metadata"`
}

// ApprovalWorkflow represents an approval workflow
type ApprovalWorkflow struct {
	ID              string            `json:"id"`
	Request         ApprovalRequest   `json:"request"`
	Status          WorkflowStatus    `json:"status"`
	Decisions       []Decision        `json:"decisions"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
	CompletedAt     *time.Time        `json:"completed_at,omitempty"`
	ExpiresAt       time.Time         `json:"expires_at"`
	NotificationsSent int             `json:"notifications_sent"`
	EscalationLevel int               `json:"escalation_level"`
}

// ApprovalDecision represents an approval decision
type ApprovalDecision struct {
	WorkflowID   string                 `json:"workflow_id"`
	DecisionType DecisionType           `json:"type"`
	ApproverID   string                 `json:"approver_id"`
	Reasoning    string                 `json:"reasoning"`
	Conditions   []string               `json:"conditions,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// ReviewRequest represents a request for code review
type ReviewRequest struct {
	WorkflowID    string                 `json:"workflow_id"`
	RequesterID   string                 `json:"requester_id"`
	ReviewerIDs   []string               `json:"reviewer_ids"`
	Title         string                 `json:"title"`
	Description   string                 `json:"description"`
	Priority      Priority               `json:"priority"`
	ReviewType    ReviewType             `json:"review_type"`
	ChangedFiles  []string               `json:"changed_files"`
	DiffURL       string                 `json:"diff_url,omitempty"`
	Context       WorkflowContext        `json:"context"`
	Checklist     []ReviewItem           `json:"checklist"`
	Metadata      map[string]interface{} `json:"metadata"`
}

// ReviewType represents types of reviews
type ReviewType string

const (
	ReviewSecurity     ReviewType = "security"
	ReviewArchitecture ReviewType = "architecture"
	ReviewPerformance  ReviewType = "performance"
	ReviewCode         ReviewType = "code"
	ReviewCompliance   ReviewType = "compliance"
)

// ReviewItem represents an item in a review checklist
type ReviewItem struct {
	ID          string `json:"id"`
	Category    string `json:"category"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Checked     bool   `json:"checked"`
	Comments    string `json:"comments,omitempty"`
}

// ReviewWorkflow represents a review workflow
type ReviewWorkflow struct {
	ID            string        `json:"id"`
	Request       ReviewRequest `json:"request"`
	Status        WorkflowStatus `json:"status"`
	Reviews       []Review      `json:"reviews"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
	CompletedAt   *time.Time    `json:"completed_at,omitempty"`
}

// Review represents a submitted review
type Review struct {
	ID           string                 `json:"id"`
	ReviewerID   string                 `json:"reviewer_id"`
	Status       ReviewStatus           `json:"status"`
	Comments     []ReviewComment        `json:"comments"`
	Rating       int                    `json:"rating"` // 1-5 scale
	Checklist    []ReviewItem           `json:"checklist"`
	Recommendation string               `json:"recommendation"`
	SubmittedAt  time.Time              `json:"submitted_at"`
	Metadata     map[string]interface{} `json:"metadata"`
}

// ReviewStatus represents the status of a review
type ReviewStatus string

const (
	ReviewApproved        ReviewStatus = "approved"
	ReviewRequestedChanges ReviewStatus = "requested_changes"
	ReviewBlocked         ReviewStatus = "blocked"
	ReviewPending         ReviewStatus = "pending"
)

// ReviewComment represents a comment in a review
type ReviewComment struct {
	ID         string                 `json:"id"`
	File       string                 `json:"file,omitempty"`
	Line       int                    `json:"line,omitempty"`
	Type       CommentType            `json:"type"`
	Severity   CommentSeverity        `json:"severity"`
	Message    string                 `json:"message"`
	Suggestion string                 `json:"suggestion,omitempty"`
	Metadata   map[string]interface{} `json:"metadata"`
}

// CommentType represents types of review comments
type CommentType string

const (
	CommentGeneral     CommentType = "general"
	CommentSecurity    CommentType = "security"
	CommentPerformance CommentType = "performance"
	CommentStyle       CommentType = "style"
	CommentLogic       CommentType = "logic"
	CommentBug         CommentType = "bug"
)

// Attachment represents file attachments in workflows
type Attachment struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Type        string            `json:"type"`
	Size        int64             `json:"size"`
	URL         string            `json:"url"`
	Metadata    map[string]interface{} `json:"metadata"`
	UploadedAt  time.Time         `json:"uploaded_at"`
	UploadedBy  string            `json:"uploaded_by"`
}

// WorkflowAuditEntry represents an audit trail entry
type WorkflowAuditEntry struct {
	ID          string                 `json:"id"`
	WorkflowID  string                 `json:"workflow_id"`
	Action      string                 `json:"action"`
	Actor       string                 `json:"actor"`
	Timestamp   time.Time              `json:"timestamp"`
	Details     map[string]interface{} `json:"details"`
	IPAddress   string                 `json:"ip_address,omitempty"`
	UserAgent   string                 `json:"user_agent,omitempty"`
}

// WorkflowFilter represents filter criteria for workflows
type WorkflowFilter struct {
	Status        []WorkflowStatus `json:"status,omitempty"`
	Type          []string         `json:"type,omitempty"`
	Priority      []Priority       `json:"priority,omitempty"`
	AssignedTo    []string         `json:"assigned_to,omitempty"`
	CreatedAfter  *time.Time       `json:"created_after,omitempty"`
	CreatedBefore *time.Time       `json:"created_before,omitempty"`
	Limit         int              `json:"limit,omitempty"`
	Offset        int              `json:"offset,omitempty"`
}