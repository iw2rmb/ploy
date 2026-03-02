package lifecycle

import (
	"encoding/json"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// ComponentState represents the health state of a subsystem.
type ComponentState string

const (
	StateOK       ComponentState = "ok"
	StateDegraded ComponentState = "degraded"
	StateError    ComponentState = "error"
	StateUnknown  ComponentState = "unknown"
)

// NodeStatus contains the complete lifecycle status snapshot for a node.
type NodeStatus struct {
	State           ComponentState     `json:"state"`
	Timestamp       time.Time          `json:"timestamp"`
	Heartbeat       time.Time          `json:"heartbeat"`
	Role            string             `json:"role"`
	NodeID          domaintypes.NodeID `json:"node_id"`
	Hostname        string             `json:"hostname"`
	Resources       NodeResources      `json:"resources"`
	Components      NodeComponents     `json:"components"`
	ResourceWarning string             `json:"resource_warning,omitempty"`
}

// NodeCapacity represents available node resources for scheduling.
type NodeCapacity struct {
	CPUFreeMillis  domaintypes.CPUmilli `json:"cpu_free_millis"`
	CPUTotalMillis domaintypes.CPUmilli `json:"cpu_total_millis"`
	MemFreeBytes   domaintypes.Bytes    `json:"mem_free_bytes"`
	MemTotalBytes  domaintypes.Bytes    `json:"mem_total_bytes"`
	DiskFreeBytes  domaintypes.Bytes    `json:"disk_free_bytes"`
	DiskTotalBytes domaintypes.Bytes    `json:"disk_total_bytes"`
	Heartbeat      time.Time            `json:"heartbeat"`
}

// NodeResources aggregates CPU, memory, disk, and network resource snapshots.
type NodeResources struct {
	CPU     CPUResources     `json:"cpu"`
	Memory  MemoryResources  `json:"memory"`
	Disk    DiskResources    `json:"disk"`
	Network NetworkResources `json:"network"`
}

// CPUResources contains CPU capacity and utilization metrics.
type CPUResources struct {
	TotalMCores float64 `json:"total_mcores"`
	FreeMCores  float64 `json:"free_mcores"`
	Load1       float64 `json:"load_1m"`
}

// MemoryResources contains memory capacity and utilization metrics.
type MemoryResources struct {
	TotalMB float64 `json:"total_mb"`
	FreeMB  float64 `json:"free_mb"`
}

// DiskResources contains disk capacity, utilization, and I/O metrics.
type DiskResources struct {
	TotalMB float64 `json:"total_mb"`
	FreeMB  float64 `json:"free_mb"`
	IO      DiskIO  `json:"io"`
}

// DiskIO contains disk I/O throughput and IOPS metrics.
type DiskIO struct {
	ReadMBPerSec  float64 `json:"read_mb_per_sec"`
	WriteMBPerSec float64 `json:"write_mb_per_sec"`
	ReadIOPS      float64 `json:"read_iops"`
	WriteIOPS     float64 `json:"write_iops"`
	InitialSample bool    `json:"initial_sample,omitempty"`
}

// NetworkResources contains network throughput metrics and per-interface details.
type NetworkResources struct {
	RXBytesPerSec   float64                     `json:"rx_bytes_per_sec"`
	TXBytesPerSec   float64                     `json:"tx_bytes_per_sec"`
	RXPacketsPerSec float64                     `json:"rx_packets_per_sec"`
	TXPacketsPerSec float64                     `json:"tx_packets_per_sec"`
	InitialSample   bool                        `json:"initial_sample,omitempty"`
	Interfaces      map[string]NetworkInterface `json:"interfaces,omitempty"`
}

// NetworkInterface contains per-interface network throughput metrics.
type NetworkInterface struct {
	RXBytesPerSec   float64 `json:"rx_bytes_per_sec"`
	TXBytesPerSec   float64 `json:"tx_bytes_per_sec"`
	RXPacketsPerSec float64 `json:"rx_packets_per_sec"`
	TXPacketsPerSec float64 `json:"tx_packets_per_sec"`
	InitialSample   bool    `json:"initial_sample,omitempty"`
}

// NodeComponents contains health status for node subsystems.
type NodeComponents struct {
	Docker ComponentStatus `json:"docker"`
	Gate   ComponentStatus `json:"gate"`
}

// ComponentStatus describes the outcome of a subsystem health probe.
type ComponentStatus struct {
	State     ComponentState `json:"state"`
	Message   string         `json:"message,omitempty"`
	Version   string         `json:"version,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
	CheckedAt time.Time      `json:"checked_at"`
}

// MarshalJSON implements encoding/json.Marshaler for NodeStatus.
func (s NodeStatus) MarshalJSON() ([]byte, error) {
	type Alias NodeStatus
	return json.Marshal(&struct {
		Alias
		Timestamp string `json:"timestamp"`
		Heartbeat string `json:"heartbeat"`
	}{
		Alias:     Alias(s),
		Timestamp: s.Timestamp.Format(time.RFC3339Nano),
		Heartbeat: s.Heartbeat.Format(time.RFC3339Nano),
	})
}

// MarshalJSON implements encoding/json.Marshaler for NodeCapacity.
func (c NodeCapacity) MarshalJSON() ([]byte, error) {
	type Alias NodeCapacity
	return json.Marshal(&struct {
		Alias
		Heartbeat string `json:"heartbeat"`
	}{
		Alias:     Alias(c),
		Heartbeat: c.Heartbeat.Format(time.RFC3339Nano),
	})
}

// MarshalJSON implements encoding/json.Marshaler for ComponentStatus.
func (cs ComponentStatus) MarshalJSON() ([]byte, error) {
	type Alias ComponentStatus
	return json.Marshal(&struct {
		Alias
		CheckedAt string `json:"checked_at"`
	}{
		Alias:     Alias(cs),
		CheckedAt: cs.CheckedAt.UTC().Format(time.RFC3339Nano),
	})
}
