package mods

// ProductionJobSubmitter defines the interface for production job submission
type ProductionJobSubmitter interface {
	RenderPlannerAssets() (*PlannerAssets, error)
	RenderReducerAssets() (*ReducerAssets, error)
	GetHCLSubmitter() HCLSubmitter
}

// jobSubmissionHelper implements the JobSubmissionHelper interface
type jobSubmissionHelper struct {
	submitter JobSubmitter           // Concrete job submitter (mock in tests, real in prod)
	runner    ProductionJobSubmitter // For accessing asset rendering methods in production
}

// NewJobSubmissionHelper creates a new job submission helper
func NewJobSubmissionHelper(submitter JobSubmitter) JobSubmissionHelper {
	return &jobSubmissionHelper{submitter: submitter}
}

// NewJobSubmissionHelperWithRunner creates a new job submission helper with runner access for production
func NewJobSubmissionHelperWithRunner(submitter JobSubmitter, runner ProductionJobSubmitter) JobSubmissionHelper {
	return &jobSubmissionHelper{submitter: submitter, runner: runner}
}
