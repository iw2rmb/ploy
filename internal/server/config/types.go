package config

import (
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// HTTPConfig configures the HTTP server.
type HTTPConfig struct {
	Listen       string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

// AuthConfig configures authentication mechanisms.
type AuthConfig struct {
	BearerTokens BearerTokenConfig
}

// BearerTokenConfig configures JWT bearer token authentication.
type BearerTokenConfig struct {
	Enabled bool
	Secret  string
}

// MetricsConfig configures the Prometheus metrics endpoint.
type MetricsConfig struct {
	Listen string
}

// AdminConfig configures the local administrative interface.
type AdminConfig struct {
	Socket string
	Listen string
}

// PKIConfig configures PKI renewal.
type PKIConfig struct {
	BundleDir   string
	Certificate string
	Key         string
	RenewBefore time.Duration
	CAEndpoint  string
}

// SchedulerConfig configures background task scheduling.
type SchedulerConfig struct {
	HousekeepingInterval time.Duration
	DiskPruneInterval    time.Duration
	// TTL is the retention period for logs, events, diffs, and artifact bundles.
	// Data older than this will be purged by the TTL worker. Default: 30 days.
	TTL time.Duration
	// TTLInterval is how often the TTL worker runs cleanup. Default: 1 hour.
	TTLInterval time.Duration
	// DropPartitions enables dropping entire monthly partitions for expired data
	// instead of row-by-row deletion. More efficient for large datasets.
	DropPartitions bool
	// StaleJobRecoveryInterval is how often stale Running jobs are recovered.
	// Set to 0 to disable stale-job recovery. Default: 30 seconds.
	StaleJobRecoveryInterval time.Duration
	// NodeStaleAfter is the heartbeat age threshold after which a node is considered stale.
	// Default: 1 minute.
	NodeStaleAfter time.Duration
}

// LoggingConfig configures logging destinations.
type LoggingConfig struct {
	Level string
}

// PostgresConfig configures PostgreSQL connection.
type PostgresConfig struct {
	DSN string
}

// GitLabConfig holds GitLab access requisites for server-owned Git operations.
type GitLabConfig struct {
	Domain string
	Token  string
}

// SpecRepos contains the startup-only Git repositories used for named-spec discovery.
type SpecRepos []domaintypes.RepoURL

// NamedSpecEnvAllowlist contains process environment names available during named-spec compilation.
type NamedSpecEnvAllowlist []string

// ObjectStoreConfig configures S3-compatible object storage (e.g., Garage).
type ObjectStoreConfig struct {
	Endpoint  string
	Bucket    string
	AccessKey string
	SecretKey string
	Secure    bool
	Region    string
}
