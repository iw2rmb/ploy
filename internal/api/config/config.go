package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	// ModeBootstrap executes the bootstrap workflow.
	ModeBootstrap = "bootstrap"
	// ModeWorker executes steady-state orchestration.
	ModeWorker = "worker"
	// ModeBeacon executes beacon responsibilities in addition to worker mode.
	ModeBeacon = "beacon"

	defaultHTTPListen        = ":8443"
	defaultMetricsListen     = ":9100"
	defaultAdminSocket       = "/run/ployd.sock"
	defaultHeartbeatInterval = 10 * time.Second
	defaultAssignmentPoll    = 5 * time.Second
	defaultStatusPublish     = 30 * time.Second
	defaultPKIRenewBefore    = time.Hour
	defaultTaskConcurrency   = 2
	defaultHousekeeping      = 5 * time.Minute
	defaultDiskPrune         = time.Hour
)

// Config represents the ployd daemon configuration.
type Config struct {
	Mode         string              `yaml:"mode"`
	HTTP         HTTPConfig          `yaml:"http"`
	Metrics      MetricsConfig       `yaml:"metrics"`
	Admin        AdminConfig         `yaml:"admin"`
	ControlPlane ControlPlaneConfig  `yaml:"control_plane"`
	PKI          PKIConfig           `yaml:"pki"`
	Bootstrap    BootstrapConfig     `yaml:"bootstrap"`
	Worker       WorkerConfig        `yaml:"worker"`
	Beacon       BeaconConfig        `yaml:"beacon"`
	Runtime      RuntimeConfig       `yaml:"runtime"`
	Scheduler    SchedulerConfig     `yaml:"scheduler"`
	Logging      LoggingConfig       `yaml:"logging"`
	FilePath     string              `yaml:"-"`
	Environment  map[string]string   `yaml:"environment"`
	Features     map[string]bool     `yaml:"features"`
	Tags         map[string]string   `yaml:"tags"`
	Metadata     map[string]any      `yaml:"metadata"`
	Extra        map[string]any      `yaml:"extra"`
	rawPlugins   map[string]struct{} `yaml:"-"`
}

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

// Load reads the configuration from the provided path.
func Load(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("config: open %s: %w", path, err)
	}
	defer f.Close()
	cfg, err := loadFromReader(f)
	if err != nil {
		return Config{}, err
	}
	cfg.FilePath = path
	applyDefaults(&cfg)
	if err := validate(&cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// loadFromReader unmarshals configuration from the reader.
func loadFromReader(r io.Reader) (Config, error) {
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		return Config{}, fmt.Errorf("config: read: %w", err)
	}
	if buf.Len() == 0 {
		cfg := defaultConfig()
		return cfg, nil
	}
	dec := yaml.NewDecoder(&buf)
	dec.KnownFields(true)
	var cfg Config
	if err := dec.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("config: decode: %w", err)
	}
	return cfg, nil
}

func defaultConfig() Config {
	return Config{
		Mode: ModeWorker,
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
			HousekeepingInterval: defaultHousekeeping,
			DiskPruneInterval:    defaultDiskPrune,
		},
		Worker: WorkerConfig{
			TaskConcurrency: defaultTaskConcurrency,
		},
	}
}

func applyDefaults(cfg *Config) {
	if cfg == nil {
		return
	}

	cfg.Mode = normalizeMode(cfg.Mode)
	if cfg.Mode == "" {
		cfg.Mode = ModeWorker
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
	if strings.TrimSpace(cfg.ControlPlane.HealthEndpoint) == "" {
		cfg.ControlPlane.HealthEndpoint = "/v1/health"
	}
	if strings.TrimSpace(cfg.ControlPlane.ConfigEndpoint) == "" {
		cfg.ControlPlane.ConfigEndpoint = "/v1/config"
	}
	if strings.TrimSpace(cfg.ControlPlane.AssignmentsEndpoint) == "" {
		cfg.ControlPlane.AssignmentsEndpoint = "/v1/assignments"
	}
	if strings.TrimSpace(cfg.ControlPlane.NodeStatusEndpoint) == "" {
		cfg.ControlPlane.NodeStatusEndpoint = "/v1/nodes"
	}

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

	normalizeRuntimeConfig(&cfg.Runtime)
	cfg.rawPlugins = make(map[string]struct{}, len(cfg.Runtime.Plugins))
	for _, plugin := range cfg.Runtime.Plugins {
		cfg.rawPlugins[plugin.Name] = struct{}{}
	}
}

func normalizeRuntimeConfig(rt *RuntimeConfig) {
	if rt == nil {
		return
	}
	plugins := make([]RuntimePluginConfig, 0, len(rt.Plugins))
	seen := make(map[string]struct{}, len(rt.Plugins))
	for _, plugin := range rt.Plugins {
		name := strings.TrimSpace(strings.ToLower(plugin.Name))
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		module := strings.TrimSpace(plugin.Module)
		if plugin.Config == nil {
			plugin.Config = make(map[string]any)
		}
		if plugin.Enabled == nil {
			plugin.Enabled = ptrTo(true)
		}
		plugins = append(plugins, RuntimePluginConfig{
			Name:    name,
			Module:  module,
			Enabled: plugin.Enabled,
			Config:  plugin.Config,
		})
	}
	sort.SliceStable(plugins, func(i, j int) bool {
		return plugins[i].Name < plugins[j].Name
	})
	rt.Plugins = plugins

	rt.DefaultAdapter = strings.TrimSpace(strings.ToLower(rt.DefaultAdapter))
	if rt.DefaultAdapter == "" {
		if len(rt.Plugins) > 0 {
			rt.DefaultAdapter = rt.Plugins[0].Name
		} else {
			rt.DefaultAdapter = "local"
		}
	}
}

func ptrTo[T any](v T) *T {
	return &v
}

func validate(cfg *Config) error {
	if cfg == nil {
		return errors.New("config: nil configuration")
	}
	switch cfg.Mode {
	case ModeBootstrap, ModeWorker, ModeBeacon:
	default:
		return fmt.Errorf("config: invalid mode %q", cfg.Mode)
	}

	if strings.TrimSpace(cfg.ControlPlane.Endpoint) == "" {
		return errors.New("config: control_plane.endpoint is required")
	}
	if strings.TrimSpace(cfg.ControlPlane.CAPath) == "" {
		return errors.New("config: control_plane.ca is required")
	}
	if strings.TrimSpace(cfg.ControlPlane.Certificate) == "" {
		return errors.New("config: control_plane.certificate is required")
	}
	if strings.TrimSpace(cfg.ControlPlane.Key) == "" {
		return errors.New("config: control_plane.key is required")
	}

	if cfg.HTTP.TLS.Enabled {
		if strings.TrimSpace(cfg.HTTP.TLS.CertPath) == "" {
			return errors.New("config: http.tls.cert required when TLS enabled")
		}
		if strings.TrimSpace(cfg.HTTP.TLS.KeyPath) == "" {
			return errors.New("config: http.tls.key required when TLS enabled")
		}
	}

	if strings.TrimSpace(cfg.Admin.Socket) == "" && strings.TrimSpace(cfg.Admin.Listen) == "" {
		return errors.New("config: admin.socket or admin.listen must be configured")
	}

	// Validate runtime plugins.
	names := make(map[string]struct{}, len(cfg.Runtime.Plugins))
	for _, plugin := range cfg.Runtime.Plugins {
		if strings.TrimSpace(plugin.Name) == "" {
			return errors.New("config: runtime plugin name required")
		}
		if _, exists := names[plugin.Name]; exists {
			return fmt.Errorf("config: duplicate runtime plugin %q", plugin.Name)
		}
		names[plugin.Name] = struct{}{}
	}
	if len(cfg.Runtime.Plugins) > 0 {
		if _, exists := names[cfg.Runtime.DefaultAdapter]; !exists {
			return fmt.Errorf("config: runtime.default_adapter %q not registered", cfg.Runtime.DefaultAdapter)
		}
	}

	return nil
}

// ResolveRelative resolves the provided path relative to the configuration location when the path is relative.
func (cfg Config) ResolveRelative(p string) string {
	if strings.TrimSpace(p) == "" {
		return ""
	}
	if filepath.IsAbs(p) || strings.HasPrefix(p, "${") {
		return p
	}
	base := filepath.Dir(cfg.FilePath)
	if base == "" || base == "." {
		return p
	}
	return filepath.Join(base, p)
}

func normalizeMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", ModeWorker:
		return ModeWorker
	case ModeBootstrap:
		return ModeBootstrap
	case ModeBeacon:
		return ModeBeacon
	default:
		return strings.ToLower(strings.TrimSpace(mode))
	}
}
