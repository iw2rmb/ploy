package lifecycle

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// HealthChecker reports the current state of a subsystem.
type HealthChecker interface {
	Check(ctx context.Context) ComponentStatus
}

// Options configure the lifecycle collector.
type Options struct {
	Role             string
	NodeID           domaintypes.NodeID
	Hostname         func() (string, error)
	Docker           HealthChecker
	Gate             HealthChecker
	Clock            func() time.Time
	IgnoreInterfaces []string
}

// Snapshot aggregates typed status and capacity payloads.
type Snapshot struct {
	Status   NodeStatus
	Capacity NodeCapacity
}

// Collector gathers node lifecycle data for status endpoints and heartbeats.
type Collector struct {
	role             string
	nodeID           domaintypes.NodeID
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
func NewCollector(opts Options) (*Collector, error) {
	hostFn := opts.Hostname
	if hostFn == nil {
		hostFn = os.Hostname
	}
	nowFn := opts.Clock
	if nowFn == nil {
		nowFn = func() time.Time { return time.Now().UTC() }
	}

	nodeID := domaintypes.NodeID(domaintypes.Normalize(opts.NodeID.String()))
	if _, err := nodeID.MarshalText(); err != nil {
		return nil, err
	}

	return &Collector{
		role:          strings.TrimSpace(opts.Role),
		nodeID:        nodeID,
		hostname:      hostFn,
		docker:        opts.Docker,
		gate:          opts.Gate,
		now:           nowFn,
		loadFunc:      load.AvgWithContext,
		memFunc:       mem.VirtualMemoryWithContext,
		diskUsageFunc: disk.UsageWithContext,
		diskCountersFunc: func(ctx context.Context) (map[string]disk.IOCountersStat, error) {
			return disk.IOCountersWithContext(ctx)
		},
		netCountersFunc:  func(ctx context.Context) ([]net.IOCountersStat, error) { return net.IOCountersWithContext(ctx, true) },
		ignoreInterfaces: normalizePatterns(opts.IgnoreInterfaces),
		metrics:          newMetricsCache(),
	}, nil
}

// Collect builds the latest status and capacity payloads.
func (c *Collector) Collect(ctx context.Context) (Snapshot, error) {
	now := c.now()
	host, err := c.hostname()
	if err != nil {
		host = "[unavailable]"
	}

	resources, resErr := c.collectResources(ctx)

	dockerStatus := c.checkComponent(ctx, c.docker)
	gateStatus := c.checkComponent(ctx, c.gate)
	components := NodeComponents{
		Docker: dockerStatus,
		Gate:   gateStatus,
	}

	statusState := aggregateComponentState(dockerStatus, gateStatus, resErr)

	status := NodeStatus{
		State:      statusState,
		Timestamp:  now,
		Heartbeat:  now,
		Role:       c.roleOrDefault(),
		NodeID:     c.nodeID,
		Hostname:   strings.TrimSpace(host),
		Resources:  resources.toNodeResources(),
		Components: components,
	}
	if resErr != nil {
		status.ResourceWarning = resErr.Error()
	}

	capacity := NodeCapacity{
		CPUFreeMillis:  domaintypes.CPUmilli(resources.CPUFreeMillis),
		CPUTotalMillis: domaintypes.CPUmilli(resources.CPUTotalMillis),
		MemFreeBytes:   domaintypes.Bytes(resources.MemoryFreeBytes),
		MemTotalBytes:  domaintypes.Bytes(resources.MemoryTotalBytes),
		DiskFreeBytes:  domaintypes.Bytes(resources.DiskFreeBytes),
		DiskTotalBytes: domaintypes.Bytes(resources.DiskTotalBytes),
		Heartbeat:      now,
	}

	return Snapshot{Status: status, Capacity: capacity}, nil
}

func (c *Collector) roleOrDefault() string {
	if trimmed := strings.TrimSpace(c.role); trimmed != "" {
		return trimmed
	}
	return "unified"
}

func (c *Collector) checkComponent(ctx context.Context, checker HealthChecker) ComponentStatus {
	if checker == nil {
		return ComponentStatus{State: StateUnknown, CheckedAt: c.now()}
	}
	status := checker.Check(ctx)
	if status.State == "" {
		status.State = StateUnknown
	}
	if status.CheckedAt.IsZero() {
		status.CheckedAt = c.now()
	}
	return status
}

// statePriority returns the severity level for a state (higher = worse).
var statePriority = map[ComponentState]int{
	StateOK:       0,
	StateUnknown:  1,
	StateDegraded: 2,
	StateError:    3,
}

// worstState returns the more severe of two states.
func worstState(current, component ComponentState) ComponentState {
	if statePriority[component] > statePriority[current] {
		return component
	}
	return current
}

// aggregateComponentState computes the overall node state from individual component statuses.
func aggregateComponentState(docker, gate ComponentStatus, resErr error) ComponentState {
	state := StateOK
	if resErr != nil {
		state = StateDegraded
	}
	state = worstState(state, docker.State)
	state = worstState(state, gate.State)
	return state
}
