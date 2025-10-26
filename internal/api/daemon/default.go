package daemon

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/iw2rmb/ploy/internal/api/admin"
	"github.com/iw2rmb/ploy/internal/api/config"
	"github.com/iw2rmb/ploy/internal/api/controlplane"
	"github.com/iw2rmb/ploy/internal/api/executor"
	"github.com/iw2rmb/ploy/internal/api/httpserver"
	"github.com/iw2rmb/ploy/internal/api/metrics"
	"github.com/iw2rmb/ploy/internal/api/pki"
	"github.com/iw2rmb/ploy/internal/api/runtime"
	"github.com/iw2rmb/ploy/internal/api/scheduler"
	"github.com/iw2rmb/ploy/internal/api/status"
	"github.com/iw2rmb/ploy/internal/config/gitlab"
	controlplaneartifacts "github.com/iw2rmb/ploy/internal/controlplane/artifacts"
	"github.com/iw2rmb/ploy/internal/controlplane/auth"
	"github.com/iw2rmb/ploy/internal/controlplane/events"
	controlplanemods "github.com/iw2rmb/ploy/internal/controlplane/mods"
	controlplanescheduler "github.com/iw2rmb/ploy/internal/controlplane/scheduler"
	"github.com/iw2rmb/ploy/internal/etcdutil"
	controlmetrics "github.com/iw2rmb/ploy/internal/metrics"
	"github.com/iw2rmb/ploy/internal/node/logstream"
	workflowartifacts "github.com/iw2rmb/ploy/internal/workflow/artifacts"
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

	role := strings.TrimSpace(cfg.Tags["role"])
	statusProvider := status.New(status.Options{Role: role})
	adminSvc := buildAdminService()

	controlPlaneHandler, controlPlaneShutdown, err := buildControlPlaneHTTP(cfg, streams)
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

	svc, err := New(Options{
		Config:               cfg,
		RuntimeRegistry:      registry,
		LogStreams:           streams,
		HTTP:                 httpSrv,
		Metrics:              metricsSrv,
		ControlPlane:         controlClient,
		PKI:                  pkiManager,
		Scheduler:            taskScheduler,
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
	return &admin.Service{EtcdEndpoints: etcdutil.LocalEndpoints()}
}

func buildControlPlaneHTTP(cfg config.Config, streams *logstream.Hub) (http.Handler, func(context.Context) error, error) {
	if streams == nil {
		return nil, nil, errors.New("control-plane: streams hub required")
	}
	etcdCfg, err := etcdutil.ConfigFromEnv()
	if err != nil {
		return nil, nil, fmt.Errorf("control-plane: etcd config: %w", err)
	}
	client, err := clientv3.New(etcdCfg)
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

	modsService, err := controlplanemods.NewService(client, controlplanemods.Options{
		Prefix:    "mods/",
		Scheduler: controlplanemods.NewSchedulerBridge(sched),
	})
	if err != nil {
		_ = sched.Close()
		_ = client.Close()
		return nil, nil, fmt.Errorf("control-plane: mods orchestrator: %w", err)
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

	artifactPublisher := buildArtifactPublisher()

	var artStore *controlplaneartifacts.Store
	if client != nil {
		if store, err := controlplaneartifacts.NewStore(client, controlplaneartifacts.StoreOptions{}); err == nil {
			artStore = store
		} else {
			log.Printf("control-plane: artifact store init failed: %v", err)
		}
	}

	var rotations *events.RotationHub
	if signer != nil {
		rotations = events.NewRotationHub(context.Background(), signer)
	}

	allowInsecure := true
	defaultRole := auth.RoleCLIAdmin

	handler := httpserver.NewControlPlaneHandler(httpserver.ControlPlaneOptions{
		Scheduler:         sched,
		Signer:            signer,
		Streams:           streams,
		Etcd:              client,
		Rotations:         rotations,
		Mods:              modsService,
		ArtifactStore:     artStore,
		ArtifactPublisher: artifactPublisher,
		Authorizer: auth.NewAuthorizer(auth.Options{
			AllowInsecure: allowInsecure,
			DefaultRole:   defaultRole,
		}),
	})

	var reconciler *controlplaneartifacts.Reconciler
	if artStore != nil && artifactPublisher != nil {
		pinMetrics, err := controlmetrics.NewArtifactPinMetrics(nil)
		if err != nil {
			log.Printf("control-plane: pin metrics init failed: %v", err)
		}
		reconciler = controlplaneartifacts.NewReconciler(controlplaneartifacts.ReconcilerOptions{
			Store:   artStore,
			Cluster: artifactPublisher,
			Metrics: pinMetrics,
			Logger:  log.Default(),
		})
		if err := reconciler.Start(context.Background()); err != nil {
			log.Printf("control-plane: artifact reconciler disabled: %v", err)
			reconciler = nil
		}
	}

	shutdown := func(ctx context.Context) error {
		_ = ctx
		if modsService != nil {
			_ = modsService.Close()
		}
		if rotations != nil {
			rotations.Close()
		}
		if sched != nil {
			_ = sched.Close()
		}
		if reconciler != nil {
			stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = reconciler.Stop(stopCtx)
			cancel()
		}
		if client != nil {
			return client.Close()
		}
		return nil
	}
	return handler, shutdown, nil
}

const defaultIPFSClusterAPI = "http://127.0.0.1:9094"

func buildArtifactPublisher() *workflowartifacts.ClusterClient {
	client, err := workflowartifacts.NewClusterClient(workflowartifacts.ClusterClientOptions{
		BaseURL: defaultIPFSClusterAPI,
	})
	if err != nil {
		log.Printf("control-plane: disabling artifact publisher: %v", err)
		return nil
	}
	return client
}
