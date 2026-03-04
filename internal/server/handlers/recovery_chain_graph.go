package handlers

import (
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

func recoveryChainPredecessor(jobID domaintypes.JobID, jobsByID map[domaintypes.JobID]store.Job) *store.Job {
	for _, candidate := range jobsByID {
		if candidate.NextID != nil && *candidate.NextID == jobID {
			c := candidate
			return &c
		}
	}
	return nil
}
