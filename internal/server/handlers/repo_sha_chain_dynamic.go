package handlers

import (
	"context"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

func normalizeRepoSHA(sha string) string {
	return strings.TrimSpace(strings.ToLower(sha))
}

func canChangeWorkspace(jobType domaintypes.JobType) bool {
	switch jobType {
	case domaintypes.JobTypeMig:
		return true
	default:
		return false
	}
}

func isNonChangingJob(jobType domaintypes.JobType) bool {
	switch jobType {
	case domaintypes.JobTypeSBOM, domaintypes.JobTypePreGate, domaintypes.JobTypePostGate:
		return true
	default:
		return false
	}
}

func effectiveCompletedRepoSHAOut(job store.Job, completionRepoSHAOut string) string {
	candidate := normalizeRepoSHA(completionRepoSHAOut)
	if sha40Pattern.MatchString(candidate) {
		return candidate
	}
	out := normalizeRepoSHA(job.RepoShaOut)
	if sha40Pattern.MatchString(out) {
		return out
	}
	if isNonChangingJob(domaintypes.JobType(job.JobType)) {
		in := normalizeRepoSHA(job.RepoShaIn)
		if sha40Pattern.MatchString(in) {
			return in
		}
	}
	return ""
}

func applyInsertedHeadRepoSHA(
	ctx context.Context,
	st store.Store,
	head store.Job,
	predecessorSHAOut string,
) error {
	sha := normalizeRepoSHA(predecessorSHAOut)
	if sha40Pattern.MatchString(sha) {
		if err := st.UpdateJobRepoSHAIn(ctx, store.UpdateJobRepoSHAInParams{
			ID:        head.ID,
			RepoShaIn: sha,
		}); err != nil {
			return err
		}
	}
	if canChangeWorkspace(domaintypes.JobType(head.JobType)) && head.NextID != nil {
		if _, err := st.ClearRepoSHAChainFromJob(ctx, store.ClearRepoSHAChainFromJobParams{
			ID:      *head.NextID,
			RunID:   head.RunID,
			RepoID:  head.RepoID,
			Attempt: head.Attempt,
		}); err != nil {
			return err
		}
	}
	return nil
}
