package health

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strings"
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
	timeout := 10 * time.Second
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

func (h *HealthChecker) jetStreamHealth(clientName string) (DependencyHealth, *nats.Conn, nats.JetStreamContext, bool) {
	start := time.Now()
	dep := DependencyHealth{Status: "healthy", Latency: time.Since(start)}
	conn, js, usedTLS, err := h.connectJetStream(clientName)
	if err != nil {
		dep.Status = "unhealthy"
		dep.Error = fmt.Sprintf("jetstream connection failed: %v", err)
		dep.Latency = time.Since(start)
		return dep, nil, nil, false
	}
	details := map[string]interface{}{"url": h.jetstreamCfg.URL}
	if usedTLS {
		details["transport"] = "tls"
	} else {
		details["transport"] = "tcp"
	}
	var errs []string
	if info, err := js.AccountInfo(); err == nil {
		details["account"] = map[string]interface{}{
			"domain":        info.Domain,
			"streams":       info.Streams,
			"consumers":     info.Consumers,
			"store_bytes":   info.Store,
			"memory_bytes":  info.Memory,
			"max_streams":   info.Limits.MaxStreams,
			"max_consumers": info.Limits.MaxConsumers,
		}
	} else {
		errs = append(errs, fmt.Sprintf("account info: %v", err))
	}
	if bucket := strings.TrimSpace(h.jetstreamCfg.EnvBucket); bucket != "" {
		if kv, err := js.KeyValue(bucket); err != nil {
			errs = append(errs, fmt.Sprintf("env bucket %s: %v", bucket, err))
		} else if status, err := kv.Status(); err == nil {
			details["env_bucket"] = map[string]interface{}{
				"name":        status.Bucket(),
				"values":      status.Values(),
				"history":     status.History(),
				"ttl_seconds": int(status.TTL().Seconds()),
				"bytes":       status.Bytes(),
				"compressed":  status.IsCompressed(),
				"store":       status.BackingStore(),
			}
		} else {
			errs = append(errs, fmt.Sprintf("env bucket status %s: %v", bucket, err))
		}
	}
	if stream := strings.TrimSpace(h.jetstreamCfg.UpdatesStream); stream != "" {
		if info, err := js.StreamInfo(stream); err == nil {
			details["updates_stream"] = map[string]interface{}{
				"name":      info.Config.Name,
				"messages":  info.State.Msgs,
				"consumers": info.State.Consumers,
				"replicas":  info.Config.Replicas,
			}
		} else {
			errs = append(errs, fmt.Sprintf("updates stream %s: %v", stream, err))
		}
	}
	if len(errs) > 0 {
		details["errors"] = errs
		dep.Status = "unhealthy"
		dep.Error = strings.Join(errs, "; ")
	}
	dep.Details = details
	dep.Latency = time.Since(start)
	return dep, conn, js, usedTLS
}

func (h *HealthChecker) connectJetStream(clientName string) (*nats.Conn, nats.JetStreamContext, bool, error) {
	conn, js, err := h.connectJetStreamOnce(clientName, false, nil)
	if err == nil {
		return conn, js, false, nil
	}
	if shouldRetryWithTLS(err) {
		tlsCfg := &tls.Config{InsecureSkipVerify: true}
		connTLS, jsTLS, errTLS := h.connectJetStreamOnce(clientName, true, tlsCfg)
		if errTLS == nil {
			return connTLS, jsTLS, true, nil
		}
		return nil, nil, false, errTLS
	}
	return nil, nil, false, err
}

func (h *HealthChecker) connectJetStreamOnce(clientName string, useTLS bool, tlsCfg *tls.Config) (*nats.Conn, nats.JetStreamContext, error) {
	cfg := h.jetstreamCfg
	if strings.TrimSpace(cfg.URL) == "" {
		return nil, nil, fmt.Errorf("jetstream url not configured")
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	opts := []nats.Option{nats.Name(clientName), nats.Timeout(timeout)}
	if strings.TrimSpace(cfg.CredentialsPath) != "" {
		opts = append(opts, nats.UserCredentials(cfg.CredentialsPath))
	}
	if strings.TrimSpace(cfg.User) != "" {
		opts = append(opts, nats.UserInfo(cfg.User, cfg.Password))
	}
	if useTLS {
		if tlsCfg == nil {
			tlsCfg = &tls.Config{InsecureSkipVerify: true}
		}
		opts = append(opts, nats.Secure(tlsCfg))
	}
	conn, err := h.jetstreamDialer.Connect(cfg.URL, opts...)
	if err != nil {
		return nil, nil, err
	}
	if err := conn.FlushTimeout(timeout); err != nil {
		conn.Close()
		return nil, nil, err
	}
	js, err := conn.JetStream()
	if err != nil {
		conn.Close()
		return nil, nil, err
	}
	return conn, js, nil
}

func shouldRetryWithTLS(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, nats.ErrNoServers) || errors.Is(err, nats.ErrTimeout) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	if strings.Contains(strings.ToLower(err.Error()), "i/o timeout") {
		return true
	}
	return false
}
