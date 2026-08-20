package lifecycle

import (
	"context"

	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
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
