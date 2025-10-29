package daemon

import (
	"context"
	"crypto/tls"
	"crypto/x509"
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
	"github.com/iw2rmb/ploy/internal/controlplane/transfers"
	"github.com/iw2rmb/ploy/internal/etcdutil"
	controlmetrics "github.com/iw2rmb/ploy/internal/metrics"
	"github.com/iw2rmb/ploy/internal/node/lifecycle"
	"github.com/iw2rmb/ploy/internal/node/logstream"
	stepworker "github.com/iw2rmb/ploy/internal/node/worker/step"
	workflowartifacts "github.com/iw2rmb/ploy/internal/workflow/artifacts"
	workflowruntime "github.com/iw2rmb/ploy/internal/workflow/runtime"
)

const (
	envIPFSClusterAPI      = "PLOY_IPFS_CLUSTER_API"
	envIPFSClusterToken    = "PLOY_IPFS_CLUSTER_TOKEN"
	envIPFSClusterUsername = "PLOY_IPFS_CLUSTER_USERNAME"
	envIPFSClusterPassword = "PLOY_IPFS_CLUSTER_PASSWORD"
	envLifecycleNetIgnore  = "PLOY_LIFECYCLE_NET_IGNORE"
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
	lifecycleCache := lifecycle.NewCache()
	statusProvider := status.New(status.Options{
		Role:   role,
		Source: lifecycleCache,
	})
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

	httpClient, err := newControlPlaneHTTPClient(cfg.ControlPlane)
	if err != nil {
		return nil, err
	}

	workerExec, err := stepworker.FromConfig(cfg, streams, httpClient)
	if err != nil {
		return nil, err
	}

	exec := executor.New(executor.Options{
		Registry:       registry,
		DefaultAdapter: cfg.Runtime.DefaultAdapter,
		LogStreams:     streams,
		Worker:         workerExec,
	})

	controlClient, err := controlplane.New(controlplane.Options{
		Config:     cfg.ControlPlane,
		Executor:   exec,
		Status:     statusProvider,
		HTTPClient: httpClient,
	})
	if err != nil {
		return nil, err
	}

	taskScheduler := scheduler.New()

	shutdownFns := make([]func(context.Context) error, 0, 4)
	if controlPlaneShutdown != nil {
		shutdownFns = append(shutdownFns, controlPlaneShutdown)
	}

	nodeID := strings.TrimSpace(cfg.ControlPlane.NodeID)

	var dockerChecker *lifecycle.DockerChecker
	if checker, err := lifecycle.NewDockerChecker(lifecycle.DockerCheckerOptions{}); err != nil {
		log.Printf("lifecycle: docker checker disabled: %v", err)
	} else {
		dockerChecker = checker
		shutdownFns = append(shutdownFns, func(context.Context) error { return dockerChecker.Close() })
	}

	ipfsChecker := buildIPFSChecker(cfg)
	shiftChecker := lifecycle.NewShiftChecker(lifecycle.ShiftCheckerOptions{})

    collector := lifecycle.NewCollector(lifecycle.Options{
        Role:             role,
        NodeID:           nodeID,
        Docker:           dockerChecker,
        Shift:            shiftChecker,
        IPFS:             ipfsChecker,
        IgnoreInterfaces: lifecycleIgnoreInterfacesFrom(cfg),
    })

	var publisher *lifecycle.Publisher
	if etcdCfg, err := etcdutil.ConfigFromEnv(); err != nil {
		log.Printf("lifecycle: etcd config: %v", err)
	} else if client, err := clientv3.New(etcdCfg); err != nil {
		log.Printf("lifecycle: etcd client: %v", err)
	} else {
		pub, err := lifecycle.NewPublisher(lifecycle.PublisherOptions{
			Client:    client,
			Collector: collector,
			Cache:     lifecycleCache,
			NodeID:    nodeID,
		})
		if err != nil {
			log.Printf("lifecycle: publisher disabled: %v", err)
			shutdownFns = append(shutdownFns, func(context.Context) error { return client.Close() })
		} else {
			publisher = pub
			shutdownFns = append(shutdownFns, func(ctx context.Context) error { return publisher.Close(ctx) })
			interval := cfg.ControlPlane.StatusPublishInterval
			if interval <= 0 {
				interval = 30 * time.Second
			}
			taskScheduler.AddTask(lifecycle.NewPublishTask(lifecycle.PublishTaskOptions{
				Interval:  interval,
				Publisher: publisher,
			}))
			if err := publisher.Publish(context.Background()); err != nil {
				log.Printf("lifecycle: initial publish failed: %v", err)
			}
		}
	}
	if publisher == nil {
		if snapshot, err := collector.Collect(context.Background()); err == nil {
			lifecycleCache.Store(snapshot.Status)
		}
	}

	combinedShutdown := combineShutdowns(shutdownFns)

	svc, err := New(Options{
		Config:               cfg,
		RuntimeRegistry:      registry,
		LogStreams:           streams,
		HTTP:                 httpSrv,
		Metrics:              metricsSrv,
		ControlPlane:         controlClient,
		PKI:                  pkiManager,
		Scheduler:            taskScheduler,
		ControlPlaneShutdown: combinedShutdown,
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

func newControlPlaneHTTPClient(cfg config.ControlPlaneConfig) (*http.Client, error) {
	timeout := 15 * time.Second
	endpoint := strings.TrimSpace(cfg.Endpoint)
	if !strings.HasPrefix(strings.ToLower(endpoint), "https://") {
		return nil, errors.New("control-plane: https endpoint required")
	}
	tlsCfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}
	caPath := strings.TrimSpace(cfg.CAPath)
	if caPath == "" {
		return nil, errors.New("control-plane: ca path required")
	}
	caBytes, err := os.ReadFile(caPath)
	if err != nil {
		return nil, fmt.Errorf("control-plane: load ca %s: %w", caPath, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caBytes) {
		return nil, fmt.Errorf("control-plane: invalid ca bundle %s", caPath)
	}
	tlsCfg.RootCAs = pool

	certPath := strings.TrimSpace(cfg.Certificate)
	keyPath := strings.TrimSpace(cfg.Key)
	if certPath == "" || keyPath == "" {
		return nil, errors.New("control-plane: client certificate and key required")
	}
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("control-plane: load client certificate: %w", err)
	}
	tlsCfg.Certificates = []tls.Certificate{cert}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsCfg
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}, nil
}

func buildControlPlaneHTTP(cfg config.Config, streams *logstream.Hub) (http.Handler, func(context.Context) error, error) {
	if streams == nil {
		return nil, nil, errors.New("control-plane: streams hub required")
	}
	clusterID := detectClusterID(cfg)
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

	var slotStore *transfers.SlotStore
	if client != nil {
		if store, err := transfers.NewSlotStore(client, transfers.SlotStoreOptions{
			ClusterID: clusterID,
		}); err == nil {
			slotStore = store
		} else {
			log.Printf("control-plane: slot store disabled: %v", err)
		}
	}

	var transferManager *transfers.Manager
	if slotStore != nil || artStore != nil || artifactPublisher != nil {
		transferManager = transfers.NewManager(transfers.Options{
			BaseDir:   cfg.Transfers.BaseDir,
			SlotStore: slotStore,
			Store:     artStore,
			Publisher: artifactPublisher,
		})
	}

	var janitor *transfers.Janitor
	var janitorCancel context.CancelFunc
	if slotStore != nil {
		if j, err := transfers.NewJanitor(transfers.JanitorOptions{
			Logger:   log.Default(),
			BaseDir:  cfg.Transfers.BaseDir,
			Store:    slotStore,
			Clock:    time.Now,
			Interval: cfg.Transfers.JanitorInterval,
		}); err != nil {
			log.Printf("control-plane: janitor disabled: %v", err)
		} else {
			janitor = j
			janitorCancel = janitor.Start(context.Background())
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
		ClusterID:         clusterID,
		Rotations:         rotations,
		Mods:              modsService,
		ArtifactStore:     artStore,
		ArtifactPublisher: artifactPublisher,
		Transfers:         transferManager,
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
		if janitorCancel != nil {
			janitorCancel()
			done := make(chan struct{})
			go func() {
				janitor.Wait()
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(2 * time.Second):
			}
		}
		if reconciler != nil {
			stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = reconciler.Stop(stopCtx)
			cancel()
		}
		if slotStore != nil {
			_ = slotStore.Close()
		}
		if client != nil {
			return client.Close()
		}
		return nil
	}
	return handler, shutdown, nil
}

func detectClusterID(cfg config.Config) string {
	if cfg.Metadata != nil {
		if value, ok := cfg.Metadata["cluster_id"]; ok {
			if str, ok := value.(string); ok {
				if trimmed := strings.TrimSpace(str); trimmed != "" {
					return trimmed
				}
			}
		}
	}
	if env := strings.TrimSpace(os.Getenv("PLOY_CLUSTER_ID")); env != "" {
		return env
	}
	if data, err := os.ReadFile(clusterIDFile); err == nil {
		if trimmed := strings.TrimSpace(string(data)); trimmed != "" {
			return trimmed
		}
	}
	return "default"
}

const defaultIPFSClusterAPI = "http://127.0.0.1:9094"
const clusterIDFile = "/etc/ploy/cluster-id"

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

func buildIPFSChecker(cfg config.Config) lifecycle.HealthChecker {
	base := resolveLifecycleConfigString(cfg, envIPFSClusterAPI)
	if base == "" {
		return nil
	}
	return lifecycle.NewIPFSChecker(lifecycle.IPFSCheckerOptions{
		BaseURL:   base,
		AuthToken: resolveLifecycleConfigString(cfg, envIPFSClusterToken),
		Username:  resolveLifecycleConfigString(cfg, envIPFSClusterUsername),
		Password:  resolveLifecycleConfigString(cfg, envIPFSClusterPassword),
	})
}

func resolveLifecycleConfigString(cfg config.Config, key string) string {
	if cfg.Environment != nil {
		if value, ok := cfg.Environment[key]; ok {
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				return trimmed
			}
		}
	}
	return strings.TrimSpace(os.Getenv(key))
}

func lifecycleIgnoreInterfacesFrom(cfg config.Config) []string {
    raw := resolveLifecycleConfigString(cfg, envLifecycleNetIgnore)
    if raw == "" {
        return nil
    }
    parts := strings.Split(raw, ",")
    out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func combineShutdowns(fns []func(context.Context) error) func(context.Context) error {
	if len(fns) == 0 {
		return nil
	}
	return func(ctx context.Context) error {
		var first error
		for i := len(fns) - 1; i >= 0; i-- {
			fn := fns[i]
			if fn == nil {
				continue
			}
			if err := fn(ctx); err != nil && first == nil {
				first = err
			}
		}
		return first
	}
}
