package arf

import (
	"context"
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

// startWorkflow initializes and starts a workflow
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

// advanceWorkflow moves the workflow to the next step if appropriate
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

// sendStepNotifications sends notifications for a workflow step
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

// validateDecision validates a decision can be applied to the workflow
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