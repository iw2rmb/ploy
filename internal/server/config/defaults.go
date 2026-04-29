package config

import (
	"strings"
	"time"
)

const (
	defaultHTTPListen             = ":8080"
	defaultMetricsListen          = ":9100"
	defaultAdminSocket            = "/run/ployd.sock"
	defaultPKIRenewBefore         = time.Hour
	defaultHousekeeping           = 5 * time.Minute
	defaultDiskPrune              = time.Hour
	defaultBatchSchedulerInterval = 5 * time.Second
	defaultStaleJobRecovery       = 30 * time.Second
	defaultNodeStaleAfter         = time.Minute
)

// defaultConfig returns the baseline configuration with baked-in defaults.
func defaultConfig() Config {
	return Config{
		HTTP: HTTPConfig{
			Listen: defaultHTTPListen,
		},
		Metrics: MetricsConfig{
			Listen: defaultMetricsListen,
		},
		Auth: AuthConfig{
			BearerTokens: BearerTokenConfig{
				Enabled: true,
			},
		},
		Admin: AdminConfig{
			Socket: defaultAdminSocket,
		},
		PKI: PKIConfig{
			RenewBefore: defaultPKIRenewBefore,
		},
		Scheduler: SchedulerConfig{
			HousekeepingInterval:     defaultHousekeeping,
			DiskPruneInterval:        defaultDiskPrune,
			BatchSchedulerInterval:   defaultBatchSchedulerInterval,
			StaleJobRecoveryInterval: defaultStaleJobRecovery,
			NodeStaleAfter:           defaultNodeStaleAfter,
		},
	}
}

// applyDefaults fills in derived defaults and allocates nested maps.
func applyDefaults(cfg *Config) {
	if cfg == nil {
		return
	}

	if strings.TrimSpace(cfg.HTTP.Listen) == "" {
		cfg.HTTP.Listen = defaultHTTPListen
	}
	if cfg.HTTP.ReadTimeout <= 0 {
		cfg.HTTP.ReadTimeout = 15 * time.Second
	}
	if cfg.HTTP.WriteTimeout <= 0 {
		cfg.HTTP.WriteTimeout = 15 * time.Second
	}
	if cfg.HTTP.IdleTimeout <= 0 {
		cfg.HTTP.IdleTimeout = 60 * time.Second
	}

	if strings.TrimSpace(cfg.Metrics.Listen) == "" {
		cfg.Metrics.Listen = defaultMetricsListen
	}

	if strings.TrimSpace(cfg.Admin.Socket) == "" && strings.TrimSpace(cfg.Admin.Listen) == "" {
		cfg.Admin.Socket = defaultAdminSocket
	}

	if strings.TrimSpace(cfg.PKI.BundleDir) == "" {
		cfg.PKI.BundleDir = "/etc/ploy/pki"
	}
	if cfg.PKI.RenewBefore <= 0 {
		cfg.PKI.RenewBefore = defaultPKIRenewBefore
	}

	// Batch scheduler interval: 0 disables the scheduler, negative uses default.
	if cfg.Scheduler.BatchSchedulerInterval < 0 {
		cfg.Scheduler.BatchSchedulerInterval = defaultBatchSchedulerInterval
	}
	// Stale recovery interval: 0 disables stale-job recovery, negative uses default.
	if cfg.Scheduler.StaleJobRecoveryInterval < 0 {
		cfg.Scheduler.StaleJobRecoveryInterval = defaultStaleJobRecovery
	}
	if cfg.Scheduler.NodeStaleAfter <= 0 {
		cfg.Scheduler.NodeStaleAfter = defaultNodeStaleAfter
	}
}
