package arf

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