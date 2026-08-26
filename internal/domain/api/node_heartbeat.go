package api

// NodeHeartbeatRequest is the node-to-control-plane resource snapshot. CPU
// fields stay wide enough for the server to reject overflow before narrowing.
type NodeHeartbeatRequest struct {
	CPUFreeMillis  int64  `json:"cpu_free_millis"`
	CPUTotalMillis int64  `json:"cpu_total_millis"`
	MemFreeBytes   int64  `json:"mem_free_bytes"`
	MemTotalBytes  int64  `json:"mem_total_bytes"`
	DiskFreeBytes  int64  `json:"disk_free_bytes"`
	DiskTotalBytes int64  `json:"disk_total_bytes"`
	Version        string `json:"version,omitempty"`
}
