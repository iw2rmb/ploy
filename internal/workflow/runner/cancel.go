package runner

// CancelRequest describes the information required to cancel a workflow run.
type CancelRequest struct {
	WorkflowID string
	RunID      string
	Reason     string
}

// CancelResult reports the observed state after issuing a cancellation request.
type CancelResult struct {
	RunID     string
	Status    StageStatus
	Requested bool
}
