package lifecycle

import (
	"context"
	"errors"
	"math"
	"path"
	"runtime"
	"strings"

	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
)

type networkInterfaceSnapshot struct {
	RXBytesPerSec   float64
	TXBytesPerSec   float64
	RXPacketsPerSec float64
	TXPacketsPerSec float64
	InitialSample   bool
}

type diskIOMetrics struct {
	ReadMBps  float64
	WriteMBps float64
	ReadIOPS  float64
	WriteIOPS float64
	Initial   bool
}

type networkMetrics struct {
	RXBytesPerSec   float64
	TXBytesPerSec   float64
	RXPacketsPerSec float64
	TXPacketsPerSec float64
	Interfaces      map[string]networkInterfaceSnapshot
	Initial         bool
}

type resourceSnapshot struct {
	CPUTotalMillis       int32
	CPUFreeMillis        int32
	CPULoad1             float64
	MemoryTotalBytes     int64
	MemoryFreeBytes      int64
	DiskTotalBytes       int64
	DiskFreeBytes        int64
	DiskReadMBps         float64
	DiskWriteMBps        float64
	DiskReadIOPS         float64
	DiskWriteIOPS        float64
	DiskInitialSample    bool
	NetworkRxBps         float64
	NetworkTxBps         float64
	NetworkRxPps         float64
	NetworkTxPps         float64
	NetworkInitialSample bool
	NetworkInterfaces    map[string]networkInterfaceSnapshot
}

// toNodeResources converts the internal resourceSnapshot to the typed NodeResources struct.
// This eliminates map[string]any casts by providing a strongly-typed representation.
func (r resourceSnapshot) toNodeResources() NodeResources {
	// Convert network interfaces from internal snapshot format to typed format.
	interfaces := make(map[string]NetworkInterface, len(r.NetworkInterfaces))
	for name, iface := range r.NetworkInterfaces {
		interfaces[name] = NetworkInterface(iface)
	}

	return NodeResources{
		CPU: CPUResources{
			TotalMCores: float64(r.CPUTotalMillis),
			FreeMCores:  float64(r.CPUFreeMillis),
			Load1:       r.CPULoad1,
		},
		Memory: MemoryResources{
			TotalMB: bytesToMBInt64(r.MemoryTotalBytes),
			FreeMB:  bytesToMBInt64(r.MemoryFreeBytes),
		},
		Disk: DiskResources{
			TotalMB: bytesToMBInt64(r.DiskTotalBytes),
			FreeMB:  bytesToMBInt64(r.DiskFreeBytes),
			IO: DiskIO{
				ReadMBPerSec:  r.DiskReadMBps,
				WriteMBPerSec: r.DiskWriteMBps,
				ReadIOPS:      r.DiskReadIOPS,
				WriteIOPS:     r.DiskWriteIOPS,
				InitialSample: r.DiskInitialSample,
			},
		},
		Network: NetworkResources{
			RXBytesPerSec:   r.NetworkRxBps,
			TXBytesPerSec:   r.NetworkTxBps,
			RXPacketsPerSec: r.NetworkRxPps,
			TXPacketsPerSec: r.NetworkTxPps,
			InitialSample:   r.NetworkInitialSample,
			Interfaces:      interfaces,
		},
	}
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

func (c *Collector) collectResources(ctx context.Context) (resourceSnapshot, error) {
	if c.resourcesFunc != nil {
		return c.resourcesFunc(ctx)
	}
	var snapshot resourceSnapshot
	var errs []string

	totalCores := runtime.NumCPU()
	totalMillis64 := int64(totalCores) * 1000
	if totalMillis64 > math.MaxInt32 {
		snapshot.CPUTotalMillis = math.MaxInt32
		errs = append(errs, "cpu:total out of range")
	} else {
		snapshot.CPUTotalMillis = int32(totalMillis64)
	}
	totalMillis64 = int64(snapshot.CPUTotalMillis)

	if c.loadFunc != nil {
		if avg, err := c.loadFunc(ctx); err == nil {
			snapshot.CPULoad1 = avg.Load1
			snapshot.CPUFreeMillis = cpuFreeMillis(avg.Load1, totalMillis64)
		} else if !errors.Is(err, context.Canceled) {
			snapshot.CPUFreeMillis = snapshot.CPUTotalMillis
			errs = append(errs, "load:"+err.Error())
		} else {
			snapshot.CPUFreeMillis = snapshot.CPUTotalMillis
		}
	} else if avg, err := load.AvgWithContext(ctx); err == nil {
		snapshot.CPULoad1 = avg.Load1
		snapshot.CPUFreeMillis = cpuFreeMillis(avg.Load1, totalMillis64)
	} else if !errors.Is(err, context.Canceled) {
		snapshot.CPUFreeMillis = snapshot.CPUTotalMillis
		errs = append(errs, "load:"+err.Error())
	} else {
		snapshot.CPUFreeMillis = snapshot.CPUTotalMillis
	}

	if c.memFunc != nil {
		if vm, err := c.memFunc(ctx); err == nil {
			snapshot.MemoryTotalBytes = uint64ToInt64(vm.Total)
			snapshot.MemoryFreeBytes = uint64ToInt64(vm.Available)
		} else if !errors.Is(err, context.Canceled) {
			errs = append(errs, "memory:"+err.Error())
		}
	} else if vm, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		snapshot.MemoryTotalBytes = uint64ToInt64(vm.Total)
		snapshot.MemoryFreeBytes = uint64ToInt64(vm.Available)
	} else {
		errs = append(errs, "memory:"+err.Error())
	}

	if c.diskUsageFunc != nil {
		if du, err := c.diskUsageFunc(ctx, "/"); err == nil {
			snapshot.DiskTotalBytes = uint64ToInt64(du.Total)
			snapshot.DiskFreeBytes = uint64ToInt64(du.Free)
		} else if !errors.Is(err, context.Canceled) {
			errs = append(errs, "disk:"+err.Error())
		}
	} else if du, err := disk.UsageWithContext(ctx, "/"); err == nil {
		snapshot.DiskTotalBytes = uint64ToInt64(du.Total)
		snapshot.DiskFreeBytes = uint64ToInt64(du.Free)
	} else if !errors.Is(err, context.Canceled) {
		errs = append(errs, "disk:"+err.Error())
	}

	if diskIO, netIO, err := c.collectIOMetrics(ctx); err == nil {
		snapshot.DiskReadMBps = diskIO.ReadMBps
		snapshot.DiskWriteMBps = diskIO.WriteMBps
		snapshot.DiskReadIOPS = diskIO.ReadIOPS
		snapshot.DiskWriteIOPS = diskIO.WriteIOPS
		snapshot.DiskInitialSample = diskIO.Initial
		snapshot.NetworkRxBps = netIO.RXBytesPerSec
		snapshot.NetworkTxBps = netIO.TXBytesPerSec
		snapshot.NetworkRxPps = netIO.RXPacketsPerSec
		snapshot.NetworkTxPps = netIO.TXPacketsPerSec
		snapshot.NetworkInitialSample = netIO.Initial
		if len(netIO.Interfaces) > 0 {
			snapshot.NetworkInterfaces = netIO.Interfaces
		}
	} else if !errors.Is(err, context.Canceled) {
		errs = append(errs, err.Error())
	}

	if len(errs) > 0 {
		return snapshot, errors.New(strings.Join(errs, "; "))
	}
	return snapshot, nil
}

func (c *Collector) collectIOMetrics(ctx context.Context) (diskIOMetrics, networkMetrics, error) {
	if c.metrics == nil {
		c.metrics = newMetricsCache()
	}
	var (
		errs      []string
		diskStats map[string]disk.IOCountersStat
		netStats  []net.IOCountersStat
	)

	if c.diskCountersFunc != nil {
		stats, err := c.diskCountersFunc(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			errs = append(errs, "disk_io:"+err.Error())
		} else {
			diskStats = stats
		}
	}
	if c.netCountersFunc != nil {
		stats, err := c.netCountersFunc(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			errs = append(errs, "net_io:"+err.Error())
		} else {
			netStats = stats
		}
	}

	filteredNet := c.filterInterfaces(netStats)
	diskMetrics, networkMetrics := c.metrics.sample(c.now(), diskStats, filteredNet)

	if len(errs) > 0 {
		return diskMetrics, networkMetrics, errors.New(strings.Join(errs, "; "))
	}
	return diskMetrics, networkMetrics, nil
}

func bytesToMB(value uint64) float64 {
	const mb = 1024 * 1024
	return float64(value) / mb
}

func bytesToMBInt64(value int64) float64 {
	if value <= 0 {
		return 0
	}
	return bytesToMB(uint64(value))
}

func uint64ToInt64(value uint64) int64 {
	if value > uint64(math.MaxInt64) {
		return math.MaxInt64
	}
	return int64(value)
}

func (c *Collector) filterInterfaces(stats []net.IOCountersStat) []net.IOCountersStat {
	if len(stats) == 0 {
		return nil
	}
	out := make([]net.IOCountersStat, 0, len(stats))
	for _, stat := range stats {
		name := strings.TrimSpace(stat.Name)
		if name == "" {
			continue
		}
		lower := strings.ToLower(name)
		if strings.HasPrefix(lower, "lo") {
			continue
		}
		if shouldIgnoreInterface(lower, c.ignoreInterfaces) {
			continue
		}
		out = append(out, stat)
	}
	return out
}

func netSliceToMap(stats []net.IOCountersStat) map[string]net.IOCountersStat {
	if len(stats) == 0 {
		return nil
	}
	dst := make(map[string]net.IOCountersStat, len(stats))
	for _, stat := range stats {
		dst[stat.Name] = stat
	}
	return dst
}

func normalizePatterns(patterns []string) []string {
	if len(patterns) == 0 {
		return nil
	}
	out := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		if trimmed := strings.TrimSpace(pattern); trimmed != "" {
			out = append(out, strings.ToLower(trimmed))
		}
	}
	return out
}

func shouldIgnoreInterface(name string, patterns []string) bool {
	if len(patterns) == 0 || name == "" {
		return false
	}
	for _, pattern := range patterns {
		if pattern == "" {
			continue
		}
		p := strings.ToLower(pattern)
		if matched, err := path.Match(p, name); err == nil && matched {
			return true
		}
	}
	return false
}
