package lifecycle

import (
	"context"
	"math"
	"testing"
	"time"

	gdisk "github.com/shirou/gopsutil/v4/disk"
	gload "github.com/shirou/gopsutil/v4/load"
	gmem "github.com/shirou/gopsutil/v4/mem"
	gnet "github.com/shirou/gopsutil/v4/net"
)

type staticChecker struct {
	status ComponentStatus
}

func (s staticChecker) Check(context.Context) ComponentStatus {
	return s.status
}

func TestCollectorSnapshotAggregatesComponents(t *testing.T) {
	t.Helper()

    collector := NewCollector(Options{
        Role:   "worker",
        NodeID: "node-1",
        Docker: staticChecker{status: ComponentStatus{State: stateOK, Version: "25.0.0", CheckedAt: time.Now()}},
        Gate:   staticChecker{status: ComponentStatus{State: stateError, Message: "missing image", CheckedAt: time.Now()}},
        IPFS:   staticChecker{status: ComponentStatus{State: stateUnknown, Message: "disabled", CheckedAt: time.Now()}},
    })
	collector.resourcesFunc = func(context.Context) (resourceSnapshot, error) {
		return resourceSnapshot{
			CPUTotalMilli: 8000,
			CPUFreeMilli:  4000,
			CPULoad1:      2,
			MemoryTotalMB: 16384,
			MemoryFreeMB:  8192,
			DiskTotalMB:   256000,
			DiskFreeMB:    128000,
		}, nil
	}

	snapshot, err := collector.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}

	if snapshot.Capacity["disk_free"].(float64) != 128000 {
		t.Fatalf("disk_free mismatch: %v", snapshot.Capacity["disk_free"])
	}
	if snapshot.Capacity["cpu_free"].(float64) != 4000 {
		t.Fatalf("cpu_free mismatch: %v", snapshot.Capacity["cpu_free"])
	}

	statusState, _ := snapshot.Status["state"].(string)
	if statusState != stateError {
		t.Fatalf("expected aggregated state %q, got %q", stateError, statusState)
	}

	components, ok := snapshot.Status["components"].(map[string]any)
	if !ok {
		t.Fatalf("components missing %T", snapshot.Status["components"])
	}
    gateComponent, ok := components["gate"].(map[string]any)
    if !ok {
        t.Fatalf("gate component missing")
    }
    if gateComponent["state"] != stateError {
        t.Fatalf("gate state mismatch: %v", gateComponent["state"])
    }
}

func TestCollectorSnapshotIncludesDiskAndNetworkMetrics(t *testing.T) {
	t.Helper()

	collector := NewCollector(Options{
		Role:   "worker",
		NodeID: "node-2",
	})
	collector.resourcesFunc = func(context.Context) (resourceSnapshot, error) {
		return resourceSnapshot{
			CPUTotalMilli: 8000,
			CPUFreeMilli:  6000,
			CPULoad1:      1,
			MemoryTotalMB: 32768,
			MemoryFreeMB:  24576,
			DiskTotalMB:   512000,
			DiskFreeMB:    256000,
			DiskReadMBps:  12.5,
			DiskWriteMBps: 6.25,
			DiskReadIOPS:  150,
			DiskWriteIOPS: 90,
			NetworkRxBps:  2_048_000,
			NetworkTxBps:  4_096_000,
			NetworkRxPps:  320,
			NetworkTxPps:  180,
			NetworkInterfaces: map[string]networkInterfaceSnapshot{
				"eth0": {
					RXBytesPerSec:   1_024_000,
					TXBytesPerSec:   2_048_000,
					RXPacketsPerSec: 160,
					TXPacketsPerSec: 90,
					InitialSample:   false,
				},
			},
		}, nil
	}

	snapshot, err := collector.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}

	resources, ok := snapshot.Status["resources"].(map[string]any)
	if !ok {
		t.Fatalf("resources missing")
	}

	disk, ok := resources["disk"].(map[string]any)
	if !ok {
		t.Fatalf("disk section missing")
	}
	ioSection, ok := disk["io"].(map[string]any)
	if !ok {
		t.Fatalf("disk io section missing")
	}
	if got := ioSection["read_mb_per_sec"]; got != 12.5 {
		t.Fatalf("unexpected disk read rate: %v", got)
	}
	if got := ioSection["write_iops"]; got != 90.0 {
		t.Fatalf("unexpected disk write iops: %v", got)
	}

	network, ok := resources["network"].(map[string]any)
	if !ok {
		t.Fatalf("network section missing")
	}
	if got := network["rx_bytes_per_sec"]; got != 2_048_000.0 {
		t.Fatalf("unexpected network rx bytes: %v", got)
	}
	interfaces, ok := network["interfaces"].(map[string]any)
	if !ok {
		t.Fatalf("network interfaces missing")
	}
	eth0, ok := interfaces["eth0"].(map[string]any)
	if !ok {
		t.Fatalf("eth0 metrics missing")
	}
	if got := eth0["tx_bytes_per_sec"]; got != 2_048_000.0 {
		t.Fatalf("unexpected eth0 tx bytes: %v", got)
	}
}

func TestCollectorComputesDiskAndNetworkRates(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	now := time.Date(2025, 10, 29, 19, 0, 0, 0, time.UTC)
	collector := NewCollector(Options{
		Role:   "worker",
		NodeID: "node-rates",
		Clock: func() time.Time {
			return now
		},
	})

	collector.resourcesFunc = nil
	collector.loadFunc = func(context.Context) (*gload.AvgStat, error) {
		return &gload.AvgStat{Load1: 1.0}, nil
	}
	collector.memFunc = func(context.Context) (*gmem.VirtualMemoryStat, error) {
		return &gmem.VirtualMemoryStat{
			Total:     64 * 1024 * 1024 * 1024,
			Available: 48 * 1024 * 1024 * 1024,
		}, nil
	}
	collector.diskUsageFunc = func(context.Context, string) (*gdisk.UsageStat, error) {
		return &gdisk.UsageStat{
			Total: 500 * 1024 * 1024 * 1024,
			Free:  200 * 1024 * 1024 * 1024,
		}, nil
	}

	var diskCounters map[string]gdisk.IOCountersStat
	collector.diskCountersFunc = func(context.Context) (map[string]gdisk.IOCountersStat, error) {
		return diskCounters, nil
	}

	var netCounters []gnet.IOCountersStat
	collector.netCountersFunc = func(context.Context) ([]gnet.IOCountersStat, error) {
		return netCounters, nil
	}

	collector.ignoreInterfaces = []string{"cni*"}

	diskCounters = map[string]gdisk.IOCountersStat{
		"sda": {
			ReadBytes:  4 * 1024 * 1024,
			WriteBytes: 1 * 1024 * 1024,
			ReadCount:  400,
			WriteCount: 200,
		},
	}
	netCounters = []gnet.IOCountersStat{
		{Name: "lo", BytesRecv: 1024, BytesSent: 2048, PacketsRecv: 2, PacketsSent: 4},
		{Name: "eth0", BytesRecv: 10 * 1024 * 1024, BytesSent: 5 * 1024 * 1024, PacketsRecv: 1000, PacketsSent: 2000},
	}

	first, err := collector.Collect(ctx)
	if err != nil {
		t.Fatalf("first Collect() error = %v", err)
	}
	firstResources := first.Status["resources"].(map[string]any)
	firstDisk := firstResources["disk"].(map[string]any)
	firstDiskIO := firstDisk["io"].(map[string]any)
	details, _ := firstDiskIO["details"].(map[string]any)
	if details == nil || details["initial_sample"] != true {
		t.Fatalf("expected initial sample details in disk io: %v", firstDiskIO["details"])
	}
	firstNetwork := firstResources["network"].(map[string]any)
	netDetails, _ := firstNetwork["details"].(map[string]any)
	if netDetails == nil || netDetails["initial_sample"] != true {
		t.Fatalf("expected initial sample details in network metrics: %v", firstNetwork["details"])
	}

	now = now.Add(2 * time.Second)
	diskCounters = map[string]gdisk.IOCountersStat{
		"sda": {
			ReadBytes:  8 * 1024 * 1024,
			WriteBytes: 3 * 1024 * 1024,
			ReadCount:  600,
			WriteCount: 400,
		},
		"loop0": {
			ReadBytes:  2 * 1024 * 1024,
			WriteBytes: 2 * 1024 * 1024,
			ReadCount:  200,
			WriteCount: 200,
		},
	}
	netCounters = []gnet.IOCountersStat{
		{Name: "lo", BytesRecv: 2048, BytesSent: 4096, PacketsRecv: 4, PacketsSent: 8},
		{Name: "eth0", BytesRecv: 14 * 1024 * 1024, BytesSent: 9 * 1024 * 1024, PacketsRecv: 1400, PacketsSent: 2600},
		{Name: "cni0", BytesRecv: 100 * 1024 * 1024, BytesSent: 100 * 1024 * 1024, PacketsRecv: 10000, PacketsSent: 10000},
	}

	second, err := collector.Collect(ctx)
	if err != nil {
		t.Fatalf("second Collect() error = %v", err)
	}

	resources := second.Status["resources"].(map[string]any)
	disk := resources["disk"].(map[string]any)
	io := disk["io"].(map[string]any)
	if !closeTo(io["read_mb_per_sec"].(float64), 2.0, 0.001) {
		t.Fatalf("unexpected disk read_mb_per_sec: %v", io["read_mb_per_sec"])
	}
	if !closeTo(io["write_mb_per_sec"].(float64), 1.0, 0.001) {
		t.Fatalf("unexpected disk write_mb_per_sec: %v", io["write_mb_per_sec"])
	}
	if !closeTo(io["read_iops"].(float64), 100.0, 0.001) {
		t.Fatalf("unexpected disk read_iops: %v", io["read_iops"])
	}
	if !closeTo(io["write_iops"].(float64), 100.0, 0.001) {
		t.Fatalf("unexpected disk write_iops: %v", io["write_iops"])
	}
	if _, ok := io["details"]; ok {
		t.Fatalf("initial sample details should be cleared: %v", io["details"])
	}

	network := resources["network"].(map[string]any)
	if !closeTo(network["rx_bytes_per_sec"].(float64), 2*1024*1024, 1) {
		t.Fatalf("unexpected network rx_bytes_per_sec: %v", network["rx_bytes_per_sec"])
	}
	if !closeTo(network["tx_bytes_per_sec"].(float64), 2*1024*1024, 1) {
		t.Fatalf("unexpected network tx_bytes_per_sec: %v", network["tx_bytes_per_sec"])
	}
	if !closeTo(network["rx_packets_per_sec"].(float64), 200, 0.1) {
		t.Fatalf("unexpected network rx_packets_per_sec: %v", network["rx_packets_per_sec"])
	}
	if !closeTo(network["tx_packets_per_sec"].(float64), 300, 0.1) {
		t.Fatalf("unexpected network tx_packets_per_sec: %v", network["tx_packets_per_sec"])
	}

	if _, ok := network["details"]; ok {
		t.Fatalf("expected network details cleared after first sample: %v", network["details"])
	}
	ifaces := network["interfaces"].(map[string]any)
	if len(ifaces) != 1 {
		t.Fatalf("expected one interface after ignore filters, got %d", len(ifaces))
	}
	eth0, ok := ifaces["eth0"].(map[string]any)
	if !ok {
		t.Fatalf("eth0 missing in interfaces map")
	}
	if !closeTo(eth0["rx_bytes_per_sec"].(float64), 2*1024*1024, 1) {
		t.Fatalf("unexpected eth0 rx rate: %v", eth0["rx_bytes_per_sec"])
	}
}

func closeTo(value float64, target float64, tolerance float64) bool {
	return math.Abs(value-target) <= tolerance
}
