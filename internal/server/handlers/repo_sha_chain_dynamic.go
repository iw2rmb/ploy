package handlers

import (
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func normalizeRepoSHA(sha string) string {
	return strings.TrimSpace(strings.ToLower(sha))
}

func isNonChangingJob(jobType domaintypes.JobType) bool {
	switch jobType {
	case domaintypes.JobTypePreGate, domaintypes.JobTypePostGate:
		return true
	default:
		return false
	}
}
