package health

import (
	"time"

	cfgsvc "github.com/iw2rmb/ploy/internal/config"
	"github.com/iw2rmb/ploy/internal/utils"
	"github.com/nats-io/nats.go"
)

// HealthChecker provides health and readiness checking functionality
type HealthChecker struct {
	storageConfigPath string
	nomadAddr         string
	metricsCollector  *HealthMetrics
	configService     *cfgsvc.Service
	disableChecks     bool
	jetstreamCfg      JetStreamHealthConfig
	jetstreamDialer   jetStreamDialer
}

// NewHealthChecker creates a new health checker instance.
func NewHealthChecker(storageConfigPath, nomadAddr string, opts ...Option) *HealthChecker {
	if nomadAddr == "" {
		nomadAddr = utils.Getenv("NOMAD_ADDR", "http://127.0.0.1:4646")
	}
	h := &HealthChecker{
		storageConfigPath: storageConfigPath,
		nomadAddr:         nomadAddr,
		metricsCollector: &HealthMetrics{
			DependencyFailures:  make(map[string]int64),
			AverageResponseTime: make(map[string]time.Duration),
		},
		jetstreamCfg:    newJetStreamHealthConfigFromEnv(),
		jetstreamDialer: defaultJetStreamDialer{},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(h)
		}
	}
	return h
}

// SetConfigService optionally injects centralized config service.
func (h *HealthChecker) SetConfigService(svc *cfgsvc.Service) { h.configService = svc }

// SetDependencyChecksEnabled toggles dependency health checks (used in tests).
func (h *HealthChecker) SetDependencyChecksEnabled(enabled bool) {
	h.disableChecks = !enabled
}

// Option configures the health checker during construction.
type Option func(*HealthChecker)

// WithJetStreamConfig overrides the default JetStream health configuration.
func WithJetStreamConfig(cfg JetStreamHealthConfig) Option {
	return func(h *HealthChecker) {
		h.jetstreamCfg = cfg
	}
}

// WithJetStreamDialer injects a custom JetStream dialer (primarily for tests).
func WithJetStreamDialer(d jetStreamDialer) Option {
	return func(h *HealthChecker) {
		if d != nil {
			h.jetstreamDialer = d
		}
	}
}

// JetStreamHealthConfig captures connection and resource details required for JetStream readiness checks.
type JetStreamHealthConfig struct {
	URL             string
	CredentialsPath string
	User            string
	Password        string
	EnvBucket       string
	UpdatesStream   string
	Timeout         time.Duration
}

type jetStreamDialer interface {
	Connect(url string, opts ...nats.Option) (*nats.Conn, error)
}

type defaultJetStreamDialer struct{}

func (defaultJetStreamDialer) Connect(url string, opts ...nats.Option) (*nats.Conn, error) {
	return nats.Connect(url, opts...)
}

func newJetStreamHealthConfigFromEnv() JetStreamHealthConfig {
	timeout := 5 * time.Second
	if v := utils.Getenv("PLOY_JETSTREAM_HEALTH_TIMEOUT", ""); v != "" {
		if parsed, err := time.ParseDuration(v); err == nil && parsed > 0 {
			timeout = parsed
		}
	}
	url := utils.Getenv("PLOY_JETSTREAM_URL", "")
	if url == "" {
		url = utils.Getenv("PLOY_UPDATES_JETSTREAM_URL", "")
	}
	cfg := JetStreamHealthConfig{
		URL:             url,
		CredentialsPath: utils.Getenv("PLOY_JETSTREAM_CREDS", ""),
		User:            utils.Getenv("PLOY_JETSTREAM_USER", ""),
		Password:        utils.Getenv("PLOY_JETSTREAM_PASSWORD", ""),
		EnvBucket:       utils.Getenv("PLOY_JETSTREAM_ENV_BUCKET", ""),
		UpdatesStream:   utils.Getenv("PLOY_UPDATES_STREAM", ""),
		Timeout:         timeout,
	}
	if cfg.EnvBucket == "" {
		cfg.EnvBucket = utils.Getenv("PLOY_JETSTREAM_KV_BUCKET", "ploy_env")
	}
	if cfg.CredentialsPath == "" {
		cfg.CredentialsPath = utils.Getenv("PLOY_UPDATES_JETSTREAM_CREDS", "")
	}
	if cfg.User == "" {
		cfg.User = utils.Getenv("PLOY_UPDATES_JETSTREAM_USER", "")
		cfg.Password = utils.Getenv("PLOY_UPDATES_JETSTREAM_PASSWORD", cfg.Password)
	} else if cfg.Password == "" {
		cfg.Password = utils.Getenv("PLOY_JETSTREAM_PASSWORD", "")
	}
	if cfg.UpdatesStream == "" {
		cfg.UpdatesStream = utils.Getenv("PLOY_UPDATES_STATUS_STREAM", "")
	}
	return cfg
}
