package lifecycle

import (
	"context"

	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// Collector gathers the capacity data sent in node heartbeats.
type Collector struct {
	loadFunc      func(context.Context) (*load.AvgStat, error)
	memFunc       func(context.Context) (*mem.VirtualMemoryStat, error)
	diskUsageFunc func(context.Context, string) (*disk.UsageStat, error)
}

// NewCollector constructs a capacity collector.
func NewCollector() *Collector {
	return &Collector{
		loadFunc:      load.AvgWithContext,
		memFunc:       mem.VirtualMemoryWithContext,
		diskUsageFunc: disk.UsageWithContext,
	}
}

// CollectCapacity builds the resource values consumed by heartbeat reporting.
func (c *Collector) CollectCapacity(ctx context.Context) NodeCapacity {
	resources := c.collectResources(ctx)
	return NodeCapacity{
		CPUFreeMillis:  domaintypes.CPUmilli(resources.CPUFreeMillis),
		CPUTotalMillis: domaintypes.CPUmilli(resources.CPUTotalMillis),
		MemFreeBytes:   domaintypes.Bytes(resources.MemoryFreeBytes),
		MemTotalBytes:  domaintypes.Bytes(resources.MemoryTotalBytes),
		DiskFreeBytes:  domaintypes.Bytes(resources.DiskFreeBytes),
		DiskTotalBytes: domaintypes.Bytes(resources.DiskTotalBytes),
	}
}
