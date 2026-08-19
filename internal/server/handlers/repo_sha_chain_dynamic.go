package handlers

import domaintypes "github.com/iw2rmb/ploy/internal/domain/types"

func normalizeRepoSHA(sha string) string {
	normalized, ok := domaintypes.NormalizeFullCommitSHA(sha)
	if !ok {
		return ""
	}
	return normalized.String()
}

func isNonChangingJob(jobType domaintypes.JobType) bool {
	switch jobType {
	case domaintypes.JobTypePreGate, domaintypes.JobTypePostGate:
		return true
	default:
		return false
	}
}
