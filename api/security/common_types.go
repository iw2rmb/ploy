package security

import "time"

// Common types shared across ARF components

// TimeRange represents a time range for queries and filters
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// ResourceUsage represents resource utilization information
type ResourceUsage struct {
	Timestamp       time.Time           `json:"timestamp"`
	TotalCPU        float64             `json:"total_cpu"`
	AvailableCPU    float64             `json:"available_cpu"`
	TotalMemory     int64               `json:"total_memory"`
	AvailableMemory int64               `json:"available_memory"`
	TotalDisk       int64               `json:"total_disk"`
	AvailableDisk   int64               `json:"available_disk"`
	Utilization     ResourceUtilization `json:"utilization"`
}

// ResourceUtilization represents resource utilization percentages
type ResourceUtilization struct {
	CPU          float64 `json:"cpu"`
	Memory       float64 `json:"memory"`
	Disk         float64 `json:"disk"`
	Network      float64 `json:"network"`
	SandboxCount int     `json:"sandbox_count"`
	WorkerCount  int     `json:"worker_count"`
}

// PerformanceMetrics represents system performance measurements
type PerformanceMetrics struct {
	Timestamp       time.Time              `json:"timestamp"`
	SystemMetrics   SystemMetrics          `json:"system_metrics"`
	ARFMetrics      ARFPerformanceMetrics  `json:"arf_metrics"`
	DatabaseMetrics DatabaseMetrics        `json:"database_metrics"`
	NetworkMetrics  NetworkMetrics         `json:"network_metrics"`
	CustomMetrics   map[string]interface{} `json:"custom_metrics"`
}

// SystemMetrics represents general system metrics
type SystemMetrics struct {
	CPUUsage        float64   `json:"cpu_usage"`
	MemoryUsage     float64   `json:"memory_usage"`
	DiskUsage       float64   `json:"disk_usage"`
	NetworkIO       NetworkIO `json:"network_io"`
	DiskIO          DiskIO    `json:"disk_io"`
	LoadAverage     float64   `json:"load_average"`
	OpenFileHandles int       `json:"open_file_handles"`
	ProcessCount    int       `json:"process_count"`
	ThreadCount     int       `json:"thread_count"`
}

// ARFPerformanceMetrics represents ARF-specific performance metrics
type ARFPerformanceMetrics struct {
	ActiveTransformations    int                 `json:"active_transformations"`
	QueuedTransformations    int                 `json:"queued_transformations"`
	CompletedTransformations int                 `json:"completed_transformations"`
	FailedTransformations    int                 `json:"failed_transformations"`
	AverageProcessingTime    time.Duration       `json:"average_processing_time"`
	ThroughputPerSecond      float64             `json:"throughput_per_second"`
	ErrorRate                float64             `json:"error_rate"`
	CacheHitRate             float64             `json:"cache_hit_rate"`
	ResourceUtilization      ResourceUtilization `json:"resource_utilization"`
	SecurityMetrics          SecurityMetrics     `json:"security_metrics"`
}

// DatabaseMetrics represents database performance metrics
type DatabaseMetrics struct {
	ConnectionCount int           `json:"connection_count"`
	ActiveQueries   int           `json:"active_queries"`
	QueryLatency    time.Duration `json:"query_latency"`
	TransactionRate float64       `json:"transaction_rate"`
	CacheHitRatio   float64       `json:"cache_hit_ratio"`
	DeadlockCount   int           `json:"deadlock_count"`
	SlowQueryCount  int           `json:"slow_query_count"`
}

// NetworkMetrics represents network performance metrics
type NetworkMetrics struct {
	Latency         time.Duration `json:"latency"`
	Throughput      float64       `json:"throughput"`
	PacketLoss      float64       `json:"packet_loss"`
	ConnectionCount int           `json:"connection_count"`
	ErrorRate       float64       `json:"error_rate"`
	BandwidthUsage  float64       `json:"bandwidth_usage"`
}

// NetworkIO represents network I/O metrics
type NetworkIO struct {
	BytesReceived   int64 `json:"bytes_received"`
	BytesSent       int64 `json:"bytes_sent"`
	PacketsReceived int64 `json:"packets_received"`
	PacketsSent     int64 `json:"packets_sent"`
}

// DiskIO represents disk I/O metrics
type DiskIO struct {
	ReadBytes  int64         `json:"read_bytes"`
	WriteBytes int64         `json:"write_bytes"`
	ReadOps    int64         `json:"read_ops"`
	WriteOps   int64         `json:"write_ops"`
	IOWaitTime time.Duration `json:"io_wait_time"`
}

// SecurityMetrics represents security-related metrics
type SecurityMetrics struct {
	VulnerabilitiesFound   int     `json:"vulnerabilities_found"`
	VulnerabilitiesFixed   int     `json:"vulnerabilities_fixed"`
	SecurityScansPerformed int     `json:"security_scans_performed"`
	ComplianceScore        float64 `json:"compliance_score"`
	ThreatDetections       int     `json:"threat_detections"`
	FailedSecurityChecks   int     `json:"failed_security_checks"`
}

// Repository represents a code repository for analysis
type Repository struct {
	ID           string            `json:"id"`
	URL          string            `json:"url"`
	Branch       string            `json:"branch"`
	Path         string            `json:"path"`
	Language     string            `json:"language"`
	Metadata     map[string]string `json:"metadata"`
	BuildTool    string            `json:"build_tool"`
	Dependencies []string          `json:"dependencies"`
}

// RiskLevel represents the risk level of a transformation
type RiskLevel int

const (
	RiskLevelLow RiskLevel = iota
	RiskLevelMedium
	RiskLevelModerate
	RiskLevelHigh
	RiskLevelCritical
)

// Comment severity levels
type CommentSeverity string

const (
	SeverityInfo     CommentSeverity = "info"
	SeverityMinor    CommentSeverity = "minor"
	SeverityMajor    CommentSeverity = "major"
	SeverityCritical CommentSeverity = "critical"
	SeverityBlocking CommentSeverity = "blocking"
)
