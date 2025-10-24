package daemon

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/iw2rmb/ploy/internal/config/gitlab"
	"github.com/iw2rmb/ploy/internal/controlplane/events"
	controlplanescheduler "github.com/iw2rmb/ploy/internal/controlplane/scheduler"
	controlmetrics "github.com/iw2rmb/ploy/internal/metrics"
	"github.com/iw2rmb/ploy/internal/node/logstream"
	"github.com/iw2rmb/ploy/internal/ployd/admin"
	"github.com/iw2rmb/ploy/internal/ployd/bootstrap"
	"github.com/iw2rmb/ploy/internal/ployd/config"
	"github.com/iw2rmb/ploy/internal/ployd/controlplane"
	"github.com/iw2rmb/ploy/internal/ployd/executor"
	"github.com/iw2rmb/ploy/internal/ployd/httpserver"
	"github.com/iw2rmb/ploy/internal/ployd/metrics"
	"github.com/iw2rmb/ploy/internal/ployd/pki"
	"github.com/iw2rmb/ploy/internal/ployd/runtime"
	"github.com/iw2rmb/ploy/internal/ployd/scheduler"
	"github.com/iw2rmb/ploy/internal/ployd/status"
	workflowruntime "github.com/iw2rmb/ploy/internal/workflow/runtime"
)

// NewDefault constructs a daemon using default component implementations.
func NewDefault(cfg config.Config) (*Daemon, error) {
	streams := logstream.NewHub(logstream.Options{})
	registry := workflowruntime.NewRegistry()

	loader := runtime.NewLoader(registry)
	runtime.RegisterDefaultFactories(loader)
	if err := loader.Apply(context.Background(), cfg.Runtime); err != nil {
		return nil, err
	}

	statusProvider := status.New(status.Options{Mode: cfg.Mode})
	adminSvc := buildAdminService()

	controlPlaneHandler, controlPlaneShutdown, err := buildControlPlaneHTTP(streams)
	if err != nil {
		return nil, err
	}

	httpSrv, err := httpserver.New(httpserver.Options{
		Config:       cfg,
		Streams:      streams,
		Status:       statusProvider,
		Admin:        adminSvc,
		ControlPlane: controlPlaneHandler,
	})
	if err != nil {
		return nil, err
	}

	metricsSrv := metrics.New(metrics.Options{Listen: cfg.Metrics.Listen})

	rotator := &fileRotator{}
	pkiManager, err := pki.New(pki.Options{
		Config:  cfg.PKI,
		Rotator: rotator,
	})
	if err != nil {
		return nil, err
	}

	exec := executor.New(executor.Options{
		Registry:       registry,
		DefaultAdapter: cfg.Runtime.DefaultAdapter,
	})

	controlClient, err := controlplane.New(controlplane.Options{
		Config:   cfg.ControlPlane,
		Executor: exec,
		Status:   statusProvider,
	})
	if err != nil {
		return nil, err
	}

	taskScheduler := scheduler.New()

	bootstrapRunner := bootstrap.NewRunner(bootstrap.Options{})

	svc, err := New(Options{
		Config:               cfg,
		RuntimeRegistry:      registry,
		LogStreams:           streams,
		HTTP:                 httpSrv,
		Metrics:              metricsSrv,
		ControlPlane:         controlClient,
		PKI:                  pkiManager,
		Scheduler:            taskScheduler,
		Bootstrap:            bootstrapRunner,
		ControlPlaneShutdown: controlPlaneShutdown,
	})
	if err != nil {
		return nil, err
	}
	return svc, nil
}

type fileRotator struct {
	mu sync.Mutex
}

func (r *fileRotator) Renew(ctx context.Context, cfg config.PKIConfig) error {
	_ = ctx
	r.mu.Lock()
	defer r.mu.Unlock()
	if cfg.Certificate != "" {
		if err := ensureFile(cfg.Certificate); err != nil {
			return err
		}
	}
	if cfg.Key != "" {
		if err := ensureFile(cfg.Key); err != nil {
			return err
		}
	}
	return nil
}

func ensureFile(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("pki: path required")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return os.WriteFile(path, []byte{}, 0o600)
	}
	return nil
}

func buildAdminService() httpserver.AdminService {
	endpoints := strings.Split(strings.TrimSpace(os.Getenv("PLOY_ETCD_ENDPOINTS")), ",")
	var cleaned []string
	for _, ep := range endpoints {
		ep = strings.TrimSpace(ep)
		if ep == "" {
			continue
		}
		cleaned = append(cleaned, ep)
	}
	if len(cleaned) == 0 {
		return nil
	}
	return &admin.Service{EtcdEndpoints: cleaned}
}

func buildControlPlaneHTTP(streams *logstream.Hub) (http.Handler, func(context.Context) error, error) {
	if streams == nil {
		return nil, nil, errors.New("control-plane: streams hub required")
	}
	endpointsEnv := strings.Split(strings.TrimSpace(os.Getenv("PLOY_ETCD_ENDPOINTS")), ",")
	var endpoints []string
	for _, ep := range endpointsEnv {
		ep = strings.TrimSpace(ep)
		if ep == "" {
			continue
		}
		endpoints = append(endpoints, ep)
	}
	if len(endpoints) == 0 {
		return nil, nil, nil
	}

	cfg := clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: 5 * time.Second,
	}

	if user := strings.TrimSpace(os.Getenv("PLOY_ETCD_USERNAME")); user != "" {
		cfg.Username = user
		cfg.Password = os.Getenv("PLOY_ETCD_PASSWORD")
	}

	tlsCfg, err := buildEtcdTLSConfigFromEnv()
	if err != nil {
		return nil, nil, fmt.Errorf("control-plane: etcd tls: %w", err)
	}
	if tlsCfg != nil {
		cfg.TLS = tlsCfg
	}

	client, err := clientv3.New(cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("control-plane: etcd: %w", err)
	}

	recorder, err := controlmetrics.NewSchedulerMetrics(nil)
	if err != nil {
		_ = client.Close()
		return nil, nil, fmt.Errorf("control-plane: metrics: %w", err)
	}

	sched, err := controlplanescheduler.New(client, controlplanescheduler.Options{
		Metrics: recorder,
	})
	if err != nil {
		_ = client.Close()
		return nil, nil, fmt.Errorf("control-plane: scheduler: %w", err)
	}

	var signer *gitlab.Signer
	if strings.TrimSpace(os.Getenv("PLOY_GITLAB_SIGNER_AES_KEY")) != "" {
		signer, err = gitlab.NewSignerFromEnv(client)
		if err != nil {
			_ = sched.Close()
			_ = client.Close()
			return nil, nil, fmt.Errorf("control-plane: gitlab signer: %w", err)
		}
	}

	var rotations *events.RotationHub
	if signer != nil {
		rotations = events.NewRotationHub(context.Background(), signer)
	}

	handler := httpserver.NewControlPlaneHandler(httpserver.ControlPlaneOptions{
		Scheduler: sched,
		Signer:    signer,
		Streams:   streams,
		Etcd:      client,
		Rotations: rotations,
	})

	shutdown := func(ctx context.Context) error {
		_ = ctx
		if rotations != nil {
			rotations.Close()
		}
		if sched != nil {
			_ = sched.Close()
		}
		if client != nil {
			return client.Close()
		}
		return nil
	}
	return handler, shutdown, nil
}

func buildEtcdTLSConfigFromEnv() (*tls.Config, error) {
	caPath := strings.TrimSpace(os.Getenv("PLOY_ETCD_TLS_CA"))
	certPath := strings.TrimSpace(os.Getenv("PLOY_ETCD_TLS_CERT"))
	keyPath := strings.TrimSpace(os.Getenv("PLOY_ETCD_TLS_KEY"))
	skipVerify := strings.EqualFold(strings.TrimSpace(os.Getenv("PLOY_ETCD_TLS_SKIP_VERIFY")), "true") ||
		strings.TrimSpace(os.Getenv("PLOY_ETCD_TLS_SKIP_VERIFY")) == "1"

	if caPath == "" && certPath == "" && keyPath == "" && !skipVerify {
		return nil, nil
	}

	tlsCfg := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: skipVerify, // #nosec G402 — allow operator override for debugging.
	}

	if caPath != "" {
		data, err := os.ReadFile(caPath)
		if err != nil {
			return nil, fmt.Errorf("load etcd ca: %w", err)
		}
		pool := x509.NewCertPool()
		if ok := pool.AppendCertsFromPEM(data); !ok {
			return nil, errors.New("control-plane: parse etcd ca")
		}
		tlsCfg.RootCAs = pool
	}

	if certPath != "" || keyPath != "" {
		if certPath == "" || keyPath == "" {
			return nil, errors.New("control-plane: both etcd client cert and key required")
		}
		cert, err := tls.LoadX509KeyPair(certPath, keyPath)
		if err != nil {
			return nil, fmt.Errorf("control-plane: load etcd client certificate: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}

	return tlsCfg, nil
}
