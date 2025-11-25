package lifecycle

import (
	"context"
	"errors"
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
	CPUTotalMilli        float64
	CPUFreeMilli         float64
	CPULoad1             float64
	MemoryTotalMB        float64
	MemoryFreeMB         float64
	DiskTotalMB          float64
	DiskFreeMB           float64
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
			TotalMCores: r.CPUTotalMilli,
			FreeMCores:  r.CPUFreeMilli,
			Load1:       r.CPULoad1,
		},
		Memory: MemoryResources{
			TotalMB: r.MemoryTotalMB,
			FreeMB:  r.MemoryFreeMB,
		},
		Disk: DiskResources{
			TotalMB: r.DiskTotalMB,
			FreeMB:  r.DiskFreeMB,
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

func (c *Collector) collectResources(ctx context.Context) (resourceSnapshot, error) {
	if c.resourcesFunc != nil {
		return c.resourcesFunc(ctx)
	}
	var snapshot resourceSnapshot
	var errs []string

	totalCores := runtime.NumCPU()
	snapshot.CPUTotalMilli = float64(totalCores) * 1000

	if c.loadFunc != nil {
		if avg, err := c.loadFunc(ctx); err == nil {
			snapshot.CPULoad1 = avg.Load1
			used := avg.Load1 * 1000
			free := snapshot.CPUTotalMilli - used
			if free < 0 {
				free = 0
			}
			snapshot.CPUFreeMilli = free
		} else if !errors.Is(err, context.Canceled) {
			snapshot.CPUFreeMilli = snapshot.CPUTotalMilli
			errs = append(errs, "load:"+err.Error())
		} else {
			snapshot.CPUFreeMilli = snapshot.CPUTotalMilli
		}
	} else if avg, err := load.AvgWithContext(ctx); err == nil {
		snapshot.CPULoad1 = avg.Load1
		used := avg.Load1 * 1000
		free := snapshot.CPUTotalMilli - used
		if free < 0 {
			free = 0
		}
		snapshot.CPUFreeMilli = free
	} else if !errors.Is(err, context.Canceled) {
		snapshot.CPUFreeMilli = snapshot.CPUTotalMilli
		errs = append(errs, "load:"+err.Error())
	} else {
		snapshot.CPUFreeMilli = snapshot.CPUTotalMilli
	}

	if c.memFunc != nil {
		if vm, err := c.memFunc(ctx); err == nil {
			snapshot.MemoryTotalMB = bytesToMB(vm.Total)
			snapshot.MemoryFreeMB = bytesToMB(vm.Available)
		} else if !errors.Is(err, context.Canceled) {
			errs = append(errs, "memory:"+err.Error())
		}
	} else if vm, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		snapshot.MemoryTotalMB = bytesToMB(vm.Total)
		snapshot.MemoryFreeMB = bytesToMB(vm.Available)
	} else {
		errs = append(errs, "memory:"+err.Error())
	}

	if c.diskUsageFunc != nil {
		if du, err := c.diskUsageFunc(ctx, "/"); err == nil {
			snapshot.DiskTotalMB = bytesToMB(du.Total)
			snapshot.DiskFreeMB = bytesToMB(du.Free)
		} else if !errors.Is(err, context.Canceled) {
			errs = append(errs, "disk:"+err.Error())
		}
	} else if du, err := disk.UsageWithContext(ctx, "/"); err == nil {
		snapshot.DiskTotalMB = bytesToMB(du.Total)
		snapshot.DiskFreeMB = bytesToMB(du.Free)
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
	} else if err != nil {
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
