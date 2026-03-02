package lifecycle

import (
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/net"
)

func cloneDiskCounters(src map[string]disk.IOCountersStat) map[string]disk.IOCountersStat {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]disk.IOCountersStat, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func deltaUint(prev, cur uint64) uint64 {
	if cur >= prev {
		return cur - prev
	}
	return 0
}

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

func computeDiskMetrics(prev, cur map[string]disk.IOCountersStat, elapsed float64) (diskIOMetrics, bool) {
	var (
		readBytes   uint64
		writeBytes  uint64
		readCount   uint64
		writeCount  uint64
		hasBaseline bool
	)
	for name, current := range cur {
		if prev == nil {
			break
		}
		prevStat, ok := prev[name]
		if !ok {
			continue
		}
		hasBaseline = true
		readBytes += deltaUint(prevStat.ReadBytes, current.ReadBytes)
		writeBytes += deltaUint(prevStat.WriteBytes, current.WriteBytes)
		readCount += deltaUint(prevStat.ReadCount, current.ReadCount)
		writeCount += deltaUint(prevStat.WriteCount, current.WriteCount)
	}

	metrics := diskIOMetrics{}
	if elapsed > 0 {
		metrics.ReadMBps = bytesToMBUint64(readBytes) / elapsed
		metrics.WriteMBps = bytesToMBUint64(writeBytes) / elapsed
		metrics.ReadIOPS = float64(readCount) / elapsed
		metrics.WriteIOPS = float64(writeCount) / elapsed
	}
	return metrics, hasBaseline
}

func computeNetworkMetrics(prev, cur map[string]net.IOCountersStat, elapsed float64) (networkMetrics, bool) {
	metrics := networkMetrics{
		Interfaces: make(map[string]networkInterfaceSnapshot, len(cur)),
	}
	var hasBaseline bool
	for name, current := range cur {
		if prev == nil {
			metrics.Interfaces[name] = networkInterfaceSnapshot{InitialSample: true}
			continue
		}
		prevStat, ok := prev[name]
		if !ok {
			metrics.Interfaces[name] = networkInterfaceSnapshot{InitialSample: true}
			continue
		}
		hasBaseline = true

		rxBytes := deltaUint(prevStat.BytesRecv, current.BytesRecv)
		txBytes := deltaUint(prevStat.BytesSent, current.BytesSent)
		rxPackets := deltaUint(prevStat.PacketsRecv, current.PacketsRecv)
		txPackets := deltaUint(prevStat.PacketsSent, current.PacketsSent)

		iface := networkInterfaceSnapshot{
			RXBytesPerSec:   float64(rxBytes) / elapsed,
			TXBytesPerSec:   float64(txBytes) / elapsed,
			RXPacketsPerSec: float64(rxPackets) / elapsed,
			TXPacketsPerSec: float64(txPackets) / elapsed,
		}
		metrics.Interfaces[name] = iface
		metrics.RXBytesPerSec += iface.RXBytesPerSec
		metrics.TXBytesPerSec += iface.TXBytesPerSec
		metrics.RXPacketsPerSec += iface.RXPacketsPerSec
		metrics.TXPacketsPerSec += iface.TXPacketsPerSec
	}
	if len(metrics.Interfaces) == 0 {
		metrics.Interfaces = nil
	}
	return metrics, hasBaseline
}
