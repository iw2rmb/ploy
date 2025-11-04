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
		role:          strings.TrimSpace(opts.Role),
		nodeID:        strings.TrimSpace(opts.NodeID),
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
