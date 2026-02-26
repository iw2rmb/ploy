package nodeagent

import (
	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
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

func runStatsJobResourcesFromGateUsage(usage *contracts.BuildGateResourceUsage) *types.RunStatsJobResources {
	if usage == nil {
		return nil
	}
	memConsumed := usage.MemMaxBytes
	if memConsumed == 0 {
		memConsumed = usage.MemUsageBytes
	}
	var diskConsumed int64
	if usage.SizeRwBytes != nil && *usage.SizeRwBytes > 0 {
		diskConsumed = *usage.SizeRwBytes
	}
	return &types.RunStatsJobResources{
		CPUConsumedNs:     saturatingInt64FromUint64(usage.CPUTotalNs),
		DiskConsumedBytes: diskConsumed,
		MemConsumedBytes:  saturatingInt64FromUint64(memConsumed),
	}
}

func nonNegativeInt64(v int64) int64 {
	if v < 0 {
		return 0
	}
	return v
}

func saturatingInt64FromUint64(v uint64) int64 {
	const maxInt64 = ^uint64(0) >> 1
	if v > maxInt64 {
		return int64(maxInt64)
	}
	return int64(v)
}
