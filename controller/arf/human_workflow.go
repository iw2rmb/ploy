package arf

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// HumanWorkflowEngine manages human-in-the-loop processes for ARF operations
type HumanWorkflowEngine struct {
	approvalService     ApprovalService
	reviewService       ReviewService
	notificationService NotificationService
	auditService        AuditService
	workflowStore       WorkflowStore
}

// ApprovalService handles approval workflows
type ApprovalService interface {
	CreateApprovalRequest(req ApprovalRequest) (*ApprovalWorkflow, error)
	ProcessApproval(workflowID string, decision ApprovalDecision) error
	GetPendingApprovals(userID string) ([]ApprovalWorkflow, error)
	ExpireApproval(workflowID string) error
}

// ReviewService handles code review workflows
type ReviewService interface {
	CreateReviewRequest(req ReviewRequest) (*ReviewWorkflow, error)
	SubmitReview(workflowID string, review Review) error
	GetPendingReviews(reviewerID string) ([]ReviewWorkflow, error)
	AssignReviewer(workflowID string, reviewerID string) error
}

// NotificationService handles notifications to users
type NotificationService interface {
	SendApprovalNotification(approval ApprovalWorkflow) error
	SendReviewNotification(review ReviewWorkflow) error
	SendEscalationNotification(workflow HumanWorkflow) error
	SendCompletionNotification(workflow HumanWorkflow) error
}

// AuditService handles audit trail for human workflows
type AuditService interface {
	LogWorkflowAction(action WorkflowAuditEntry) error
	GetWorkflowAudit(workflowID string) ([]WorkflowAuditEntry, error)
	GetUserActivity(userID string, timeRange TimeRange) ([]WorkflowAuditEntry, error)
}

// WorkflowStore persists workflow state
type WorkflowStore interface {
	CreateWorkflow(workflow HumanWorkflow) error
	UpdateWorkflow(workflow HumanWorkflow) error
	GetWorkflow(workflowID string) (*HumanWorkflow, error)
	ListWorkflows(filter WorkflowFilter) ([]HumanWorkflow, error)
	DeleteWorkflow(workflowID string) error
}

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

// CommentSeverity types are defined in common_types.go

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

// TimeRange type is defined in common_types.go

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

// NewHumanWorkflowEngine creates a new human workflow engine
func NewHumanWorkflowEngine(
	approvalService ApprovalService,
	reviewService ReviewService,
	notificationService NotificationService,
	auditService AuditService,
	workflowStore WorkflowStore,
) *HumanWorkflowEngine {
	return &HumanWorkflowEngine{
		approvalService:     approvalService,
		reviewService:       reviewService,
		notificationService: notificationService,
		auditService:        auditService,
		workflowStore:       workflowStore,
	}
}

// CreateRemediationApprovalWorkflow creates an approval workflow for remediation
func (h *HumanWorkflowEngine) CreateRemediationApprovalWorkflow(
	ctx context.Context,
	recipe RemediationRecipe,
	vulns []VulnerabilityInfo,
	requesterID string,
) (*HumanWorkflow, error) {
	// Determine stakeholders based on vulnerability severity and context
	stakeholders := h.determineStakeholders(vulns, recipe)
	
	// Calculate priority and impact
	priority := h.calculatePriority(vulns)
	impactAssessment := h.assessImpact(recipe, vulns)
	
	// Create workflow context
	context := WorkflowContext{
		RecipeID:         recipe.ID,
		VulnerabilityIDs: recipe.Vulnerabilities,
		RiskLevel:        h.determineRiskLevel(vulns),
		ImpactAssessment: impactAssessment,
		BusinessContext:  h.extractBusinessContext(recipe),
		TechnicalContext: h.extractTechnicalContext(recipe),
	}
	
	// Create workflow steps
	steps := h.createRemediationSteps(recipe, stakeholders, priority)
	
	workflow := HumanWorkflow{
		ID:           generateID(),
		Type:         "remediation_approval",
		Status:       WorkflowPending,
		Priority:     priority,
		Context:      context,
		Stakeholders: stakeholders,
		Steps:        steps,
		CurrentStep:  0,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		ExpiresAt:    time.Now().Add(h.calculateTimeout(priority)),
		Metadata: map[string]interface{}{
			"recipe_name": recipe.Name,
			"requester":   requesterID,
		},
	}
	
	if err := h.workflowStore.CreateWorkflow(workflow); err != nil {
		return nil, fmt.Errorf("failed to create workflow: %w", err)
	}
	
	// Log audit entry
	h.auditService.LogWorkflowAction(WorkflowAuditEntry{
		ID:         generateID(),
		WorkflowID: workflow.ID,
		Action:     "created",
		Actor:      requesterID,
		Timestamp:  time.Now(),
		Details: map[string]interface{}{
			"type":     workflow.Type,
			"priority": workflow.Priority,
		},
	})
	
	// Start the workflow
	if err := h.startWorkflow(ctx, &workflow); err != nil {
		return nil, fmt.Errorf("failed to start workflow: %w", err)
	}
	
	return &workflow, nil
}

// CreateSecurityReviewWorkflow creates a security review workflow
func (h *HumanWorkflowEngine) CreateSecurityReviewWorkflow(
	ctx context.Context,
	recipe RemediationRecipe,
	changedFiles []string,
	requesterID string,
) (*HumanWorkflow, error) {
	stakeholders := h.determineSecurityReviewers(recipe)
	priority := h.calculateReviewPriority(recipe)
	
	context := WorkflowContext{
		RecipeID:      recipe.ID,
		AffectedFiles: changedFiles,
		RiskLevel:     "medium", // Security reviews are generally medium risk
	}
	
	steps := h.createSecurityReviewSteps(recipe, stakeholders)
	
	workflow := HumanWorkflow{
		ID:           generateID(),
		Type:         "security_review",
		Status:       WorkflowPending,
		Priority:     priority,
		Context:      context,
		Stakeholders: stakeholders,
		Steps:        steps,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		ExpiresAt:    time.Now().Add(24 * time.Hour), // 24 hour timeout for reviews
	}
	
	if err := h.workflowStore.CreateWorkflow(workflow); err != nil {
		return nil, fmt.Errorf("failed to create workflow: %w", err)
	}
	
	return &workflow, nil
}

// ProcessWorkflowDecision processes a decision in a workflow
func (h *HumanWorkflowEngine) ProcessWorkflowDecision(
	ctx context.Context,
	workflowID string,
	decision Decision,
) error {
	workflow, err := h.workflowStore.GetWorkflow(workflowID)
	if err != nil {
		return fmt.Errorf("failed to get workflow: %w", err)
	}
	
	// Validate decision
	if err := h.validateDecision(*workflow, decision); err != nil {
		return fmt.Errorf("invalid decision: %w", err)
	}
	
	// Add decision to workflow
	workflow.Decisions = append(workflow.Decisions, decision)
	workflow.UpdatedAt = time.Now()
	
	// Update current step status
	if workflow.CurrentStep < len(workflow.Steps) {
		currentStep := &workflow.Steps[workflow.CurrentStep]
		
		switch decision.DecisionType {
		case DecisionApprove:
			currentStep.ReceivedApprovals++
			if currentStep.ReceivedApprovals >= currentStep.RequiredApprovals {
				currentStep.Status = StepCompleted
				currentStep.CompletedAt = &decision.MadeAt
			}
		case DecisionReject:
			workflow.Status = WorkflowRejected
			currentStep.Status = StepFailed
		case DecisionEscalate:
			workflow.EscalationLevel++
			workflow.Status = WorkflowEscalated
		}
	}
	
	// Check if workflow should advance to next step
	if err := h.advanceWorkflow(ctx, workflow); err != nil {
		return fmt.Errorf("failed to advance workflow: %w", err)
	}
	
	// Update workflow in store
	if err := h.workflowStore.UpdateWorkflow(*workflow); err != nil {
		return fmt.Errorf("failed to update workflow: %w", err)
	}
	
	// Log audit entry
	h.auditService.LogWorkflowAction(WorkflowAuditEntry{
		ID:         generateID(),
		WorkflowID: workflowID,
		Action:     "decision_made",
		Actor:      decision.MadeBy,
		Timestamp:  decision.MadeAt,
		Details: map[string]interface{}{
			"decision_type": decision.DecisionType,
			"step_id":       decision.StepID,
			"reasoning":     decision.Reasoning,
		},
	})
	
	return nil
}

// GetPendingWorkflows returns workflows pending for a user
func (h *HumanWorkflowEngine) GetPendingWorkflows(userID string) ([]HumanWorkflow, error) {
	filter := WorkflowFilter{
		Status:     []WorkflowStatus{WorkflowPending, WorkflowInProgress},
		AssignedTo: []string{userID},
	}
	
	workflows, err := h.workflowStore.ListWorkflows(filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list workflows: %w", err)
	}
	
	return workflows, nil
}

// Helper methods

func (h *HumanWorkflowEngine) determineStakeholders(vulns []VulnerabilityInfo, recipe RemediationRecipe) []Stakeholder {
	stakeholders := []Stakeholder{
		{
			ID:   "security-team",
			Name: "Security Team",
			Role: RoleSecurityOfficer,
			Permissions: []Permission{PermissionApprove, PermissionReject},
			Required: true,
		},
	}
	
	// Add architect for high-impact changes
	if h.isHighImpactChange(recipe) {
		stakeholders = append(stakeholders, Stakeholder{
			ID:   "tech-architect",
			Name: "Technical Architect", 
			Role: RoleArchitect,
			Permissions: []Permission{PermissionApprove, PermissionReject, PermissionReview},
			Required: true,
		})
	}
	
	// Add product owner for business-critical applications
	if h.isBusinessCritical(recipe) {
		stakeholders = append(stakeholders, Stakeholder{
			ID:   "product-owner",
			Name: "Product Owner",
			Role: RoleProductOwner,
			Permissions: []Permission{PermissionApprove, PermissionReject},
			Required: false,
		})
	}
	
	return stakeholders
}

func (h *HumanWorkflowEngine) calculatePriority(vulns []VulnerabilityInfo) Priority {
	maxSeverity := "low"
	for _, vuln := range vulns {
		if vuln.Severity == "critical" {
			return PriorityCritical
		}
		if vuln.Severity == "high" && maxSeverity != "critical" {
			maxSeverity = "high"
		}
		if vuln.Severity == "medium" && maxSeverity != "high" && maxSeverity != "critical" {
			maxSeverity = "medium"
		}
	}
	
	switch maxSeverity {
	case "critical":
		return PriorityCritical
	case "high":
		return PriorityHigh
	case "medium":
		return PriorityMedium
	default:
		return PriorityLow
	}
}

func (h *HumanWorkflowEngine) assessImpact(recipe RemediationRecipe, vulns []VulnerabilityInfo) ImpactAssessment {
	// Simplified impact assessment - in practice, this would be more sophisticated
	securityImpact := ImpactHigh // Remediation always has security impact
	
	performanceImpact := ImpactLow
	if len(recipe.Recipe.Operations) > 5 {
		performanceImpact = ImpactMedium
	}
	
	stabilityImpact := ImpactMedium
	if recipe.Recipe.Type == "code_transformation" {
		stabilityImpact = ImpactHigh
	}
	
	return ImpactAssessment{
		SecurityImpact:    securityImpact,
		PerformanceImpact: performanceImpact,
		StabilityImpact:   stabilityImpact,
		BusinessImpact:    ImpactMedium,
		UserImpact:        ImpactLow,
		ComplianceImpact:  ImpactMedium,
	}
}

func (h *HumanWorkflowEngine) createRemediationSteps(recipe RemediationRecipe, stakeholders []Stakeholder, priority Priority) []WorkflowStep {
	var steps []WorkflowStep
	
	// Step 1: Security approval
	steps = append(steps, WorkflowStep{
		ID:                "security-approval",
		Type:              StepApproval,
		Name:              "Security Team Approval",
		Description:       "Security team must approve the remediation approach",
		AssignedTo:        h.getStakeholderIDs(stakeholders, RoleSecurityOfficer),
		Status:            StepPending,
		RequiredApprovals: 1,
		Timeout:           h.getStepTimeout(priority),
	})
	
	// Step 2: Architecture review (for high-impact changes)
	if h.isHighImpactChange(recipe) {
		steps = append(steps, WorkflowStep{
			ID:                "architecture-review",
			Type:              StepReview,
			Name:              "Architecture Review",
			Description:       "Technical architecture review of the remediation",
			AssignedTo:        h.getStakeholderIDs(stakeholders, RoleArchitect),
			Status:            StepPending,
			RequiredApprovals: 1,
			Timeout:           h.getStepTimeout(priority),
			Dependencies:      []string{"security-approval"},
		})
	}
	
	// Step 3: Final approval
	steps = append(steps, WorkflowStep{
		ID:                "final-approval",
		Type:              StepApproval,
		Name:              "Final Approval",
		Description:       "Final approval to proceed with remediation",
		AssignedTo:        h.getStakeholderIDs(stakeholders, RoleApprover),
		Status:            StepPending,
		RequiredApprovals: 1,
		Timeout:           h.getStepTimeout(priority),
	})
	
	return steps
}

func (h *HumanWorkflowEngine) startWorkflow(ctx context.Context, workflow *HumanWorkflow) error {
	if len(workflow.Steps) == 0 {
		return fmt.Errorf("workflow has no steps")
	}
	
	// Start first step
	currentStep := &workflow.Steps[0]
	currentStep.Status = StepInProgress
	now := time.Now()
	currentStep.StartedAt = &now
	
	workflow.Status = WorkflowInProgress
	workflow.UpdatedAt = now
	
	// Send notifications for first step
	return h.sendStepNotifications(ctx, *workflow, *currentStep)
}

func (h *HumanWorkflowEngine) advanceWorkflow(ctx context.Context, workflow *HumanWorkflow) error {
	if workflow.CurrentStep >= len(workflow.Steps) {
		return nil // Already at end
	}
	
	currentStep := workflow.Steps[workflow.CurrentStep]
	
	// Check if current step is completed
	if currentStep.Status == StepCompleted {
		// Move to next step
		workflow.CurrentStep++
		
		if workflow.CurrentStep >= len(workflow.Steps) {
			// Workflow completed
			workflow.Status = WorkflowCompleted
			now := time.Now()
			workflow.CompletedAt = &now
			
			// Send completion notification
			return h.notificationService.SendCompletionNotification(*workflow)
		} else {
			// Start next step
			nextStep := &workflow.Steps[workflow.CurrentStep]
			nextStep.Status = StepInProgress
			now := time.Now()
			nextStep.StartedAt = &now
			
			// Send notifications for next step
			return h.sendStepNotifications(ctx, *workflow, *nextStep)
		}
	}
	
	return nil
}

func (h *HumanWorkflowEngine) sendStepNotifications(ctx context.Context, workflow HumanWorkflow, step WorkflowStep) error {
	// Send notifications to assigned stakeholders
	for _, stakeholderID := range step.AssignedTo {
		// Find stakeholder details
		var stakeholder *Stakeholder
		for _, s := range workflow.Stakeholders {
			if s.ID == stakeholderID {
				stakeholder = &s
				break
			}
		}
		
		if stakeholder == nil {
			continue
		}
		
		// Create appropriate notification based on step type
		switch step.Type {
		case StepApproval:
			approvalWorkflow := ApprovalWorkflow{
				ID:     workflow.ID,
				Status: WorkflowInProgress,
				Request: ApprovalRequest{
					WorkflowID:  workflow.ID,
					Title:       fmt.Sprintf("Approval Required: %s", step.Name),
					Description: step.Description,
					Priority:    workflow.Priority,
					Context:     workflow.Context,
				},
			}
			if err := h.notificationService.SendApprovalNotification(approvalWorkflow); err != nil {
				return err
			}
		case StepReview:
			reviewWorkflow := ReviewWorkflow{
				ID:     workflow.ID,
				Status: WorkflowInProgress,
				Request: ReviewRequest{
					WorkflowID:  workflow.ID,
					Title:       fmt.Sprintf("Review Required: %s", step.Name),
					Description: step.Description,
					Priority:    workflow.Priority,
					ReviewType:  ReviewSecurity,
					Context:     workflow.Context,
				},
			}
			if err := h.notificationService.SendReviewNotification(reviewWorkflow); err != nil {
				return err
			}
		}
	}
	
	return nil
}

func (h *HumanWorkflowEngine) validateDecision(workflow HumanWorkflow, decision Decision) error {
	if workflow.Status != WorkflowInProgress && workflow.Status != WorkflowPending {
		return fmt.Errorf("cannot make decision on workflow with status: %s", workflow.Status)
	}
	
	if workflow.CurrentStep >= len(workflow.Steps) {
		return fmt.Errorf("workflow has no active step")
	}
	
	currentStep := workflow.Steps[workflow.CurrentStep]
	if decision.StepID != currentStep.ID {
		return fmt.Errorf("decision step ID does not match current step")
	}
	
	// Check if decision maker is authorized for this step
	authorized := false
	for _, assignedTo := range currentStep.AssignedTo {
		if assignedTo == decision.MadeBy {
			authorized = true
			break
		}
	}
	
	if !authorized {
		return fmt.Errorf("decision maker not authorized for this step")
	}
	
	return nil
}

// Utility helper methods
func (h *HumanWorkflowEngine) determineRiskLevel(vulns []VulnerabilityInfo) string {
	for _, vuln := range vulns {
		if vuln.Severity == "critical" {
			return "critical"
		}
		if vuln.Severity == "high" {
			return "high"
		}
	}
	return "medium"
}

func (h *HumanWorkflowEngine) extractBusinessContext(recipe RemediationRecipe) BusinessContext {
	// This would extract business context from recipe metadata
	return BusinessContext{
		ApplicationName: "Unknown",
		Environment:     "production",
		ServiceTier:     "tier2",
	}
}

func (h *HumanWorkflowEngine) extractTechnicalContext(recipe RemediationRecipe) TechnicalContext {
	// This would extract technical context from recipe metadata
	return TechnicalContext{
		Language:     "java",
		Framework:    "spring",
		Architecture: "microservices",
	}
}

func (h *HumanWorkflowEngine) calculateTimeout(priority Priority) time.Duration {
	switch priority {
	case PriorityCritical:
		return 4 * time.Hour
	case PriorityHigh:
		return 8 * time.Hour
	case PriorityMedium:
		return 24 * time.Hour
	default:
		return 48 * time.Hour
	}
}

func (h *HumanWorkflowEngine) isHighImpactChange(recipe RemediationRecipe) bool {
	return recipe.Recipe.Type == "code_transformation" || len(recipe.Recipe.Operations) > 3
}

func (h *HumanWorkflowEngine) isBusinessCritical(recipe RemediationRecipe) bool {
	// This would check business criticality from metadata
	return false
}

func (h *HumanWorkflowEngine) determineSecurityReviewers(recipe RemediationRecipe) []Stakeholder {
	return []Stakeholder{
		{
			ID:   "security-reviewer",
			Name: "Security Reviewer",
			Role: RoleReviewer,
			Permissions: []Permission{PermissionReview, PermissionComment},
			Required: true,
		},
	}
}

func (h *HumanWorkflowEngine) calculateReviewPriority(recipe RemediationRecipe) Priority {
	if recipe.Recipe.Type == "code_transformation" {
		return PriorityHigh
	}
	return PriorityMedium
}

func (h *HumanWorkflowEngine) createSecurityReviewSteps(recipe RemediationRecipe, stakeholders []Stakeholder) []WorkflowStep {
	return []WorkflowStep{
		{
			ID:                "security-review",
			Type:              StepReview,
			Name:              "Security Review",
			Description:       "Review remediation for security implications",
			AssignedTo:        h.getStakeholderIDs(stakeholders, RoleReviewer),
			Status:            StepPending,
			RequiredApprovals: 1,
			Timeout:           24 * time.Hour,
		},
	}
}

func (h *HumanWorkflowEngine) getStakeholderIDs(stakeholders []Stakeholder, role StakeholderRole) []string {
	var ids []string
	for _, s := range stakeholders {
		if s.Role == role {
			ids = append(ids, s.ID)
		}
	}
	return ids
}

func (h *HumanWorkflowEngine) getStepTimeout(priority Priority) time.Duration {
	switch priority {
	case PriorityCritical:
		return 2 * time.Hour
	case PriorityHigh:
		return 4 * time.Hour
	case PriorityMedium:
		return 8 * time.Hour
	default:
		return 24 * time.Hour
	}
}