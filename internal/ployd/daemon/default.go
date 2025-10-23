package daemon

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/iw2rmb/ploy/internal/node/logstream"
	"github.com/iw2rmb/ploy/internal/ployd/admin"
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

	httpSrv, err := httpserver.New(httpserver.Options{
		Config:  cfg,
		Streams: streams,
		Status:  statusProvider,
		Admin:   adminSvc,
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

	svc, err := New(Options{
		Config:          cfg,
		RuntimeRegistry: registry,
		LogStreams:      streams,
		HTTP:            httpSrv,
		Metrics:         metricsSrv,
		ControlPlane:    controlClient,
		PKI:             pkiManager,
		Scheduler:       taskScheduler,
		Bootstrap:       noopBootstrap{},
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

type noopBootstrap struct{}

func (noopBootstrap) Run(context.Context, config.Config) error { return nil }

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
