package config

import (
	"strings"
	"time"
)

const (
	defaultHTTPListen             = ":8443"
	defaultMetricsListen          = ":9100"
	defaultAdminSocket            = "/run/ployd.sock"
	defaultHeartbeatInterval      = 10 * time.Second
	defaultAssignmentPoll         = 5 * time.Second
	defaultStatusPublish          = 30 * time.Second
	defaultPKIRenewBefore         = time.Hour
	defaultTaskConcurrency        = 2
	defaultHousekeeping           = 5 * time.Minute
	defaultDiskPrune              = time.Hour
	defaultTransfersBaseDir       = "/var/lib/ploy/ssh-artifacts"
	defaultGuardBinary            = "/usr/lib/openssh/sftp-server"
	defaultJanitorInterval        = time.Minute
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
		Admin: AdminConfig{
			Socket: defaultAdminSocket,
		},
		PKI: PKIConfig{
			RenewBefore: defaultPKIRenewBefore,
		},
		Runtime: RuntimeConfig{
			DefaultAdapter: "local",
		},
		Scheduler: SchedulerConfig{
			HousekeepingInterval:     defaultHousekeeping,
			DiskPruneInterval:        defaultDiskPrune,
			BatchSchedulerInterval:   defaultBatchSchedulerInterval,
			StaleJobRecoveryInterval: defaultStaleJobRecovery,
			NodeStaleAfter:           defaultNodeStaleAfter,
		},
		Worker: WorkerConfig{
			TaskConcurrency: defaultTaskConcurrency,
		},
		Transfers: TransfersConfig{
			BaseDir:         defaultTransfersBaseDir,
			GuardBinary:     defaultGuardBinary,
			JanitorInterval: defaultJanitorInterval,
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

	if cfg.ControlPlane.HeartbeatInterval <= 0 {
		cfg.ControlPlane.HeartbeatInterval = defaultHeartbeatInterval
	}
	if cfg.ControlPlane.AssignmentPollInterval <= 0 {
		cfg.ControlPlane.AssignmentPollInterval = defaultAssignmentPoll
	}
	if cfg.ControlPlane.StatusPublishInterval <= 0 {
		cfg.ControlPlane.StatusPublishInterval = defaultStatusPublish
	}
	// Job* endpoints removed; nodes use /v1/nodes/* and clients stream via /v1/migs/*.

	if cfg.ControlPlane.AssignmentBatchSize <= 0 {
		cfg.ControlPlane.AssignmentBatchSize = 1
	}
	if cfg.ControlPlane.MaxBackoff <= 0 {
		cfg.ControlPlane.MaxBackoff = 2 * time.Minute
	}
	if cfg.ControlPlane.InitialBackoff <= 0 {
		cfg.ControlPlane.InitialBackoff = 2 * time.Second
	}

	if strings.TrimSpace(cfg.PKI.BundleDir) == "" {
		cfg.PKI.BundleDir = "/etc/ploy/pki"
	}
	if cfg.PKI.RenewBefore <= 0 {
		cfg.PKI.RenewBefore = defaultPKIRenewBefore
	}

	if cfg.Bootstrap.ReportInterval <= 0 {
		cfg.Bootstrap.ReportInterval = 5 * time.Second
	}

	if cfg.Worker.TaskConcurrency <= 0 {
		cfg.Worker.TaskConcurrency = defaultTaskConcurrency
	}

	if cfg.Runtime.FeatureFlags == nil {
		cfg.Runtime.FeatureFlags = make(map[string]bool)
	}

	if cfg.Environment == nil {
		cfg.Environment = make(map[string]string)
	}
	if cfg.Tags == nil {
		cfg.Tags = make(map[string]string)
	}
	if cfg.Metadata == nil {
		cfg.Metadata = make(map[string]any)
	}
	if cfg.Extra == nil {
		cfg.Extra = make(map[string]any)
	}

	if strings.TrimSpace(cfg.Transfers.BaseDir) == "" {
		cfg.Transfers.BaseDir = defaultTransfersBaseDir
	}
	if strings.TrimSpace(cfg.Transfers.GuardBinary) == "" {
		cfg.Transfers.GuardBinary = defaultGuardBinary
	}
	if cfg.Transfers.JanitorInterval <= 0 {
		cfg.Transfers.JanitorInterval = defaultJanitorInterval
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

	normalizeRuntimeConfig(&cfg.Runtime)
	cfg.rawPlugins = make(map[string]struct{}, len(cfg.Runtime.Plugins))
	for _, plugin := range cfg.Runtime.Plugins {
		cfg.rawPlugins[plugin.Name] = struct{}{}
	}
}
