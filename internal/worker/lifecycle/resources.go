package lifecycle

import (
	"context"
	"math"
	"runtime"
)

type resourceSnapshot struct {
	CPUTotalMillis   int32
	CPUFreeMillis    int32
	MemoryTotalBytes int64
	MemoryFreeBytes  int64
	DiskTotalBytes   int64
	DiskFreeBytes    int64
}

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

func (c *Collector) collectResources(ctx context.Context) resourceSnapshot {
	var snapshot resourceSnapshot

	totalMillis := int64(runtime.NumCPU()) * 1000
	if totalMillis > math.MaxInt32 {
		snapshot.CPUTotalMillis = math.MaxInt32
	} else {
		snapshot.CPUTotalMillis = int32(totalMillis)
	}
	totalMillis = int64(snapshot.CPUTotalMillis)

	if avg, err := c.loadFunc(ctx); err == nil {
		snapshot.CPUFreeMillis = cpuFreeMillis(avg.Load1, totalMillis)
	} else {
		snapshot.CPUFreeMillis = snapshot.CPUTotalMillis
	}

	if vm, err := c.memFunc(ctx); err == nil {
		snapshot.MemoryTotalBytes = uint64ToInt64(vm.Total)
		snapshot.MemoryFreeBytes = uint64ToInt64(vm.Available)
	}

	if usage, err := c.diskUsageFunc(ctx, "/"); err == nil {
		snapshot.DiskTotalBytes = uint64ToInt64(usage.Total)
		snapshot.DiskFreeBytes = uint64ToInt64(usage.Free)
	}

	return snapshot
}

func uint64ToInt64(value uint64) int64 {
	if value > uint64(math.MaxInt64) {
		return math.MaxInt64
	}
	return int64(value)
}
