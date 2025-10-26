package config

import "time"

// HTTPConfig configures the Fiber HTTP server.
type HTTPConfig struct {
	Listen       string        `yaml:"listen"`
	TLS          TLSConfig     `yaml:"tls"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
	IdleTimeout  time.Duration `yaml:"idle_timeout"`
}

// TLSConfig configures TLS for HTTP servers.
type TLSConfig struct {
	Enabled           bool   `yaml:"enabled"`
	CertPath          string `yaml:"cert"`
	KeyPath           string `yaml:"key"`
	ClientCAPath      string `yaml:"client_ca"`
	RequireClientCert bool   `yaml:"require_client_cert"`
}

// MetricsConfig configures the Prometheus metrics endpoint.
type MetricsConfig struct {
	Listen string `yaml:"listen"`
}

// AdminConfig configures the local administrative interface.
type AdminConfig struct {
	Socket string `yaml:"socket"`
	Listen string `yaml:"listen"`
}

// ControlPlaneConfig configures the control-plane integration.
type ControlPlaneConfig struct {
	Endpoint                string        `yaml:"endpoint"`
	NodeID                  string        `yaml:"node_id"`
	CAPath                  string        `yaml:"ca"`
	Certificate             string        `yaml:"certificate"`
	Key                     string        `yaml:"key"`
	HeartbeatInterval       time.Duration `yaml:"heartbeat_interval"`
	AssignmentPollInterval  time.Duration `yaml:"assignment_poll_interval"`
	StatusPublishInterval   time.Duration `yaml:"status_publish_interval"`
	AssignmentBatchSize     int           `yaml:"assignment_batch_size"`
	MaxBackoff              time.Duration `yaml:"max_backoff"`
	InitialBackoff          time.Duration `yaml:"initial_backoff"`
	HealthEndpoint          string        `yaml:"health_endpoint"`
	ConfigEndpoint          string        `yaml:"config_endpoint"`
	AssignmentsEndpoint     string        `yaml:"assignments_endpoint"`
	NodeStatusEndpoint      string        `yaml:"node_status_endpoint"`
	LogStreamEndpoint       string        `yaml:"log_stream_endpoint"`
	ArtifactEndpoint        string        `yaml:"artifact_endpoint"`
	MetricsEndpoint         string        `yaml:"metrics_endpoint"`
	AdminEndpoint           string        `yaml:"admin_endpoint"`
	ControlPlaneCACachePath string        `yaml:"control_plane_ca_cache_path"`
}

// PKIConfig configures PKI renewal.
type PKIConfig struct {
	BundleDir   string        `yaml:"bundle_dir"`
	Certificate string        `yaml:"certificate"`
	Key         string        `yaml:"key"`
	RenewBefore time.Duration `yaml:"renew_before"`
	CAEndpoint  string        `yaml:"ca_endpoint"`
}

// BootstrapConfig configures bootstrap mode.
type BootstrapConfig struct {
	Enabled        bool          `yaml:"enabled"`
	Steps          []string      `yaml:"steps"`
	ScriptPath     string        `yaml:"script_path"`
	ReportInterval time.Duration `yaml:"report_interval"`
}

// WorkerConfig configures worker mode.
type WorkerConfig struct {
	ArtifactDir     string `yaml:"artifact_dir"`
	LogDir          string `yaml:"log_dir"`
	RuntimeAdapter  string `yaml:"runtime_adapter"`
	TaskConcurrency int    `yaml:"task_concurrency"`
}

// BeaconConfig configures beacon mode specifics.
type BeaconConfig struct {
	Enabled         bool          `yaml:"enabled"`
	DNSZone         string        `yaml:"dns_zone"`
	PublishInterval time.Duration `yaml:"publish_interval"`
}

// RuntimeConfig configures runtime plugins.
type RuntimeConfig struct {
	DefaultAdapter string                `yaml:"default_adapter"`
	Plugins        []RuntimePluginConfig `yaml:"plugins"`
	FeatureFlags   map[string]bool       `yaml:"feature_flags"`
}

// RuntimePluginConfig describes a runtime plugin.
type RuntimePluginConfig struct {
	Name    string         `yaml:"name"`
	Module  string         `yaml:"module"`
	Enabled *bool          `yaml:"enabled"`
	Config  map[string]any `yaml:"config"`
}

// SchedulerConfig configures background task scheduling.
type SchedulerConfig struct {
	HousekeepingInterval time.Duration `yaml:"housekeeping_interval"`
	DiskPruneInterval    time.Duration `yaml:"disk_prune_interval"`
}

// LoggingConfig configures logging destinations.
type LoggingConfig struct {
	Level        string            `yaml:"level"`
	File         string            `yaml:"file"`
	MaxSizeMB    int               `yaml:"max_size_mb"`
	MaxBackups   int               `yaml:"max_backups"`
	MaxAgeDays   int               `yaml:"max_age_days"`
	JSON         bool              `yaml:"json"`
	StaticFields map[string]string `yaml:"static_fields"`
}
