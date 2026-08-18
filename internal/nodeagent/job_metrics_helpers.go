package nodeagent

import (
	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/step"
)

func runStatsJobResourcesFromStepUsage(usage *step.ContainerResourceUsage) *types.RunStatsJobResources {
	if usage == nil {
		return nil
	}
	return &types.RunStatsJobResources{
		CPUConsumedNs:     nonNegativeInt64(usage.CPUConsumedNs),
		DiskConsumedBytes: nonNegativeInt64(usage.DiskConsumedBytes),
		MemConsumedBytes:  nonNegativeInt64(usage.MemConsumedBytes),
	}
}

func nonNegativeInt64(v int64) int64 {
	if v < 0 {
		return 0
	}
	return v
}
