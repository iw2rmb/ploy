package config

import (
	"time"
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

// PKIConfig configures PKI renewal.
type PKIConfig struct {
	BundleDir   string        `yaml:"bundle_dir"`
	Certificate string        `yaml:"certificate"`
	Key         string        `yaml:"key"`
	RenewBefore time.Duration `yaml:"renew_before"`
	CAEndpoint  string        `yaml:"ca_endpoint"`
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
