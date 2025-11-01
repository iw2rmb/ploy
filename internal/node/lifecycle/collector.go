package lifecycle

import (
	"context"
	"errors"
	"os"
	"path"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
)

const (
	stateOK       = "ok"
	stateDegraded = "degraded"
	stateError    = "error"
	stateUnknown  = "unknown"
)

// ComponentStatus describes the outcome of a subsystem health probe.
type ComponentStatus struct {
	State     string
	Message   string
	Version   string
	Details   map[string]any
	CheckedAt time.Time
}

// HealthChecker reports the current state of a subsystem.
type HealthChecker interface {
	Check(ctx context.Context) ComponentStatus
}

// Options configure the lifecycle collector.
type Options struct {
	Role             string
	NodeID           string
	Hostname         func() (string, error)
	Docker           HealthChecker
	Gate             HealthChecker
	Clock            func() time.Time
	IgnoreInterfaces []string
}

// Snapshot aggregates status and capacity payloads.
type Snapshot struct {
	Status   map[string]any
	Capacity map[string]any
}

// Collector gathers node lifecycle data for status endpoints and heartbeats.
type Collector struct {
	role             string
	nodeID           string
	hostname         func() (string, error)
	docker           HealthChecker
	gate             HealthChecker
	now              func() time.Time
	resourcesFunc    func(context.Context) (resourceSnapshot, error)
	loadFunc         func(context.Context) (*load.AvgStat, error)
	memFunc          func(context.Context) (*mem.VirtualMemoryStat, error)
	diskUsageFunc    func(context.Context, string) (*disk.UsageStat, error)
	diskCountersFunc func(context.Context) (map[string]disk.IOCountersStat, error)
	netCountersFunc  func(context.Context) ([]net.IOCountersStat, error)
	ignoreInterfaces []string
	metrics          *metricsCache
}

// NewCollector constructs a lifecycle collector with the supplied options.
func NewCollector(opts Options) *Collector {
	hostFn := opts.Hostname
	if hostFn == nil {
		hostFn = os.Hostname
	}
	nowFn := opts.Clock
	if nowFn == nil {
		nowFn = func() time.Time { return time.Now().UTC() }
	}
	return &Collector{
		role:     strings.TrimSpace(opts.Role),
		nodeID:   strings.TrimSpace(opts.NodeID),
		hostname: hostFn,
		docker:   opts.Docker,
		gate:     opts.Gate,
		now:      nowFn,
		loadFunc: load.AvgWithContext,
		memFunc:  mem.VirtualMemoryWithContext,
		diskUsageFunc: func(ctx context.Context, path string) (*disk.UsageStat, error) {
			return disk.UsageWithContext(ctx, path)
		},
		diskCountersFunc: func(ctx context.Context) (map[string]disk.IOCountersStat, error) {
			return disk.IOCountersWithContext(ctx)
		},
		netCountersFunc: func(ctx context.Context) ([]net.IOCountersStat, error) {
			return net.IOCountersWithContext(ctx, true)
		},
		ignoreInterfaces: normalizePatterns(opts.IgnoreInterfaces),
		metrics:          newMetricsCache(),
	}
}

// Collect builds the latest status and capacity payloads.
func (c *Collector) Collect(ctx context.Context) (Snapshot, error) {
	now := c.now()
	host, _ := c.hostname()

	resources, resErr := c.collectResources(ctx)

	components := map[string]ComponentStatus{
		"docker": c.checkComponent(ctx, c.docker),
		"gate":   c.checkComponent(ctx, c.gate),
	}

	statusState := aggregateState(components, resErr)

	status := map[string]any{
		"state":      statusState,
		"timestamp":  now.Format(time.RFC3339Nano),
		"heartbeat":  now.Format(time.RFC3339Nano),
		"role":       c.roleOrDefault(),
		"node_id":    c.nodeID,
		"hostname":   strings.TrimSpace(host),
		"resources":  resources.asMap(),
		"components": componentsToMap(components),
	}
	if resErr != nil {
		status["resource_warning"] = resErr.Error()
	}

	capacity := map[string]any{
		"cpu_free":  resources.CPUFreeMilli,
		"mem_free":  resources.MemoryFreeMB,
		"disk_free": resources.DiskFreeMB,
		"heartbeat": now.Format(time.RFC3339Nano),
	}

	return Snapshot{
		Status:   status,
		Capacity: capacity,
	}, nil
}

func (c *Collector) roleOrDefault() string {
	if trimmed := strings.TrimSpace(c.role); trimmed != "" {
		return trimmed
	}
	return "unified"
}

func (c *Collector) checkComponent(ctx context.Context, checker HealthChecker) ComponentStatus {
	if checker == nil {
		return ComponentStatus{State: stateUnknown, CheckedAt: c.now()}
	}
	status := checker.Check(ctx)
	if status.State == "" {
		status.State = stateUnknown
	}
	if status.CheckedAt.IsZero() {
		status.CheckedAt = c.now()
	}
	return status
}

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

func (r resourceSnapshot) asMap() map[string]any {
	resources := map[string]any{
		"cpu": map[string]any{
			"total_mcores": r.CPUTotalMilli,
			"free_mcores":  r.CPUFreeMilli,
			"load_1m":      r.CPULoad1,
		},
		"memory": map[string]any{
			"total_mb": r.MemoryTotalMB,
			"free_mb":  r.MemoryFreeMB,
		},
	}

	diskMap := map[string]any{
		"total_mb": r.DiskTotalMB,
		"free_mb":  r.DiskFreeMB,
	}
	ioMap := map[string]any{
		"read_mb_per_sec":  r.DiskReadMBps,
		"write_mb_per_sec": r.DiskWriteMBps,
		"read_iops":        r.DiskReadIOPS,
		"write_iops":       r.DiskWriteIOPS,
	}
	if r.DiskInitialSample {
		ioMap["details"] = map[string]any{"initial_sample": true}
	}
	diskMap["io"] = ioMap
	resources["disk"] = diskMap

	networkMap := map[string]any{
		"rx_bytes_per_sec":   r.NetworkRxBps,
		"tx_bytes_per_sec":   r.NetworkTxBps,
		"rx_packets_per_sec": r.NetworkRxPps,
		"tx_packets_per_sec": r.NetworkTxPps,
	}
	if r.NetworkInitialSample {
		networkMap["details"] = map[string]any{"initial_sample": true}
	}
	if len(r.NetworkInterfaces) > 0 {
		ifaces := make(map[string]any, len(r.NetworkInterfaces))
		for name, iface := range r.NetworkInterfaces {
			entry := map[string]any{
				"rx_bytes_per_sec":   iface.RXBytesPerSec,
				"tx_bytes_per_sec":   iface.TXBytesPerSec,
				"rx_packets_per_sec": iface.RXPacketsPerSec,
				"tx_packets_per_sec": iface.TXPacketsPerSec,
			}
			if iface.InitialSample {
				entry["details"] = map[string]any{"initial_sample": true}
			}
			ifaces[name] = entry
		}
		networkMap["interfaces"] = ifaces
	}
	resources["network"] = networkMap

	return resources
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
	var errs []string

	var diskStats map[string]disk.IOCountersStat
	if c.diskCountersFunc != nil {
		stats, err := c.diskCountersFunc(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			errs = append(errs, "disk_io:"+err.Error())
		} else {
			diskStats = stats
		}
	}

	var netStats []net.IOCountersStat
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

func deltaUint(prev, cur uint64) uint64 {
	if cur >= prev {
		return cur - prev
	}
	return 0
}

func componentsToMap(components map[string]ComponentStatus) map[string]any {
	out := make(map[string]any, len(components))
	for name, status := range components {
		component := map[string]any{
			"state":      status.State,
			"checked_at": status.CheckedAt.UTC().Format(time.RFC3339Nano),
		}
		if strings.TrimSpace(status.Message) != "" {
			component["message"] = status.Message
		}
		if strings.TrimSpace(status.Version) != "" {
			component["version"] = status.Version
		}
		if len(status.Details) > 0 {
			component["details"] = cloneAnyMap(status.Details)
		}
		out[name] = component
	}
	return out
}

func aggregateState(components map[string]ComponentStatus, resErr error) string {
	state := stateOK
	if resErr != nil {
		state = stateDegraded
	}
	for _, comp := range components {
		switch strings.ToLower(comp.State) {
		case stateError:
			return stateError
		case stateDegraded:
			if state != stateError {
				state = stateDegraded
			}
		case stateUnknown:
			if state == stateOK {
				state = stateUnknown
			}
		}
	}
	return state
}

func bytesToMB(value uint64) float64 {
	const mb = 1024 * 1024
	return float64(value) / mb
}
