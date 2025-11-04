package lifecycle

import (
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/net"
)

type metricsCache struct {
	mu       sync.Mutex
	lastDisk map[string]disk.IOCountersStat
	lastNet  map[string]net.IOCountersStat
	lastAt   time.Time
	primed   bool
}

func newMetricsCache() *metricsCache {
	return &metricsCache{}
}

func (m *metricsCache) sample(now time.Time, diskStats map[string]disk.IOCountersStat, netStats []net.IOCountersStat) (diskIOMetrics, networkMetrics) {
	if m == nil {
		return diskIOMetrics{Initial: true}, networkMetrics{Initial: true}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	curDisk := cloneDiskCounters(diskStats)
	curNet := netSliceToMap(netStats)

	if !m.primed {
		m.lastDisk = curDisk
		m.lastNet = curNet
		m.lastAt = now
		m.primed = true
		return diskIOMetrics{Initial: true}, networkMetrics{Initial: true}
	}

	elapsed := now.Sub(m.lastAt).Seconds()
	if elapsed <= 0 {
		elapsed = 1
	}

	diskMetrics, diskBaseline := computeDiskMetrics(m.lastDisk, curDisk, elapsed)
	networkMetrics, netBaseline := computeNetworkMetrics(m.lastNet, curNet, elapsed)

	if !diskBaseline {
		diskMetrics.Initial = true
	}
	if !netBaseline {
		networkMetrics.Initial = true
	}

	m.lastDisk = curDisk
	m.lastNet = curNet
	m.lastAt = now

	return diskMetrics, networkMetrics
}
