package handlers

import domaintypes "github.com/iw2rmb/ploy/internal/domain/types"

type jobBuildStatusProjection struct {
	Status   string
	Terminal bool
	Success  bool
}

func projectJobBuildStatus(status domaintypes.JobStatus) jobBuildStatusProjection {
	projection := jobBuildStatusProjection{
		Status: string(status),
	}
	switch status {
	case domaintypes.JobStatusSuccess:
		projection.Terminal = true
		projection.Success = true
	case domaintypes.JobStatusFail, domaintypes.JobStatusError, domaintypes.JobStatusCancelled:
		projection.Terminal = true
	}
	return projection
}
