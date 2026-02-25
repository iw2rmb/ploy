package config

import (
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// HTTPConfig configures the HTTP server.
type HTTPConfig struct {
	Listen       string        `yaml:"listen"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
	IdleTimeout  time.Duration `yaml:"idle_timeout"`
}

// AuthConfig configures authentication mechanisms.
type AuthConfig struct {
	BearerTokens BearerTokenConfig `yaml:"bearer_tokens"`
}

// BearerTokenConfig configures JWT bearer token authentication.
type BearerTokenConfig struct {
	Enabled bool   `yaml:"enabled"`
	Secret  string `yaml:"secret"` // JWT signing secret (or load from env)
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
	Endpoint               string             `yaml:"endpoint"`
	NodeID                 domaintypes.NodeID `yaml:"node_id"`
	CAPath                 string             `yaml:"ca"`
	Certificate            string             `yaml:"certificate"`
	Key                    string             `yaml:"key"`
	HeartbeatInterval      time.Duration      `yaml:"heartbeat_interval"`
	AssignmentPollInterval time.Duration      `yaml:"assignment_poll_interval"`
	StatusPublishInterval  time.Duration      `yaml:"status_publish_interval"`
	AssignmentBatchSize    int                `yaml:"assignment_batch_size"`
	MaxBackoff             time.Duration      `yaml:"max_backoff"`
	InitialBackoff         time.Duration      `yaml:"initial_backoff"`
	// Legacy endpoint fields removed; server exposes fixed routes.
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
	// TTL is the retention period for logs, events, diffs, and artifact bundles.
	// Data older than this will be purged by the TTL worker. Default: 30 days.
	TTL time.Duration `yaml:"ttl"`
	// TTLInterval is how often the TTL worker runs cleanup. Default: 1 hour.
	TTLInterval time.Duration `yaml:"ttl_interval"`
	// DropPartitions enables dropping entire monthly partitions for expired data
	// instead of row-by-row deletion. More efficient for large datasets.
	DropPartitions bool `yaml:"drop_partitions"`
	// BatchSchedulerInterval is how often the batch scheduler checks for pending repos.
	// Set to 0 to disable the batch scheduler. Default: 5 seconds.
	BatchSchedulerInterval time.Duration `yaml:"batch_scheduler_interval"`
	// StaleJobRecoveryInterval is how often stale Running jobs are recovered.
	// Set to 0 to disable stale-job recovery. Default: 30 seconds.
	StaleJobRecoveryInterval time.Duration `yaml:"stale_job_recovery_interval"`
	// NodeStaleAfter is the heartbeat age threshold after which a node is considered stale.
	// Default: 1 minute.
	NodeStaleAfter time.Duration `yaml:"node_stale_after"`
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

// TransfersConfig configures transfer guard and janitor behaviour.
type TransfersConfig struct {
	BaseDir         string        `yaml:"base_dir"`
	GuardBinary     string        `yaml:"guard_binary"`
	JanitorInterval time.Duration `yaml:"janitor_interval"`
}

// PostgresConfig configures PostgreSQL connection.
type PostgresConfig struct {
	DSN string `yaml:"dsn"`
}

// GitLabConfig configures GitLab integration.
type GitLabConfig struct {
	Domain    string `yaml:"domain"`
	Token     string `yaml:"token"`
	TokenFile string `yaml:"token_file"`
}

// ObjectStoreConfig configures S3-compatible object storage (e.g., Garage).
type ObjectStoreConfig struct {
	Endpoint  string `yaml:"endpoint"`
	Bucket    string `yaml:"bucket"`
	AccessKey string `yaml:"access_key"`
	SecretKey string `yaml:"secret_key"`
	Secure    bool   `yaml:"secure"`
	Region    string `yaml:"region,omitempty"`
}
