package workflowkit

import domaintypes "github.com/iw2rmb/ploy/internal/domain/types"

// RunOrchestrationScenario provides consistent domain IDs for recovery and
// orchestration scenarios spanning server/recovery and nodeagent modules.
// Use NewRunOrchestrationScenario to construct one with generated IDs.
type RunOrchestrationScenario struct {
	RunID  domaintypes.RunID
	RepoID domaintypes.RepoID
	JobID  domaintypes.JobID
}

// NewRunOrchestrationScenario returns a scenario with generated IDs.
func NewRunOrchestrationScenario() RunOrchestrationScenario {
	return RunOrchestrationScenario{
		RunID:  domaintypes.NewRunID(),
		RepoID: domaintypes.NewRepoID(),
		JobID:  domaintypes.NewJobID(),
	}
}
