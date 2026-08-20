package lifecycle

import (
	"context"
	"math"
	"runtime"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// cpuFreeMillis calculates free CPU millis from load average and total capacity.
// Clamps result to [0, totalMillis] range.
func cpuFreeMillis(load1 float64, totalMillis int64) int32 {
	usedMillis := int64(math.Round(load1 * 1000))
	freeMillis := totalMillis - usedMillis
	if freeMillis < 0 {
		freeMillis = 0
	}
	if freeMillis > totalMillis {
		freeMillis = totalMillis
	}
	return int32(freeMillis)
}

// CollectCapacity builds the resource values consumed by heartbeat reporting.
func (c *Collector) CollectCapacity(ctx context.Context) NodeCapacity {
	var capacity NodeCapacity

	totalMillis := int64(runtime.NumCPU()) * 1000
	if totalMillis > math.MaxInt32 {
		capacity.CPUTotalMillis = domaintypes.CPUmilli(math.MaxInt32)
	} else {
		capacity.CPUTotalMillis = domaintypes.CPUmilli(totalMillis)
	}
	totalMillis = int64(capacity.CPUTotalMillis)

	if avg, err := c.loadFunc(ctx); err == nil {
		capacity.CPUFreeMillis = domaintypes.CPUmilli(cpuFreeMillis(avg.Load1, totalMillis))
	} else {
		capacity.CPUFreeMillis = capacity.CPUTotalMillis
	}

	if vm, err := c.memFunc(ctx); err == nil {
		capacity.MemTotalBytes = domaintypes.Bytes(uint64ToInt64(vm.Total))
		capacity.MemFreeBytes = domaintypes.Bytes(uint64ToInt64(vm.Available))
	}

	if usage, err := c.diskUsageFunc(ctx, "/"); err == nil {
		capacity.DiskTotalBytes = domaintypes.Bytes(uint64ToInt64(usage.Total))
		capacity.DiskFreeBytes = domaintypes.Bytes(uint64ToInt64(usage.Free))
	}

	return capacity
}

func uint64ToInt64(value uint64) int64 {
	if value > uint64(math.MaxInt64) {
		return math.MaxInt64
	}
	return int64(value)
}
