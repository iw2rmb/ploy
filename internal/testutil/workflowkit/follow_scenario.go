package workflowkit

import domaintypes "github.com/iw2rmb/ploy/internal/domain/types"

// FollowStreamScenario provides consistent IDs for follow-stream engine tests
// spanning cli/follow and related job-rendering modules.
// Use NewFollowStreamScenario to construct one with generated IDs.
type FollowStreamScenario struct {
	RunID     domaintypes.RunID
	MigRepoID domaintypes.MigRepoID
	JobID     domaintypes.JobID
}

// NewFollowStreamScenario returns a scenario with generated IDs.
func NewFollowStreamScenario() FollowStreamScenario {
	return FollowStreamScenario{
		RunID:     domaintypes.NewRunID(),
		MigRepoID: domaintypes.NewMigRepoID(),
		JobID:     domaintypes.NewJobID(),
	}
}
