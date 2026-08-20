package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestCollectCapacityUsesResourceProbes(t *testing.T) {
	collector := NewCollector()
	collector.loadFunc = func(context.Context) (*load.AvgStat, error) {
		return &load.AvgStat{Load1: 1.5}, nil
	}
	collector.memFunc = func(context.Context) (*mem.VirtualMemoryStat, error) {
		return &mem.VirtualMemoryStat{Total: 8192, Available: 4096}, nil
	}
	collector.diskUsageFunc = func(context.Context, string) (*disk.UsageStat, error) {
		return &disk.UsageStat{Total: 16384, Free: 12288}, nil
	}

	capacity := collector.CollectCapacity(context.Background())
	wantFreeCPU := cpuFreeMillis(1.5, int64(capacity.CPUTotalMillis))
	if capacity.CPUFreeMillis != domaintypes.CPUmilli(wantFreeCPU) {
		t.Fatalf("CPUFreeMillis = %d, want %d", capacity.CPUFreeMillis, wantFreeCPU)
	}
	if capacity.MemTotalBytes != 8192 || capacity.MemFreeBytes != 4096 {
		t.Fatalf("memory capacity = %d/%d, want 4096/8192", capacity.MemFreeBytes, capacity.MemTotalBytes)
	}
	if capacity.DiskTotalBytes != 16384 || capacity.DiskFreeBytes != 12288 {
		t.Fatalf("disk capacity = %d/%d, want 12288/16384", capacity.DiskFreeBytes, capacity.DiskTotalBytes)
	}
}

func TestCollectCapacityKeepsPartialValuesWhenProbesFail(t *testing.T) {
	probeErr := errors.New("probe failed")
	collector := NewCollector()
	collector.loadFunc = func(context.Context) (*load.AvgStat, error) { return nil, probeErr }
	collector.memFunc = func(context.Context) (*mem.VirtualMemoryStat, error) { return nil, probeErr }
	collector.diskUsageFunc = func(context.Context, string) (*disk.UsageStat, error) { return nil, probeErr }

	capacity := collector.CollectCapacity(context.Background())
	if capacity.CPUFreeMillis != capacity.CPUTotalMillis {
		t.Fatalf("CPUFreeMillis = %d, want total %d", capacity.CPUFreeMillis, capacity.CPUTotalMillis)
	}
	if capacity.MemFreeBytes != 0 || capacity.MemTotalBytes != 0 || capacity.DiskFreeBytes != 0 || capacity.DiskTotalBytes != 0 {
		t.Fatalf("failed probes produced non-zero capacity: %+v", capacity)
	}
}
