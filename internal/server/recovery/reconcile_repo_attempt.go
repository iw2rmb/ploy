package recovery

import (
	"encoding/json"
	"fmt"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// RepoAttemptReconcileEval is the pure evaluation result for repo-attempt completion.
type RepoAttemptReconcileEval struct {
	ShouldUpdate bool
	Status       domaintypes.RunStatus
	LastJob      *store.Job
}

// EvaluateRepoAttemptTerminalStatus determines whether runs.status can be finalized.
func EvaluateRepoAttemptTerminalStatus(jobs []store.Job) (RepoAttemptReconcileEval, error) {
	var (
		byNextIDMeta  *store.Job
		maxNextIDMeta int64
		bestTail      *store.Job
		bestFallback  *store.Job
	)
	for i := range jobs {
		job := &jobs[i]

		switch job.Status {
		case domaintypes.JobStatusSuccess, domaintypes.JobStatusFail, domaintypes.JobStatusError, domaintypes.JobStatusCancelled:
		default:
			return RepoAttemptReconcileEval{ShouldUpdate: false}, nil
		}

		if bestFallback == nil || job.ID.String() > bestFallback.ID.String() {
			bestFallback = job
		}
		if nextID, ok := nextIDFromMeta(job.Meta); ok {
			if byNextIDMeta == nil || nextID > maxNextIDMeta || (nextID == maxNextIDMeta && job.ID.String() > byNextIDMeta.ID.String()) {
				maxNextIDMeta = nextID
				byNextIDMeta = job
			}
		}
		if job.NextID == nil || job.NextID.IsZero() {
			if bestTail == nil || job.ID.String() > bestTail.ID.String() {
				bestTail = job
			}
		}
	}

	if bestFallback == nil {
		return RepoAttemptReconcileEval{ShouldUpdate: false}, nil
	}

	lastJob := bestFallback
	if bestTail != nil {
		lastJob = bestTail
	}
	if byNextIDMeta != nil {
		lastJob = byNextIDMeta
	}

	var runStatus domaintypes.RunStatus
	switch lastJob.Status {
	case domaintypes.JobStatusSuccess:
		runStatus = domaintypes.RunStatusSuccess
	case domaintypes.JobStatusFail, domaintypes.JobStatusError:
		runStatus = domaintypes.RunStatusFail
	case domaintypes.JobStatusCancelled:
		runStatus = domaintypes.RunStatusCancelled
	default:
		return RepoAttemptReconcileEval{}, fmt.Errorf("unexpected last job status %q for job_id=%s", lastJob.Status, lastJob.ID)
	}

	return RepoAttemptReconcileEval{
		ShouldUpdate: true,
		Status:       runStatus,
		LastJob:      lastJob,
	}, nil
}

func nextIDFromMeta(meta []byte) (int64, bool) {
	if len(meta) == 0 {
		return 0, false
	}
	var m struct {
		NextID *int64 `json:"next_id"`
	}
	if err := json.Unmarshal(meta, &m); err != nil || m.NextID == nil {
		return 0, false
	}
	return *m.NextID, true
}
