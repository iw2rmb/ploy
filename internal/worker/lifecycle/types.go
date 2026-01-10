package lifecycle

import (
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// NodeStatus contains the complete lifecycle status snapshot for a node.
// This type replaces untyped map[string]any usage in status reporting,
// providing compile-time type safety for status fields.
// Uses domain type (NodeID) for type-safe identification.
type NodeStatus struct {
	State           string
	Timestamp       time.Time
	Heartbeat       time.Time
	Role            string
	NodeID          domaintypes.NodeID // Node ID (NanoID-backed)
	Hostname        string
	Resources       NodeResources
	Components      NodeComponents
	ResourceWarning string // Optional error message from resource collection
}

// NodeCapacity represents available node resources for scheduling.
// This type is used for heartbeat payloads and capacity tracking.
type NodeCapacity struct {
	CPUFreeMillis  int32
	CPUTotalMillis int32
	MemFreeBytes   int64
	MemTotalBytes  int64
	DiskFreeBytes  int64
	DiskTotalBytes int64
	Heartbeat      time.Time
}

// NodeResources aggregates CPU, memory, disk, and network resource snapshots.
type NodeResources struct {
	CPU     CPUResources
	Memory  MemoryResources
	Disk    DiskResources
	Network NetworkResources
}

// CPUResources contains CPU capacity and utilization metrics.
type CPUResources struct {
	TotalMCores float64
	FreeMCores  float64
	Load1       float64
}

// MemoryResources contains memory capacity and utilization metrics.
type MemoryResources struct {
	TotalMB float64
	FreeMB  float64
}

// DiskResources contains disk capacity, utilization, and I/O metrics.
type DiskResources struct {
	TotalMB float64
	FreeMB  float64
	IO      DiskIO
}

// DiskIO contains disk I/O throughput and IOPS metrics.
type DiskIO struct {
	ReadMBPerSec  float64
	WriteMBPerSec float64
	ReadIOPS      float64
	WriteIOPS     float64
	InitialSample bool // True if metrics are from the first sample (no delta available)
}

// NetworkResources contains network throughput metrics and per-interface details.
type NetworkResources struct {
	RXBytesPerSec   float64
	TXBytesPerSec   float64
	RXPacketsPerSec float64
	TXPacketsPerSec float64
	InitialSample   bool // True if metrics are from the first sample (no delta available)
	Interfaces      map[string]NetworkInterface
}

// NetworkInterface contains per-interface network throughput metrics.
type NetworkInterface struct {
	RXBytesPerSec   float64
	TXBytesPerSec   float64
	RXPacketsPerSec float64
	TXPacketsPerSec float64
	InitialSample   bool
}

// NodeComponents contains health status for node subsystems.
type NodeComponents struct {
	Docker ComponentStatus
	Gate   ComponentStatus
}

// ToMap converts NodeStatus to map[string]any for JSON serialization.
// Called at serialization boundaries (e.g., status.Provider.Snapshot).
func (s NodeStatus) ToMap() map[string]any {
	status := map[string]any{
		"state":      s.State,
		"timestamp":  s.Timestamp.Format(time.RFC3339Nano),
		"heartbeat":  s.Heartbeat.Format(time.RFC3339Nano),
		"role":       s.Role,
		"node_id":    s.NodeID,
		"hostname":   s.Hostname,
		"resources":  s.Resources.toMap(),
		"components": s.Components.toMap(),
	}
	if s.ResourceWarning != "" {
		status["resource_warning"] = s.ResourceWarning
	}
	return status
}

// ToMap converts NodeCapacity to map[string]any for JSON serialization.
func (c NodeCapacity) ToMap() map[string]any {
	return map[string]any{
		"cpu_free_millis":  c.CPUFreeMillis,
		"cpu_total_millis": c.CPUTotalMillis,
		"mem_free_bytes":   c.MemFreeBytes,
		"mem_total_bytes":  c.MemTotalBytes,
		"disk_free_bytes":  c.DiskFreeBytes,
		"disk_total_bytes": c.DiskTotalBytes,
		"heartbeat":        c.Heartbeat.Format(time.RFC3339Nano),
	}
}

// toMap converts NodeResources to map[string]any for JSON serialization.
func (r NodeResources) toMap() map[string]any {
	return map[string]any{
		"cpu":     r.CPU.toMap(),
		"memory":  r.Memory.toMap(),
		"disk":    r.Disk.toMap(),
		"network": r.Network.toMap(),
	}
}

// toMap converts CPUResources to map[string]any for JSON serialization.
func (c CPUResources) toMap() map[string]any {
	return map[string]any{
		"total_mcores": c.TotalMCores,
		"free_mcores":  c.FreeMCores,
		"load_1m":      c.Load1,
	}
}

// toMap converts MemoryResources to map[string]any for JSON serialization.
func (m MemoryResources) toMap() map[string]any {
	return map[string]any{
		"total_mb": m.TotalMB,
		"free_mb":  m.FreeMB,
	}
}

// toMap converts DiskResources to map[string]any for JSON serialization.
func (d DiskResources) toMap() map[string]any {
	disk := map[string]any{
		"total_mb": d.TotalMB,
		"free_mb":  d.FreeMB,
	}

	ioMap := map[string]any{
		"read_mb_per_sec":  d.IO.ReadMBPerSec,
		"write_mb_per_sec": d.IO.WriteMBPerSec,
		"read_iops":        d.IO.ReadIOPS,
		"write_iops":       d.IO.WriteIOPS,
	}
	if d.IO.InitialSample {
		ioMap["details"] = map[string]any{"initial_sample": true}
	}
	disk["io"] = ioMap

	return disk
}

// toMap converts NetworkResources to map[string]any for JSON serialization.
func (n NetworkResources) toMap() map[string]any {
	network := map[string]any{
		"rx_bytes_per_sec":   n.RXBytesPerSec,
		"tx_bytes_per_sec":   n.TXBytesPerSec,
		"rx_packets_per_sec": n.RXPacketsPerSec,
		"tx_packets_per_sec": n.TXPacketsPerSec,
	}
	if n.InitialSample {
		network["details"] = map[string]any{"initial_sample": true}
	}
	if len(n.Interfaces) > 0 {
		ifaces := make(map[string]any, len(n.Interfaces))
		for name, iface := range n.Interfaces {
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
		network["interfaces"] = ifaces
	}
	return network
}

// toMap converts NodeComponents to map[string]any for JSON serialization.
func (c NodeComponents) toMap() map[string]any {
	return map[string]any{
		"docker": componentToMap(c.Docker),
		"gate":   componentToMap(c.Gate),
	}
}

// componentToMap converts a single ComponentStatus to map[string]any.
func componentToMap(status ComponentStatus) map[string]any {
	component := map[string]any{
		"state":      status.State,
		"checked_at": status.CheckedAt.UTC().Format(time.RFC3339Nano),
	}
	if status.Message != "" {
		component["message"] = status.Message
	}
	if status.Version != "" {
		component["version"] = status.Version
	}
	if len(status.Details) > 0 {
		component["details"] = cloneAnyMap(status.Details)
	}
	return component
}
