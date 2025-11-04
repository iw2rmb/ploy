package lifecycle

import (
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/net"
)

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
		metrics.ReadMBps = bytesToMB(readBytes) / elapsed
		metrics.WriteMBps = bytesToMB(writeBytes) / elapsed
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
